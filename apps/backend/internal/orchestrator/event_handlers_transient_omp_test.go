package orchestrator

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	acptransport "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

const ompCapturedSpendChunk = `429 {"type":"error","error":{"type":"rate_limit_error","message":"This request would exceed your account's monthly spend limit. Please try again later."},"request_id":"req_011CfL9vJs9DYV45eqL6jbh9"} retry-after-ms=274579000`

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.2
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.5
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.6
func TestClassifyKanbanFailureOMPCapturedFrames(t *testing.T) {
	for _, tc := range []struct {
		name, chunk string
		code        routingerr.Code
		scope       string
		delay       int64
	}{
		{name: "observed spend failure", chunk: ompCapturedSpendChunk, code: routingerr.CodeQuotaLimited, scope: "account", delay: 274579000},
		{name: "rate type with ordinary message", chunk: `429 {"type":"error","error":{"type":"rate_limit_error","message":"Too many requests"}} retry-after-ms=41501`, code: routingerr.CodeRateLimited, scope: "model", delay: 41501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := ompTerminalFrame(t, tc.chunk)
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			var persisted streams.AgentEvent
			if err := json.Unmarshal(encoded, &persisted); err != nil {
				t.Fatal(err)
			}
			classified := classifyKanbanFailure(watcher.AgentEventData{
				AgentID: "omp-acp", ErrorMessage: persisted.Error, ProviderError: persisted.ProviderError,
			})
			if classified.Code != tc.code || classified.LimitScope != tc.scope {
				t.Fatalf("classified failure = %+v; want %s/%s", classified, tc.code, tc.scope)
			}
			wantReset := persisted.ProviderError.OccurredAt.Add(time.Duration(tc.delay) * time.Millisecond)
			if classified.ResetHint == nil || !classified.ResetHint.Equal(wantReset) {
				t.Fatalf("reset = %v; want chunk observation + %dms = %v", classified.ResetHint, tc.delay, wantReset)
			}
			if strings.Contains(string(encoded), "req_") || strings.Contains(classified.RawExcerpt, "req_") || strings.Contains(persisted.ProviderError.Message, "retry-after-ms") {
				t.Fatalf("raw provider metadata leaked: %s; excerpt=%s", encoded, classified.RawExcerpt)
			}
		})
	}
}

func ompTerminalFrame(t *testing.T, chunk string) streams.AgentEvent {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatal(err)
	}
	adapter := acptransport.NewAdapter(&shared.Config{AgentID: "omp-acp", WorkDir: t.TempDir()}, log)
	clientReader, clientWriter := io.Pipe()
	agentReader, agentWriter := io.Pipe()
	t.Cleanup(func() { _ = adapter.Close(); _ = clientWriter.Close(); _ = agentWriter.Close() })
	if err := adapter.Connect(clientWriter, agentReader); err != nil {
		t.Fatal(err)
	}
	agent := &ompCapturedFrameAgent{chunk: chunk}
	agent.connection = sdk.NewAgentSideConnection(agent, agentWriter, clientReader)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	if err := adapter.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.NewSession(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Prompt(ctx, "Try the selected model", nil, 71); err != nil {
		t.Fatal(err)
	}
	var terminal streams.AgentEvent
	var failures, completes int
	for {
		select {
		case event := <-adapter.Updates():
			if event.Type == streams.EventTypeComplete {
				completes++
			}
			if event.Type == streams.EventTypeError {
				terminal = event
				failures++
			}
		default:
			if failures != 1 || completes != 0 || terminal.ProviderError == nil {
				t.Fatalf("failures=%d completes=%d terminal=%+v", failures, completes, terminal)
			}
			if terminal.PromptGeneration != 71 || terminal.ProviderError.ModelID != "anthropic/claude-opus-5-5" {
				t.Fatalf("terminal attribution = %+v", terminal)
			}
			return terminal
		}
	}
}

type ompCapturedFrameAgent struct {
	sdk.Agent
	connection *sdk.AgentSideConnection
	chunk      string
}

func (*ompCapturedFrameAgent) Initialize(_ context.Context, request sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	return sdk.InitializeResponse{ProtocolVersion: request.ProtocolVersion}, nil
}

func (*ompCapturedFrameAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	var response sdk.NewSessionResponse
	err := json.Unmarshal([]byte(`{"sessionId":"omp-captured","configOptions":[{"type":"select","id":"model","name":"Model","currentValue":"anthropic/claude-opus-5-5","options":[{"value":"anthropic/claude-opus-5-5","name":"Opus"}]}]}`), &response)
	return response, err
}

func (a *ompCapturedFrameAgent) Prompt(ctx context.Context, _ sdk.PromptRequest) (sdk.PromptResponse, error) {
	err := a.connection.SessionUpdate(ctx, sdk.SessionNotification{
		SessionId: "omp-captured",
		Update:    sdk.SessionUpdate{AgentMessageChunk: &sdk.SessionUpdateAgentMessageChunk{Content: sdk.TextBlock(a.chunk)}},
	})
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, err
}
