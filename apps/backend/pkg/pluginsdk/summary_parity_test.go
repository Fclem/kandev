package pluginsdk

import (
	"testing"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func summaryParityTask() Task {
	partial, changed, reviews := false, int32(4), int32(0)
	return Task{ID: "task-one", PullRequestsPartial: &partial, PullRequests: []TaskPullRequest{{
		Number: 12, Provider: "github", CreatedAt: "2026-01-02T03:04:05Z", ChangedFiles: &changed,
		ApprovedReviewerCount: &reviews, SizeObserved: true,
	}}}
}

func summaryParityCodeStats() SessionCodeStats {
	committed, pending := int64(5), int64(3)
	return SessionCodeStats{SessionID: "session-one", CommittedLinesAvailable: true, FilesChangedCommitted: &committed, FilesChangedPeakPending: &pending}
}

// @covers AC-PLUGINS-EFFORT-SIGNALS-003.5 AC-PLUGINS-EFFORT-SIGNALS-005.1
func testSummaryFieldRoundTrips(t *testing.T) {
	task := summaryParityTask()
	wire, err := task.toProto()
	require.NoError(t, err)
	output, err := taskFromProto(wire)
	require.NoError(t, err)
	require.Equal(t, task, output)
	stats := summaryParityCodeStats()
	require.Equal(t, stats, sessionCodeStatsFromProto(stats.toProto()))
	legacy := sessionCodeStatsFromProto(&pluginv1.SessionCodeStats{SessionId: "old-session"})
	require.Nil(t, legacy.FilesChangedCommitted)
	require.Nil(t, legacy.FilesChangedPeakPending)
	zero := int64(0)
	measured := SessionCodeStats{SessionID: "zero-session", CommittedLinesAvailable: true, FilesChangedCommitted: &zero, FilesChangedPeakPending: &zero}
	require.Equal(t, measured, sessionCodeStatsFromProto(measured.toProto()))
}

// The earlier schema retains its shipped fields and decodes newer messages
// without interpreting the appended summary fields.
func testSummaryLegacyDecode(t *testing.T) {
	file := protodesc.ToFileDescriptorProto(pluginv1.File_kandev_plugin_v1_plugin_proto)
	file.Service = nil
	for _, message := range file.MessageType {
		ceiling := map[string]int32{"Task": 37, "TaskPullRequest": 19, "SessionCodeStats": 6}[message.GetName()]
		if ceiling == 0 {
			continue
		}
		pruneSummaryFields(message, ceiling)
	}
	descriptor, err := protodesc.NewFile(file, protoregistry.GlobalFiles)
	require.NoError(t, err)
	input := summaryParityTask()
	current, err := input.toProto()
	require.NoError(t, err)
	payload, err := proto.Marshal(current)
	require.NoError(t, err)
	oldTask := dynamicpb.NewMessage(descriptor.Messages().ByName("Task"))
	require.NoError(t, proto.Unmarshal(payload, oldTask))
	require.Equal(t, "task-one", oldTask.Get(oldTask.Descriptor().Fields().ByNumber(1)).String())
	oldPR := oldTask.Get(oldTask.Descriptor().Fields().ByNumber(19)).List().Get(0).Message()
	require.Equal(t, int64(12), oldPR.Get(oldPR.Descriptor().Fields().ByNumber(1)).Int())
	require.Equal(t, "github", oldPR.Get(oldPR.Descriptor().Fields().ByNumber(8)).String())
	require.Nil(t, oldTask.Descriptor().Fields().ByNumber(38))
	require.Nil(t, oldPR.Descriptor().Fields().ByNumber(20))
	stats := summaryParityCodeStats()
	payload, err = proto.Marshal(stats.toProto())
	require.NoError(t, err)
	oldStats := dynamicpb.NewMessage(descriptor.Messages().ByName("SessionCodeStats"))
	require.NoError(t, proto.Unmarshal(payload, oldStats))
	require.Equal(t, "session-one", oldStats.Get(oldStats.Descriptor().Fields().ByNumber(1)).String())
	require.True(t, oldStats.Get(oldStats.Descriptor().Fields().ByNumber(6)).Bool())
	require.Nil(t, oldStats.Descriptor().Fields().ByNumber(protoreflect.FieldNumber(7)))
}

func pruneSummaryFields(message *descriptorpb.DescriptorProto, ceiling int32) {
	fields := message.Field[:0]
	oneofs := make([]*descriptorpb.OneofDescriptorProto, 0, len(message.OneofDecl))
	indices := make(map[int32]int32)
	for _, field := range message.Field {
		if field.GetNumber() > ceiling {
			continue
		}
		if field.OneofIndex != nil {
			old := field.GetOneofIndex()
			next, found := indices[old]
			if !found {
				next = int32(len(oneofs))
				indices[old] = next
				oneofs = append(oneofs, message.OneofDecl[old])
			}
			field.OneofIndex = proto.Int32(next)
		}
		fields = append(fields, field)
	}
	message.Field = fields
	message.OneofDecl = oneofs
}
