package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type runStopperStub struct {
	stopped bool
	err     error
	onStop  func()
}

func (s runStopperStub) StopAutomationRun(context.Context, string, string, string) (bool, error) {
	if s.onStop != nil {
		s.onStop()
	}
	return s.stopped, s.err
}

type runLivenessStub struct {
	live bool
	err  error
}

func (s runLivenessStub) AutomationRunLive(context.Context, string, string, string) (bool, error) {
	return s.live, s.err
}

// @covers AC-OFFICE-AUTOMATION-CONTINUITY-003.3
// @covers AC-OFFICE-AUTOMATION-TARGETS-002.4
func TestStopRun(t *testing.T) {
	hardErr := errors.New("runtime stop failed")
	tests := []struct {
		name               string
		status             RunStatus
		stopped            bool
		stopErr            error
		settleStatus       RunStatus
		settleMessage      string
		missing            bool
		automationMismatch bool
		wantErr            error
		wantStatus         RunStatus
		wantMessage        string
	}{
		{name: "active turn stopped", status: RunStatusTaskCreated, stopped: true, wantStatus: RunStatusFailed, wantMessage: "stopped by user"},
		{name: "stale open turn settles", status: RunStatusTaskCreated, stopped: false, wantStatus: RunStatusFailed, wantMessage: "stopped by user"},
		{name: "completion settles concurrently", status: RunStatusTaskCreated, settleStatus: RunStatusSucceeded, settleMessage: "completed", wantStatus: RunStatusSucceeded, wantMessage: "completed"},
		{name: "missing run", missing: true, wantErr: ErrAutomationNotFound},
		{name: "foreign run", status: RunStatusTaskCreated, automationMismatch: true, wantErr: ErrAutomationNotFound, wantStatus: RunStatusTaskCreated},
		{name: "terminal run", status: RunStatusSucceeded, wantErr: ErrAutomationNotFound, wantStatus: RunStatusSucceeded},
		{name: "hard stop error", status: RunStatusTaskCreated, stopErr: hardErr, wantErr: hardErr, wantStatus: RunStatusTaskCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			requested := &Automation{WorkspaceID: "workspace", Name: "requested", Enabled: true}
			if err := svc.store.CreateAutomation(ctx, requested); err != nil {
				t.Fatal(err)
			}
			runID := "missing-run"
			if !tt.missing {
				owner := requested
				if tt.automationMismatch {
					owner = &Automation{WorkspaceID: "workspace", Name: "other", Enabled: true}
					if err := svc.store.CreateAutomation(ctx, owner); err != nil {
						t.Fatal(err)
					}
				}
				run := &AutomationRun{
					AutomationID: owner.ID,
					TriggerType:  TriggerTypeScheduled,
					Status:       tt.status,
					TaskID:       "task",
					SessionID:    "session",
					TurnID:       "turn",
				}
				if err := svc.store.CreateRun(ctx, run); err != nil {
					t.Fatal(err)
				}
				runID = run.ID
			}
			stopper := runStopperStub{stopped: tt.stopped, err: tt.stopErr}
			if tt.settleStatus != "" {
				stopper.onStop = func() {
					if err := svc.store.MarkRunTerminal(ctx, runID, "session", "turn", tt.settleStatus, tt.settleMessage); err != nil {
						t.Fatal(err)
					}
				}
			}
			svc.SetRunStopper(stopper)

			got, err := svc.StopRun(ctx, requested.ID, runID)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("StopRun error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if got == nil || got.Status != tt.wantStatus || got.ErrorMessage != tt.wantMessage {
					t.Fatalf("StopRun result = %+v, want status %q and message %q", got, tt.wantStatus, tt.wantMessage)
				}
			}
			if tt.missing {
				return
			}
			stored, getErr := svc.store.GetRun(ctx, runID)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if stored.Status != tt.wantStatus || stored.ErrorMessage != tt.wantMessage {
				t.Fatalf("stored run = %+v, want status %q and message %q", stored, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}
func TestBindRunTaskMissingRunIsNotDispatchable(t *testing.T) {
	svc := newTestService(t)
	require.ErrorIs(t, svc.BindRunTask(context.Background(), "missing-run", "task"), ErrAutomationRunNotDispatchable)
}
func TestRetrySuccessWinsAgainstDisableAtAutomationLock(t *testing.T) {
	svc := newTestService(t)
	svc.store.db.SetMaxOpenConns(1)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-success-disable", Name: "success disable", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	group := &RetryGroup{
		ID: "group-success-disable", AutomationID: a.ID, Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		TaskID: "task-success-disable", SessionID: "session-success-disable", TurnID: "turn-success-disable",
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))

	disableStarted := make(chan struct{})
	disableDone := make(chan error, 1)
	require.NoError(t, svc.WithRetryRunLock(ctx, run.ID, func(locked context.Context) error {
		go func() {
			close(disableStarted)
			disableDone <- svc.DisableAutomation(ctx, a.ID)
		}()
		<-disableStarted
		return svc.MarkAutomationRetrySucceeded(locked, run.ID, 1)
	}))
	require.NoError(t, <-disableDone)
	stored, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusSucceeded, stored.Status)
	require.Equal(t, RetryStateCompleted, stored.RetryState)
}

type recordingRunStopper struct {
	taskID, sessionID, turnID string
}

func (s *recordingRunStopper) StopAutomationRun(_ context.Context, taskID, sessionID, turnID string) (bool, error) {
	s.taskID, s.sessionID, s.turnID = taskID, sessionID, turnID
	return true, nil
}

func TestDisableAutomationStopsBoundRetryRunBeforeTerminalizing(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-disable", Name: "disable", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-disable", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		TaskID: "task-disable", SessionID: "session-disable", TurnID: "turn-disable",
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	stopper := &recordingRunStopper{}
	svc.SetRunStopper(stopper)

	require.NoError(t, svc.DisableAutomation(ctx, a.ID))
	require.Equal(t, "task-disable", stopper.taskID)
	require.Equal(t, "session-disable", stopper.sessionID)
	require.Equal(t, "turn-disable", stopper.turnID)
	stored, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusFailed, stored.Status)
	require.Equal(t, RetryStateCancelled, stored.RetryState)
}

func TestUpdateAutomationDisableCancelsRetryGroups(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-update-disable", Name: "update disable", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	group := &RetryGroup{
		ID: "group-update-disable", AutomationID: a.ID, Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateScheduled,
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	enabled := false

	_, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Enabled: &enabled})
	require.NoError(t, err)

	storedGroup, err := svc.store.GetRetryGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, RetryGroupCancelled, storedGroup.State)
	storedRun, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RetryStateCancelled, storedRun.RetryState)
}

func TestDisableAutomationCancelsRetriesWhenStoppingRunFails(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-disable-error", Name: "disable error", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-disable-error", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		TaskID: "task-disable-error", SessionID: "session-disable-error", TurnID: "turn-disable-error",
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	stopErr := errors.New("stop failed")
	svc.SetRunStopper(runStopperStub{err: stopErr})

	require.ErrorIs(t, svc.DisableAutomation(ctx, a.ID), stopErr)
	stored, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RetryStateCancelled, stored.RetryState)
}

func TestUpdateAutomationDisableCancelsRetriesWhenStoppingRunFails(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-update-disable-error", Name: "update disable error", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-update-disable-error", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		TaskID: "task-update-disable-error", SessionID: "session-update-disable-error", TurnID: "turn-update-disable-error",
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	stopErr := errors.New("stop failed")
	svc.SetRunStopper(runStopperStub{err: stopErr})
	enabled := false

	_, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Enabled: &enabled})
	require.ErrorIs(t, err, stopErr)
	stored, getErr := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, getErr)
	require.Equal(t, RetryStateCancelled, stored.RetryState)
}

func TestDeleteRunCancelsLiveRetryGroupBeforeDeletingRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-delete-retry", Name: "delete retry", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	group := &RetryGroup{
		ID: "group-delete-retry", AutomationID: a.ID, Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		TaskID: "task-delete-retry", SessionID: "session-delete-retry", TurnID: "turn-delete-retry",
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	var observedState RetryState
	svc.SetRunStopper(runStopperStub{
		stopped: true,
		onStop: func() {
			stored, err := svc.store.GetRun(ctx, run.ID)
			require.NoError(t, err)
			observedState = stored.RetryState
		},
	})

	require.NoError(t, svc.DeleteRun(ctx, run.ID))
	require.Equal(t, RetryStateTriggered, observedState)

	storedGroup, err := svc.store.GetRetryGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, RetryGroupCancelled, storedGroup.State)
	storedRun, err := svc.store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Nil(t, storedRun)
}

func TestDeleteAutomationStopsRetryRunBeforeCancellingGroup(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-delete-automation", Name: "delete automation", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	group := &RetryGroup{
		ID: "group-delete-automation", AutomationID: a.ID, Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		TaskID: "task-delete-automation", SessionID: "session-delete-automation", TurnID: "turn-delete-automation",
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	var observedState RetryState
	svc.SetRunStopper(runStopperStub{
		stopped: true,
		onStop: func() {
			stored, err := svc.store.GetRun(ctx, run.ID)
			require.NoError(t, err)
			observedState = stored.RetryState
		},
	})

	require.NoError(t, svc.DeleteAutomation(ctx, a.ID))
	require.Equal(t, RetryStateTriggered, observedState)
}
func TestWithRetryRunLockSerializesAndSupportsNestedDispatch(t *testing.T) {
	svc := newTestService(t)
	svc.store.db.SetMaxOpenConns(1)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-lock", Name: "lock", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	run := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTriggered}
	require.NoError(t, svc.store.CreateRun(ctx, run))

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- svc.WithRetryRunLock(ctx, run.ID, func(locked context.Context) error {
			close(entered)
			<-release
			return svc.DispatchRun(locked, run.ID, ThreadActionCreated, "created", func() (RunDispatch, error) {
				return RunDispatch{TaskID: "task-lock", SessionID: "session-lock", TurnID: "turn-lock"}, nil
			})
		})
	}()
	<-entered
	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- svc.WithRetryRunLock(ctx, run.ID, func(context.Context) error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("second retry operation entered while first lock was held")
	default:
	}
	close(release)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
}

// @covers AC-OFFICE-AUTOMATION-CONTINUITY-003.4
func TestReconcileOpenRuns(t *testing.T) {
	transientErr := errors.New("database temporarily unavailable")
	tests := []struct {
		name        string
		bound       bool
		live        bool
		livenessErr error
		wantStatus  RunStatus
		wantMessage string
	}{
		{name: "transient liveness error preserves run", bound: true, livenessErr: transientErr, wantStatus: RunStatusTaskCreated},
		{name: "not live settles run", bound: true, wantStatus: RunStatusFailed, wantMessage: "automation turn was stale after backend recovery"},
		{name: "live run stays open", bound: true, live: true, wantStatus: RunStatusTaskCreated},
		{name: "unbound admitted run settles", wantStatus: RunStatusFailed, wantMessage: "backend stopped before the automation turn was bound"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			automation := &Automation{WorkspaceID: "workspace", Name: "reconcile", Enabled: true}
			if err := svc.store.CreateAutomation(ctx, automation); err != nil {
				t.Fatal(err)
			}
			run := &AutomationRun{
				AutomationID: automation.ID,
				TriggerType:  TriggerTypeScheduled,
				Status:       RunStatusTriggered,
			}
			if tt.bound {
				run.Status = RunStatusTaskCreated
				run.TaskID = "task"
				run.SessionID = "session"
				run.TurnID = "turn"
			}
			if err := svc.store.CreateRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			svc.SetRunLivenessChecker(runLivenessStub{live: tt.live, err: tt.livenessErr})

			if err := svc.ReconcileOpenRuns(ctx); err != nil {
				t.Fatal(err)
			}

			stored, err := svc.store.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != tt.wantStatus || stored.ErrorMessage != tt.wantMessage {
				t.Fatalf("stored run = %+v, want status %q and message %q", stored, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}
