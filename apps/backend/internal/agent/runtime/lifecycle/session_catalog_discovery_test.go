package lifecycle

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogDiscoveryResult(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal *sessioncatalog.Discovery
	}{
		{"complete-empty", &sessioncatalog.Discovery{Outcome: "complete", Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{}}}},
		{"mode-only", &sessioncatalog.Discovery{Outcome: "complete", Modes: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "code", Name: "Code"}}}}},
		{"unsupported", &sessioncatalog.Discovery{Outcome: "unsupported"}},
		{"failed", &sessioncatalog.Discovery{Outcome: "failed"}},
		{"restore-rejected", &sessioncatalog.Discovery{Outcome: "failed"}},
		{"absent-report", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			mock.handler = func(message ws.Message) *ws.Message {
				if message.Action == "agent.session.load" {
					if tc.name == "restore-rejected" {
						response, err := ws.NewError(message.ID, message.Action, "SESSION_RESTORE_BLOCKED", "saved conversation cannot be restored", nil)
						require.NoError(t, err)
						return response
					}
					response, err := ws.NewResponse(message.ID, message.Action, map[string]any{"success": true, "session_id": "native-conversation", "catalog_discovery": tc.terminal})
					require.NoError(t, err)
					return response
				}
				if message.Action == "agent.session.catalog_discovery" {
					response, err := ws.NewResponse(message.ID, message.Action, map[string]any{"session_id": "native-conversation", "catalog_discovery": tc.terminal})
					require.NoError(t, err)
					return response
				}
				return mock.defaultHandler(message)
			}
			client := createTestClient(t, mock.server.URL)
			connectAgentStream(t, mock, client)
			t.Cleanup(client.Close)
			eventBus := &MockEventBusWithTracking{}
			manager := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
			manager.SetDependencies(NewEventPublisher(eventBus, newSessionTestLogger()), nil, nil, nil)
			execution := &AgentExecution{ID: "replacement-execution", TaskID: "task", SessionID: "session", ExecutionProfileID: "concrete-profile", ACPSessionID: "native-conversation", agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1)}
			source := execution.beginStartupAttemptWithID("accepted-resume")
			config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP, SessionConfig: agents.SessionConfig{NativeSessionResume: true}}}
			initializeErr := manager.InitializeAndPromptWithLayers(context.Background(), execution, config, "", nil, nil, func(string) error { return nil }, "", "", nil, "", "", nil, StartModelPolicy{})
			if tc.name == "restore-rejected" {
				require.Error(t, initializeErr)
			} else {
				require.NoError(t, initializeErr)
			}
			var terminalEvents []AgentStreamEventPayload
			for _, event := range eventBus.getStreamEvents() {
				if event.Data != nil && event.Data.Type == "session_catalog_discovery" {
					terminalEvents = append(terminalEvents, event)
				}
			}
			if tc.terminal == nil {
				require.Empty(t, terminalEvents, "an absent model state or ready ACK must not fabricate discovery")
				return
			}
			require.Len(t, terminalEvents, 1, "explicit empty/mode-only/unsupported discovery must settle without an ordinary model event")
			require.Equal(t, execution.ID, terminalEvents[0].ExecutionID)
			require.Equal(t, "accepted-resume", terminalEvents[0].AttemptID)
			require.Equal(t, source, terminalEvents[0].SessionSettingsSourceGeneration)
			payload, err := json.Marshal(terminalEvents[0].Data)
			require.NoError(t, err)
			var wire struct {
				Catalog *sessioncatalog.Discovery `json:"catalog_discovery"`
			}
			require.NoError(t, json.Unmarshal(payload, &wire))
			require.Equal(t, tc.terminal, wire.Catalog)
			require.Equal(t, "native-conversation", execution.ACPSessionID)
			for _, action := range mock.getActionLog() {
				require.NotEqual(t, "agent.prompt", action)
				require.NotEqual(t, "agent.session.set_model", action)
			}
		})
	}
}

type replacedCatalogStartupStore struct{}

func (*replacedCatalogStartupStore) SetSessionMetadataKey(context.Context, string, string, interface{}) error {
	return nil
}
func (*replacedCatalogStartupStore) BeginSessionCatalogDiscovery(context.Context, string, string, sessioncatalog.Owner, string) (*sessioncatalog.Launch, error) {
	return nil, nil
}

func TestSessionCatalogDiscoveryResultStaleOwnerDoesNotInitializeProvider(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	client := createTestClient(t, mock.server.URL)
	connectAgentStream(t, mock, client)
	t.Cleanup(client.Close)
	sessions := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	manager := &Manager{sessionManager: sessions}
	manager.SetSessionSettingsSnapshotWriter(&replacedCatalogStartupStore{})
	execution := &AgentExecution{ID: "obsolete-execution", TaskID: "task", SessionID: "session", ExecutionProfileID: "profile", ACPSessionID: "native-conversation", agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1)}
	execution.beginStartupAttemptWithID("obsolete-attempt")
	config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP, SessionConfig: agents.SessionConfig{NativeSessionResume: true}}}
	err := sessions.InitializeAndPromptWithLayers(context.Background(), execution, config, "", nil, nil, func(string) error { return nil }, "", "", nil, "", "", nil, StartModelPolicy{})
	require.Error(t, err, "a displaced launch must stop before reopening the native conversation")
	for _, action := range mock.getActionLog() {
		require.NotEqual(t, "agent.initialize", action)
		require.NotEqual(t, "agent.session.load", action)
		require.NotEqual(t, "agent.prompt", action)
	}
	require.Equal(t, "native-conversation", execution.ACPSessionID)
}

func TestSessionCatalogDiscoveryResultAfterSettingsSettlement(t *testing.T) {
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	var applied atomic.Bool
	before := &sessioncatalog.Discovery{Outcome: "complete", Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "before-setting", Name: "Before setting"}}}}
	after := &sessioncatalog.Discovery{Outcome: "complete", Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "after-setting", Name: "After setting"}}}}
	mock.handler = func(message ws.Message) *ws.Message {
		switch message.Action {
		case "agent.session.load":
			response, err := ws.NewResponse(message.ID, message.Action, map[string]any{"success": true, "session_id": "native-conversation", "catalog_discovery": before, "model_state": map[string]any{"current_model_id": "selected-model", "models": []map[string]string{{"model_id": "selected-model", "name": "Selected model"}}, "config_options": []map[string]any{{"id": "effort", "type": "select", "name": "Effort", "current_value": "low", "options": []map[string]string{{"value": "low", "name": "Low"}, {"value": "high", "name": "High"}}}}, "config_options_settled": true}})
			require.NoError(t, err)
			return response
		case "agent.session.set_config_option":
			applied.Store(true)
		case "agent.session.catalog_discovery":
			require.True(t, applied.Load(), "catalog finalization must follow applicable saved settings")
			response, err := ws.NewResponse(message.ID, message.Action, map[string]any{"session_id": "native-conversation", "catalog_discovery": after})
			require.NoError(t, err)
			return response
		}
		return mock.defaultHandler(message)
	}
	client := createTestClient(t, mock.server.URL)
	connectAgentStream(t, mock, client)
	t.Cleanup(client.Close)
	events := &MockEventBusWithTracking{}
	sessions := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
	sessions.SetDependencies(NewEventPublisher(events, newSessionTestLogger()), nil, nil, nil)
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", ExecutionProfileID: "profile", ACPSessionID: "native-conversation", agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1)}
	execution.beginStartupAttemptWithID("accepted-attempt")
	config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP, SessionConfig: agents.SessionConfig{NativeSessionResume: true}}}
	require.NoError(t, sessions.InitializeAndPromptWithLayers(context.Background(), execution, config, "", nil, nil, func(string) error { return nil }, "", "", map[string]string{"effort": "high"}, "", "", nil, StartModelPolicy{}))
	require.True(t, applied.Load())
	var terminal *sessioncatalog.Discovery
	for _, event := range events.getStreamEvents() {
		if event.Data != nil && event.Data.Type == "session_catalog_discovery" {
			terminal = event.Data.CatalogDiscovery
		}
	}
	require.NotNil(t, terminal)
	require.Equal(t, after, terminal, "the initial handshake catalog cannot overwrite the catalog after saved settings settle")
	require.Equal(t, "native-conversation", execution.ACPSessionID)
}

func TestSessionCatalogDiscoveryResultSurvivesRejectedSettings(t *testing.T) {
	for _, tc := range []struct {
		name     string
		terminal *sessioncatalog.Discovery
	}{
		{"complete-empty", &sessioncatalog.Discovery{Outcome: "complete", Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{}}}},
		{"mode-only", &sessioncatalog.Discovery{Outcome: "complete", Modes: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{{ID: "available-mode", Name: "Available mode"}}}}},
		{"unsupported", &sessioncatalog.Discovery{Outcome: "unsupported"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terminal := tc.terminal
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			mock.handler = func(message ws.Message) *ws.Message {
				switch message.Action {
				case "agent.session.load":
					response, err := ws.NewResponse(message.ID, message.Action, map[string]any{"success": true, "session_id": "native-conversation", "catalog_discovery": terminal})
					require.NoError(t, err)
					return response
				case "agent.session.set_mode":
					response, err := ws.NewError(message.ID, message.Action, "MODE_NOT_SUPPORTED", "requested mode is not advertised", nil)
					require.NoError(t, err)
					return response
				case "agent.session.catalog_discovery":
					response, err := ws.NewResponse(message.ID, message.Action, map[string]any{"session_id": "native-conversation", "catalog_discovery": terminal})
					require.NoError(t, err)
					return response
				}
				return mock.defaultHandler(message)
			}
			client := createTestClient(t, mock.server.URL)
			connectAgentStream(t, mock, client)
			t.Cleanup(client.Close)
			events := &MockEventBusWithTracking{}
			sessions := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
			sessions.SetDependencies(NewEventPublisher(events, newSessionTestLogger()), nil, nil, nil)
			execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", ExecutionProfileID: "profile", ACPSessionID: "native-conversation", agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1)}
			source := execution.beginStartupAttemptWithID("accepted-attempt")
			config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP, SessionConfig: agents.SessionConfig{NativeSessionResume: true}}}
			err := sessions.InitializeAndPromptWithLayers(context.Background(), execution, config, "", nil, nil, func(string) error { return nil }, "", "rejected-mode", nil, "", "", nil, StartModelPolicy{})
			require.Error(t, err)
			var reports []AgentStreamEventPayload
			for _, event := range events.getStreamEvents() {
				if event.Data != nil && event.Data.Type == "session_catalog_discovery" {
					reports = append(reports, event)
				}
			}
			require.Len(t, reports, 1)
			require.Equal(t, terminal, reports[0].Data.CatalogDiscovery, "a settings rejection must not replace explicit provider availability with a fabricated discovery failure")
			require.Equal(t, source, reports[0].SessionSettingsSourceGeneration)
			require.Equal(t, "accepted-attempt", reports[0].AttemptID)
			require.Equal(t, "native-conversation", execution.ACPSessionID)
			require.False(t, execution.isSessionInitialized(), "availability must not bypass rejected settings")
			for _, action := range mock.getActionLog() {
				require.NotEqual(t, "agent.prompt", action)
			}
		})
	}
}
