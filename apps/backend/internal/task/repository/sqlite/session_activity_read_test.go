package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSessionUserActivityRead(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 2, 3, 4, 5, 0, time.UTC)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "activity-read-workspace", Name: "Activity reads"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "activity-read-task", WorkspaceID: "activity-read-workspace", Title: "activity read"}); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(true)
	if err := repo.RecordCaptureTransition(ctx, true, base.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"a-measured", "b-measured-zero", "c-unmeasured"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: "activity-read-task", StartedAt: base}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE task_sessions SET metadata = '{}' WHERE id = ?`, "c-unmeasured"); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{"actor-one", "actor-two"} {
		if _, err := repo.RecordUserActivity(ctx, actor, "a-measured", base.Add(10*time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.RecordUserActivity(ctx, actor, "a-measured", base.Add(20*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	firstPage, err := repo.ListUserActivitySessions(ctx, models.SessionActivityFilter{TaskIDs: []string{"activity-read-task"}}, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Sessions) != 2 || firstPage.Sessions[0].SessionID != "a-measured" || firstPage.Sessions[1].SessionID != "b-measured-zero" || !firstPage.HasMore || firstPage.NextCursor == "" {
		t.Fatalf("first activity-session page = %+v, want ordered first two rows and cursor", firstPage)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "d-inserted-later", TaskID: "activity-read-task", StartedAt: base.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	secondPage, err := repo.ListUserActivitySessions(ctx, models.SessionActivityFilter{TaskIDs: []string{"activity-read-task"}}, firstPage.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Sessions) != 2 || secondPage.Sessions[0].SessionID != "c-unmeasured" || secondPage.Sessions[1].SessionID != "d-inserted-later" || secondPage.HasMore {
		t.Fatalf("second activity-session page after insert = %+v, want remaining rows without duplicates", secondPage)
	}

	summaries, err := repo.SumUserActivity(ctx, []string{"a-measured", "b-measured-zero", "c-unmeasured"})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]models.UserActivitySummary, len(summaries))
	for _, summary := range summaries {
		byID[summary.SessionID] = summary
	}
	measured := byID["a-measured"]
	if !measured.ActivityMeasured || measured.IntervalCount != 2 || measured.ActiveMS != 20000 {
		t.Fatalf("multi-actor summary = %+v, want two intervals totaling 20000ms", measured)
	}
	if !measured.FirstAt.Equal(base.Add(10*time.Second)) || !measured.LastAt.Equal(base.Add(20*time.Second)) {
		t.Fatalf("activity range = %s to %s, want %s to %s", measured.FirstAt, measured.LastAt, base.Add(10*time.Second), base.Add(20*time.Second))
	}
	zero := byID["b-measured-zero"]
	if !zero.ActivityMeasured || zero.IntervalCount != 0 || zero.ActiveMS != 0 {
		t.Fatalf("measured-zero summary = %+v, want measured true with zero totals", zero)
	}
	if byID["c-unmeasured"].ActivityMeasured {
		t.Fatalf("unmeasured summary = %+v, want ActivityMeasured=false", byID["c-unmeasured"])
	}
}

func TestSessionTurnsRead(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 3, 3, 4, 5, 0, time.UTC)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "turn-read-workspace", Name: "Turn reads"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "turn-read-task", WorkspaceID: "turn-read-workspace", Title: "turn read"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "turn-read-session", TaskID: "turn-read-task", StartedAt: base}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"turn-a", "turn-b", "turn-c"} {
		if err := repo.CreateTurn(ctx, &models.Turn{
			ID: id, TaskSessionID: "turn-read-session", TaskID: "turn-read-task", StartedAt: base,
			Metadata: map[string]interface{}{"private": "not part of the read DTO"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	filter := models.SessionActivityFilter{SessionIDs: []string{"turn-read-session"}, TaskIDs: []string{"turn-read-task"}}
	firstPage, err := repo.ListSessionTurns(ctx, filter, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Turns) != 1 || firstPage.Turns[0].ID != "turn-a" || !firstPage.HasMore || firstPage.NextCursor == "" || firstPage.NextCursor == "1" {
		t.Fatalf("first turn page = %+v, want turn-a with opaque cursor", firstPage)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-d", TaskSessionID: "turn-read-session", TaskID: "turn-read-task", StartedAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	secondPage, err := repo.ListSessionTurns(ctx, filter, firstPage.NextCursor, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Turns) != 3 || secondPage.Turns[0].ID != "turn-b" || secondPage.Turns[1].ID != "turn-c" || secondPage.Turns[2].ID != "turn-d" || secondPage.HasMore {
		t.Fatalf("second turn page after insert = %+v, want remaining ordered turns", secondPage)
	}
	if secondPage.Turns[0].SessionID != "turn-read-session" || secondPage.Turns[0].TaskID != "turn-read-task" {
		t.Fatalf("turn read row = %+v, want session and task identity", secondPage.Turns[0])
	}
}
