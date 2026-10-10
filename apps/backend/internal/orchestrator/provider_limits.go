package orchestrator

import (
	"context"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) SetProviderLimitService(limits *providerlimit.Service) { s.providerLimits = limits }

func (s *Service) clearProviderLimitSuccess(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession) {
	if s.providerLimits == nil || session == nil || session.RouteGeneration > 0 || data.DynamicRouteAttempt {
		return
	}
	office, err := s.lookupOfficeTask(ctx, data.TaskID)
	if err != nil || office {
		return
	}
	subject, model, err := s.providerLimitSubjectForSession(ctx, session)
	if err != nil || subject.Dynamic {
		if err != nil {
			s.logger.Warn("resolve successful provider limit subject failed", zap.String("session_id", data.SessionID), zap.Error(err))
		}
		return
	}
	if err := s.completeProviderLimitWaitSuccess(ctx, data, session, subject, model); err != nil {
		s.logger.Warn("clear successful provider limit ownership failed", zap.String("task_id", data.TaskID), zap.String("session_id", data.SessionID), zap.Error(err))
	}
}

func (s *Service) providerLimitSubjectForSession(ctx context.Context, session *models.TaskSession) (providerlimit.Subject, string, error) {
	subject, err := s.providerLimits.ResolveSubject(ctx, session.AgentProfileID)
	if err != nil || subject.Dynamic {
		return subject, "", err
	}
	if session.ExecutionProfileID != "" && session.ExecutionProfileID != session.AgentProfileID {
		subject, err = s.providerLimits.ResolveSubject(ctx, session.ExecutionProfileID)
		if err != nil {
			return providerlimit.Subject{}, "", err
		}
	}
	model := subject.Profile.Model
	if runtime, ok := models.LoadEffectiveSessionRuntimeConfig(session); ok && runtime.Model != "" {
		model = runtime.Model
	}
	if getter, ok := s.agentManager.(interface {
		GetModelStateForSession(string) *agentruntime.CachedModelState
	}); ok {
		if state := getter.GetModelStateForSession(session.ID); state != nil && state.CurrentModelID != "" {
			model = state.CurrentModelID
		}
	}
	return subject, model, nil
}
