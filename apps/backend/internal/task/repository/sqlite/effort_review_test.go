package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func reviewActivitySession(t *testing.T) (*Repository, *models.TaskSession) {
	t.Helper()
	r := newRepoForSessionTests(t)
	ctx := context.Background()
	require.NoError(t, r.CreateWorkspace(ctx, &models.Workspace{ID: "review-workspace", Name: "Review workspace"}))
	require.NoError(t, r.CreateTask(ctx, &models.Task{ID: "review-task", WorkspaceID: "review-workspace", Title: "Review task"}))
	r.SetUserActivityCapture(true)
	at := time.Date(2026, 5, 2, 5, 29, 28, 685000000, time.UTC)
	require.NoError(t, r.RecordCaptureTransition(ctx, true, at.Add(-time.Hour)))
	session := &models.TaskSession{ID: "review-session", TaskID: "review-task", StartedAt: at}
	require.NoError(t, r.CreateTaskSession(ctx, session))
	return r, session
}

func TestSessionTurnsExcludeLifecycleOnly(t *testing.T) {
	r, s := reviewActivitySession(t)
	ctx := context.Background()
	for _, row := range []*models.Turn{
		{ID: "lifecycle", TaskID: s.TaskID, TaskSessionID: s.ID, StartedAt: s.StartedAt, Metadata: map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true}},
		{ID: "real-work", TaskID: s.TaskID, TaskSessionID: s.ID, StartedAt: s.StartedAt},
	} {
		require.NoError(t, r.CreateTurn(ctx, row))
	}
	page, err := r.ListSessionTurns(ctx, models.SessionActivityFilter{SessionIDs: []string{s.ID}}, "", 1)
	require.NoError(t, err)
	require.Len(t, page.Turns, 1)
	require.Equal(t, "real-work", page.Turns[0].ID)
	require.False(t, page.HasMore)
}

func TestActivityMillisecondsAreExact(t *testing.T) {
	r, s := reviewActivitySession(t)
	ctx := context.Background()
	for _, actor := range []string{"one", "two"} {
		_, err := r.RecordUserActivity(ctx, actor, s.ID, s.StartedAt)
		require.NoError(t, err)
		_, err = r.RecordUserActivity(ctx, actor, s.ID, s.StartedAt.Add(15*time.Second))
		require.NoError(t, err)
	}
	rows, err := r.SumUserActivity(ctx, []string{s.ID})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(30000), rows[0].ActiveMS)
}

func TestActivityCaptureStampSurvivesMetadataRewrite(t *testing.T) {
	r, s := reviewActivitySession(t)
	require.Equal(t, true, s.Metadata["user_activity_capture"], "insert must return the persisted capture stamp to its caller")
	s.Metadata["other"] = "retained"
	require.NoError(t, r.UpdateTaskSessionWithMetadata(context.Background(), s, s.Metadata))
	stored, err := r.GetTaskSession(context.Background(), s.ID)
	require.NoError(t, err)
	require.Equal(t, true, stored.Metadata["user_activity_capture"])
	rows, err := r.SumUserActivity(context.Background(), []string{s.ID})
	require.NoError(t, err)
	require.True(t, rows[0].ActivityMeasured)
}

func TestActivityReadsOnlyEnumerableSessions(t *testing.T) {
	r, s := reviewActivitySession(t)
	ctx := context.Background()
	for _, task := range []*models.Task{
		{ID: "config-task", WorkspaceID: "review-workspace", Title: "Configuration", Metadata: map[string]interface{}{"config_mode": true}},
		{ID: "orphan-task", Title: "Missing workspace"},
	} {
		require.NoError(t, r.CreateTask(ctx, task))
		require.NoError(t, r.CreateTaskSession(ctx, &models.TaskSession{ID: task.ID + "-session", TaskID: task.ID, StartedAt: s.StartedAt}))
	}
	for _, sessionID := range []string{s.ID, "config-task-session", "orphan-task-session"} {
		session, err := r.GetTaskSession(ctx, sessionID)
		require.NoError(t, err)
		require.NoError(t, r.CreateTurn(ctx, &models.Turn{ID: sessionID + "-turn", TaskSessionID: sessionID, TaskID: session.TaskID, StartedAt: s.StartedAt}))
	}
	for name, filter := range map[string]models.SessionActivityFilter{"all": {}, "explicit": {SessionIDs: []string{s.ID, "config-task-session", "orphan-task-session"}}} {
		t.Run(name+"/sessions", func(t *testing.T) {
			sessions, err := r.ListUserActivitySessions(ctx, filter, "", 10)
			require.NoError(t, err)
			require.Len(t, sessions.Sessions, 1)
			require.Equal(t, s.ID, sessions.Sessions[0].SessionID)
		})
		t.Run(name+"/turns", func(t *testing.T) {
			turns, err := r.ListSessionTurns(ctx, filter, "", 10)
			require.NoError(t, err)
			require.Len(t, turns.Turns, 1)
			require.Equal(t, s.ID, turns.Turns[0].SessionID)
		})
	}
}
