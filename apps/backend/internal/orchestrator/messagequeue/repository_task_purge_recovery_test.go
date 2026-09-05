package messagequeue

import (
	"context"
	"testing"
)

func TestSQLiteTaskPurgeRemovesOrdinaryDispatchClaim(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := insertTestEntry(t, repo, "session-purge-dispatch", "task-purge-dispatch", "prompt", QueuedByUser, nil, nil)
	reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
	if err != nil || reserved == nil {
		t.Fatalf("reserve ordinary head = %#v, err=%v", reserved, err)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := PurgeTaskInTransaction(ctx, tx, repo.db, source.TaskID, []string{source.SessionID})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatalf("removed visible rows = %d, want 0", removed)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("dispatch claims after task purge = %#v, want empty", pending)
	}
}

func TestSQLiteTaskPurgeRemovesAttachmentCleanup(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	cleanup := AttachmentCleanup{
		SessionID: "session-purge-cleanup", EntryID: "entry-purge-cleanup",
		OperationID: "operation-purge-cleanup", TaskID: "task-purge-cleanup",
		Attachments: []MessageAttachment{{AttachmentID: "attachment-purge-cleanup"}},
	}
	if err := repo.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
		t.Fatal(err)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PurgeTaskInTransaction(ctx, tx, repo.db, cleanup.TaskID, []string{cleanup.SessionID}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("attachment cleanups after task purge = %#v, want empty", pending)
	}
}

func TestSQLiteTaskPurgePreservesOtherTaskRecoveryInSharedSession(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	const (
		sessionID = "session-shared-purge"
		taskA     = "task-shared-purge-a"
		taskB     = "task-shared-purge-b"
	)
	entryA := insertTestEntry(t, repo, sessionID, taskA, "prompt-a", QueuedByUser, nil, nil)
	entryB := insertTestEntry(t, repo, sessionID, taskB, "prompt-b", QueuedByUser, nil, nil)
	for _, wantID := range []string{entryA.ID, entryB.ID} {
		reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, sessionID)
		if err != nil || reserved == nil || reserved.ID != wantID {
			t.Fatalf("reserve %s = %#v, err=%v", wantID, reserved, err)
		}
	}
	for _, cleanup := range []AttachmentCleanup{
		{
			SessionID: sessionID, EntryID: entryA.ID, OperationID: "cleanup-a", TaskID: taskA,
			Attachments: []MessageAttachment{{AttachmentID: "attachment-a"}},
		},
		{
			SessionID: sessionID, EntryID: entryB.ID, OperationID: "cleanup-b", TaskID: taskB,
			Attachments: []MessageAttachment{{AttachmentID: "attachment-b"}},
		},
	} {
		if err := repo.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
			t.Fatal(err)
		}
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PurgeTaskInTransaction(ctx, tx, repo.db, taskA, []string{sessionID}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	claims, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].Message.ID != entryB.ID {
		t.Fatalf("remaining dispatch claims = %#v, want only %s", claims, entryB.ID)
	}
	cleanups, err := repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanups) != 1 || cleanups[0].EntryID != entryB.ID {
		t.Fatalf("remaining attachment cleanups = %#v, want only %s", cleanups, entryB.ID)
	}
}

func TestSQLiteTaskPurgeCreatesAbsentRecoveryTables(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PurgeTaskInTransaction(
		ctx, tx, repo.db, "task-without-recovery-tables", []string{"session-without-recovery-tables"},
	); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var claims int
	if err := repo.db.GetContext(ctx, &claims, `SELECT COUNT(*) FROM queue_dispatch_claims`); err != nil {
		t.Fatalf("query recovery table after purge: %v", err)
	}
}

func TestSQLiteRepositoryTaskPurgeDiscoversOrdinaryDispatchSession(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := insertTestEntry(t, repo, "session-direct-purge", "task-direct-purge", "prompt", QueuedByUser, nil, nil)
	reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
	if err != nil || reserved == nil {
		t.Fatalf("reserve ordinary head = %#v, err=%v", reserved, err)
	}
	if _, err := repo.PurgeTask(ctx, source.TaskID); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("dispatch claims after direct task purge = %#v, want empty", pending)
	}
}
