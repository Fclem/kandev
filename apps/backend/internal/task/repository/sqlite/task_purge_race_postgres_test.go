package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// newTaskPostgresRepoPair opens one isolated Postgres schema and constructs
// two task repositories over it (plus the queue tables the purge touches,
// which production creates on the same database). Two instances simulate two
// backend processes sharing a queue database.
func newTaskPostgresRepoPair(t *testing.T) (*Repository, *Repository) {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	setupDB, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if _, err := setupDB.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = setupDB.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = setupDB.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = setupDB.Close()
	})
	open := func() *Repository {
		db, err := sqlx.Open("pgx", dsn)
		if err != nil {
			t.Fatalf("open postgres: %v", err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.Exec("SET search_path TO " + schema); err != nil {
			t.Fatalf("set postgres search_path %s: %v", schema, err)
		}
		if _, err := messagequeue.NewSQLiteRepository(db, db); err != nil {
			t.Fatalf("init queue schema: %v", err)
		}
		repo, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("init task schema: %v", err)
		}
		return repo
	}
	repoA, repoB := open(), open()
	return repoA, repoB
}

func seedTaskWithSession(t *testing.T, repo *Repository, taskID, workspaceID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Purge race"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: "wf-" + taskID,
		WorkflowStepID: "step-" + taskID, Title: taskID, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, State: models.TaskSessionStateIdle}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

// TestPostgresRepository_DeleteTask_LocksEmptySessionDuringPurge proves
// DeleteTask captures the task's session set BEFORE deleting the task row
// (task_sessions cascades on deletion): the purge must lock the session even
// when its queue is empty, so a concurrent admission cannot survive the
// purge. The competing backend holds the session lock; DeleteTask blocks on
// it, proving the empty session was still locked.
func TestPostgresRepository_DeleteTask_LocksEmptySessionDuringPurge(t *testing.T) {
	repoA, repoB := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const (
		taskID = "task-del-race"
		sessID = "sess-del-race"
	)
	seedTaskWithSession(t, repoA, taskID, "ws-del-race", sessID)

	dbA := repoA.db
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('sess-del-race')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'sess-del-race' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	delDone := make(chan error, 1)
	go func() {
		delDone <- repoB.DeleteTask(ctx, taskID)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			-- A blocked row-lock waiter is observable as an ungranted
			-- transactionid lock on the blocking transaction.
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("DeleteTask never blocked on the empty session's lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-delDone; err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var count int
	if err := dbA.GetContext(ctx, &count, `SELECT count(*) FROM tasks WHERE id = 'task-del-race'`); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("task survived delete: count=%d", count)
	}
}

// TestPostgresRepository_WorkspaceCascade_GuardsTaskRowsBeforeSessionLocks
// proves the cascade establishes the global task-row -> session-lock order:
// it must block on a held task row BEFORE taking any queue session lock,
// otherwise lifecycle admission (task row first, then session lock) and the
// cascade deadlock on Postgres.
func TestPostgresRepository_WorkspaceCascade_GuardsTaskRowsBeforeSessionLocks(t *testing.T) {
	repoA, repoB := newTaskPostgresRepoPair(t)
	ctx := context.Background()
	const taskID = "task-cascade-race"
	seedTaskWithSession(t, repoA, taskID, "ws-cascade-race", "sess-cascade-race")

	dbA := repoA.db
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM tasks WHERE id = 'task-cascade-race' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock task row: %v", err)
	}

	cascadeDone := make(chan error, 1)
	go func() {
		_, _, err := repoB.deleteWorkspaceCascade(ctx, "ws-cascade-race", nil, nil)
		cascadeDone <- err
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cascade never blocked on the task row guard (lock order inverted)")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-cascadeDone; err != nil {
		t.Fatalf("workspace cascade: %v", err)
	}
	var count int
	if err := dbA.GetContext(ctx, &count, `SELECT count(*) FROM workspaces WHERE id = 'ws-cascade-race'`); err != nil {
		t.Fatalf("count workspaces: %v", err)
	}
	if count != 0 {
		t.Fatalf("workspace survived cascade: count=%d", count)
	}
}
