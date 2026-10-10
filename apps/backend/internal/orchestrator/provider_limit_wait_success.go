package orchestrator

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) completeProviderLimitWaitSuccess(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession, subject providerlimit.Subject, model string) error {
	identity, _ := session.Metadata[providerLimitWaitIdentityKey].(string)
	if identity == "" {
		return s.completeUnownedProviderLimitSuccess(ctx, data, session, subject, model)
	}
	record, _, err := s.repo.GetTaskDeferredLaunch(ctx, session.TaskID)
	if err != nil {
		return err
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		return err
	}
	wait, exists := waits[identity]
	if !exists {
		return errors.New("successful provider wait owner missing")
	}
	generation := data.PromptGeneration
	if generation == 0 {
		generation = s.promptGenerationForSession(ctx, session.ID)
	}
	if wait.ProbeTurnID == "" || wait.ProbeExecutionID != data.AgentExecutionID || wait.ProbeGeneration == 0 || wait.ProbeGeneration != generation {
		return errors.New("successful provider wait completion does not own the observed probe")
	}
	wait, err = s.recordProviderLimitWaitSuccess(ctx, session.TaskID, identity, wait)
	if err != nil {
		return err
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil {
		return err
	}
	return s.finishSuccessfulProviderLimitWait(ctx, providerLimitWaitCandidate{task: task, identity: identity, wait: wait})
}

// completeUnownedProviderLimitSuccess handles success on a session without a
// wait: it settles any launch-probe owner, clears the mark, and resets the
// session's wait budget.
func (s *Service) completeUnownedProviderLimitSuccess(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession, subject providerlimit.Subject, model string) error {
	if handled, err := s.completeProviderLimitProbeOwnerSuccess(ctx, data, session, subject, model); handled || err != nil {
		return err
	}
	if _, err := s.providerLimits.ClearOnSuccess(ctx, subject, model); err != nil {
		return err
	}
	if _, exists := session.Metadata[models.ProviderLimitWaitsKey]; !exists {
		return nil
	}
	updater, capable := s.repo.(interface {
		UpdateTaskSessionIfCurrentSnapshot(context.Context, *models.TaskSession, models.TaskSessionState, time.Time, map[string]interface{}) (bool, error)
	})
	if !capable {
		return errors.New("provider wait budget reset unavailable")
	}
	metadata := maps.Clone(session.Metadata)
	delete(metadata, models.ProviderLimitWaitsKey)
	changed, err := updater.UpdateTaskSessionIfCurrentSnapshot(ctx, session, session.State, session.UpdatedAt, metadata)
	if err != nil {
		return err
	}
	if !changed {
		return errors.New("successful provider wait budget ownership changed")
	}
	return nil
}

// recordProviderLimitWaitSuccess durably marks the wait's probe as succeeded
// while its lease is still the mark's current probe.
func (s *Service) recordProviderLimitWaitSuccess(ctx context.Context, taskID, identity string, wait models.ProviderLimitWait) (models.ProviderLimitWait, error) {
	if wait.ProbeSucceeded {
		return wait, nil
	}
	if wait.ProbeLease != nil {
		mark, exists := s.providerLimits.Get(wait.MarkKey)
		if !exists || !mark.ProbeUntil.Equal(wait.ProbeLease.Until) {
			return wait, errors.New("successful provider wait token has been superseded")
		}
	}
	changed, err := s.updateProviderLimitWait(ctx, taskID, identity, func(current models.ProviderLimitWait) (models.ProviderLimitWait, bool) {
		if current.ProbeTurnID != wait.ProbeTurnID || current.ProbeExecutionID != wait.ProbeExecutionID || current.ProbeGeneration != wait.ProbeGeneration {
			return current, false
		}
		current.ProbeSucceeded = true
		return current, true
	})
	if err != nil {
		return wait, err
	}
	if !changed {
		return wait, errors.New("successful provider wait ownership changed")
	}
	wait.ProbeSucceeded = true
	return wait, nil
}

func (s *Service) finishSuccessfulProviderLimitWait(ctx context.Context, candidate providerLimitWaitCandidate) error {
	wait := candidate.wait
	session, err := s.repo.GetTaskSession(ctx, wait.SessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("successful provider wait session missing")
	}
	subject, _, err := s.providerLimitSubjectForSession(ctx, session)
	if err != nil {
		return err
	}
	if _, err := s.providerLimits.ClearOnSuccess(ctx, subject, wait.Model); err != nil {
		return err
	}
	if wait.ProbeLease != nil {
		if mark, exists := s.providerLimits.Get(wait.MarkKey); exists && !mark.ProbeUntil.IsZero() && !mark.ProbeUntil.Equal(wait.ProbeLease.Until) {
			return errors.New("successful provider wait cannot release another probe owner")
		}
		if _, err := s.providerLimits.ReleaseProbe(ctx, dynamic.ProbeLease{Key: wait.ProbeLease.Key, ExpiresAt: wait.ProbeLease.Until}, true); err != nil {
			return err
		}
	}
	remover, capable := s.repo.(providerLimitWaitRemover)
	if !capable {
		return errors.New("successful provider wait cleanup unavailable")
	}
	changed, err := remover.RemoveProviderLimitWait(ctx, candidate.task.ID, candidate.identity, wait.ProbeLease, true)
	if err != nil {
		return err
	}
	if !changed {
		return errors.New("successful provider wait cleanup ownership changed")
	}
	providerlimit.ObserveWait(s.logger.Zap(), providerlimit.MetricContextKanban, providerlimit.MetricWaitResumed)
	s.resolveProviderLimitWaitNotice(ctx, wait.SessionID, candidate.identity)
	s.publishTaskUpdatedByID(ctx, candidate.task.ID)
	s.ReconcileProviderLimitWaits(ctx)
	return nil
}
