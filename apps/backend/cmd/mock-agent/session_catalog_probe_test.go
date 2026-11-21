package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

// @covers AC-AGENTS-SESSION-CATALOG-001.8
func TestMockSessionCatalogProbeReadsItsOwnContextWithoutSessions(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	t.Setenv("MOCK_AGENT_SESSION_CATALOG_FILE", "task-catalog.json")
	response := acp.NewSessionResponse{ConfigOptions: mockSessionConfigOptions(), Modes: mockSessionModes(mockDefaultMode)}
	models := response.ConfigOptions[0].Select.Options.Ungrouped
	*models = append(*models, acp.SessionConfigSelectOption{Value: "task-model", Name: "Task model"})
	response.Modes.AvailableModes = append(response.Modes.AvailableModes, acp.SessionMode{Id: "task-mode", Name: "Task mode"})
	writeCatalogResponse(t, filepath.Join(work, "task-catalog.json"), response)

	agent := &mockAgent{conn: newCapturingUpdater(), sessions: make(map[acp.SessionId]bool), commandsEmitted: make(map[acp.SessionId]bool)}
	initialized, err := agent.Initialize(context.Background(), acp.InitializeRequest{Meta: map[string]any{sessioncatalog.MockCatalogProbeMetaKey: true}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(initialized.Meta[sessioncatalog.MockCatalogProbeMetaKey])
	if err != nil {
		t.Fatal(err)
	}
	var discovery sessioncatalog.Discovery
	if err := json.Unmarshal(payload, &discovery); err != nil {
		t.Fatalf("probe result missing: %s", payload)
	}
	if err := sessioncatalog.ValidateDiscovery(&discovery); err != nil {
		t.Fatalf("invalid probe result: %v", err)
	}
	if !containsChoice(discovery.Models.Choices, "task-model") || !containsChoice(discovery.Modes.Choices, "task-mode") {
		t.Fatalf("probe = %+v, want this context's own models and modes", discovery)
	}
	if len(agent.sessions) != 0 {
		t.Fatal("a catalog probe must not create a session")
	}

	ordinary, err := (&mockAgent{conn: newCapturingUpdater(), sessions: make(map[acp.SessionId]bool), commandsEmitted: make(map[acp.SessionId]bool)}).Initialize(context.Background(), acp.InitializeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := ordinary.Meta[sessioncatalog.MockCatalogProbeMetaKey]; present {
		t.Fatal("ordinary initialize must not answer the probe extension")
	}
	_ = os.Remove(filepath.Join(work, "task-catalog.json"))
}

func containsChoice(choices []sessioncatalog.Choice, id string) bool {
	for _, choice := range choices {
		if choice.ID == id {
			return true
		}
	}
	return false
}
