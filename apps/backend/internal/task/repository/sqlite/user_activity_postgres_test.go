package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresUserActivityLedger(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task repository: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "activity-pg-task", Title: "activity"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := repo.RecordCaptureTransition(ctx, true, base); err != nil {
		t.Fatalf("RecordCaptureTransition: %v", err)
	}
	repo.SetUserActivityCapture(true)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "activity-pg-session", TaskID: "activity-pg-task", StartedAt: base}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	for _, at := range []time.Time{base, base.Add(4 * time.Second), base.Add(10 * time.Second), base.Add(47 * time.Second)} {
		if _, err := repo.RecordUserActivity(ctx, "actor", "activity-pg-session", at); err != nil {
			t.Fatalf("RecordUserActivity: %v", err)
		}
	}
	summaries, err := repo.SumUserActivity(ctx, []string{"activity-pg-session"})
	if err != nil {
		t.Fatalf("SumUserActivity: %v", err)
	}
	if len(summaries) != 1 || summaries[0].IntervalCount != 2 || summaries[0].ActiveMS != 6000 || !summaries[0].ActivityMeasured {
		t.Fatalf("summary = %+v, want two measured intervals totaling 6000ms", summaries)
	}

	const beats = 12
	start := make(chan struct{})
	errs := make(chan error, beats)
	var wg sync.WaitGroup
	for range beats {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := repo.RecordUserActivity(ctx, "actor", "activity-pg-session", base.Add(100*time.Second))
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent RecordUserActivity: %v", err)
		}
	}
	summaries, err = repo.SumUserActivity(ctx, []string{"activity-pg-session"})
	if err != nil {
		t.Fatalf("SumUserActivity after concurrent beats: %v", err)
	}
	if len(summaries) != 1 || summaries[0].IntervalCount != 3 || summaries[0].ActiveMS != 6000 {
		t.Fatalf("summary after concurrent beats = %+v, want one additional non-overlapping zero-length interval", summaries)
	}
}

func TestPostgresSessionActivityKeysetReads(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task repository: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "activity-pg-workspace", Name: "Activity reads"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "activity-page-pg-task", WorkspaceID: "activity-pg-workspace", Title: "activity page"}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 5, 4, 3, 4, 5, 0, time.UTC)
	for _, task := range []*models.Task{
		{ID: "pg-config-task", WorkspaceID: "activity-pg-workspace", Title: "Config", Metadata: map[string]interface{}{"config_mode": true}},
		{ID: "pg-orphan-task", Title: "Missing workspace"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		sessionID := task.ID + "-session"
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: task.ID, StartedAt: base}); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateTurn(ctx, &models.Turn{ID: task.ID + "-turn", TaskSessionID: sessionID, TaskID: task.ID, StartedAt: base}); err != nil {
			t.Fatal(err)
		}
	}
	for _, sessionID := range []string{"pg-session-a", "pg-session-b"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: "activity-page-pg-task", StartedAt: base}); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := repo.ListUserActivitySessions(ctx, models.SessionActivityFilter{}, "", 1)
	if err != nil {
		t.Fatalf("ListUserActivitySessions: %v", err)
	}
	if len(sessions.Sessions) != 1 || sessions.Sessions[0].SessionID != "pg-session-a" || !sessions.HasMore {
		t.Fatalf("session page = %+v, want pg-session-a with a continuation", sessions)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "pg-lifecycle-only", TaskSessionID: "pg-session-a", TaskID: "activity-page-pg-task", StartedAt: base,
		Metadata: map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"pg-turn-a", "pg-turn-b"} {
		if err := repo.CreateTurn(ctx, &models.Turn{ID: id, TaskSessionID: "pg-session-a", TaskID: "activity-page-pg-task", StartedAt: base}); err != nil {
			t.Fatal(err)
		}
	}
	turns, err := repo.ListSessionTurns(ctx, models.SessionActivityFilter{}, "", 1)
	if err != nil {
		t.Fatalf("ListSessionTurns: %v", err)
	}
	if len(turns.Turns) != 1 || turns.Turns[0].ID != "pg-turn-a" || !turns.HasMore {
		t.Fatalf("turn page = %+v, want pg-turn-a with a continuation", turns)
	}
}
