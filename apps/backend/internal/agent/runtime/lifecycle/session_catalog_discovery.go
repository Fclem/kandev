package lifecycle

import (
	"context"
	"errors"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
)

type catalogDiscoveryStartupStore interface {
	BeginSessionCatalogDiscovery(context.Context, string, string, sessioncatalog.Owner, string) (*sessioncatalog.Launch, error)
}

func (sm *SessionManager) reserveSessionCatalogDiscovery(ctx context.Context, execution *AgentExecution, source uint64, attemptID string) error {
	if sm.catalogDiscoveryStore == nil || source == 0 || execution.SessionID == "" {
		return nil
	}
	profileID := execution.ExecutionProfileID
	if profileID == "" {
		profileID = execution.AgentProfileID
	}
	if profileID == "" {
		return nil
	}
	if execution.startupAttemptSnapshot() != source || execution.currentStartupAttemptID() != attemptID {
		return errors.New("catalog discovery startup owner changed")
	}
	owner := sessioncatalog.Owner{ExecutionID: execution.ID, AttemptID: attemptID, SourceGeneration: source}
	policy := sessioncatalog.PolicyStrict
	if execution.sessionSettingsStartupPolicy() == SessionSettingsPolicyProviderRestored {
		policy = sessioncatalog.PolicyProviderRestored
	}
	launch, err := sm.catalogDiscoveryStore.BeginSessionCatalogDiscovery(ctx, execution.SessionID, profileID, owner, policy)
	if err != nil {
		return err
	}
	if launch == nil || launch.Owner != owner || execution.startupAttemptSnapshot() != source || execution.currentStartupAttemptID() != attemptID {
		return errors.New("catalog discovery startup owner changed")
	}
	return nil
}

func (sm *SessionManager) finalSessionCatalogDiscovery(ctx context.Context, execution *AgentExecution, nativeID string, initial *sessioncatalog.Discovery) *sessioncatalog.Discovery {
	if initial == nil {
		return nil
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return &sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeFailed}
	}
	observeCtx, cancel := context.WithTimeout(ctx, freshSessionModelStateWait)
	defer cancel()
	result, err := client.ReadSessionCatalogDiscovery(observeCtx, nativeID)
	if err != nil {
		return &sessioncatalog.Discovery{Outcome: sessioncatalog.OutcomeFailed}
	}
	return result
}

func (sm *SessionManager) publishSessionCatalogDiscovery(execution *AgentExecution, nativeID string, source uint64, attemptID string, discovery *sessioncatalog.Discovery) {
	if discovery == nil || sm.eventPublisher == nil {
		return
	}
	switch discovery.Outcome {
	case sessioncatalog.OutcomeComplete, sessioncatalog.OutcomeUnsupported, sessioncatalog.OutcomeFailed:
	default:
		return
	}
	execution.withStartupAttempt(source, func(currentAttempt string) {
		if currentAttempt != attemptID {
			return
		}
		sm.eventPublisher.publishAgentStreamEventWithAttempt(execution, agentctl.AgentEvent{
			Type: streams.EventTypeSessionCatalogDiscovery, SessionID: nativeID,
			SessionSettingsSourceGeneration: source, SessionSettingsGeneration: discovery.Generation, CatalogDiscovery: discovery,
		}, attemptID)
	})
}
