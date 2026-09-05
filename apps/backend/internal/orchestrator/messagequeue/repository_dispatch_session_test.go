package messagequeue

import (
	"context"
	"testing"
)

func TestSQLiteTransferSessionMovesOrdinaryDispatchClaim(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := insertTestEntry(t, repo, "session-transfer-old", "task-transfer", "prompt", QueuedByUser, nil, nil)
	reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
	if err != nil || reserved == nil {
		t.Fatalf("reserve ordinary head = %#v, err=%v", reserved, err)
	}
	if err := repo.TransferSession(ctx, source.SessionID, "session-transfer-new"); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Message.SessionID != "session-transfer-new" {
		t.Fatalf("dispatch claims after transfer = %#v", pending)
	}
	if err := repo.MarkPendingQueueDispatchAccepted(ctx, source.SessionID, source.ID); err != nil {
		t.Fatalf("mark migrated claim through original dispatch identity: %v", err)
	}
}

func TestSQLiteReplaceSessionInvalidatesOrdinaryDispatchClaims(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	source := insertTestEntry(t, repo, "session-replace", "task-replace", "prompt", QueuedByUser, nil, nil)
	reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
	if err != nil || reserved == nil {
		t.Fatalf("reserve ordinary head = %#v, err=%v", reserved, err)
	}
	if err := repo.ReplaceSession(ctx, source.SessionID, nil, nil); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.ListPendingQueueDispatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("dispatch claims after replacement = %#v, want empty", pending)
	}
}
