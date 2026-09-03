package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestWsUpdateMessageDoesNotReleaseUnclaimedAttachmentsOnLeaseConflict(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session", "task", "original", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	firstLease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)
	require.NoError(t, queue.EndEdit(ctx, entry.SessionID, entry.ID, firstLease.LeaseID, "connection-a"))
	secondLease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-b")
	require.NoError(t, err)

	response, err := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-a"), createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
		"session_id":               entry.SessionID,
		"entry_id":                 entry.ID,
		"lease_id":                 firstLease.LeaseID,
		"operation_id":             "operation-1",
		"expected_target_revision": firstLease.TargetRevision,
		"content":                  "edited",
		"attachments": []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: "staged", Name: "staged.txt", MimeType: "text/plain",
		}},
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, claimer.claims)
	require.Empty(t, claimer.releases)
	require.NoError(t, queue.EndEdit(ctx, entry.SessionID, entry.ID, secondLease.LeaseID, "connection-b"))
}
