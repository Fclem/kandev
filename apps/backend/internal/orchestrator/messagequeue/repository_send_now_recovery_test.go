package messagequeue

import (
	"context"
	"errors"
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
		ListPendingSendNowClaims(context.Context) ([]PendingSendNowClaim, error)
	})
	if !ok {
		t.Fatal("SQLite queue repository does not persist pending Send Now claims")
	}
	pending, err := persistent.ListPendingSendNowClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Claim.Dispatch.ID != claim.Dispatch.ID {
		t.Fatalf("pending claims = %#v, want dispatch %s", pending, claim.Dispatch.ID)
	}

	if err := repo.RestoreSendNowClaim(ctx, &pending[0].Claim); err != nil {
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

func TestSQLiteSendNowAcknowledgeGenerationChangeRetiresClaim(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	first := insertTestEntry(t, repo, "session-1", "task-1", "first", QueuedByUser, nil, nil)
	claim, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*first})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DeleteAllBySession(ctx, "session-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AcknowledgeSendNowClaim(ctx, claim); !errors.Is(err, ErrSendNowClaimChanged) {
		t.Fatalf("acknowledge after generation change error = %v, want %v", err, ErrSendNowClaimChanged)
	}

	second := insertTestEntry(t, repo, "session-1", "task-1", "second", QueuedByUser, nil, nil)
	if _, err := repo.ClaimSendNow(ctx, "session-1", []QueuedMessage{*second}); err != nil {
		t.Fatalf("second same-process Send Now claim: %v", err)
	}
}
