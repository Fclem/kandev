package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/sysprompt"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const orderedMessageDeletedEvent = "message.deleted"

var orderedEventTypeByAction = map[string]string{
	ws.ActionSessionMessageAdded:   "message.added",
	ws.ActionSessionMessageUpdated: "message.updated",
	ws.ActionSessionMessageDeleted: orderedMessageDeletedEvent,
	ws.ActionSessionTurnStarted:    "session.turn.started",
	ws.ActionSessionTurnCompleted:  "session.turn.completed",
	ws.ActionSessionRemoved:        "session.removed",
}

const orderedContentPayloadKey = "content"

//nolint:cyclop // This boundary coordinates journal sync, poison recording, and live fan-out.
func (h *Hub) appendAndBroadcastOrderedSessionEvent(sessionID string, message *ws.Message) {
	h.orderedSessionMu.Lock()
	defer h.orderedSessionMu.Unlock()
	service := h.pluginConversationService
	eventType := orderedEventTypeByAction[message.Action]
	if service == nil || eventType == "" {
		return
	}
	if service.HasConversationJournal() {
		events, err := service.SyncCommittedSessionEvents(context.Background(), sessionID)
		if err != nil {
			if h.logger != nil {
				h.logger.Error("mirror committed ordered session events", zap.Error(err))
			}
			return
		}
		for _, event := range events {
			h.broadcastCommittedOrderedSessionEvent(service, event)
		}
		return
	}
	var source map[string]any
	if err := json.Unmarshal(message.Payload, &source); err != nil {
		source = map[string]any{"raw": message.Payload}
	}
	payload := sanitizedOrderedSessionPayload(eventType, source)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var taskID *string
	if value, ok := payload["task_id"].(string); ok && value != "" {
		taskID = &value
	}
	event, err := service.SessionEvents().Append(sessionID, taskID, eventType, encoded)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("append ordered session event", zap.Error(err))
		}
		return
	}
	recipients := h.orderedSessionRecipients(sessionID)
	if poison, poisoned := service.SessionEvents().Poison(sessionID, event.ID); poisoned {
		// A delivery attempt is counted only when at least one subscriber
		// exists to receive the frame; mirroring with nobody subscribed does
		// not burn an attempt.
		if len(recipients) > 0 {
			if failureErr := service.SessionDelivery().RecordFailure(sessionID, event.ID, time.Now().UTC()); failureErr != nil && h.logger != nil {
				h.logger.Error(
					"record ordered session poison",
					zap.String("event_id", event.ID),
					zap.String("validation_error", poison.LastError),
					zap.Error(failureErr),
				)
			}
		}
	}
	// Poison frames are delivered to live subscribers on every path (append
	// and mirror, live and replay alike) so clients can run their recovery:
	// the web core stream treats the frame as poison and re-subscribes with
	// replace_cursor, and plugin scopes rebind past the poison. Consumers
	// never acknowledge the poisoned sequence itself, so the ACK block stays
	// the durable rebind trigger for lagging cursors.
	for _, client := range recipients {
		client.sendOrderedSessionEvent(event)
	}
}

// broadcastCommittedOrderedSessionEvent fans a mirrored primary-journal event
// out to ordered subscribers with the same poison contract as the append
// path: the frame is delivered (recovery depends on seeing it), and the
// delivery attempt is counted only when recipients exist.
func (h *Hub) broadcastCommittedOrderedSessionEvent(service *plugins.Service, event plugins.SessionEvent) {
	recipients := h.orderedSessionRecipients(event.SessionID)
	if poison, poisoned := service.SessionEvents().Poison(event.SessionID, event.ID); poisoned {
		if len(recipients) > 0 {
			if failureErr := service.SessionDelivery().RecordFailure(event.SessionID, event.ID, time.Now().UTC()); failureErr != nil && h.logger != nil {
				h.logger.Error(
					"record ordered session poison",
					zap.String("event_id", event.ID),
					zap.String("validation_error", poison.LastError),
					zap.Error(failureErr),
				)
			}
		}
	}
	for _, client := range recipients {
		client.sendOrderedSessionEvent(event)
	}
}

func sanitizedOrderedSessionPayload(eventType string, source map[string]any) map[string]any {
	payload := map[string]any{eventTypePayloadKey: eventType}
	keys := []string{sessionIDPayloadKey, taskIDPayloadKey}
	switch eventType {
	case "message.added", "message.updated":
		keys = append(keys,
			"message_id", "turn_id", "author_type", orderedContentPayloadKey,
			"created_at", "updated_at", "prompt_index",
		)
		if messageType, exists := source[eventTypePayloadKey]; exists {
			payload["message_type"] = messageType
		}
		if senderTaskID, ok := source["sender_task_id"].(string); ok && senderTaskID != "" {
			payload["sender_task_id"] = senderTaskID
		} else if metadata, ok := source["metadata"].(map[string]any); ok {
			if senderTaskID, ok := metadata["sender_task_id"].(string); ok && senderTaskID != "" {
				payload["sender_task_id"] = senderTaskID
			}
		}
	case orderedMessageDeletedEvent:
		keys = append(keys, "message_id")
	case "session.turn.started", "session.turn.completed":
		keys = append(keys, "id", "started_at", "completed_at", "updated_at")
	}
	for _, key := range keys {
		if value, exists := source[key]; exists {
			if key == orderedContentPayloadKey {
				if content, ok := value.(string); ok {
					value = sysprompt.StripSystemContent(content)
				}
			}
			payload[key] = value
		}
	}
	return payload
}

func (h *Hub) orderedSessionRecipients(sessionID string) []*Client {
	h.mu.RLock()
	clients := make([]*Client, 0)
	for client := range h.clients {
		if client.hasOrderedSessionSubscription(sessionID) {
			clients = append(clients, client)
		}
	}
	h.mu.RUnlock()
	// Re-authorize at fanout time, mirroring the legacy session recipients: a
	// client whose workspace/session membership was revoked after subscribing
	// must not keep receiving ordered frames. Denied clients lose the ordered
	// subscription itself (same revocation semantics as the legacy path).
	allowed, denied := h.partitionAuthorized(clients, sessionID, h.authPolicy.Subscriptions.Session)
	for _, client := range denied {
		client.mu.Lock()
		delete(client.orderedSessionSubscriptions, sessionID)
		client.mu.Unlock()
	}
	return allowed
}

func (c *Client) hasOrderedSessionSubscription(sessionID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.orderedSessionSubscriptions[sessionID]) > 0
}

func sessionEventFrame(event plugins.SessionEvent) ([]byte, error) {
	return json.Marshal(map[string]any{
		eventTypePayloadKey: "session.event", "protocol_version": event.ProtocolVersion,
		"event_type": event.EventType, sessionIDPayloadKey: event.SessionID,
		taskIDPayloadKey: event.TaskID, eventSequencePayloadKey: event.Sequence,
		"event_id": event.ID, "payload": event.Payload,
	})
}
