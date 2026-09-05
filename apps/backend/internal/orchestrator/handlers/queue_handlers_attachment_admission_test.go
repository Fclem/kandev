package handlers

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type admissionCleanupAssertingClaimer struct {
	store          queueAttachmentCleanupStore
	claimAttempts  atomic.Int32
	sawObligation  atomic.Bool
	releaseAttempt atomic.Int32
}

func (c *admissionCleanupAssertingClaimer) ClaimMessageAttachments(
	ctx context.Context,
	_, _ string,
	_ []v1.MessageAttachment,
) error {
	c.claimAttempts.Add(1)
	cleanups, err := c.store.ListAttachmentCleanups(ctx)
	if err == nil && len(cleanups) == 1 && cleanups[0].ClaimPending {
		c.sawObligation.Store(true)
	}
	return errors.New("claim outcome unavailable")
}

func (c *admissionCleanupAssertingClaimer) ReleaseMessageAttachments(
	context.Context,
	string,
	string,
	[]v1.MessageAttachment,
) error {
	c.releaseAttempt.Add(1)
	return nil
}

func TestQueueAttachmentClaimFailureHasDurableCleanupBeforeClaim(t *testing.T) {
	handlers, queue, db := newPersistentCleanupQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	defer func() { _ = db.Close() }()
	claimer := &admissionCleanupAssertingClaimer{store: queue}
	handlers.SetAttachmentClaimer(claimer)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})

	response, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"session_id": "session-admission-cleanup",
		"task_id":    "task-admission-cleanup",
		"content":    "queued",
		"attachments": []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-admission-cleanup",
			Name: "attachment.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	}))

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Equal(t, int32(1), claimer.claimAttempts.Load())
	require.True(t, claimer.sawObligation.Load(), "cleanup obligation must exist before the claim starts")
	require.Equal(t, int32(1), claimer.releaseAttempt.Load())
	require.Empty(t, queue.GetStatus(ctx, "session-admission-cleanup").Entries)
	cleanups, err := queue.ListAttachmentCleanups(ctx)
	require.NoError(t, err)
	require.Empty(t, cleanups)
}

func TestAdmissionAttachmentCleanupRemovesEntryAfterRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-admission-restart", "task-admission-restart", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-admission-restart",
			Name: "attachment.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "admission-restart",
		TaskID: entry.TaskID, OwnerID: "owner", RemoveEntry: true,
		Attachments: entry.Attachments,
	}))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	claimer := &controlledCleanupClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		_, getErr := restartedQueue.GetEntry(ctx, entry.SessionID, entry.ID)
		return errors.Is(getErr, messagequeue.ErrEntryNotFound) && claimer.released.Load() == 1
	}, time.Second, 10*time.Millisecond)
	cleanups, err := restartedQueue.ListAttachmentCleanups(ctx)
	require.NoError(t, err)
	require.Empty(t, cleanups)
}

type admissionClaimRecoveryClaimer struct {
	claims   atomic.Int32
	releases atomic.Int32
}

func (c *admissionClaimRecoveryClaimer) ClaimMessageAttachments(
	context.Context,
	string,
	string,
	[]v1.MessageAttachment,
) error {
	c.claims.Add(1)
	return nil
}

func (c *admissionClaimRecoveryClaimer) ReleaseMessageAttachments(
	context.Context,
	string,
	string,
	[]v1.MessageAttachment,
) error {
	c.releases.Add(1)
	return nil
}

func TestPendingAdmissionClaimResumesWithoutRemovingEntry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-claim-restart", "task-claim-restart", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-claim-restart",
			Name: "attachment.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "claim-restart",
		TaskID: entry.TaskID, OwnerID: "owner", ClaimPending: true,
		Attachments: entry.Attachments,
	}))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	claimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		cleanups, listErr := restartedQueue.ListAttachmentCleanups(ctx)
		return listErr == nil && len(cleanups) == 0 && claimer.claims.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Zero(t, claimer.releases.Load())
	current, err := restartedQueue.GetEntry(ctx, entry.SessionID, entry.ID)
	require.NoError(t, err)
	require.Equal(t, entry.ID, current.ID)
}
