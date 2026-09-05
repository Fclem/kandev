package messagequeue

import (
	"context"
	"errors"
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

func TestSQLiteStaleOrdinarySettlementCannotResurrectAfterSessionMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *Service, *QueuedMessage) error
		settle func(context.Context, *Service, *QueuedMessage) error
	}{
		{
			name: "transfer before restore",
			mutate: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				return service.TransferSession(ctx, msg.SessionID, "session-transfer-destination")
			},
			settle: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				_, err := service.RestoreMessage(ctx, msg)
				return err
			},
		},
		{
			name: "replace before requeue",
			mutate: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				return service.repo.ReplaceSession(ctx, msg.SessionID, nil, nil)
			},
			settle: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				return service.RequeueAtHead(ctx, msg)
			},
		},
		{
			name: "clear before restore",
			mutate: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				_, err := service.CancelAll(ctx, msg.SessionID)
				return err
			},
			settle: func(ctx context.Context, service *Service, msg *QueuedMessage) error {
				_, err := service.RestoreMessage(ctx, msg)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newTestSQLiteRepo(t)
			service := setupService(t)
			service.repo = repo
			source, err := service.QueueMessage(
				ctx, "session-stale", "task-stale", "prompt", "", QueuedByUser, false, nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			reserved, ok := service.ReserveQueued(ctx, source.SessionID)
			if !ok || reserved.ID != source.ID {
				t.Fatalf("reserved = %#v, ok=%t", reserved, ok)
			}
			if err := tc.mutate(ctx, service, reserved); err != nil {
				t.Fatal(err)
			}
			if err := tc.settle(ctx, service, reserved); !errors.Is(err, ErrQueueDispatchClaimChanged) {
				t.Fatalf("stale settlement error = %v, want %v", err, ErrQueueDispatchClaimChanged)
			}
			if entries, err := repo.ListBySession(ctx, source.SessionID); err != nil || len(entries) != 0 {
				t.Fatalf("stale source queue = %#v, err=%v", entries, err)
			}
		})
	}
}
