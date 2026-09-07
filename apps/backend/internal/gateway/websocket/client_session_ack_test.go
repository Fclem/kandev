package websocket

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins"
)

func TestSessionAckIdentityMatchesExactConsumerBranch(t *testing.T) {
	pluginKey := plugins.SessionDeliveryCursorKey{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
	}
	coreKey := plugins.SessionDeliveryCursorKey{
		SessionID: "session-1", ConsumerKind: orderedConsumerCore, WireID: "wire-1",
	}
	tests := []struct {
		name string
		req  SessionAckRequest
		key  plugins.SessionDeliveryCursorKey
		want bool
	}{
		{
			name: "plugin identity",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
				PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1"},
			key: pluginKey, want: true,
		},
		{
			name: "plugin cannot also send wire identity",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
				PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1", WireID: "wire-1"},
			key: pluginKey,
		},
		{
			name: "plugin id must match",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
				PluginID: "other", Generation: 7, ConsumerID: "consumer-1"},
			key: pluginKey,
		},
		{
			name: "core identity",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerCore,
				WireID: "wire-1"},
			key: coreKey, want: true,
		},
		{
			name: "core cannot send plugin fields",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerCore,
				WireID: "wire-1", PluginID: "plugin-1", Generation: 7},
			key: coreKey,
		},
		{
			name: "core wire must match",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerCore,
				WireID: "other"},
			key: coreKey,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity, valid := sessionAckSubscriptionID(test.req)
			if test.want {
				if !valid || identity == "" || !sessionAckMatchesKey(test.req, test.key) {
					t.Fatalf("valid ACK identity rejected: valid=%v identity=%q", valid, identity)
				}
				return
			}
			if valid && sessionAckMatchesKey(test.req, test.key) {
				t.Fatal("malformed or conflicting ACK identity accepted")
			}
		})
	}
}

func TestPoisonRequeueRequiresOperatorCapability(t *testing.T) {
	tests := []struct {
		name     string
		identity authn.Identity
		want     bool
	}{
		{name: "instance operator", identity: authn.Identity{UserID: "operator-1", Instance: true}, want: true},
		{name: "single user operator", identity: authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true}, want: true},
		{name: "organization admin has no operator capability", identity: authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin}},
		{name: "anonymous instance bit is insufficient", identity: authn.Identity{Instance: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canRequeueSessionEvents(test.identity); got != test.want {
				t.Fatalf("canRequeueSessionEvents() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOrderedReplayIsExplicitOnly(t *testing.T) {
	service := plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger())
	_, err := service.SessionEvents().Append(
		"session-1",
		nil,
		"message.deleted",
		json.RawMessage(`{"type":"message.deleted","session_id":"session-1","message_id":"message-1"}`),
	)
	if err != nil {
		t.Fatalf("append event: %v", err)
	}
	key := plugins.SessionDeliveryCursorKey{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1", UserID: "user-1",
	}

	fresh := resolveOrderedSessionReplay(service, SessionSubscribeRequest{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
	}, key)
	if fresh.result != "fresh" || fresh.watermark != 1 || len(fresh.events) != 0 {
		t.Fatalf("fresh result = %+v, want no replay at watermark 1", fresh)
	}

	zero := uint64(0)
	replay := resolveOrderedSessionReplay(service, SessionSubscribeRequest{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
		LastSeenSequence: &zero,
	}, key)
	if replay.result != "replay" || len(replay.events) != 1 {
		t.Fatalf("explicit replay result = %+v, want one replay event", replay)
	}

	invalid := resolveOrderedSessionReplay(service, SessionSubscribeRequest{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
		LastSeenSequence: &zero, ResumeToken: "invalid",
	}, key)
	if invalid.result != "invalid_resume" || len(invalid.events) != 0 {
		t.Fatalf("invalid resume result = %+v, want replacement without replay", invalid)
	}
}
