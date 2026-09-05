package messagequeue

import (
	"context"
	"errors"
	"fmt"
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
func TestSQLitePurgeRemovesDurableSendNowClaims(t *testing.T) {
	for _, purge := range []struct {
		name string
		run  func(context.Context, Repository) error
	}{
		{
			name: "session",
			run: func(ctx context.Context, repo Repository) error {
				_, err := repo.PurgeSession(ctx, "session-purge-send-now")
				return err
			},
		},
		{
			name: "task",
			run: func(ctx context.Context, repo Repository) error {
				_, err := repo.PurgeTask(ctx, "task-purge-send-now")
				return err
			},
		},
	} {
		for _, accepted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/accepted=%t", purge.name, accepted), func(t *testing.T) {
				repo := newTestSQLiteRepo(t)
				ctx := context.Background()
				entry := insertTestEntry(
					t, repo, "session-purge-send-now", "task-purge-send-now",
					"durable source", QueuedByUser, nil, nil,
				)
				if _, err := repo.ClaimSendNow(ctx, entry.SessionID, []QueuedMessage{*entry}); err != nil {
					t.Fatal(err)
				}
				persistent := repo.(pendingSendNowClaimRepository)
				if accepted {
					if err := persistent.MarkPendingSendNowClaimAccepted(ctx, entry.SessionID); err != nil {
						t.Fatal(err)
					}
				}

				if err := purge.run(ctx, repo); err != nil {
					t.Fatal(err)
				}
				claims, err := persistent.ListPendingSendNowClaims(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(claims) != 0 {
					t.Fatalf("durable Send Now claims after purge = %#v, want none", claims)
				}
			})
		}
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
