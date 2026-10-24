package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func activityMeasured(t *testing.T, repo *Repository, sessionID string) bool {
	t.Helper()
	rows, err := repo.SumUserActivity(context.Background(), []string{sessionID})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return rows[0].ActivityMeasured
}

// A boot whose capture transition never commits runs with capture disabled, so
// sessions alive during that boot must not read as measured, either while it
// runs or after a later boot succeeds.
func TestUnconfirmedBootBecomesCaptureGap(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 6, 9, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "boot-task", Title: "boot"}))

	require.NoError(t, repo.RecordCaptureTransition(ctx, true, base))
	repo.SetUserActivityCapture(true)
	spanning := &models.TaskSession{ID: "spans-failed-boot", TaskID: "boot-task", StartedAt: base.Add(time.Second)}
	require.NoError(t, repo.CreateTaskSession(ctx, spanning))
	require.True(t, activityMeasured(t, repo, spanning.ID))

	_, err := repo.db.ExecContext(ctx, `
		CREATE TRIGGER fail_capture_confirmation
		BEFORE UPDATE ON kandev_meta
		WHEN OLD.key = 'user_activity_capture' AND NEW.value NOT LIKE '%unconfirmed_since%'
		BEGIN SELECT RAISE(ABORT, 'injected capture confirmation failure'); END
	`)
	require.NoError(t, err)
	failedBoot := base.Add(10 * time.Second)
	require.Error(t, repo.RecordCaptureTransition(ctx, true, failedBoot))
	repo.SetUserActivityCapture(false)
	require.False(t, activityMeasured(t, repo, spanning.ID), "reads during the unconfirmed boot must report the session unmeasured")

	_, err = repo.db.ExecContext(ctx, `DROP TRIGGER fail_capture_confirmation`)
	require.NoError(t, err)
	nextBoot := base.Add(20 * time.Second)
	require.NoError(t, repo.RecordCaptureTransition(ctx, true, nextBoot))
	repo.SetUserActivityCapture(true)
	later := &models.TaskSession{ID: "after-failed-boot", TaskID: "boot-task", StartedAt: nextBoot.Add(time.Second)}
	require.NoError(t, repo.CreateTaskSession(ctx, later))

	require.False(t, activityMeasured(t, repo, spanning.ID), "a session alive during the unconfirmed boot stays unmeasured")
	require.True(t, activityMeasured(t, repo, later.ID), "the gap closes when the next boot confirms capture")
	state, err := repo.readUserActivityCaptureState(ctx)
	require.NoError(t, err)
	require.True(t, state.Enabled)
	require.Len(t, state.Gaps, 1)
	require.True(t, state.Gaps[0].Off.Equal(failedBoot), "gap starts when the unconfirmed boot started: %+v", state.Gaps[0])
	require.NotNil(t, state.Gaps[0].On)
	require.True(t, state.Gaps[0].On.Equal(nextBoot), "gap ends when the next boot confirms: %+v", state.Gaps[0])
}
