package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	acp "github.com/coder/acp-go-sdk"
)

func TestMockSessionCatalogFileCapturesProcessAdvertisement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	t.Setenv("MOCK_AGENT_SESSION_CATALOG_FILE", path)
	response := acp.NewSessionResponse{ConfigOptions: mockSessionConfigOptions(), Modes: mockSessionModes(mockDefaultMode)}
	writeCatalogResponse(t, path, response)
	old := newCatalogFixtureAgent(t)
	initial, err := old.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatal(err)
	}

	models := response.ConfigOptions[0].Select.Options.Ungrouped
	*models = append(*models, acp.SessionConfigSelectOption{Value: "file-model", Name: "File model"})
	response.Modes.AvailableModes = append(response.Modes.AvailableModes, acp.SessionMode{Id: "file-mode", Name: "File mode"})
	writeCatalogResponse(t, path, response)

	retained, err := old.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogModel(t, initial.ConfigOptions, "file-model", false)
	assertCatalogModel(t, retained.ConfigOptions, "file-model", false)
	fresh := newCatalogFixtureAgent(t)
	probed, err := fresh.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogModel(t, probed.ConfigOptions, "file-model", true)
	if len(probed.Modes.AvailableModes) != 3 || probed.Modes.AvailableModes[2].Id != "file-mode" {
		t.Fatalf("fresh advertised modes = %#v", probed.Modes)
	}

	const nativeID = acp.SessionId("retained-native-conversation")
	loaded, err := fresh.LoadSession(context.Background(), acp.LoadSessionRequest{SessionId: nativeID})
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogModel(t, loaded.ConfigOptions, "file-model", true)
	set, err := fresh.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{SessionId: nativeID, ConfigId: modelConfigID, Value: "file-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogModel(t, set.ConfigOptions, "file-model", true)
	if got := mockConfigOptionValue(set.ConfigOptions, modelConfigID); got != "file-model" {
		t.Fatalf("selected model = %q", got)
	}
	if _, err := fresh.SetSessionMode(context.Background(), acp.SetSessionModeRequest{SessionId: nativeID, ModeId: "file-mode"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.SetSessionMode(context.Background(), acp.SetSessionModeRequest{SessionId: nativeID, ModeId: "not-advertised"}); err == nil {
		t.Fatal("unadvertised mode was accepted")
	}
}

func writeCatalogResponse(t *testing.T, path string, response acp.NewSessionResponse) {
	t.Helper()
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newCatalogFixtureAgent(t *testing.T) *mockAgent {
	t.Helper()
	agent := &mockAgent{conn: newCapturingUpdater(), sessions: make(map[acp.SessionId]bool), commandsEmitted: make(map[acp.SessionId]bool)}
	if _, err := agent.Initialize(context.Background(), acp.InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	return agent
}

func assertCatalogModel(t *testing.T, options []acp.SessionConfigOption, model acp.SessionConfigValueId, want bool) {
	t.Helper()
	for _, option := range options {
		if option.Select != nil && option.Select.Id == modelConfigID {
			if got := mockConfigOptionContainsValue(option.Select.Options, model); got != want {
				t.Fatalf("advertised model %q = %v, want %v", model, got, want)
			}
			return
		}
	}
	t.Fatal("model advertisement missing")
}
