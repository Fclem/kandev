package websocket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SESSION-CATALOG-001.4
func TestSessionCatalogRefreshScopedDelivery(t *testing.T) {
	hub := newAccessTestHub(t)
	eventBus := bus.NewMemoryEventBus(testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = RegisterSessionStreamNotifications(ctx, eventBus, hub, testLogger())
	clients := []*Client{
		registerAccessClient(t, hub, "catalog-subscriber", authn.Identity{UserID: "user-1", Role: authn.RoleMember}),
		registerAccessClient(t, hub, "sibling-subscriber", authn.Identity{UserID: "user-1", Role: authn.RoleMember}),
	}
	for i, client := range clients {
		sessionID := "catalog-session"
		if i == 1 {
			sessionID = "sibling-session"
		}
		raw, err := json.Marshal(map[string]any{"session_id": sessionID})
		require.NoError(t, err)
		client.handleSessionSubscribe(&ws.Message{ID: "subscribe", Type: ws.MessageTypeRequest, Action: ws.ActionSessionSubscribe, Payload: raw})
	}
	observation := sessioncatalog.Observation{Version: 1, Revision: 7, ProfileID: "profile", Disposition: "host_candidates", Models: []sessioncatalog.Choice{{ID: "model-only-for-target", Name: "Target"}}}
	require.NoError(t, eventBus.Publish(ctx, events.BuildSessionCatalogRefreshSubject("catalog-session"), bus.NewEvent(events.SessionCatalogRefreshUpdated, "test", sessioncatalog.Update{TaskID: "task", SessionID: "catalog-session", Record: observation})))
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		var payload []byte
		select {
		case payload = <-clients[0].send:
		case payload = <-clients[0].controlSend:
		case <-deadline.C:
			t.Fatal("subscribed session never received its committed catalog observation")
		}
		var message ws.Message
		require.NoError(t, json.Unmarshal(payload, &message))
		if message.Action != "session.catalog_refresh_updated" {
			continue
		}
		var delivered sessioncatalog.Update
		require.NoError(t, json.Unmarshal(message.Payload, &delivered))
		require.Equal(t, observation, delivered.Record)
		break
	}
	for {
		var payload []byte
		select {
		case payload = <-clients[1].send:
		case payload = <-clients[1].controlSend:
		default:
			return
		}
		var message ws.Message
		require.NoError(t, json.Unmarshal(payload, &message))
		require.NotEqual(t, "session.catalog_refresh_updated", message.Action, "catalog must not leak to sibling subscription")
	}
}
