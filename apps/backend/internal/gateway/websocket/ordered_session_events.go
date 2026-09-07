package websocket

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/sysprompt"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

var orderedEventTypeByAction = map[string]string{
	ws.ActionSessionMessageAdded:   "message.added",
	ws.ActionSessionMessageUpdated: "message.updated",
	ws.ActionSessionMessageDeleted: "message.deleted",
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
	if poison, poisoned := service.SessionEvents().Poison(sessionID, event.ID); poisoned {
		if failureErr := service.SessionDelivery().RecordFailure(sessionID, event.ID, event.CreatedAt); failureErr != nil && h.logger != nil {
			h.logger.Error(
				"record ordered session poison",
				zap.String("event_id", event.ID),
				zap.String("validation_error", poison.LastError),
				zap.Error(failureErr),
			)
		}
		return
	}
	for _, client := range h.orderedSessionRecipients(sessionID) {
		client.sendOrderedSessionEvent(event)
	}
}

func (h *Hub) broadcastCommittedOrderedSessionEvent(service *plugins.Service, event plugins.SessionEvent) {
	if poison, poisoned := service.SessionEvents().Poison(event.SessionID, event.ID); poisoned {
		if failureErr := service.SessionDelivery().RecordFailure(event.SessionID, event.ID, event.CreatedAt); failureErr != nil && h.logger != nil {
			h.logger.Error(
				"record ordered session poison",
				zap.String("event_id", event.ID),
				zap.String("validation_error", poison.LastError),
				zap.Error(failureErr),
			)
		}
	}
	for _, client := range h.orderedSessionRecipients(event.SessionID) {
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
	case "message.deleted":
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
	return clients
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
