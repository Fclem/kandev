package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
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
	// meta reads the full metadata of any turn.
	meta func(turnID string) map[string]interface{}
	bus  *recordingEventBus
	msgs *mockMessageCreator
	// setRunning returns the session to RUNNING, as a successor prompt does.
	setRunning func()
	// agentMgr records the execution cleanup agent.completed schedules.
	agentMgr *mockAgentManager
}

// newTimingFlow wires a service whose timing clock is controlled by the test.
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
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.scheduler = scheduler.NewScheduler(queue.NewTaskQueue(100), svc.executor, taskRepo, testLogger(), scheduler.SchedulerConfig{})
	svc.turnService = &repoTurnService{repo: repo}
	svc.eventBus = &recordingEventBus{}
	svc.messageCreator = &mockMessageCreator{}
	turn, err := svc.turnService.StartTurn(ctx, "s1")
	require.NoError(t, err)

	f := &timingFlow{
		t: t, ctx: ctx, svc: svc, turn: turn, bus: svc.eventBus.(*recordingEventBus),
		msgs: svc.messageCreator.(*mockMessageCreator), agentMgr: agentMgr,
		now: time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC),
	}
	svc.timingClock = func() time.Time { return f.now }
	f.meta = func(turnID string) map[string]interface{} {
		got, err := repo.GetTurn(ctx, turnID)
		require.NoError(t, err)
		return got.Metadata
	}
	f.get = func() map[string]interface{} {
		timing, _ := f.meta(turn.ID)["timing"].(map[string]interface{})
		return timing
	}
	f.setRunning = func() {
		require.NoError(t, repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateRunning, ""))
	}
	return f
}

func (f *timingFlow) advance(d time.Duration) { f.now = f.now.Add(d) }

func (f *timingFlow) stream(data *lifecycle.AgentStreamEventData) {
	f.svc.handleAgentStreamEvent(f.ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "t1", SessionID: "s1", ExecutionID: "exec-1", Data: data,
	})
}

func (f *timingFlow) ready() { f.readyGen(7) }

func (f *timingFlow) readyGen(generation uint64) {
	f.svc.handleAgentReady(f.ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", PromptGeneration: generation,
	})
}

func (f *timingFlow) start() {
	f.stream(&lifecycle.AgentStreamEventData{Type: streams.EventTypeTurnStarted})
}

func (f *timingFlow) message(text string, generation uint64) {
	f.stream(&lifecycle.AgentStreamEventData{
		Type: "message_streaming", Text: text, PromptGeneration: generation, MessageID: "msg-" + text,
	})
}

// nextTurn admits a successor turn the way a new prompt would: a new active
// turn and a session that is running again after the prior ready.
func (f *timingFlow) nextTurn() *models.Turn {
	turn, err := f.svc.turnService.StartTurn(f.ctx, "s1")
	require.NoError(f.t, err)
	f.setRunning()
	return turn
}

func (f *timingFlow) complete(data *lifecycle.AgentStreamEventData) {
	data.Type = agentEventComplete
	f.stream(data)
}

func (f *timingFlow) hasRecord() bool {
	f.svc.turnTimingMu.Lock()
	defer f.svc.turnTimingMu.Unlock()
	_, ok := f.svc.turnTimings["s1"]
	return ok
}

func (f *timingFlow) timingOf(turnID string) map[string]interface{} {
	timing, _ := f.meta(turnID)["timing"].(map[string]interface{})
	return timing
}

func (f *timingFlow) agentCompleted() {
	f.svc.handleAgentCompleted(f.ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1",
	})
	waitForStopCall(f.t, f.agentMgr)
}

func (f *timingFlow) turnCompletePublishes() []map[string]string {
	var out []map[string]string
	for _, rec := range f.bus.events {
		if rec.subject != events.AgentTurnMessageSaved {
			continue
		}
		data, ok := rec.event.Data.(map[string]string)
		require.True(f.t, ok)
		out = append(out, data)
	}
	return out
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

func TestTurnTimingAgentIDOnlyCompleteUsesReadyTurn(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "terminal"}[terminal], func(t *testing.T) {
			f := newTimingFlow(t)
			f.start()
			f.advance(100 * time.Millisecond)
			f.message("first", 7)
			f.advance(time.Second)
			f.ready()
			if terminal {
				f.svc.markExecutionCompleted("s1", "exec-1")
			}
			f.svc.handleAgentStreamEvent(f.ctx, &lifecycle.AgentStreamEventPayload{
				TaskID: "t1", SessionID: "s1", AgentID: "exec-1",
				Data: &lifecycle.AgentStreamEventData{Type: agentEventComplete, PromptGeneration: 7},
			})
			require.EqualValues(t, 100, f.get()["ttft_ms"])
			require.EqualValues(t, 1000, f.get()["generation_ms"])
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

// Thinking output is first output: it stamps TTFT.
func TestTurnTimingThinkingOutputStampsTTFT(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(80 * time.Millisecond)
	f.stream(&lifecycle.AgentStreamEventData{Type: "thinking_streaming", Text: "pondering", PromptGeneration: 7})
	f.advance(time.Second)
	f.ready()
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: f.turn.ID})

	timing := f.get()
	require.EqualValues(t, 80, timing["ttft_ms"])
	require.EqualValues(t, 1000, timing["generation_ms"])
}

// A provider diagnostic chunk is not output: TTFT is measured from the first
// real text that follows it.
func TestTurnTimingIgnoresProviderDiagnosticChunk(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.stream(&lifecycle.AgentStreamEventData{
		Type: "message_streaming", Text: gatewayServerFailureSample, PromptGeneration: 7,
		ProviderDiagnosticCandidate: true,
	})
	f.advance(400 * time.Millisecond)
	f.message("real text", 7)
	f.advance(time.Second)
	f.ready()
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: f.turn.ID})

	timing := f.get()
	require.EqualValues(t, 500, timing["ttft_ms"])
	require.EqualValues(t, 1000, timing["generation_ms"])
}

// A cancelled completion stores no timing and consumes the record.
func TestTurnTimingCancelledCompletionStoresNothing(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("hi", 7)
	f.advance(time.Second)
	f.ready()
	f.complete(&lifecycle.AgentStreamEventData{
		PromptGeneration: 7, TurnID: f.turn.ID,
		Data: map[string]interface{}{"stop_reason": stopReasonCancelled},
	})

	require.Nil(t, f.get())
	require.False(t, f.hasRecord())
}

// An agent error between output and complete discards the record.
func TestTurnTimingAgentErrorDiscardsRecord(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("hi", 7)
	f.advance(time.Second)
	f.stream(&lifecycle.AgentStreamEventData{Type: agentEventError, Error: "boom", PromptGeneration: 7})
	require.False(t, f.hasRecord())
	f.advance(time.Second)
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: f.turn.ID})

	require.Nil(t, f.get())
}

// agent.completed without a preceding ready closes generation at its own
// close mark, not at complete-stream processing time.
func TestTurnTimingAgentCompletedWithoutReadyClosesGeneration(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("hi", 7)
	f.advance(2 * time.Second)
	f.agentCompleted()
	f.advance(3 * time.Second) // delayed terminal complete-stream processing
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7})

	timing := f.get()
	require.EqualValues(t, 100, timing["ttft_ms"])
	require.EqualValues(t, 2000, timing["generation_ms"])
}

// agent.completed after a ready close keeps the ready timestamp.
func TestTurnTimingAgentCompletedKeepsReadyClose(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("hi", 7)
	f.advance(time.Second)
	f.ready()
	f.advance(2 * time.Second)
	f.agentCompleted()
	f.advance(3 * time.Second)
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7})

	timing := f.get()
	require.EqualValues(t, 100, timing["ttft_ms"])
	require.EqualValues(t, 1000, timing["generation_ms"])
}

// A second complete for the same turn must not write timing again.
func TestTurnTimingSecondCompleteLeavesTimingUnchanged(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("hi", 7)
	f.advance(time.Second)
	f.ready()
	complete := func() {
		f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: f.turn.ID})
	}
	complete()
	require.EqualValues(t, 100, f.get()["ttft_ms"])
	require.False(t, f.hasRecord())

	require.NoError(t, f.svc.turnService.PatchTurnMetadata(f.ctx, "s1", f.turn.ID,
		map[string]interface{}{"timing": map[string]interface{}{"ttft_ms": 424242}}))
	f.advance(time.Second)
	complete()

	require.EqualValues(t, 424242, f.get()["ttft_ms"])
}

// A complete for T1 arriving after turn_started for T2 writes nothing for T1
// and leaves T2's record intact.
func TestTurnTimingLateCompleteForPriorTurnWritesNothing(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("one", 7)
	f.advance(time.Second)
	f.ready()

	t2 := f.nextTurn()
	f.advance(time.Second)
	f.start()
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: f.turn.ID})

	require.Nil(t, f.get())
	require.True(t, f.hasRecord(), "T2's record must survive a stale T1 complete")

	f.advance(250 * time.Millisecond)
	f.message("two", 7)
	f.advance(time.Second)
	f.ready()
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: t2.ID})

	timing := f.timingOf(t2.ID)
	require.EqualValues(t, 250, timing["ttft_ms"])
	require.EqualValues(t, 1000, timing["generation_ms"])
	require.Nil(t, f.get())
}

func TestTurnTimingLateAgentIDOnlyCompletePreservesSuccessor(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("one", 7)
	f.advance(time.Second)
	f.ready()
	t2 := f.nextTurn()
	f.start()
	f.svc.handleAgentStreamEvent(f.ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: "t1", SessionID: "s1", AgentID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{Type: agentEventComplete, PromptGeneration: 7},
	})
	require.Nil(t, f.get())
	require.True(t, f.hasRecord())
	f.advance(250 * time.Millisecond)
	f.message("two", 7)
	f.advance(time.Second)
	f.ready()
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: t2.ID})
	require.EqualValues(t, 250, f.timingOf(t2.ID)["ttft_ms"])
	require.EqualValues(t, 1000, f.timingOf(t2.ID)["generation_ms"])
}

func TestTurnTimingLateTerminalExecutionPreservesReplacement(t *testing.T) {
	for _, closed := range []bool{false, true} {
		name := "before successor close"
		if closed {
			name = "after successor close"
		}
		t.Run(name, func(t *testing.T) {
			f := newTimingFlow(t)
			f.start()
			f.svc.markExecutionCompleted("s1", "exec-1")
			f.agentMgr.currentPromptExecutionID = "exec-2"
			stream := func(data *lifecycle.AgentStreamEventData) {
				f.svc.handleAgentStreamEvent(f.ctx, &lifecycle.AgentStreamEventPayload{
					TaskID: "t1", SessionID: "s1", ExecutionID: "exec-2", Data: data,
				})
			}
			ready := func() {
				f.svc.handleAgentReady(f.ctx, watcher.AgentEventData{
					TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-2",
				})
			}
			stream(&lifecycle.AgentStreamEventData{Type: streams.EventTypeTurnStarted})
			f.advance(250 * time.Millisecond)
			stream(&lifecycle.AgentStreamEventData{Type: "message_streaming", Text: "successor output"})
			f.advance(time.Second)
			if closed {
				ready()
			}

			f.complete(&lifecycle.AgentStreamEventData{TurnID: f.turn.ID})
			require.Nil(t, f.get(), "a predecessor must not publish successor timing")
			require.True(t, f.hasRecord(), "successor timing must survive the predecessor's terminal flush")
			if !closed {
				ready()
			}
			stream(&lifecycle.AgentStreamEventData{Type: agentEventComplete, TurnID: f.turn.ID})
			require.EqualValues(t, 250, f.get()["ttft_ms"])
			require.EqualValues(t, 1000, f.get()["generation_ms"])
			require.False(t, f.hasRecord())
		})
	}
}

func TestTurnTimingLateTerminalPromptPreservesSuccessorGeneration(t *testing.T) {
	for _, tc := range []struct {
		name             string
		startGeneration  uint64
		outputGeneration uint64
	}{
		{name: "generation from start", startGeneration: 8},
		{name: "generation from first output", outputGeneration: 8},
		{name: "generation from ready close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTimingFlow(t)
			f.start()
			f.agentMgr.currentPromptGeneration.Store(8)
			f.stream(&lifecycle.AgentStreamEventData{
				Type: streams.EventTypeTurnStarted, PromptGeneration: tc.startGeneration,
			})
			f.advance(250 * time.Millisecond)
			f.message("successor output", tc.outputGeneration)
			f.advance(time.Second)
			f.readyGen(8)
			f.svc.markExecutionCompleted("s1", "exec-1")

			f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: f.turn.ID})
			require.Nil(t, f.get(), "a predecessor prompt must not publish successor timing")
			require.True(t, f.hasRecord(), "successor generation must survive the predecessor's terminal flush")

			f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 8, TurnID: f.turn.ID})
			require.EqualValues(t, 250, f.get()["ttft_ms"])
			require.EqualValues(t, 1000, f.get()["generation_ms"])
			require.False(t, f.hasRecord())
		})
	}
}

func TestTurnTimingStalePromptFramesPreserveSuccessor(t *testing.T) {
	for _, eventType := range []string{"message_streaming", "thinking_streaming", agentEventError, streams.EventTypeTurnStarted} {
		t.Run(eventType, func(t *testing.T) {
			f := newTimingFlow(t)
			f.start()
			f.advance(100 * time.Millisecond)
			f.message("one", 7)
			f.advance(time.Second)
			f.ready()
			f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: f.turn.ID})

			f.agentMgr.currentPromptGeneration.Store(8)
			successor := f.nextTurn()
			reservation := f.svc.markQueuedDispatchInFlight("s1", "successor")
			accepted, err := f.svc.claimQueuedDispatchForExecution("s1", "successor", reservation)
			require.NoError(t, err)
			require.True(t, accepted)
			f.svc.bindAcceptedDispatchTurn("s1", successor.ID)
			f.start()
			f.advance(100 * time.Millisecond)
			f.stream(&lifecycle.AgentStreamEventData{
				Type: eventType, Text: "stale output", Error: "stale error",
				PromptGeneration: 7, TurnID: f.turn.ID,
			})
			require.True(t, f.hasRecord(), "stale frames must preserve successor timing")
			require.Equal(t, successor.ID, f.svc.nonCreatingActiveTurnID(f.ctx, "s1"))

			f.advance(200 * time.Millisecond)
			f.message("two", 8)
			f.advance(time.Second)
			f.readyGen(8)
			f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 8, TurnID: successor.ID})
			timing := f.timingOf(successor.ID)
			require.EqualValues(t, 300, timing["ttft_ms"])
			require.EqualValues(t, 1000, timing["generation_ms"])
		})
	}
}

// A terminal complete after a ready close flushes text, prompt metadata and
// timing onto the ready-marked turn and publishes one agent-turn-complete.
func TestTurnTimingTerminalFlushTargetsReadyMarkTurn(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("hi", 7)
	f.advance(time.Second)
	f.ready()
	// A successor turn is already active when the terminal marker is taken, so
	// the marker's own turn snapshot points at T2 while the ready mark names T1.
	t2, err := f.svc.turnService.StartTurn(f.ctx, "s1")
	require.NoError(t, err)
	f.svc.markExecutionCompleted("s1", "exec-1")
	f.advance(time.Second)
	f.complete(&lifecycle.AgentStreamEventData{
		PromptGeneration: 7, Text: "final answer",
		Usage: &streams.PromptUsage{InputTokens: 10, OutputTokens: 5},
	})

	require.EqualValues(t, 100, f.get()["ttft_ms"])
	require.Contains(t, f.meta(f.turn.ID), "prompt_usage")
	require.NotContains(t, f.meta(t2.ID), "prompt_usage")
	require.Nil(t, f.timingOf(t2.ID))
	require.Len(t, f.msgs.agentMessages, 1)
	require.Equal(t, "final answer", f.msgs.agentMessages[0].content)
	require.Equal(t, f.turn.ID, f.msgs.agentMessages[0].turnID)
	published := f.turnCompletePublishes()
	require.Len(t, published, 1)
	require.Equal(t, f.turn.ID, published[0]["turn_id"])
}

// A self-resume turn is created lazily by its first message: turn_started
// finds no active turn, and the first output binds the record to the new turn.
func TestTurnTimingLazilyCreatedTurnBindsAtFirstOutput(t *testing.T) {
	f := newTimingFlow(t)
	f.svc.completeTurnForSession(f.ctx, "s1")
	require.Empty(t, f.svc.nonCreatingActiveTurnID(f.ctx, "s1"))

	f.start()
	f.advance(50 * time.Millisecond)
	f.message("self resume", 7)
	lazyID := f.svc.nonCreatingActiveTurnID(f.ctx, "s1")
	require.NotEmpty(t, lazyID)
	require.NotEqual(t, f.turn.ID, lazyID)
	f.advance(time.Second)
	f.ready()
	f.complete(&lifecycle.AgentStreamEventData{PromptGeneration: 7, TurnID: lazyID})

	timing := f.timingOf(lazyID)
	require.EqualValues(t, 50, timing["ttft_ms"])
	require.EqualValues(t, 1000, timing["generation_ms"])
}

// A TurnID-bearing complete still consumes its generation-less ready mark, so
// a later compatibility complete is matched to its own turn, not the stale mark.
func TestTurnTimingMixedTurnIDAndCompatibilityCompletesStayFIFO(t *testing.T) {
	f := newTimingFlow(t)
	f.start()
	f.advance(100 * time.Millisecond)
	f.message("one", 0)
	f.advance(time.Second)
	f.readyGen(0)
	f.complete(&lifecycle.AgentStreamEventData{TurnID: f.turn.ID})
	require.EqualValues(t, 100, f.get()["ttft_ms"])

	t2 := f.nextTurn()
	f.advance(time.Second)
	f.start()
	f.advance(200 * time.Millisecond)
	f.message("two", 0)
	f.advance(time.Second)
	f.readyGen(0)
	f.complete(&lifecycle.AgentStreamEventData{Text: "answer two"})

	timing := f.timingOf(t2.ID)
	require.EqualValues(t, 200, timing["ttft_ms"])
	require.EqualValues(t, 1000, timing["generation_ms"])
	require.Len(t, f.msgs.agentMessages, 1)
	require.Equal(t, t2.ID, f.msgs.agentMessages[0].turnID)
}

// Records that never receive an admitted complete are pruned once past
// retention, the next time any session starts a turn.
func TestTurnTimingPrunesRecordsPastRetention(t *testing.T) {
	svc := &Service{}
	started := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	svc.startTurnTiming("stale", "exec-1", "turn-stale", 0, started)
	svc.startTurnTiming("recent", "exec-1", "turn-recent", 0, started)
	svc.turnTimingMu.Lock()
	stale := svc.turnTimings["stale"]
	stale.trackedAt = time.Now().Add(-turnTimingRetention - time.Minute)
	svc.turnTimings["stale"] = stale
	svc.turnTimingMu.Unlock()

	svc.startTurnTiming("fresh", "exec-1", "turn-fresh", 0, started)

	svc.turnTimingMu.Lock()
	defer svc.turnTimingMu.Unlock()
	require.NotContains(t, svc.turnTimings, "stale")
	require.Contains(t, svc.turnTimings, "recent")
	require.Contains(t, svc.turnTimings, "fresh")
}
