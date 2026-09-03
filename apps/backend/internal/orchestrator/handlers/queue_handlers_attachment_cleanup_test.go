package handlers

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

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
	err error
}

func (r *contextRecordingReleaser) ClaimMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return nil
}

func (r *contextRecordingReleaser) ReleaseMessageAttachments(ctx context.Context, _ string, _ string, _ []v1.MessageAttachment) error {
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
