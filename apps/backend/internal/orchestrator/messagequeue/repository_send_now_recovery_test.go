package messagequeue

import (
	"context"
	"testing"
)

func TestSQLiteSendNowClaimPersistsOrdinarySourceRecovery(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	source := insertTestEntry(t, repo, "session-1", "task-1", "ordinary prompt", QueuedByUser, nil, nil)

	claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*source})
	if err != nil {
		t.Fatal(err)
	}
	if entries, listErr := repo.ListBySession(ctx, "session-1"); listErr != nil || len(entries) != 0 {
		t.Fatalf("queue after claim = %#v, err=%v, want ordinary source removed", entries, listErr)
	}
	persistent, ok := repo.(interface {
		ListPendingSendNowClaims(context.Context) ([]SendNowClaim, error)
	})
	if !ok {
		t.Fatal("SQLite queue repository does not persist pending Send Now claims")
	}
	pending, err := persistent.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Dispatch.ID != claim.Dispatch.ID {
		t.Fatalf("pending claims = %#v, want dispatch %s", pending, claim.Dispatch.ID)
	}

	if err := repo.RestoreSendNowClaim(ctx, &pending[0]); err != nil {
		t.Fatal(err)
	}
	pending, err = persistent.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending claims after restore = %#v", pending)
	}
	entries, err := repo.ListBySession(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != source.ID {
		t.Fatalf("restored queue = %#v, want source %s", entries, source.ID)
	}
}
