package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type timingFlow struct {
	t    *testing.T
	ctx  context.Context
	svc  *Service
	turn *models.Turn
	now  time.Time
	get  func() map[string]interface{}
}

// newTimingFlow wires a service whose timing clock is controlled by the test.
// The base is the wall clock: ready-mark expiry is checked against time.Now.
func newTimingFlow(t *testing.T) *timingFlow {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateInProgress)
	agentMgr := &mockAgentManager{isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(7)
	svc := createTestServiceWithAgent(repo, stepGetter, taskRepo, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	svc.eventBus = &recordingEventBus{}
	svc.messageCreator = &mockMessageCreator{}
	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	f := &timingFlow{t: t, ctx: ctx, svc: svc, turn: turn, now: time.Now().UTC().Truncate(time.Second)}
	svc.timingClock = func() time.Time { return f.now }
	f.get = func() map[string]interface{} {
		got, err := repo.GetTurn(ctx, turn.ID)
		require.NoError(t, err)
		timing, _ := got.Metadata["timing"].(map[string]interface{})
		return timing
	}
	return f
}

func (f *timingFlow) advance(d time.Duration) { f.now = f.now.Add(d) }

func (f *timingFlow) stream(data *lifecycle.AgentStreamEventData) {
	f.svc.handleAgentStreamEvent(f.ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "t1", SessionID: "s1", ExecutionID: "exec-1", Data: data,
	})
}

func (f *timingFlow) ready() {
	f.svc.handleAgentReady(f.ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", PromptGeneration: 7,
	})
}

// TTFT is measured from backend receipt of turn_started, and a tool delay
// between first output and ready is part of generation; delayed complete-stream
// processing does not extend it.
func TestTurnTimingFlowMeasuresReceiptToReadyClose(t *testing.T) {
	f := newTimingFlow(t)
	f.stream(&lifecycle.AgentStreamEventData{Type: streams.EventTypeTurnStarted})
	f.advance(125 * time.Millisecond)
	f.stream(&lifecycle.AgentStreamEventData{Type: "message_streaming", Text: "hi", PromptGeneration: 7})
	f.advance(4875 * time.Millisecond) // includes a tool-use delay
	f.ready()
	f.advance(4 * time.Second) // delayed complete-stream processing
	f.stream(&lifecycle.AgentStreamEventData{Type: agentEventComplete, PromptGeneration: 7, TurnID: f.turn.ID})

	timing := f.get()
	require.EqualValues(t, 125, timing["ttft_ms"])
	require.EqualValues(t, 4875, timing["generation_ms"])
}

// ready close at t1, terminal marker at t2, stream processing at t3: the
// recorded close stays t1 for both TurnID-bearing and compatibility payloads.
func TestTurnTimingKeepsReadyCloseAcrossTerminalMarker(t *testing.T) {
	for _, tc := range []struct {
		name   string
		turnID bool
	}{{"turn id payload", true}, {"compatibility payload", false}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTimingFlow(t)
			f.stream(&lifecycle.AgentStreamEventData{Type: streams.EventTypeTurnStarted})
			f.advance(100 * time.Millisecond)
			f.stream(&lifecycle.AgentStreamEventData{Type: "message_streaming", Text: "hi", PromptGeneration: 7})
			f.advance(time.Second)
			f.ready() // t1
			f.advance(time.Second)
			f.svc.markExecutionCompleted("s1", "exec-1") // t2
			f.advance(time.Second)                       // t3
			data := &lifecycle.AgentStreamEventData{Type: agentEventComplete, PromptGeneration: 7}
			if tc.turnID {
				data.TurnID = f.turn.ID
			}
			f.stream(data)

			timing := f.get()
			require.EqualValues(t, 100, timing["ttft_ms"])
			require.EqualValues(t, 1000, timing["generation_ms"])
		})
	}
}

// Without a live ready close, generation_ms is omitted; TTFT is kept.
func TestTurnTimingOmitsGenerationWithoutLiveCloseMark(t *testing.T) {
	f := newTimingFlow(t)
	f.stream(&lifecycle.AgentStreamEventData{Type: streams.EventTypeTurnStarted})
	f.advance(100 * time.Millisecond)
	f.stream(&lifecycle.AgentStreamEventData{Type: "message_streaming", Text: "hi", PromptGeneration: 7})
	f.advance(time.Second)
	f.stream(&lifecycle.AgentStreamEventData{Type: agentEventComplete, PromptGeneration: 7, TurnID: f.turn.ID})

	timing := f.get()
	require.EqualValues(t, 100, timing["ttft_ms"])
	require.NotContains(t, timing, "generation_ms")
}

// A turn with no output stores no timing.
func TestTurnTimingFlowOmitsTurnWithoutOutput(t *testing.T) {
	f := newTimingFlow(t)
	f.stream(&lifecycle.AgentStreamEventData{Type: streams.EventTypeTurnStarted})
	f.advance(time.Second)
	f.ready()
	f.stream(&lifecycle.AgentStreamEventData{Type: agentEventComplete, PromptGeneration: 7, TurnID: f.turn.ID})

	require.Nil(t, f.get())
}
