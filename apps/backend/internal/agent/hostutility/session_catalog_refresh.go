package hostutility

import "context"

type profileCapabilityCurrency struct {
	manager           *Manager
	agentType         string
	baseRevision      string
	runtimeGeneration uint64
	contextGeneration uint64
}

func (m *Manager) withProfilePublicationCurrency(result ProfileCapabilityResult, agentType, baseRevision string, runtimeGeneration, contextGeneration uint64) ProfileCapabilityResult {
	result.currency = profileCapabilityCurrency{manager: m, agentType: agentType, baseRevision: baseRevision, runtimeGeneration: runtimeGeneration, contextGeneration: contextGeneration}
	return result
}

// CommitIfCurrent serializes a session publication with probe supersession and
// runtime activation. The caller retains request cancellation and saved-profile
// currency checks; process-local generations never become durable revisions.
func (r ProfileCapabilityResult) CommitIfCurrent(ctx context.Context, commit func() error) error {
	currency := r.currency
	if currency.manager == nil || commit == nil {
		return errProfileContextChanged
	}
	manager := currency.manager
	manager.modelGenerationMu.Lock()
	defer manager.modelGenerationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if manager.profileGenerations[currency.agentType] != currency.runtimeGeneration || manager.profileContextGenerations[currency.baseRevision] != currency.contextGeneration {
		return errProfileContextChanged
	}
	return commit()
}

// CommitProfileCapability binds publication to this manager's probe owner.
func (m *Manager) CommitProfileCapability(ctx context.Context, result ProfileCapabilityResult, commit func() error) error {
	if result.currency.manager != m {
		return errProfileContextChanged
	}
	return result.CommitIfCurrent(ctx, commit)
}
