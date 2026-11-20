package codexappserver

import "github.com/kandev/kandev/internal/common/sessioncatalog"

func (a *Adapter) beginCatalogHandshake() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.catalogHandshakeGeneration++
	a.catalogDiscovery = nil
	return a.catalogHandshakeGeneration
}

func (a *Adapter) finishCatalogHandshake(generation uint64, success bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if generation != a.catalogHandshakeGeneration {
		return
	}
	if !success {
		a.catalogDiscovery = &sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeFailed}
		return
	}
	// Initialize commits info only after model/list actually succeeds, including
	// its explicit empty response. Thread readiness alone is not discovery proof.
	if a.info == nil {
		return
	}
	a.catalogDiscovery = &sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeComplete, Models: sessioncatalog.Surface{Present: true, Supported: true}}
}

func (a *Adapter) GetSessionCatalogDiscovery() *sessioncatalog.Discovery {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.catalogDiscovery == nil {
		return nil
	}
	result := *a.catalogDiscovery
	if result.Outcome != sessioncatalog.OutcomeComplete {
		return &result
	}
	result.Models.Choices = make([]sessioncatalog.Choice, 0, len(a.models))
	for _, model := range a.models {
		result.Models.Choices = append(result.Models.Choices, sessioncatalog.Choice{ID: model.ModelID, Name: model.Name, Description: model.Description})
	}
	return &result
}

// SessionCatalogSourceGeneration identifies the current conversation handshake.
func (a *Adapter) SessionCatalogSourceGeneration() uint64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.catalogHandshakeGeneration
}
