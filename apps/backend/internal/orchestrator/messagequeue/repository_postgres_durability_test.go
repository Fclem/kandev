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
		if err := persistent.MarkPendingSendNowClaimAccepted(ctx, source.SessionID); err != nil {
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
		if err := persistent.MarkPendingQueueDispatchAccepted(ctx, source.SessionID, source.ID); err != nil {
			t.Fatal(err)
		}
		dispatches, err := persistent.ListPendingQueueDispatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(dispatches) != 1 || !dispatches[0].Accepted || dispatches[0].Message.ID != source.ID {
			t.Fatalf("pending ordinary dispatches = %#v", dispatches)
		}
		if err := persistent.DeletePendingQueueDispatch(ctx, source.SessionID, source.ID); err != nil {
			t.Fatal(err)
		}
	})
}
