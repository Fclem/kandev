package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCaptureGapKeepsOnlyOverlappingSessionUnmeasured(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 5, 6, 7, 8, 0, time.UTC)
	if err := repo.CreateTask(ctx, &models.Task{ID: "capture-task", Title: "capture"}); err != nil {
		t.Fatal(err)
	}
	completedAt := base.Add(7 * time.Second)
	ended := &models.TaskSession{ID: "ended-in-gap", TaskID: "capture-task", StartedAt: base.Add(time.Second), CompletedAt: &completedAt}
	if err := repo.RecordCaptureTransition(ctx, true, base); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(true)
	if err := repo.CreateTaskSession(ctx, ended); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, false, base.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(false)
	if err := repo.RecordCaptureTransition(ctx, true, base.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(true)
	later := &models.TaskSession{ID: "created-after-gap", TaskID: "capture-task", StartedAt: base.Add(11 * time.Second)}
	if err := repo.CreateTaskSession(ctx, later); err != nil {
		t.Fatal(err)
	}
	got, err := repo.SumUserActivity(ctx, []string{ended.ID, later.ID})
	if err != nil {
		t.Fatal(err)
	}
	measured := make(map[string]bool, len(got))
	for _, summary := range got {
		measured[summary.SessionID] = summary.ActivityMeasured
	}
	if len(got) != 2 || measured[ended.ID] || !measured[later.ID] {
		t.Fatalf("measured state = %+v, want gap session false and post-gap session true", got)
	}
}

func TestCaptureTransitionIsIdempotentAndFirstRunOffCreatesNoGap(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 5, 7, 0, 0, 0, time.UTC)
	if err := repo.RecordCaptureTransition(ctx, false, base); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, true, base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, true, base.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, false, base.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, false, base.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, true, base.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	state, err := repo.readUserActivityCaptureState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || len(state.Gaps) != 1 || state.Gaps[0].On == nil {
		t.Fatalf("capture state = %+v, want one closed gap after first-run-off", state)
	}
}

func TestCaptureGapMeasuredStateUsesSessionLifetimeNotUpdatedAt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 5, 8, 0, 0, 0, time.UTC)
	if err := repo.CreateTask(ctx, &models.Task{ID: "lifetime-task", Title: "lifetime"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, true, base); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(true)
	completedAt := base.Add(4 * time.Second)
	completedAtOff := base.Add(5 * time.Second)
	startedAtOn := base.Add(10 * time.Second)
	sessions := []*models.TaskSession{
		{ID: "ended-before-gap", TaskID: "lifetime-task", StartedAt: base, CompletedAt: &completedAt},
		{ID: "ends-at-disable", TaskID: "lifetime-task", StartedAt: base.Add(time.Second), CompletedAt: &completedAtOff},
		{ID: "open-at-disable", TaskID: "lifetime-task", StartedAt: base.Add(time.Second)},
		{ID: "resumed-across-gap", TaskID: "lifetime-task", StartedAt: base.Add(2 * time.Second), State: models.TaskSessionStateRunning},
	}
	for _, session := range sessions {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.UpdateTaskSessionState(ctx, "resumed-across-gap", models.TaskSessionStateIdle, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, false, base.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(false)
	if err := repo.RecordCaptureTransition(ctx, true, base.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(true)
	changed, _, err := repo.UpdateTaskSessionStateIfCurrent(
		ctx, "resumed-across-gap", models.TaskSessionStateIdle, models.TaskSessionStateRunning, "",
	)
	if err != nil || !changed {
		t.Fatalf("resume via state CAS = %v, %v", changed, err)
	}
	if err := repo.SetSessionPrimary(ctx, "open-at-disable"); err != nil {
		t.Fatal(err)
	}
	startsAtOn := &models.TaskSession{ID: "starts-at-reenable", TaskID: "lifetime-task", StartedAt: startedAtOn}
	if err := repo.CreateTaskSession(ctx, startsAtOn); err != nil {
		t.Fatal(err)
	}
	afterGap := &models.TaskSession{ID: "created-after-gap", TaskID: "lifetime-task", StartedAt: base.Add(11 * time.Second)}
	if err := repo.CreateTaskSession(ctx, afterGap); err != nil {
		t.Fatal(err)
	}
	got, err := repo.SumUserActivity(ctx, []string{"ended-before-gap", "ends-at-disable", "open-at-disable", "resumed-across-gap", startsAtOn.ID, afterGap.ID})
	if err != nil {
		t.Fatal(err)
	}
	measured := make(map[string]bool, len(got))
	for _, summary := range got {
		measured[summary.SessionID] = summary.ActivityMeasured
	}
	if len(got) != 6 || !measured["ended-before-gap"] || measured["ends-at-disable"] ||
		measured["open-at-disable"] || measured["resumed-across-gap"] ||
		!measured[startsAtOn.ID] || !measured[afterGap.ID] {
		t.Fatalf("measured state at gap boundaries, after resume, and after promotion = %+v", got)
	}
	state, err := repo.readUserActivityCaptureState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Gaps) != 1 || state.Gaps[0].On == nil {
		t.Fatalf("capture gaps = %+v, want exactly one closed interval", state.Gaps)
	}
}

func TestCaptureTransitionWriteFailureLeavesPriorStateAtomic(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC)
	if err := repo.RecordCaptureTransition(ctx, true, base); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TRIGGER fail_user_activity_capture_update
		BEFORE UPDATE ON kandev_meta
		WHEN OLD.key = 'user_activity_capture'
		BEGIN SELECT RAISE(ABORT, 'injected capture-state write failure'); END
	`); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, false, base.Add(time.Second)); err == nil {
		t.Fatal("RecordCaptureTransition error = nil, want injected persistence failure")
	}
	state, err := repo.readUserActivityCaptureState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || len(state.Gaps) != 0 {
		t.Fatalf("capture state after failed transition = %+v, want original enabled state", state)
	}
}
