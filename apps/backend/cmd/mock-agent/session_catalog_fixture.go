package main

import (
	"encoding/json"
	"fmt"
	"os"

	acp "github.com/coder/acp-go-sdk"
)

// The file changes provider data without changing the process's launch inputs.
// Each initialized process keeps its own advertisement until it is replaced.
func (a *mockAgent) loadCatalogFixture() error {
	path := os.Getenv("MOCK_AGENT_SESSION_CATALOG_FILE")
	if path == "" || a.catalogFixture != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read mock catalog fixture: %w", err)
	}
	var response acp.NewSessionResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("decode mock catalog fixture: %w", err)
	}
	a.catalogFixture = &response
	return nil
}

func (a *mockAgent) catalogConfigOptions(model string) []acp.SessionConfigOption {
	if a.catalogFixture == nil {
		return mockSessionConfigOptionsForModel(model)
	}
	options := cloneSessionConfigOptions(a.catalogFixture.ConfigOptions)
	for _, option := range options {
		if option.Select != nil && option.Select.Id == modelConfigID {
			option.Select.CurrentValue = acp.SessionConfigValueId(model)
		}
	}
	return options
}

func (a *mockAgent) catalogModes(current acp.SessionModeId) *acp.SessionModeState {
	if a.catalogFixture == nil {
		return mockSessionModes(current)
	}
	if a.catalogFixture.Modes == nil {
		return nil
	}
	modes := *a.catalogFixture.Modes
	modes.CurrentModeId = current
	return &modes
}
