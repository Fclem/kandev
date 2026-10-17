package orchestrator

import (
	"context"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// Native direct usage requires durable provider ownership, never arrival-time ownership.
func (s *Service) nativeUsageTurnID(ctx context.Context, payload *agentruntime.AgentStreamEventPayload) string {
	observation := payload.Data.UsageObservation
	if observation.Scope == streams.NativeUsageScopeChild {
		return payload.Data.TurnID
	}
	lookup, ok := s.repo.(interface {
		FindTurnIDByNativeCodexTurnID(context.Context, string, string, string) (string, error)
	})
	if !ok || observation.Scope != streams.NativeUsageScopeDirect {
		return ""
	}
	turnID, err := lookup.FindTurnIDByNativeCodexTurnID(ctx, payload.TaskID, payload.SessionID, observation.ProviderTurnID)
	if err != nil || turnID == "" || payload.Data.TurnID != "" && payload.Data.TurnID != turnID {
		s.logger.Warn("skipping native usage without unique durable turn ownership",
			zap.String("session_id", payload.SessionID), zap.String("provider_turn_id", observation.ProviderTurnID), zap.Error(err))
		return ""
	}
	return turnID
}

// Coverage gaps are monotonic native-turn state, not zero-valued usage observations.
func (s *Service) persistNativeUsageCoverageGap(ctx context.Context, payload *agentruntime.AgentStreamEventPayload, turnID string) bool {
	observation := payload.Data.UsageObservation
	if s.turnService == nil || observation.SchemaVersion != 1 || observation.Source != streams.NativeUsageSourceResponse || observation.ProviderResponseID == "" {
		return false
	}
	prefix := models.NativeUsageGapDirectPrefix
	switch observation.Scope {
	case streams.NativeUsageScopeDirect:
	case streams.NativeUsageScopeChild:
		prefix = models.NativeUsageGapChildPrefix
	default:
		return false
	}
	key := fmt.Sprintf("%s%q:%q", prefix, observation.ProviderThreadID, observation.ProviderTurnID)
	if err := s.turnService.PatchTurnMetadata(ctx, payload.SessionID, turnID, map[string]interface{}{key: true}); err != nil {
		s.logger.Warn("failed to persist native usage coverage gap", zap.String("session_id", payload.SessionID), zap.String(metaKeyTurnID, turnID), zap.Error(err))
		return false
	}
	data := map[string]string{metaKeyTaskID: payload.TaskID, metaKeySessionID: payload.SessionID, metaKeyTurnID: turnID}
	if err := s.eventBus.Publish(ctx, events.BuildSessionUsageUpdatedSubject(payload.SessionID), bus.NewEvent(events.SessionUsageUpdated, "orchestrator", data)); err != nil {
		s.logger.Warn("failed to publish native coverage invalidation", zap.String("session_id", payload.SessionID), zap.Error(err))
	}
	return true
}
