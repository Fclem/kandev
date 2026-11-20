package acp

import (
	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

func (a *Adapter) beginCatalogHandshake() {
	a.mu.Lock()
	a.catalogHandshakeGeneration++
	a.catalogDiscovery = nil
	a.catalogHandshakeReady = false
	a.mu.Unlock()
}

func (a *Adapter) finishCatalogHandshake(success bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !success {
		a.catalogDiscovery = &sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeFailed}
	}
	a.catalogHandshakeReady = true
}

func catalogSurfacePresence(meta map[string]any, options []acpsdk.SessionConfigOption, models, modes bool) (bool, bool) {
	for _, option := range options {
		if option.Select == nil {
			continue
		}
		normalized := streams.ConfigOption{ID: string(option.Select.Id)}
		if option.Select.Category != nil {
			normalized.Category = string(*option.Select.Category)
		}
		models = models || normalized.ID == configOptionIDModel || normalized.Category == configOptionIDModel
		modes = modes || isModeConfigOption(normalized)
	}
	if len(options) == 0 {
		for _, option := range extractConfigOptions(meta) {
			models = models || option.ID == configOptionIDModel || option.Category == configOptionIDModel
			modes = modes || isModeConfigOption(option)
		}
	}
	return models, modes
}

func (a *Adapter) recordCatalogHandshakeLocked(models, modes bool) {
	outcome := sessioncatalog.OutcomeUnsupported
	if models || modes {
		outcome = sessioncatalog.OutcomeComplete
	}
	a.catalogDiscovery = &sessioncatalog.Discovery{Outcome: outcome, Models: sessioncatalog.Surface{Present: models, Supported: models}, Modes: sessioncatalog.Surface{Present: modes, Supported: modes}}
}

// Only real provider option lists establish support; dialect fallbacks do not.
func (a *Adapter) noteCatalogOptionsLocked(options []streams.ConfigOption) {
	if !a.catalogHandshakeReady || a.catalogDiscovery == nil || a.catalogDiscovery.Outcome == sessioncatalog.OutcomeFailed {
		return
	}
	for _, option := range options {
		if option.ID == configOptionIDModel || option.Category == configOptionIDModel {
			a.catalogDiscovery.Models.Present, a.catalogDiscovery.Models.Supported = true, true
		}
		if isModeConfigOption(option) {
			a.catalogDiscovery.Modes.Present, a.catalogDiscovery.Modes.Supported = true, true
		}
	}
	if a.catalogDiscovery.Models.Supported || a.catalogDiscovery.Modes.Supported {
		a.catalogDiscovery.Outcome = sessioncatalog.OutcomeComplete
	}
}

// GetSessionCatalogDiscovery reports an explicit completed handshake, including
// supported empty surfaces. Missing transient model state is never terminal proof.
func (a *Adapter) GetSessionCatalogDiscovery() *sessioncatalog.Discovery {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.catalogDiscoveryLocked()
}

// SessionCatalogSourceGeneration changes for handshakes, not settings edits.
func (a *Adapter) SessionCatalogSourceGeneration() uint64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.catalogHandshakeGeneration
}

func (a *Adapter) catalogDiscoveryLocked() *sessioncatalog.Discovery {
	if !a.catalogHandshakeReady || a.catalogDiscovery == nil {
		return nil
	}
	result := *a.catalogDiscovery
	result.Generation = a.sessionSettingsGeneration
	if result.Outcome != sessioncatalog.OutcomeComplete {
		return &result
	}
	if result.Models.Supported {
		result.Models.Choices = make([]sessioncatalog.Choice, 0, len(a.availableModels))
		foundOption := false
		for _, option := range a.availableConfigOptions {
			if option.ID != configOptionIDModel && option.Category != configOptionIDModel {
				continue
			}
			foundOption = true
			result.Models.Catalog = option.Catalog
			for _, choice := range option.Options {
				result.Models.Choices = append(result.Models.Choices, sessioncatalog.Choice{ID: choice.Value, Name: choice.Name, Description: choice.Description})
			}
			break
		}
		if !foundOption {
			for _, model := range a.availableModels {
				description := ""
				if model.Description != nil {
					description = *model.Description
				}
				result.Models.Choices = append(result.Models.Choices, sessioncatalog.Choice{ID: model.ModelId, Name: model.Name, Description: description})
			}
		}
	}
	if result.Modes.Supported {
		result.Modes.Choices = make([]sessioncatalog.Choice, 0, len(a.availableModes))
		for _, mode := range a.availableModes {
			result.Modes.Choices = append(result.Modes.Choices, sessioncatalog.Choice{ID: mode.ID, Name: mode.Name, Description: mode.Description})
		}
	}
	return &result
}

func (a *Adapter) enrichProviderCatalogLocked(event *AgentEvent) {
	if event.Type != streams.EventTypeSessionModels || event.SessionID != a.sessionID {
		return
	}
	source, _ := event.Data[streams.ConfigOptionsSourceKey].(string)
	if source == streams.ConfigOptionsSourceProviderUpdate || source == streams.ConfigOptionsSourceProviderResponse {
		event.CatalogDiscovery = a.catalogDiscoveryLocked()
	}
}
