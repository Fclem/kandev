package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCreateTaskSessionStampsCaptureState(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "stamp-task", Title: "stamp"}); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(true)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "stamped-session", TaskID: "stamp-task", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var metadata string
	if err := repo.db.QueryRowContext(ctx, `SELECT metadata FROM task_sessions WHERE id = ?`, "stamped-session").Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var values map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &values); err != nil {
		t.Fatal(err)
	}
	if stamped, _ := values["user_activity_capture"].(bool); !stamped {
		t.Fatalf("session metadata = %s, want user_activity_capture=true", metadata)
	}
}

func TestRecordUserActivityDisabledStoresNothing(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "disabled-task", Title: "disabled"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "disabled-session", TaskID: "disabled-task", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	outcome, err := repo.RecordUserActivity(ctx, "actor", "disabled-session", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if outcome != userActivityOutcomeDisabled {
		t.Fatalf("outcome = %q, want %q", outcome, userActivityOutcomeDisabled)
	}
	var count int
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_session_user_activity`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stored intervals = %d, want none while capture is disabled", count)
	}
}

func TestEveryCreateTaskSessionVariantStampsCaptureState(t *testing.T) {
	variants := []struct {
		name   string
		create func(*testing.T, *Repository, context.Context, *models.TaskSession) error
	}{
		{name: "CreateTaskSession", create: func(_ *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) error {
			return repo.CreateTaskSession(ctx, session)
		}},
		{name: "CreateTaskSessionWithInitialRuntimeSeed", create: func(_ *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) error {
			return repo.CreateTaskSessionWithInitialRuntimeSeed(ctx, session)
		}},
		{name: "CreateTaskSessionWithInitialRuntimeSeedAndWorkflowRoute", create: func(_ *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) error {
			return repo.CreateTaskSessionWithInitialRuntimeSeedAndWorkflowRoute(ctx, session, nil)
		}},
		{name: "CreateTaskSessionWithWorkflowSessionRoute", create: func(_ *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) error {
			return repo.CreateTaskSessionWithWorkflowSessionRoute(ctx, session, nil)
		}},
		{name: "CreateTaskSessionWithWorkspaceBinding", create: func(t *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) error {
			return repo.CreateTaskSessionWithWorkspaceBinding(ctx, session, &models.TaskEnvironment{
				TaskID: session.TaskID, ExecutorType: string(models.ExecutorTypeLocal),
			})
		}},
		{name: "CreateTaskSessionWithWorkspaceBindingAndWorkflowRoute", create: func(t *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) error {
			return repo.CreateTaskSessionWithWorkspaceBindingAndWorkflowRoute(ctx, session, &models.TaskEnvironment{
				TaskID: session.TaskID, ExecutorType: string(models.ExecutorTypeLocal),
			}, nil)
		}},
		{name: "CreateTaskSessionWithSharedGroupWorkspaceBinding", create: func(t *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) error {
			return createSharedGroupSessionForCaptureTest(t, repo, ctx, session, false)
		}},
		{name: "CreateTaskSessionWithSharedGroupWorkspaceBindingAndWorkflowRoute", create: func(t *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession) error {
			return createSharedGroupSessionForCaptureTest(t, repo, ctx, session, true)
		}},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			workspaceID := "workspace-" + variant.name
			taskID := "task-" + variant.name
			seedWorkspace(t, repo, workspaceID)
			if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID}); err != nil {
				t.Fatal(err)
			}
			repo.SetUserActivityCapture(true)
			session := &models.TaskSession{ID: "session-" + variant.name, TaskID: taskID}
			if err := variant.create(t, repo, ctx, session); err != nil {
				t.Fatal(err)
			}
			assertUserActivityCaptureStamp(t, repo, ctx, session.ID)
		})
	}
}

func createSharedGroupSessionForCaptureTest(t *testing.T, repo *Repository, ctx context.Context, session *models.TaskSession, routeAware bool) error {
	t.Helper()
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TABLE task_workspace_groups (
			id TEXT PRIMARY KEY,
			materialized_environment_id TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE task_workspace_group_members (
			workspace_group_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			released_at TIMESTAMP NULL
		);
		INSERT INTO task_workspace_groups (id, updated_at) VALUES ('capture-group', CURRENT_TIMESTAMP);
		INSERT INTO task_workspace_group_members (workspace_group_id, task_id)
		VALUES ('capture-group', ?);
	`, session.TaskID); err != nil {
		return err
	}
	candidate := &models.TaskEnvironment{TaskID: session.TaskID, ExecutorType: string(models.ExecutorTypeLocal)}
	if routeAware {
		return repo.CreateTaskSessionWithSharedGroupWorkspaceBindingAndWorkflowRoute(ctx, session, candidate, "capture-group", nil)
	}
	return repo.CreateTaskSessionWithSharedGroupWorkspaceBinding(ctx, session, candidate, "capture-group")
}

func assertUserActivityCaptureStamp(t *testing.T, repo *Repository, ctx context.Context, sessionID string) {
	t.Helper()
	var metadata string
	if err := repo.db.QueryRowContext(ctx, `SELECT metadata FROM task_sessions WHERE id = ?`, sessionID).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var values map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &values); err != nil {
		t.Fatal(err)
	}
	if stamped, _ := values["user_activity_capture"].(bool); !stamped {
		t.Fatalf("session metadata = %s, want user_activity_capture=true", metadata)
	}
}

func TestActivityBeatsAndCaptureTransitionsDoNotChangeSessionUpdatedAt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	if err := repo.CreateTask(ctx, &models.Task{ID: "updated-at-task", Title: "updated at"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, true, base); err != nil {
		t.Fatal(err)
	}
	repo.SetUserActivityCapture(true)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "updated-at-session", TaskID: "updated-at-task", StartedAt: base}); err != nil {
		t.Fatal(err)
	}
	before, err := repo.GetTaskSession(ctx, "updated-at-session")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RecordUserActivity(ctx, "actor", before.ID, base); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RecordUserActivity(ctx, "actor", before.ID, base.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordCaptureTransition(ctx, false, base.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}
	after, err := repo.GetTaskSession(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("session updated_at changed from %s to %s", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestRecordUserActivityRejectsArchivedAndCleanupBlockedTasks(t *testing.T) {
	for _, scenario := range []string{"archived", "cleanup-in-progress"} {
		t.Run(scenario, func(t *testing.T) {
			repo := newBarrierTestRepo(t)
			ctx := context.Background()
			base := time.Date(2026, 4, 5, 11, 0, 0, 0, time.UTC)
			taskID := "activity-" + scenario
			sessionID := "session-" + scenario
			seedBarrierTask(t, repo, taskID)
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, StartedAt: base}); err != nil {
				t.Fatal(err)
			}
			repo.SetUserActivityCapture(true)
			if err := repo.RecordCaptureTransition(ctx, true, base); err != nil {
				t.Fatal(err)
			}
			if scenario == "archived" {
				if _, err := repo.db.ExecContext(ctx, `UPDATE tasks SET archived_at = ? WHERE id = ?`, base, taskID); err != nil {
					t.Fatal(err)
				}
			} else {
				reserveBarrier(t, repo, taskID, "activity-cleanup-"+taskID)
			}
			outcome, err := repo.RecordUserActivity(ctx, "actor", sessionID, base.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			if outcome != userActivityOutcomeRejected {
				t.Fatalf("outcome = %q, want %q", outcome, userActivityOutcomeRejected)
			}
			var count int
			if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_session_user_activity`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("stored intervals = %d, want none for %s task", count, scenario)
			}
		})
	}
}
