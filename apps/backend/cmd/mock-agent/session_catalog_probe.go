package main

import (
	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

// catalogProbeRequested reports the deterministic fixture's read-only
// initialize extension. It is test support, never provider qualification.
func catalogProbeRequested(req acp.InitializeRequest) bool {
	requested, _ := req.Meta[sessioncatalog.MockCatalogProbeMetaKey].(bool)
	return requested
}

// catalogProbeDiscovery reports this fresh process's own catalog, read from
// its catalog file at initialize. It opens, loads and prompts no session.
func (a *mockAgent) catalogProbeDiscovery() sessioncatalog.Discovery {
	discovery := sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeComplete}
	for _, option := range a.catalogConfigOptions(modelFast) {
		if option.Select == nil || option.Select.Id != modelConfigID {
			continue
		}
		discovery.Models = sessioncatalog.Surface{
			Present: true, Supported: true,
			Choices: catalogProbeSelectChoices(option.Select.Options),
			Catalog: acpcompat.CatalogFromSelectOptions(option.Select.Options),
		}
	}
	if modes := a.catalogModes(mockDefaultMode); modes != nil {
		choices := make([]sessioncatalog.Choice, 0, len(modes.AvailableModes))
		for _, mode := range modes.AvailableModes {
			choices = append(choices, sessioncatalog.Choice{ID: string(mode.Id), Name: mode.Name, Description: derefString(mode.Description)})
		}
		discovery.Modes = sessioncatalog.Surface{Present: true, Supported: true, Choices: choices}
	}
	if !discovery.Models.Supported && !discovery.Modes.Supported {
		return sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeUnsupported}
	}
	return discovery
}

func catalogProbeSelectChoices(options acp.SessionConfigSelectOptions) []sessioncatalog.Choice {
	var flat []acp.SessionConfigSelectOption
	if options.Ungrouped != nil {
		flat = append(flat, (*options.Ungrouped)...)
	}
	if options.Grouped != nil {
		for _, group := range *options.Grouped {
			flat = append(flat, group.Options...)
		}
	}
	choices := make([]sessioncatalog.Choice, 0, len(flat))
	for _, option := range flat {
		choices = append(choices, sessioncatalog.Choice{ID: string(option.Value), Name: option.Name, Description: derefString(option.Description)})
	}
	return choices
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
