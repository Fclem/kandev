package pluginsdk

import (
	"context"
	"net"
	"reflect"
	"testing"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestHostSDKProtoParity(t *testing.T) {
	message := Message{ID: "message-one", AuthorType: "user", Content: "prompt", Origin: "human"}
	activity := SessionUserActivity{
		SessionID: "session-one", TaskID: "task-one", ActiveMS: 24000, IntervalCount: 3,
		FirstActivityAt: "2026-01-02T03:04:05Z", LastActivityAt: "2026-01-02T03:05:05Z", ActivityMeasured: true,
	}
	completed := "2026-01-02T03:06:05Z"
	turn := SessionTurn{ID: "turn-one", SessionID: "session-one", TaskID: "task-one", StartedAt: activity.FirstActivityAt, CompletedAt: &completed}
	impl := &dataRecordingHost{
		tasks:     map[string]Task{"task-one": summaryParityTask()},
		codeStats: []SessionCodeStats{summaryParityCodeStats()},
		messages:  []Message{message}, messagePage: &PageInfo{HasMore: false},
		messageDispatch: MessageDispatch{SessionID: "session-one", Status: "sent"},
		sessionActivity: []SessionUserActivity{activity}, activityPage: &PageInfo{HasMore: false},
		sessionTurns: []SessionTurn{turn}, turnsPage: &PageInfo{HasMore: false},
	}
	host, methods := dialParityHost(t, &feedbackParityHost{dataRecordingHost: impl})
	mappings := []struct {
		sdkMethod string
		rpcMethod string
		call      func(Host) error
	}{
		{
			sdkMethod: "Messages().List",
			rpcMethod: "/kandev.plugin.v1.Host/ListMessages",
			call: func(host Host) error {
				got, _, err := host.Messages().List(context.Background(), MessageFilter{}, Page{})
				if err == nil && len(got) != 1 {
					t.Fatalf("Messages().List returned %d messages, want 1", len(got))
				}
				if err == nil && got[0].Origin != "human" {
					t.Fatalf("Messages().List origin = %q, want human", got[0].Origin)
				}
				return err
			},
		},
		{
			sdkMethod: "Messages().Send",
			rpcMethod: "/kandev.plugin.v1.Host/SendMessage",
			call: func(host Host) error {
				got, err := host.Messages().Send(context.Background(), "task-one", "session-one", "prompt")
				if err == nil && (got.SessionID != "session-one" || got.Status != "sent") {
					t.Fatalf("Messages().Send result = %+v", got)
				}
				return err
			},
		},

		{
			sdkMethod: "Sessions().UserActivity",
			rpcMethod: "/kandev.plugin.v1.Host/ListSessionUserActivity",
			call: func(host Host) error {
				got, _, err := host.Sessions().UserActivity(context.Background(), SessionActivityFilter{SessionIDs: []string{"session-one"}}, Page{})
				if err == nil && (len(got) != 1 || !got[0].ActivityMeasured) {
					t.Fatalf("Sessions().UserActivity() = %+v, want one measured row", got)
				}
				return err
			},
		},
		{
			sdkMethod: "Sessions().Turns",
			rpcMethod: "/kandev.plugin.v1.Host/ListSessionTurns",
			call: func(host Host) error {
				got, _, err := host.Sessions().Turns(context.Background(), SessionActivityFilter{SessionIDs: []string{"session-one"}}, Page{})
				if err == nil && (len(got) != 1 || got[0].ID != "turn-one") {
					t.Fatalf("Sessions().Turns() = %+v, want turn-one", got)
				}
				return err
			},
		},
		{
			sdkMethod: "Tasks().Get",
			rpcMethod: "/kandev.plugin.v1.Host/GetTask",
			call: func(host Host) error {
				got, err := host.Tasks().Get(context.Background(), "task-one")
				if err == nil && !reflect.DeepEqual(*got, summaryParityTask()) {
					t.Fatalf("task summary over RPC = %+v, want %+v", got, summaryParityTask())
				}
				return err
			},
		},
		{
			sdkMethod: "Sessions().CodeStats",
			rpcMethod: "/kandev.plugin.v1.Host/ListSessionCodeStats",
			call: func(host Host) error {
				got, _, err := host.Sessions().CodeStats(context.Background(), SessionFilter{}, Page{})
				if err == nil && (len(got) != 1 || !reflect.DeepEqual(got[0], summaryParityCodeStats())) {
					t.Fatalf("code summary over RPC = %+v, want %+v", got, summaryParityCodeStats())
				}
				return err
			},
		},
		{
			sdkMethod: "ExactQueries().ListChangeRequestFeedback",
			rpcMethod: "/kandev.plugin.v1.Host/ListChangeRequestFeedbackExact",
			call: func(host Host) error {
				manager, ok := HostExactQueries(host)
				if !ok {
					t.Fatal("exact queries unavailable")
				}
				page, err := manager.ListChangeRequestFeedback(context.Background(), ExactChangeRequestFeedbackQuery{
					RequestID: "feedback", WorkspaceID: "workspace-one", TaskIDs: []string{"task-one"}, Page: ExactReadPage{Limit: 1},
				})
				if err == nil && (len(page.Items) != 1 || !reflect.DeepEqual(page.Items[0], feedbackParityRow()) || page.PageInfo.Receipt.RequestID != "feedback") {
					t.Fatalf("feedback RPC = %+v", page)
				}
				return err
			},
		},
	}
	for _, mapping := range mappings {
		t.Run(mapping.sdkMethod+" maps to "+mapping.rpcMethod, func(t *testing.T) {
			err := mapping.call(host)
			if status.Code(err) == codes.Unimplemented {
				t.Fatalf("SDK method %s reached Unimplemented at %s", mapping.sdkMethod, mapping.rpcMethod)
			}
			if err != nil {
				t.Fatalf("SDK method %s: %v", mapping.sdkMethod, err)
			}
			if got := <-methods; got != mapping.rpcMethod {
				t.Fatalf("SDK method %s invoked %s, want %s", mapping.sdkMethod, got, mapping.rpcMethod)
			}
		})
	}

	roundTrips := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "message origin including exact observation",
			run: func(t *testing.T) {
				input := Message{ID: "message-origin", AuthorType: "user", Origin: "automation"}
				wire := input.toProto()
				if got := wire.GetOrigin(); got != "automation" {
					t.Fatalf("wire origin = %q, want automation", got)
				}
				observation := &pluginv1.ExactMessageObservation{Message: wire}
				output := messageFromProto(observation.GetMessage())
				if output.Origin != "automation" {
					t.Fatalf("exact observation origin = %q, want automation", output.Origin)
				}
			},
		},
		{
			name: "author types",
			run: func(t *testing.T) {
				input := MessageFilter{AuthorTypes: []string{"user", "agent"}}
				wire := input.toProto()
				if !reflect.DeepEqual(wire.GetAuthorTypes(), []string{"user", "agent"}) {
					t.Fatalf("wire author_types = %v, want [user agent]", wire.GetAuthorTypes())
				}
				output := messageFilterFromProto(wire)
				if !reflect.DeepEqual(output.AuthorTypes, []string{"user", "agent"}) {
					t.Fatalf("SDK author types = %v, want [user agent]", output.AuthorTypes)
				}
			},
		},
		{
			name: "session activity filter",
			run: func(t *testing.T) {
				input := SessionActivityFilter{SessionIDs: []string{"session-one"}, TaskIDs: []string{"task-one"}}
				wire := input.toProto()
				if !reflect.DeepEqual(wire.GetSessionIds(), input.SessionIDs) || !reflect.DeepEqual(wire.GetTaskIds(), input.TaskIDs) {
					t.Fatalf("wire filter = %+v, want %+v", wire, input)
				}
				if output := sessionActivityFilterFromProto(wire); !reflect.DeepEqual(output, input) {
					t.Fatalf("SDK filter = %+v, want %+v", output, input)
				}
			},
		},
		{
			name: "session user activity fields",
			run: func(t *testing.T) {
				input := SessionUserActivity{
					SessionID: "session-one", TaskID: "task-one", ActiveMS: 9876, IntervalCount: 2,
					FirstActivityAt: "first", LastActivityAt: "last", ActivityMeasured: true,
				}
				wire := input.toProto()
				if output := sessionUserActivityFromProto(wire); !reflect.DeepEqual(output, input) {
					t.Fatalf("activity round trip = %+v, want %+v", output, input)
				}
			},
		},
		{
			name: "session turn fields",
			run: func(t *testing.T) {
				completedAt := "completed"
				input := SessionTurn{ID: "turn-one", SessionID: "session-one", TaskID: "task-one", StartedAt: "started", CompletedAt: &completedAt}
				wire := input.toProto()
				if output := sessionTurnFromProto(wire); !reflect.DeepEqual(output, input) {
					t.Fatalf("turn round trip = %+v, want %+v", output, input)
				}
			},
		},
		{name: "Task05 summary fields", run: testSummaryFieldRoundTrips},
		{name: "Task05 older SDK decodes additive fields", run: testSummaryLegacyDecode},
		{name: "feedback fields including optional resolution", run: testFeedbackRoundTrip},
	}
	for _, roundTrip := range roundTrips {
		t.Run(roundTrip.name, roundTrip.run)
	}
}

func dialParityHost(t *testing.T, impl Host) (Host, <-chan string) {
	t.Helper()
	methods := make(chan string, 4)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		methods <- info.FullMethod
		return handler(ctx, req)
	}))
	registerHostServer(server, impl)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///host-parity", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return newHostClient(conn), methods
}
