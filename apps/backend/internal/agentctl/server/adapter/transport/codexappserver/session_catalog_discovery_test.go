package codexappserver

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/stretchr/testify/require"
)

type terminalCatalogProvider interface {
	GetSessionCatalogDiscovery() *sessioncatalog.Discovery
}

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogDiscoveryResult(t *testing.T) {
	server := newProtocolServer(t, func(request map[string]json.RawMessage, write func(any) error) error {
		switch readString(request, "method") {
		case "initialize":
			return write(resultFrame(request["id"], map[string]any{"userAgent": "Codex CLI 0.154.0"}))
		case "initialized":
			return nil
		case "model/list":
			return write(resultFrame(request["id"], map[string]any{"data": []any{}}))
		case "thread/start":
			return write(resultFrame(request["id"], map[string]any{"thread": map[string]any{"id": "native-conversation"}}))
		default:
			return write(errorFrame(request["id"], -32601, "unsupported"))
		}
	})
	defer server.close()
	adapter := NewAdapter(&shared.Config{}, logger.Default())
	defer func() { _ = adapter.Close() }()
	provider, ok := any(adapter).(terminalCatalogProvider)
	if !ok {
		t.Fatal("native handshake cannot report an explicit supported empty discovery result")
	}
	require.Nil(t, provider.GetSessionCatalogDiscovery())
	require.NoError(t, adapter.Connect(server.clientWriter, server.clientReader))
	require.NoError(t, adapter.Initialize(t.Context()))
	require.Nil(t, provider.GetSessionCatalogDiscovery(), "initialize alone is not a completed conversation handshake")
	sessionID, err := adapter.NewSession(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, "native-conversation", sessionID)
	result := provider.GetSessionCatalogDiscovery()
	require.NotNil(t, result)
	require.Equal(t, "complete", result.Outcome)
	require.True(t, result.Models.Present)
	require.True(t, result.Models.Supported)
	require.Empty(t, result.Models.Choices)
	require.False(t, result.Modes.Supported)
}

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogDiscoveryRejectsMissingModelsArray(t *testing.T) {
	for _, tc := range []struct {
		name string
		data map[string]any
	}{
		{name: "absent", data: map[string]any{}},
		{name: "null", data: map[string]any{"data": nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newProtocolServer(t, func(request map[string]json.RawMessage, write func(any) error) error {
				switch readString(request, "method") {
				case "initialize":
					return write(resultFrame(request["id"], map[string]any{"userAgent": "Codex CLI 0.154.0"}))
				case "initialized":
					return nil
				case "model/list":
					return write(resultFrame(request["id"], tc.data))
				default:
					return write(errorFrame(request["id"], -32601, "unsupported"))
				}
			})
			defer server.close()
			adapter := NewAdapter(&shared.Config{}, logger.Default())
			defer func() { _ = adapter.Close() }()
			require.NoError(t, adapter.Connect(server.clientWriter, server.clientReader))
			require.Error(t, adapter.Initialize(t.Context()))
			require.Nil(t, adapter.GetSessionCatalogDiscovery())
		})
	}
}
