package pluginsdk

import (
	"testing"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// @covers AC-PLUGINS-EFFORT-SIGNALS-003.5 AC-PLUGINS-EFFORT-SIGNALS-003.6
func TestTaskPullRequestsPartialPresence(t *testing.T) {
	for _, state := range []string{"absent", "complete", "partial"} {
		t.Run(state, func(t *testing.T) {
			wire := []byte{}
			var expected *bool
			if state != "absent" {
				wire = protowire.AppendTag(wire, 38, protowire.VarintType)
				value := uint64(0)
				partial := false
				if state == "partial" {
					value = 1
					partial = true
				}
				expected = &partial
				wire = protowire.AppendVarint(wire, value)
			}
			var message pluginv1.Task
			require.NoError(t, proto.Unmarshal(wire, &message))
			task, err := taskFromProto(&message)
			require.NoError(t, err)
			require.Equal(t, expected, task.PullRequestsPartial)
			roundTrip, err := task.toProto()
			require.NoError(t, err)
			bytes, err := proto.Marshal(roundTrip)
			require.NoError(t, err)
			require.Equal(t, wire, bytes)
		})
	}
}
