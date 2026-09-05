package messagequeue

import (
	"context"
	"testing"
)

func TestPostgresRepository_DurableQueueRecoveryTables(t *testing.T) {
	ctx := context.Background()
	repo := newTestPostgresRepo(t)
	persistent := repo.(*sqliteRepository)

	t.Run("attachment cleanup replays schema and upsert", func(t *testing.T) {
		cleanup := AttachmentCleanup{
			SessionID: "cleanup-session", EntryID: "cleanup-entry", OperationID: "cleanup-op",
			TaskID: "cleanup-task", OwnerID: "owner-1", LeaseID: "lease-1",
			Attachments: []MessageAttachment{{Name: "first.txt", Data: "first"}},
		}
		if err := persistent.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
			t.Fatal(err)
		}
		cleanup.OwnerID = "owner-2"
		cleanup.Attachments = []MessageAttachment{{Name: "second.txt", Data: "second"}}
		if err := persistent.UpsertAttachmentCleanup(ctx, cleanup); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			cleanups, err := persistent.ListAttachmentCleanups(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(cleanups) != 1 || cleanups[0].OwnerID != "owner-2" || cleanups[0].Attachments[0].Name != "second.txt" {
				t.Fatalf("attachment cleanups = %#v", cleanups)
			}
		}
		if err := persistent.DeleteAttachmentCleanup(ctx, cleanup.SessionID, cleanup.EntryID, cleanup.OperationID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("session transfer compensation replays schema and upsert", func(t *testing.T) {
		compensation := SessionTransferCompensation{
			TaskID: "transfer-task", FromSessionID: "transfer-old", ToSessionID: "transfer-new",
			EntryIDs: []string{"entry-1"},
		}
		if err := persistent.UpsertSessionTransferCompensation(ctx, compensation); err != nil {
			t.Fatal(err)
		}
		compensation.EntryIDs = []string{"entry-1", "entry-2"}
		if err := persistent.UpsertSessionTransferCompensation(ctx, compensation); err != nil {
			t.Fatal(err)
		}
		compensations, err := persistent.ListSessionTransferCompensations(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(compensations) != 1 || len(compensations[0].EntryIDs) != 2 || compensations[0].EntryIDs[1] != "entry-2" {
			t.Fatalf("session transfer compensations = %#v", compensations)
		}
		if err := persistent.DeleteSessionTransferCompensation(ctx, compensation.TaskID, compensation.FromSessionID, compensation.ToSessionID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("send now claim records acceptance", func(t *testing.T) {
		source := insertTestEntry(t, repo, "send-now-session", "send-now-task", "prompt", QueuedByUser, nil, nil)
		claim, err := repo.ClaimSendNow(ctx, source.SessionID, []QueuedMessage{*source})
		if err != nil {
			t.Fatal(err)
		}
		if err := persistent.MarkPendingSendNowClaimAccepted(ctx, claim); err != nil {
			t.Fatal(err)
		}
		claims, err := persistent.ListPendingSendNowClaims(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(claims) != 1 || !claims[0].Accepted || claims[0].Claim.Dispatch.ID != claim.Dispatch.ID {
			t.Fatalf("pending Send Now claims = %#v", claims)
		}
		if err := repo.AcknowledgeSendNowClaim(ctx, claim); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ordinary dispatch claim records acceptance", func(t *testing.T) {
		source := insertTestEntry(t, repo, "dispatch-session", "dispatch-task", "prompt", QueuedByUser, nil, nil)
		reserved, enabled, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		if !enabled || reserved == nil || reserved.ID != source.ID {
			t.Fatalf("ordinary reservation = %#v, enabled=%t", reserved, enabled)
		}
		if err := persistent.MarkPendingQueueDispatchAccepted(ctx, reserved); err != nil {
			t.Fatal(err)
		}
		dispatches, err := persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 1 || !dispatches[0].Accepted || dispatches[0].Message.ID != source.ID {
			t.Fatalf("pending ordinary dispatches = %#v", dispatches)
		}
		if err := persistent.DeletePendingQueueDispatch(ctx, reserved); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("task purge preserves attachment recovery obligation", func(t *testing.T) {
		source := insertTestEntry(t, repo, "purge-session", "purge-task", "prompt", QueuedByUser, nil, nil)
		if err := persistent.UpsertAttachmentCleanup(ctx, AttachmentCleanup{
			SessionID: source.SessionID, EntryID: source.ID, OperationID: "purge-operation",
			TaskID: source.TaskID, Attachments: []MessageAttachment{{AttachmentID: "purge-attachment"}},
		}); err != nil {
			t.Fatal(err)
		}
		if reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID); err != nil || reserved == nil {
			t.Fatalf("reserve purge source = %#v, err=%v", reserved, err)
		}
		if _, err := repo.PurgeTask(ctx, source.TaskID); err != nil {
			t.Fatal(err)
		}
		dispatches, err := persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		cleanups, err := persistent.ListAttachmentCleanups(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 0 || len(cleanups) != 1 {
			t.Fatalf("recovery rows after purge: dispatches=%#v cleanups=%#v", dispatches, cleanups)
		}
	})

	t.Run("session mutation reconciles ordinary dispatch claims", func(t *testing.T) {
		source := insertTestEntry(t, repo, "mutation-old", "mutation-task", "prompt", QueuedByUser, nil, nil)
		if reserved, _, err := repo.ReserveHeadIfAutoRun(ctx, source.SessionID); err != nil || reserved == nil {
			t.Fatalf("reserve mutation source = %#v, err=%v", reserved, err)
		}
		if err := repo.TransferSession(ctx, source.SessionID, "mutation-new"); err != nil {
			t.Fatal(err)
		}
		dispatches, err := persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 1 || dispatches[0].Message.SessionID != "mutation-new" {
			t.Fatalf("transferred dispatch claims = %#v", dispatches)
		}
		if err := repo.ReplaceSession(ctx, "mutation-new", nil, nil); err != nil {
			t.Fatal(err)
		}
		dispatches, err = persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 0 {
			t.Fatalf("dispatch claims after replacement = %#v", dispatches)
		}
	})
}

func TestPostgresAttachmentCleanupMigratesAndTransfersCurrentSession(t *testing.T) {
	ctx := context.Background()
	repo := newTestPostgresRepo(t).(*sqliteRepository)
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE IF EXISTS queue_attachment_cleanups`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TABLE queue_attachment_cleanups (
			session_id TEXT NOT NULL,
			entry_id TEXT NOT NULL,
			operation_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL,
			owner_id TEXT NOT NULL DEFAULT '',
			lease_id TEXT NOT NULL DEFAULT '',
			remove_entry INTEGER NOT NULL DEFAULT 0,
			claim_pending INTEGER NOT NULL DEFAULT 0,
			entry_fingerprint TEXT NOT NULL DEFAULT '',
			attachments_json TEXT NOT NULL DEFAULT '[]',
			created_at TIMESTAMP NOT NULL,
			PRIMARY KEY (session_id, entry_id, operation_id)
		)
	`); err != nil {
		t.Fatal(err)
	}
	const (
		sourceSessionID = "postgres-cleanup-source"
		middleSessionID = "postgres-cleanup-middle"
		finalSessionID  = "postgres-cleanup-final"
	)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO queue_attachment_cleanups
			(session_id, entry_id, operation_id, task_id, attachments_json, created_at)
		VALUES (?, 'postgres-cleanup-entry', 'postgres-cleanup-operation',
			'postgres-cleanup-task', '[]', CURRENT_TIMESTAMP)
	`), sourceSessionID); err != nil {
		t.Fatal(err)
	}

	cleanups, err := repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanups) != 1 || cleanups[0].CurrentSessionID != sourceSessionID {
		t.Fatalf("migrated PostgreSQL cleanup = %#v", cleanups)
	}
	if err := repo.TransferSession(ctx, sourceSessionID, middleSessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.TransferSession(ctx, middleSessionID, finalSessionID); err != nil {
		t.Fatal(err)
	}
	cleanups, err = repo.ListAttachmentCleanups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanups) != 1 ||
		cleanups[0].SessionID != sourceSessionID ||
		cleanups[0].CurrentSessionID != finalSessionID {
		t.Fatalf("multi-hop PostgreSQL cleanup = %#v", cleanups)
	}
}

type replaceBeforeReservationRepository struct {
	Repository
	replacement Repository
}

func (r *replaceBeforeReservationRepository) ReserveHeadIfAutoRun(
	ctx context.Context,
	sessionID string,
) (*QueuedMessage, bool, error) {
	entries, err := r.replacement.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, true, err
	}
	if err := r.replacement.ReplaceSession(ctx, sessionID, entries, nil); err != nil {
		return nil, true, err
	}
	return r.Repository.ReserveHeadIfAutoRun(ctx, sessionID)
}

func TestPostgresRepository_ReservationCapturesGenerationAfterConcurrentReplacement(t *testing.T) {
	repoA, repoB, _ := newTestPostgresRepoPair(t)
	ctx := context.Background()
	source := insertTestEntry(t, repoA, "reservation-race-session", "reservation-race-task", "prompt", QueuedByUser, nil, nil)
	service := setupService(t)
	service.repo = &replaceBeforeReservationRepository{Repository: repoA, replacement: repoB}

	reserved, exists, autoRun := service.ReserveQueuedWithAutoRun(ctx, source.SessionID)
	if !exists || !autoRun || reserved == nil {
		t.Fatalf("reservation = %#v, exists=%t, autoRun=%t", reserved, exists, autoRun)
	}
	generation, err := repoA.SessionGeneration(ctx, source.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reserved.reservationGenerationsCaptured || reserved.reservationSessionGeneration != generation {
		t.Fatalf(
			"reservation generation = %d (captured=%t), want current %d",
			reserved.reservationSessionGeneration, reserved.reservationGenerationsCaptured, generation,
		)
	}
}
