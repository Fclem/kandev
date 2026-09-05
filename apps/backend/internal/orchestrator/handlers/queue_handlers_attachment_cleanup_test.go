package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	eventtypes "github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestWsRemoveEntryReleasesClaimedAttachments(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)
	entry, err := queue.QueueMessage(context.Background(), "session", "task", "queued", "", "user", false, []messagequeue.MessageAttachment{{
		Type:         "resource",
		AttachmentID: "attachment",
		Name:         "report.txt",
		MimeType:     "text/plain",
	}})
	require.NoError(t, err)

	response, err := handlers.wsRemoveEntry(context.Background(), createTestMessage(t, ws.ActionMessageQueueRemove, map[string]string{
		"session_id": "session",
		"entry_id":   entry.ID,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, []string{"attachment"}, claimer.releases)
}

func TestWsRemoveEntryPreservesAttachmentReferencedByAnotherEntry(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()
	attachment := messagequeue.MessageAttachment{
		Type: "resource", AttachmentID: "shared-attachment", Name: "shared.txt", MimeType: "text/plain",
	}
	first, err := queue.QueueMessage(ctx, "session-shared", "task-shared", "first", "", "user", false, []messagequeue.MessageAttachment{attachment})
	require.NoError(t, err)
	_, err = queue.QueueMessage(ctx, "session-shared", "task-shared", "second", "", "user", false, []messagequeue.MessageAttachment{attachment})
	require.NoError(t, err)

	response, err := handlers.wsRemoveEntry(ctx, createTestMessage(t, ws.ActionMessageQueueRemove, map[string]string{
		"session_id": "session-shared",
		"entry_id":   first.ID,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Empty(t, claimer.releases)
}
func TestWsRemoveEntryUsesAtomicRemovedEntryForAttachmentCleanup(t *testing.T) {
	handlers, _ := setupQueueHandlers(t)
	stale := &messagequeue.QueuedMessage{
		ID: "entry", SessionID: "session", TaskID: "task",
		Attachments: []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "stale", Name: "stale.txt", MimeType: "text/plain",
		}},
	}
	current := *stale
	current.Attachments = []messagequeue.MessageAttachment{{
		Type: "resource", AttachmentID: "current", Name: "current.txt", MimeType: "text/plain",
	}}
	queue := &atomicRemoveQueueService{
		QueueService: handlers.queueService,
		stale:        stale,
		removed:      &current,
	}
	handlers.queueService = queue
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)

	response, err := handlers.wsRemoveEntry(context.Background(), createTestMessage(t, ws.ActionMessageQueueRemove, map[string]string{
		"session_id": "session",
		"entry_id":   "entry",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, 1, queue.atomicCalls)
	require.Equal(t, []string{"current"}, claimer.releases)
}

type atomicRemoveQueueService struct {
	QueueService
	stale, removed *messagequeue.QueuedMessage
	atomicCalls    int
}

func (s *atomicRemoveQueueService) GetEntry(context.Context, string, string) (*messagequeue.QueuedMessage, error) {
	return s.stale, nil
}

func (s *atomicRemoveQueueService) RemoveEntry(context.Context, string, string) error {
	return nil
}

func (s *atomicRemoveQueueService) RemoveEntryWithEntry(context.Context, string, string) (*messagequeue.QueuedMessage, error) {
	s.atomicCalls++
	return s.removed, nil
}

func TestWsCancelAllReleasesClaimedAttachments(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)
	for _, attachmentID := range []string{"first", "second"} {
		_, err := queue.QueueMessage(context.Background(), "session-cancel", "task-cancel", "queued", "", "user", false, []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: attachmentID, Name: attachmentID + ".txt", MimeType: "text/plain",
		}})
		require.NoError(t, err)
	}

	response, err := handlers.wsCancelAll(context.Background(), createTestMessage(t, ws.ActionMessageQueueCancel, map[string]string{
		"session_id": "session-cancel",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.ElementsMatch(t, []string{"first", "second"}, claimer.releases)
}
func TestWsCancelAllReportsOnlyRemovedEntries(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	ctx := context.Background()

	_, _, accepted, err := queue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "session-cancel-count", "task", "lifecycle", "", "", false, nil,
		nil, "lifecycle", true,
	)
	require.NoError(t, err)
	require.True(t, accepted)
	_, ok := queue.ReserveQueued(ctx, "session-cancel-count")
	require.True(t, ok)
	_, err = queue.QueueMessage(ctx, "session-cancel-count", "task", "ordinary", "", "user", false, nil)
	require.NoError(t, err)

	response, err := handlers.wsCancelAll(ctx, createTestMessage(t, ws.ActionMessageQueueCancel, map[string]string{
		"session_id": "session-cancel-count",
	}))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.EqualValues(t, 1, payload["removed"])
	require.Equal(t, 0, queue.GetStatus(ctx, "session-cancel-count").Count)
}

func TestWsUpdateMessageRollsBackBeforeSuccessorCanAcquireEdit(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session", "task", "original", "", "user", false, []messagequeue.MessageAttachment{{
		Type:         "resource",
		AttachmentID: "new-attachment",
		Name:         "new.txt",
		MimeType:     "text/plain",
	}})
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	claimer := &successorEditRaceClaimer{
		queue:         queue,
		sessionID:     entry.SessionID,
		entryID:       entry.ID,
		leaseID:       lease.LeaseID,
		successorID:   "connection-b",
		successorDone: make(chan struct{}),
	}
	handlers.SetAttachmentClaimer(claimer)
	response, err := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-a"),
		createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id":               "session",
			"entry_id":                 entry.ID,
			"lease_id":                 lease.LeaseID,
			"operation_id":             "operation-1",
			"expected_target_revision": lease.TargetRevision,
			"content":                  "edited",
			"attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "new-attachment", Name: "new.txt", MimeType: "text/plain",
			}},
		}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	<-claimer.successorDone
	require.False(t, claimer.successBeforeRelease.Load())
	require.True(t, claimer.releaseObserved.Load())
}
func TestWsUpdateMessageWithLeaseReleasesSupersededAttachments(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session-update", "task-update", "before", "", "user", false, []messagequeue.MessageAttachment{{
		Type: "resource", AttachmentID: "old-attachment", Name: "old.txt", MimeType: "text/plain",
	}})
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)

	response, err := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-a"),
		createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id":               entry.SessionID,
			"entry_id":                 entry.ID,
			"lease_id":                 lease.LeaseID,
			"operation_id":             "operation-update",
			"expected_target_revision": lease.TargetRevision,
			"content":                  "after",
			"attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "new-attachment", Name: "new.txt", MimeType: "text/plain",
			}},
		}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, []string{"new-attachment"}, claimer.claims)
	require.Equal(t, []string{"old-attachment"}, claimer.releases)
}

type failingThenSuccessfulQueueAttachmentClaimer struct {
	failures atomic.Int32
	attempts atomic.Int32
	released atomic.Int32
}

func (c *failingThenSuccessfulQueueAttachmentClaimer) ClaimMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return nil
}

func (c *failingThenSuccessfulQueueAttachmentClaimer) ReleaseMessageAttachments(_ context.Context, _ string, _ string, attachments []v1.MessageAttachment) error {
	c.attempts.Add(1)
	if c.failures.Load() > 0 {
		c.failures.Add(-1)
		return errors.New("transient attachment release failure")
	}
	for _, attachment := range attachments {
		if attachment.AttachmentID != "" {
			c.released.Add(1)
		}
	}
	return nil
}

func TestWsUpdateMessageRetriesSupersededAttachmentCleanupAfterLeaseEnd(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &failingThenSuccessfulQueueAttachmentClaimer{}
	claimer.failures.Store(1)
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session-retry-cleanup", "task-retry-cleanup", "before", "", "user", false,
		[]messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "old-attachment", Name: "old.txt", MimeType: "text/plain",
		}})
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-retry")
	require.NoError(t, err)
	message := func() *ws.Message {
		return createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id":               entry.SessionID,
			"entry_id":                 entry.ID,
			"lease_id":                 lease.LeaseID,
			"operation_id":             "operation-retry",
			"expected_target_revision": lease.TargetRevision,
			"content":                  "after",
			"attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "new-attachment", Name: "new.txt", MimeType: "text/plain",
			}},
		})
	}

	firstResponse, err := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-retry"), message())
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, firstResponse.Type)
	require.NoError(t, queue.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection-retry"))
	require.Eventually(t, func() bool {
		return claimer.attempts.Load() >= 2 && claimer.released.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, int32(2), claimer.attempts.Load())
	require.Equal(t, int32(1), claimer.released.Load())
}

func TestWsRemoveEntryRetriesFailedAttachmentRelease(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &failingThenSuccessfulQueueAttachmentClaimer{}
	claimer.failures.Store(1)
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session-remove-retry", "task-remove-retry", "queued", "", "user", false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment-remove"}})
	require.NoError(t, err)

	response, err := handlers.wsRemoveEntry(ctx, createTestMessage(t, ws.ActionMessageQueueRemove, map[string]string{
		"session_id": entry.SessionID,
		"entry_id":   entry.ID,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Eventually(t, func() bool {
		return claimer.attempts.Load() == 2 && claimer.released.Load() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestWsCancelAllRetriesFailedAttachmentRelease(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &failingThenSuccessfulQueueAttachmentClaimer{}
	claimer.failures.Store(1)
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()
	_, err := queue.QueueMessage(ctx, "session-cancel-retry", "task-cancel-retry", "queued", "", "user", false,
		[]messagequeue.MessageAttachment{{AttachmentID: "attachment-cancel"}})
	require.NoError(t, err)

	response, err := handlers.wsCancelAll(ctx, createTestMessage(t, ws.ActionMessageQueueCancel, map[string]string{
		"session_id": "session-cancel-retry",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Eventually(t, func() bool {
		return claimer.attempts.Load() == 2 && claimer.released.Load() == 1
	}, time.Second, 10*time.Millisecond)
}

func TestPendingAttachmentCleanupResumesAfterHandlerRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue.db")
	handlers, queue, db := newPersistentCleanupQueue(t, dbPath)
	firstClaimer := &failingThenSuccessfulQueueAttachmentClaimer{}
	firstClaimer.failures.Store(100)
	handlers.SetAttachmentClaimer(firstClaimer)
	handlers.Start(context.Background())
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session-cleanup-restart", "task-cleanup-restart", "before", "", "user", false,
		[]messagequeue.MessageAttachment{{AttachmentID: "old-attachment"}})
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-restart")
	require.NoError(t, err)

	response, err := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-restart"),
		createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id": entry.SessionID, "entry_id": entry.ID, "lease_id": lease.LeaseID,
			"operation_id": "operation-restart", "expected_target_revision": lease.TargetRevision,
			"content": "after", "attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "new-attachment", Name: "new.txt", MimeType: "text/plain",
			}},
		}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, int32(1), firstClaimer.attempts.Load())
	handlers.Stop()
	require.NoError(t, db.Close())

	restarted, _, restartedDB := newPersistentCleanupQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	secondClaimer := &failingThenSuccessfulQueueAttachmentClaimer{}
	restarted.SetAttachmentClaimer(secondClaimer)
	restarted.Start(context.Background())
	t.Cleanup(restarted.Stop)

	require.Eventually(t, func() bool { return secondClaimer.released.Load() == 1 }, time.Second, 10*time.Millisecond)
}

func newPersistentCleanupQueue(t *testing.T, dbPath string) (*QueueHandlers, *messagequeue.Service, *sqlx.DB) {
	t.Helper()
	raw, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	repo, err := messagequeue.NewSQLiteRepository(db, db)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	queue := messagequeue.NewService(repo, messagequeue.DefaultMaxPerSession, log)
	queue.SetAutoMergeEnabled(false)
	handlers := NewQueueHandlers(queue, &mockEventBus{}, log, nil, allowQueueAccess{}, nil)
	return handlers, queue, db
}

type controlledCleanupClaimer struct {
	failures    atomic.Int32
	attempts    atomic.Int32
	released    atomic.Int32
	lastSession atomic.Value
	lastUser    atomic.Value
}

func (c *controlledCleanupClaimer) ClaimMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return nil
}

func (c *controlledCleanupClaimer) ReleaseMessageAttachments(ctx context.Context, _ string, sessionID string, attachments []v1.MessageAttachment) error {
	c.attempts.Add(1)
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.UserID == "" {
		return errors.New("missing attachment authorization")
	}
	if c.failures.Load() > 0 {
		c.failures.Add(-1)
		return errors.New("transient attachment release failure")
	}
	c.lastSession.Store(sessionID)
	c.lastUser.Store(identity.UserID)
	c.released.Add(int32(len(attachments)))
	return nil
}

func TestWsUpdateMessageCleanupRetryPreservesAuthAndPublishesStatus(t *testing.T) {
	handlers, queue, events := setupQueueHandlersWithResolver(t, func(context.Context, string) (string, error) {
		return "task-cleanup", nil
	})
	claimer := &controlledCleanupClaimer{}
	claimer.failures.Store(1)
	handlers.SetAttachmentClaimer(claimer)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-cleanup"})
	entry, err := queue.QueueMessage(ctx, "session-cleanup", "task-cleanup", "before", "", "user", false,
		[]messagequeue.MessageAttachment{{Type: "resource", AttachmentID: "old", Name: "old.txt", MimeType: "text/plain"}})
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-cleanup")
	require.NoError(t, err)

	response, err := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-cleanup"),
		createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id": entry.SessionID, "entry_id": entry.ID, "lease_id": lease.LeaseID,
			"operation_id": "operation-cleanup", "expected_target_revision": lease.TargetRevision,
			"content": "after", "attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "new", Name: "new.txt", MimeType: "text/plain",
			}},
		}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, "session-cleanup", events.lastData[fieldSessionID])
	require.NoError(t, queue.EndEdit(ctx, entry.SessionID, entry.ID, lease.LeaseID, "connection-cleanup"))
	require.Equal(t, eventtypes.MessageQueueStatusChanged, events.lastSubject)
	require.Eventually(t, func() bool { return claimer.released.Load() == 1 }, time.Second, 10*time.Millisecond)
	require.Equal(t, "user-cleanup", claimer.lastUser.Load())
}

func TestWsUpdateMessageCleanupRetryReconcilesRemovedEntry(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &controlledCleanupClaimer{}
	claimer.failures.Store(1)
	handlers.SetAttachmentClaimer(claimer)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-cleanup"})
	entry, err := queue.QueueMessage(ctx, "session-removed", "task-removed", "before", "", "user", false,
		[]messagequeue.MessageAttachment{{Type: "resource", AttachmentID: "old", Name: "old.txt", MimeType: "text/plain"}})
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-removed")
	require.NoError(t, err)
	_, err = handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-removed"),
		createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id": entry.SessionID, "entry_id": entry.ID, "lease_id": lease.LeaseID,
			"operation_id": "operation-removed", "expected_target_revision": lease.TargetRevision,
			"content": "after", "attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "new", Name: "new.txt", MimeType: "text/plain",
			}},
		}))
	require.NoError(t, err)
	require.NoError(t, queue.RemoveEntry(ctx, entry.SessionID, entry.ID))
	require.Eventually(t, func() bool { return claimer.released.Load() == 1 }, time.Second, 10*time.Millisecond)
}

func TestWsUpdateMessageCleanupRetryFollowsTransferredEntry(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &controlledCleanupClaimer{}
	claimer.failures.Store(1)
	handlers.SetAttachmentClaimer(claimer)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-cleanup"})
	entry, err := queue.QueueMessage(ctx, "session-transfer-old", "task-transfer", "before", "", "user", false,
		[]messagequeue.MessageAttachment{{Type: "resource", AttachmentID: "old", Name: "old.txt", MimeType: "text/plain"}})
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-transfer")
	require.NoError(t, err)
	_, err = handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-transfer"),
		createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id": entry.SessionID, "entry_id": entry.ID, "lease_id": lease.LeaseID,
			"operation_id": "operation-transfer", "expected_target_revision": lease.TargetRevision,
			"content": "after", "attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "new", Name: "new.txt", MimeType: "text/plain",
			}},
		}))
	require.NoError(t, err)
	require.NoError(t, queue.TransferSession(ctx, entry.SessionID, "session-transfer-new"))
	require.Eventually(t, func() bool { return claimer.released.Load() == 1 }, time.Second, 10*time.Millisecond)
	require.Equal(t, "session-transfer-new", claimer.lastSession.Load())
}

type successorEditRaceClaimer struct {
	queue                *messagequeue.Service
	sessionID            string
	entryID              string
	leaseID              string
	successorID          string
	successorDone        chan struct{}
	successBeforeRelease atomic.Bool
	releaseObserved      atomic.Bool
}

func (c *successorEditRaceClaimer) ClaimMessageAttachments(ctx context.Context, _, _ string, _ []v1.MessageAttachment) error {
	if err := c.queue.EndEdit(ctx, c.sessionID, c.entryID, c.leaseID, "connection-a"); err != nil {
		return err
	}
	go func() {
		defer close(c.successorDone)
		lease, err := c.queue.BeginEdit(context.Background(), c.sessionID, c.entryID, c.successorID)
		if err != nil {
			return
		}
		if !c.releaseObserved.Load() {
			c.successBeforeRelease.Store(true)
		}
		_ = c.queue.EndEdit(context.Background(), c.sessionID, c.entryID, lease.LeaseID, c.successorID)
	}()
	select {
	case <-c.successorDone:
	case <-time.After(100 * time.Millisecond):
	}
	return nil
}

func (c *successorEditRaceClaimer) ReleaseMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	c.releaseObserved.Store(true)
	return nil
}

type contextRecordingReleaser struct {
	err   error
	calls int
}

func (r *contextRecordingReleaser) ClaimMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return nil
}

func (r *contextRecordingReleaser) ReleaseMessageAttachments(ctx context.Context, _ string, _ string, _ []v1.MessageAttachment) error {
	r.calls++
	r.err = ctx.Err()
	return nil
}

func TestQueueAttachmentCleanupDetachesCancelledContext(t *testing.T) {
	handlers, _ := setupQueueHandlers(t)
	releaser := &contextRecordingReleaser{}
	handlers.SetAttachmentClaimer(releaser)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handlers.releaseQueuedAttachments(ctx, &messagequeue.QueuedMessage{
		SessionID: "session",
		TaskID:    "task",
		Attachments: []messagequeue.MessageAttachment{{
			AttachmentID: "attachment",
		}},
	})

	if releaser.err != nil {
		t.Fatalf("attachment cleanup context error = %v, want nil", releaser.err)
	}
}
func TestWsRemoveEntryDetachesCancelledCleanupContext(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	entry, err := queue.QueueMessage(context.Background(), "session", "task", "queued", "", "user", false, []messagequeue.MessageAttachment{{
		AttachmentID: "attachment",
	}})
	require.NoError(t, err)
	cleanupQueue := &cleanupContextQueueService{QueueService: queue, entry: entry}
	handlers.queueService = cleanupQueue
	claimer := &contextRecordingReleaser{}
	handlers.SetAttachmentClaimer(claimer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	response, err := handlers.wsRemoveEntry(ctx, createTestMessage(t, ws.ActionMessageQueueRemove, map[string]string{
		"session_id": entry.SessionID,
		"entry_id":   entry.ID,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.NoError(t, cleanupQueue.referenceErr)
	require.NoError(t, claimer.err)
}

type cleanupContextQueueService struct {
	QueueService
	entry        *messagequeue.QueuedMessage
	getEntryErr  error
	referenceErr error
}

func (s *cleanupContextQueueService) GetEntry(ctx context.Context, _, _ string) (*messagequeue.QueuedMessage, error) {
	s.getEntryErr = ctx.Err()
	return s.entry, nil
}

func (s *cleanupContextQueueService) ReferencedQueueAttachmentIDs(ctx context.Context, _, _ string, _ []string) (map[string]struct{}, error) {
	s.referenceErr = ctx.Err()
	return nil, nil
}
func (s *cleanupContextQueueService) RemoveEntryWithEntry(context.Context, string, string) (*messagequeue.QueuedMessage, error) {
	return s.entry, nil
}

func TestQueueAttachmentCleanupDetachesReadContext(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	current := &messagequeue.QueuedMessage{
		ID: "entry", SessionID: "session", TaskID: "task",
		Attachments: []messagequeue.MessageAttachment{{
			AttachmentID: "replacement",
		}},
	}
	cleanupQueue := &cleanupContextQueueService{QueueService: queue, entry: current}
	handlers.queueService = cleanupQueue
	releaser := &contextRecordingReleaser{}
	previous := &messagequeue.QueuedMessage{
		ID: "entry", SessionID: "session", TaskID: "task",
		Attachments: []messagequeue.MessageAttachment{{
			AttachmentID: "original",
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := handlers.releaseSupersededQueueAttachmentsAdmitted(ctx, wsUpdateMessageRequest{
		SessionID: "session", EntryID: "entry",
	}, previous, releaser)
	require.NoError(t, err)

	require.NoError(t, cleanupQueue.getEntryErr)
	require.NoError(t, cleanupQueue.referenceErr)
	require.NoError(t, releaser.err)
}

func TestQueueAttachmentCleanupDetachesCancelledReferenceLookup(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	cleanupQueue := &cleanupContextQueueService{QueueService: queue}
	handlers.queueService = cleanupQueue
	releaser := &contextRecordingReleaser{}
	previous := []messagequeue.MessageAttachment{{AttachmentID: "attachment"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := handlers.releaseQueueAttachmentCandidates(ctx, "task", wsUpdateMessageRequest{
		SessionID: "session", EntryID: "entry",
	}, previous, releaser)

	require.NoError(t, err)
	require.NoError(t, cleanupQueue.referenceErr)
	require.NoError(t, releaser.err)
	require.Equal(t, 1, releaser.calls)
}

func TestQueueAttachmentFailureCleanupDetachesReadContext(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	cleanupQueue := &cleanupContextQueueService{QueueService: queue}
	handlers.queueService = cleanupQueue
	releaser := &contextRecordingReleaser{}
	previous := &messagequeue.QueuedMessage{ID: "entry", TaskID: "task"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handlers.releaseQueuedAttachmentUpdateFailure(ctx, previous, "session", []messagequeue.MessageAttachment{{
		AttachmentID: "claimed",
	}}, releaser)

	require.NoError(t, cleanupQueue.referenceErr)
	require.NoError(t, releaser.err)
}

type failingQueueAttachmentReleaser struct{}

func (failingQueueAttachmentReleaser) ClaimMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return nil
}

func (failingQueueAttachmentReleaser) ReleaseMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return errors.New("release unavailable")
}

func TestPendingAttachmentCleanupAdoptsExplicitLifecycleContext(t *testing.T) {
	handlers, queue := setupQueueHandlersWithoutStart(t, nil)
	current, err := queue.QueueMessage(
		context.Background(), "session", "task", "current", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "current"}},
	)
	require.NoError(t, err)
	previous := *current
	previous.Attachments = []messagequeue.MessageAttachment{{AttachmentID: "previous"}}
	lifecycleCtx, cancelLifecycle := context.WithCancel(context.Background())

	handlers.queuePendingAttachmentCleanup(
		context.Background(),
		wsUpdateMessageRequest{
			SessionID: "session", EntryID: current.ID, LeaseID: "lease", OperationID: "operation",
		},
		&previous,
		failingQueueAttachmentReleaser{},
	)
	handlers.Start(lifecycleCtx)
	cancelLifecycle()

	select {
	case <-handlers.attachmentCleanupCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("pending cleanup retained its lazy background context instead of the explicit lifecycle context")
	}
}
