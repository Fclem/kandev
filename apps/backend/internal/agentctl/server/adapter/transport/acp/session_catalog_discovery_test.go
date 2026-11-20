package acp

import (
	"encoding/json"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/stretchr/testify/require"
)

type terminalCatalogProvider interface {
	GetSessionCatalogDiscovery() *sessioncatalog.Discovery
}

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogDiscoveryResult(t *testing.T) {
	for _, tc := range []struct {
		name, response, outcome                                  string
		modelPresent, modelSupported, modePresent, modeSupported bool
		modes                                                    []sessioncatalog.Choice
	}{
		{name: "strict complete empty", response: `{"models":{"currentModelId":"","availableModels":[]}}`, outcome: "complete", modelPresent: true, modelSupported: true},
		{name: "strict mode only", response: `{"modes":{"currentModeId":"code","availableModes":[{"id":"code","name":"Code"}]}}`, outcome: "complete", modePresent: true, modeSupported: true, modes: []sessioncatalog.Choice{{ID: "code", Name: "Code"}}},
		{name: "strict unsupported", response: `{}`, outcome: "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, fake := newSessionResumeAdapter(t, true, true)
			provider, ok := any(adapter).(terminalCatalogProvider)
			if !ok {
				t.Fatal("strict restore has no terminal empty or unsupported discovery evidence")
			}
			require.Nil(t, provider.GetSessionCatalogDiscovery(), "no handshake is not terminal evidence")
			fake.resumeResponse = &acpsdk.ResumeSessionResponse{}
			require.NoError(t, json.Unmarshal([]byte(tc.response), fake.resumeResponse))
			require.NoError(t, adapter.LoadSession(t.Context(), "saved-conversation", nil))
			result := provider.GetSessionCatalogDiscovery()
			require.NotNil(t, result, "a completed restore must report discovery even without a session_models event")
			require.Equal(t, tc.outcome, result.Outcome)
			require.Equal(t, tc.modelPresent, result.Models.Present)
			require.Equal(t, tc.modelSupported, result.Models.Supported)
			require.Empty(t, result.Models.Choices)
			require.Equal(t, tc.modePresent, result.Modes.Present)
			require.Equal(t, tc.modeSupported, result.Modes.Supported)
			require.Equal(t, tc.modes, result.Modes.Choices)
			require.Equal(t, "saved-conversation", adapter.GetSessionID())
		})
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogDiscoveryResultDirectResume(t *testing.T) {
	adapter, fake := newSessionResumeAdapter(t, true, true)
	require.NoError(t, adapter.LoadSession(t.Context(), "saved-conversation", nil))
	fake.resumeResponse = &acpsdk.ResumeSessionResponse{}
	require.NoError(t, json.Unmarshal([]byte(`{"models":{"currentModelId":"","availableModels":[]}}`), fake.resumeResponse))
	require.NoError(t, adapter.ResumeSession(t.Context(), "saved-conversation", nil))
	result := any(adapter).(terminalCatalogProvider).GetSessionCatalogDiscovery()
	require.NotNil(t, result)
	require.Equal(t, "complete", result.Outcome)
	require.True(t, result.Models.Supported)
	require.Empty(t, result.Models.Choices, "explicit empty resume must retire the prior advertised model surface")
	require.False(t, result.Modes.Supported)
	require.Equal(t, "saved-conversation", adapter.GetSessionID())
}

func TestSessionCatalogDiscoveryResultAuthoritativeSurfaceUpdate(t *testing.T) {
	adapter, fake := newSessionResumeAdapter(t, true, true)
	fake.resumeResponse = &acpsdk.ResumeSessionResponse{}
	require.NoError(t, adapter.LoadSession(t.Context(), "saved-conversation", nil))
	initial := adapter.GetSessionCatalogDiscovery()
	require.Equal(t, "unsupported", initial.Outcome)
	var options []acpsdk.SessionConfigOption
	require.NoError(t, json.Unmarshal([]byte(`[{"type":"select","id":"model","name":"Model","category":"model","currentValue":"selected","options":[{"value":"selected","name":"Selected"},{"value":"new","name":"New"}]}]`), &options))
	adapter.handleACPUpdate(acpsdk.SessionNotification{SessionId: "saved-conversation", Update: acpsdk.SessionUpdate{ConfigOptionUpdate: &acpsdk.SessionConfigOptionUpdate{SessionUpdate: "config_option_update", ConfigOptions: options}}}, 0)
	result := adapter.GetSessionCatalogDiscovery()
	require.Equal(t, "complete", result.Outcome, "a real later provider advertisement must supersede initial unsupported discovery")
	require.True(t, result.Models.Present)
	require.True(t, result.Models.Supported)
	require.Equal(t, []sessioncatalog.Choice{{ID: "selected", Name: "Selected"}, {ID: "new", Name: "New"}}, result.Models.Choices)
	require.False(t, result.Modes.Supported)
	require.Greater(t, result.Generation, initial.Generation)
	require.Equal(t, "saved-conversation", adapter.GetSessionID())
}
