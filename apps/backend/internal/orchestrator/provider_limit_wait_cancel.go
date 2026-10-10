package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type providerLimitWaitRemover interface {
	RemoveProviderLimitWait(context.Context, string, string, *models.ProviderLimitProbeLease, bool) (bool, error)
}

func (s *Service) CancelProviderLimitWait(ctx context.Context, taskID, sessionID, identity string) bool {
	if identity == "" || s.authorizeTaskSessionPair(ctx, taskID, sessionID) != nil {
		return false
	}
	guard, release := s.acquireCancelInFlightGuard(sessionID)
	guard.Lock()
	session, readErr := s.repo.GetTaskSession(ctx, sessionID)
	if readErr != nil || session == nil || session.State != models.TaskSessionStateWaitingForInput {
		guard.Unlock()
		release()
		return false
	}
	changed, err := s.removePendingProviderLimitWait(ctx, taskID, sessionID, identity, false)
	guard.Unlock()
	release()
	if err != nil {
		s.logger.Warn("cancel provider limit wait failed", zap.String("session_id", sessionID), zap.Error(err))
		return false
	}
	if !changed {
		return false
	}
	providerlimit.ObserveWait(s.logger.Zap(), providerlimit.MetricContextKanban, providerlimit.MetricWaitCancelled)
	s.handleRecoverableFailure(ctx, watcher.AgentEventData{TaskID: taskID, SessionID: sessionID, UserInitiated: true, FailureCode: "provider_limit_wait_cancelled", ErrorMessage: "Automatic provider reset wait cancelled. Resume or start fresh to continue."})
	s.ReconcileProviderLimitWaits(ctx)
	return true
}

// The caller holds the session dispatch/cancellation guard.
func (s *Service) removePendingProviderLimitWait(ctx context.Context, taskID, sessionID, identity string, success bool) (bool, error) {
	remover, capable := s.repo.(providerLimitWaitRemover)
	if !capable {
		return false, nil
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if session == nil {
		return false, nil
	}
	current, _ := session.Metadata[providerLimitWaitIdentityKey].(string)
	if identity == "" {
		identity = current
	}
	if identity == "" || current != identity {
		return false, nil
	}
	record, _, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
	if err != nil {
		return false, err
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		return false, err
	}
	wait, exists := waits[identity]
	if !exists || wait.SessionID != sessionID {
		return false, nil
	}
	if wait.ProbeLease != nil {
		if s.providerLimits == nil {
			return false, errors.New("provider limit probe service unavailable")
		}
		_, err = s.providerLimits.ReleaseProbe(ctx, dynamic.ProbeLease{Key: wait.ProbeLease.Key, ExpiresAt: wait.ProbeLease.Until}, success)
		if err != nil {
			return false, err
		}
	}
	changed, err := remover.RemoveProviderLimitWait(ctx, taskID, identity, wait.ProbeLease, success)
	if changed {
		s.resolveProviderLimitWaitNotice(ctx, sessionID, identity)
	}
	return changed, err
}

func (s *Service) resolveProviderLimitWaitNotice(ctx context.Context, sessionID, identity string) {
	if s.transientRetryMessages == nil {
		return
	}
	messages, err := s.transientRetryMessages.ListMessages(ctx, sessionID)
	if err != nil {
		return
	}
	for _, message := range messages {
		if message != nil && message.Metadata["limit_wait_identity"] == identity {
			if err := s.transientRetryMessages.DeleteMessage(ctx, message.ID); err != nil {
				s.logger.Warn("remove settled provider limit wait notice failed", zap.String("session_id", sessionID), zap.Error(err))
			}
		}
	}
}

func (s *Service) discardInvalidProviderLimitWait(ctx context.Context, taskID, identity string, wait models.ProviderLimitWait) {
	remover, capable := s.repo.(providerLimitWaitRemover)
	if !capable {
		return
	}
	if wait.ProbeLease != nil {
		if _, err := s.providerLimits.ReleaseProbe(ctx, dynamic.ProbeLease{Key: wait.ProbeLease.Key, ExpiresAt: wait.ProbeLease.Until}, false); err != nil {
			s.logger.Warn("release invalid provider limit wait probe failed", zap.String("session_id", wait.SessionID), zap.Error(err))
			return
		}
	}
	changed, err := remover.RemoveProviderLimitWait(ctx, taskID, identity, wait.ProbeLease, false)
	if err != nil {
		s.logger.Warn("remove invalid provider limit wait failed", zap.String("session_id", wait.SessionID), zap.Error(err))
		return
	}
	if changed {
		s.resolveProviderLimitWaitNotice(ctx, wait.SessionID, identity)
		s.publishTaskUpdatedByID(ctx, taskID)
	}
}

func (s *Service) retireFailedProviderLimitProbe(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession) (*models.TaskSession, error) {
	identity, _ := session.Metadata[providerLimitWaitIdentityKey].(string)
	if identity == "" {
		return session, nil
	}
	record, _, err := s.repo.GetTaskDeferredLaunch(ctx, session.TaskID)
	if err != nil {
		return nil, err
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		return nil, err
	}
	wait, exists := waits[identity]
	if !exists || wait.ProbeTurnID == "" {
		return session, nil
	}
	generation := data.PromptGeneration
	if generation == 0 {
		generation = s.promptGenerationForSession(ctx, session.ID)
	}
	if wait.ProbeExecutionID != data.AgentExecutionID || wait.ProbeGeneration != generation {
		return nil, ErrCeilingLaunchSuperseded
	}
	changed, err := s.removePendingProviderLimitWait(ctx, session.TaskID, session.ID, identity, false)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, ErrCeilingLaunchSuperseded
	}
	providerlimit.ObserveWait(s.logger.Zap(), providerlimit.MetricContextKanban, providerlimit.MetricWaitProbeFailed)
	return s.repo.GetTaskSession(ctx, session.ID)
}
