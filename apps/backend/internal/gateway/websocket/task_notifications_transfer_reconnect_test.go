package websocket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestTaskNotificationTransferDepartureAcrossReconnect pins the delivery
// schedule the Web client must survive (AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.4):
// an ordinary source lifecycle publication held upstream before fan-out is
// delivered through the browser's CURRENT socket after the socket was
// replaced, and after the source-only reader already received the redacted
// departure marker. Replacing the transport therefore never drains old
// events, and source-only readers never receive destination payloads.
func TestTaskNotificationTransferDepartureAcrossReconnect(t *testing.T) {
	for _, tt := range []struct {
		name          string
		deliverMarker bool
	}{
		{name: "marker delivered before reconnect", deliverMarker: true},
		{name: "marker missed while disconnected", deliverMarker: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := testLogger()
			eventBus := &queuedTransportEventBus{MemoryEventBus: bus.NewMemoryEventBus(log)}
			hub := newAccessTestHub(t)
			hub.setAuthPolicy(AuthPolicy{
				Enforced: func() bool { return true },
				WorkspaceReaders: func(_ context.Context, workspaceID string) ([]string, error) {
					if workspaceID == "ws-source" {
						return []string{"source-reader", "both-reader"}, nil
					}
					return []string{"both-reader"}, nil
				},
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_ = RegisterTaskNotifications(ctx, eventBus, hub, log)

			sourceIdentity := authn.Identity{UserID: "source-reader", Role: authn.RoleMember}
			replaced := registerAccessClient(t, hub, "source-socket-1", sourceIdentity)
			if !tt.deliverMarker {
				hub.Unregister(replaced)
			}

			// The ordinary lifecycle event is published first but held upstream.
			require.NoError(t, eventBus.Publish(ctx, events.TaskStateChanged, bus.NewEvent(
				events.TaskStateChanged, "test", map[string]interface{}{
					"task_id": "task-transfer", "workspace_id": "ws-source", "workflow_id": "wf-source",
					"state": "COMPLETED",
				})))
			require.NoError(t, eventBus.Publish(ctx, events.TaskUpdated, bus.NewEvent(
				events.TaskUpdated, "test", map[string]interface{}{
					"task_id": "task-transfer", "workspace_id": "ws-destination",
					"workflow_id": "wf-destination", "source_workspace_id": "ws-source",
					"source_workflow_id": "wf-source", "old_workflow_id": "wf-source",
					"transfer_operation_id": "operation-1", "title": "private destination title",
				})))
			require.NoError(t, eventBus.deliverSubjectFirst(ctx, events.TaskUpdated))

			if tt.deliverMarker {
				marker := nextNotification(t, replaced)
				require.Equal(t, ws.ActionTaskDeleted, marker.Action)
				var payload map[string]interface{}
				require.NoError(t, json.Unmarshal(marker.Payload, &payload))
				require.Equal(t, map[string]interface{}{
					"task_id": "task-transfer", "workspace_id": "ws-source", "workflow_id": "wf-source",
					"transfer_operation_id": "operation-1",
				}, payload, "source-only reader received destination data")
				hub.Unregister(replaced)
			}

			// The browser reconnects with a new socket for the same identity.
			current := registerAccessClient(t, hub, "source-socket-2", sourceIdentity)
			require.NoError(t, eventBus.deliverAll(ctx))

			held := nextNotification(t, current)
			require.Equal(t, ws.ActionTaskStateChanged, held.Action)
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(held.Payload, &payload))
			require.Equal(t, "ws-source", payload["workspace_id"],
				"the held source event reaches the current socket with its stale placement")
			require.Empty(t, receivedActions(current))
		})
	}
}

func nextNotification(t *testing.T, client *Client) ws.Message {
	t.Helper()
	select {
	case raw := <-client.send:
		var message ws.Message
		require.NoError(t, json.Unmarshal(raw, &message))
		return message
	case <-time.After(2 * time.Second):
		t.Fatalf("client %s received no notification", client.ID)
		return ws.Message{}
	}
}
