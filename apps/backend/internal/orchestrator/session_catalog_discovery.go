package orchestrator

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type catalogDiscoverySettlementStore interface {
	SettleSessionCatalogDiscovery(context.Context, string, string, string, any, sessioncatalog.Observation) (*sessioncatalog.Observation, error)
	PersistSessionCatalogAvailability(context.Context, string, sessioncatalog.Owner, sessioncatalog.Discovery) (*sessioncatalog.ProviderCatalog, error)
}

func (s *Service) handleSessionCatalogDiscovery(ctx context.Context, payload *agentruntime.AgentStreamEventPayload) {
	if payload == nil || payload.Data == nil || payload.SessionID == "" || payload.ExecutionID == "" || payload.SessionSettingsSourceGeneration == 0 {
		return
	}
	if err := sessioncatalog.ValidateDiscovery(payload.Data.CatalogDiscovery); err != nil {
		return
	}
	store, ok := s.repo.(catalogDiscoverySettlementStore)
	if !ok {
		return
	}
	lock := s.sessionSettingsSnapshotLock(payload.SessionID)
	lock.Lock()
	defer lock.Unlock()
	session, err := s.repo.GetTaskSession(ctx, payload.SessionID)
	if err != nil || session.TaskID != payload.TaskID {
		return
	}
	owner := sessioncatalog.Owner{ExecutionID: payload.ExecutionID, AttemptID: payload.AttemptID, SourceGeneration: payload.SessionSettingsSourceGeneration}
	provider, err := store.PersistSessionCatalogAvailability(ctx, session.ID, owner, *payload.Data.CatalogDiscovery)
	if err != nil {
		s.logCatalogSettlementFailure(payload, err)
		return
	}
	if provider != nil {
		if err := s.publishProviderCatalog(ctx, session.TaskID, session.ID, *provider); err != nil {
			s.logCatalogSettlementFailure(payload, err)
		}
	}
	s.settleCatalogDiscovery(ctx, store, session, payload, &owner)
}

func (s *Service) settleCatalogDiscovery(ctx context.Context, store catalogDiscoverySettlementStore, session *models.TaskSession, payload *agentruntime.AgentStreamEventPayload, owner *sessioncatalog.Owner) {
	prior := session.Metadata[sessioncatalog.MetadataKey]
	previous, ok := loadCatalogObservation(prior)
	if !ok || previous.Disposition == sessioncatalog.DispositionCleared {
		return
	}
	next := previous
	next.Owner = *owner
	next.Models, next.Modes, next.Catalog = nil, nil, nil
	next.Source, next.Probe, next.Digest = "", nil, ""
	next.Result = payload.Data.CatalogDiscovery
	next.Disposition, next.Reason = sessioncatalog.DispositionRediscoveryRequired, next.Result.Outcome
	if next.Result.Outcome == sessioncatalog.OutcomeComplete {
		next.Disposition, next.Reason = sessioncatalog.DispositionCleared, sessioncatalog.ReasonDiscoveryComplete
	}
	stored, err := store.SettleSessionCatalogDiscovery(ctx, session.ID, payload.ExecutionID, previous.ProfileID, prior, next)
	if err != nil {
		s.logCatalogSettlementFailure(payload, err)
		return
	}
	if stored == nil {
		return
	}
	if err := s.publishCatalogObservation(ctx, session.TaskID, session.ID, *stored); err != nil {
		s.logCatalogSettlementFailure(payload, err)
	}
}

func (s *Service) logCatalogSettlementFailure(payload *agentruntime.AgentStreamEventPayload, err error) {
	s.logger.Warn("session catalog settlement failed", zap.String("task_id", payload.TaskID), zap.String("session_id", payload.SessionID), zap.String("execution_id", payload.ExecutionID), zap.String("attempt_id", payload.AttemptID), zap.Uint64("source_generation", payload.SessionSettingsSourceGeneration), zap.Error(err))
}

func (s *Service) publishCatalogObservation(ctx context.Context, taskID, sessionID string, record sessioncatalog.Observation) error {
	if s.eventBus == nil {
		return errors.New("catalog event delivery is unavailable")
	}
	// Request cancellation stops remaining fan-out, not delivery of committed state.
	ctx = context.WithoutCancel(ctx)
	return s.eventBus.Publish(ctx, events.BuildSessionCatalogRefreshSubject(sessionID), bus.NewEvent(events.SessionCatalogRefreshUpdated, "orchestrator", sessioncatalog.Update{TaskID: taskID, SessionID: sessionID, Record: record}))
}

func (s *Service) publishProviderCatalog(ctx context.Context, taskID, sessionID string, provider sessioncatalog.ProviderCatalog) error {
	if s.eventBus == nil {
		return errors.New("provider catalog event delivery is unavailable")
	}
	update := sessioncatalog.ProviderUpdate{TaskID: taskID, SessionID: sessionID, Provider: provider}
	return s.eventBus.Publish(context.WithoutCancel(ctx), events.BuildSessionCatalogDiscoverySubject(sessionID), bus.NewEvent(events.SessionCatalogDiscoveryUpdated, "orchestrator", update))
}

// Live provider advertisements update availability, never notice settlement.
func (s *Service) persistProviderCatalogFromEvent(ctx context.Context, payload *agentruntime.AgentStreamEventPayload) error {
	if payload.Data.CatalogDiscovery == nil || payload.SessionSettingsSourceGeneration == 0 {
		return nil
	}
	if err := sessioncatalog.ValidateDiscovery(payload.Data.CatalogDiscovery); err != nil {
		return err
	}
	store, ok := s.repo.(catalogDiscoverySettlementStore)
	if !ok {
		return nil
	}
	owner := sessioncatalog.Owner{ExecutionID: payload.ExecutionID, AttemptID: payload.AttemptID, SourceGeneration: payload.SessionSettingsSourceGeneration}
	provider, err := store.PersistSessionCatalogAvailability(ctx, payload.SessionID, owner, *payload.Data.CatalogDiscovery)
	if err != nil || provider == nil {
		return err
	}
	return s.publishProviderCatalog(ctx, payload.TaskID, payload.SessionID, *provider)
}
