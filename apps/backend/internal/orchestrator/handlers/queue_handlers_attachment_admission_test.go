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

func TestQueuedMessageFingerprintSurvivesSQLiteRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-fingerprint", "task-fingerprint", "queued", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{Type: "resource", AttachmentID: "attachment-fingerprint"}},
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	_, reopenedQueue, reopenedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = reopenedDB.Close() }()
	stored, err := reopenedQueue.GetEntry(ctx, entry.SessionID, entry.ID)
	require.NoError(t, err)
	before, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	after, err := queuedMessageFingerprint(stored)
	require.NoError(t, err)
	require.Equal(t, before, after, "queued message changed during SQLite restart:\nbefore: %#v\nafter: %#v", entry, stored)
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
	fingerprint, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "admission-restart",
		TaskID: entry.TaskID, OwnerID: "owner", RemoveEntry: true,
		EntryFingerprint: fingerprint, Attachments: entry.Attachments,
	}))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	reloadedCleanups, err := restartedQueue.ListAttachmentCleanups(ctx)
	require.NoError(t, err)
	require.Len(t, reloadedCleanups, 1)
	require.Equal(t, fingerprint, reloadedCleanups[0].EntryFingerprint)
	reloadedEntry, err := restartedQueue.GetEntry(ctx, entry.SessionID, entry.ID)
	require.NoError(t, err)
	reloadedFingerprint, err := queuedMessageFingerprint(reloadedEntry)
	require.NoError(t, err)
	require.Equal(t, fingerprint, reloadedFingerprint)
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
	fingerprint, err := queuedMessageFingerprint(entry)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: entry.SessionID, EntryID: entry.ID, OperationID: "claim-restart",
		TaskID: entry.TaskID, OwnerID: "owner", ClaimPending: true,
		EntryFingerprint: fingerprint, Attachments: entry.Attachments,
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

type failSecondCleanupUpsertQueue struct {
	QueueService
	service *messagequeue.Service
	upserts atomic.Int32
}

func (q *failSecondCleanupUpsertQueue) AttachmentCleanupPersistenceAvailable() bool { return true }

func (q *failSecondCleanupUpsertQueue) UpsertAttachmentCleanup(
	ctx context.Context,
	cleanup messagequeue.AttachmentCleanup,
) error {
	if q.upserts.Add(1) == 2 {
		return errors.New("cleanup state update unavailable")
	}
	return q.service.UpsertAttachmentCleanup(ctx, cleanup)
}

func (q *failSecondCleanupUpsertQueue) DeleteAttachmentCleanup(
	ctx context.Context,
	sessionID, entryID, operationID string,
) error {
	return q.service.DeleteAttachmentCleanup(ctx, sessionID, entryID, operationID)
}

func (q *failSecondCleanupUpsertQueue) ListAttachmentCleanups(
	ctx context.Context,
) ([]messagequeue.AttachmentCleanup, error) {
	return q.service.ListAttachmentCleanups(ctx)
}

func (q *failSecondCleanupUpsertQueue) WithSessionAdmission(
	ctx context.Context,
	sessionID string,
	fn func(context.Context) error,
) error {
	return q.service.WithSessionAdmission(ctx, sessionID, fn)
}

func (q *failSecondCleanupUpsertQueue) RemoveEntryWithEntry(
	ctx context.Context,
	sessionID, entryID string,
) (*messagequeue.QueuedMessage, error) {
	return q.service.RemoveEntryWithEntry(ctx, sessionID, entryID)
}

func TestAdmissionClaimFailureRemovesEntryWhenCleanupStateUpdateFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	handlers, queue, db := newPersistentCleanupQueue(t, dbPath)
	handlers.queueService = &failSecondCleanupUpsertQueue{QueueService: queue, service: queue}
	claimer := &admissionCleanupAssertingClaimer{store: queue}
	handlers.SetAttachmentClaimer(claimer)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})

	response, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"session_id": "session-admission-update-failure",
		"task_id":    "task-admission-update-failure",
		"content":    "queued",
		"attachments": []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-admission-update-failure",
			Name: "attachment.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, queue.GetStatus(ctx, "session-admission-update-failure").Entries)
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	recoveryClaimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(recoveryClaimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		cleanups, listErr := restartedQueue.ListAttachmentCleanups(ctx)
		return listErr == nil && len(cleanups) == 0 && recoveryClaimer.releases.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Zero(t, recoveryClaimer.claims.Load())
}

func TestAdmissionCleanupDoesNotMutateReplacementEntryAfterRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	_, queue, db := newPersistentCleanupQueue(t, dbPath)
	ctx := context.Background()
	original, err := queue.QueueMessage(
		ctx, "session-cleanup-fence", "task-cleanup-fence", "original", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-original",
			Name: "original.txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	)
	require.NoError(t, err)
	fingerprint, err := queuedMessageFingerprint(original)
	require.NoError(t, err)
	require.NoError(t, queue.UpsertAttachmentCleanup(ctx, messagequeue.AttachmentCleanup{
		SessionID: original.SessionID, EntryID: original.ID, OperationID: "admission-fenced",
		TaskID: original.TaskID, OwnerID: "owner", RemoveEntry: true,
		EntryFingerprint: fingerprint, Attachments: original.Attachments,
	}))
	require.NoError(t, queue.UpdateMessageWithMetadata(
		ctx, original.SessionID, original.ID, "replacement",
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "attachment-replacement",
			Name: "replacement.txt", MimeType: "text/plain", SizeBytes: 1,
		}}, nil, messagequeue.QueuedByUser,
	))
	require.NoError(t, db.Close())

	restarted, restartedQueue, restartedDB := newPersistentCleanupQueue(t, dbPath)
	defer func() { _ = restartedDB.Close() }()
	claimer := &admissionClaimRecoveryClaimer{}
	restarted.SetAttachmentClaimer(claimer)
	restarted.Start(context.Background())
	defer restarted.Stop()

	require.Eventually(t, func() bool {
		cleanups, listErr := restartedQueue.ListAttachmentCleanups(ctx)
		return listErr == nil && len(cleanups) == 0 && claimer.releases.Load() == 1
	}, time.Second, 10*time.Millisecond)
	current, err := restartedQueue.GetEntry(ctx, original.SessionID, original.ID)
	require.NoError(t, err)
	require.Equal(t, "replacement", current.Content)
	require.Zero(t, claimer.claims.Load())
}

func TestAdmissionCleanupWaitsForAnyActiveEditLease(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	ctx := context.Background()
	entry, err := queue.QueueMessage(
		ctx, "session-admission-lease", "task-admission-lease", "queued", "",
		messagequeue.QueuedByUser, false, nil,
	)
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	pending := &pendingQueueAttachmentCleanup{
		req:     wsUpdateMessageRequest{SessionID: entry.SessionID, EntryID: entry.ID},
		authCtx: ctx,
	}

	require.True(t, handlers.editLeaseActive(pending))
	require.NoError(t, queue.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection"))
	require.False(t, handlers.editLeaseActive(pending))
}
