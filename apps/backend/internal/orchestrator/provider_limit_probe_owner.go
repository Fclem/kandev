package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

func (s *Service) updateProviderLimitProbeOwner(ctx context.Context, taskID, identity string, mutate func(models.ProviderLimitProbeOwner) (models.ProviderLimitProbeOwner, bool)) (bool, error) {
	for range deferredLaunchCASRetryBudget {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return false, err
		}
		owners, err := models.ReadProviderLimitProbeOwners(record)
		if err != nil {
			return false, err
		}
		owner, exists := owners[identity]
		if !exists {
			return false, nil
		}
		owner, changed := mutate(owner)
		if !changed {
			return false, nil
		}
		updated, err := models.PutProviderLimitProbeOwner(record, owner)
		if err != nil {
			return false, err
		}
		stored, lost, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
		if err != nil || stored {
			return stored, err
		}
		if !lost {
			return false, errors.New("provider probe owner write returned no outcome")
		}
	}
	return false, errors.New("provider probe owner changed during persistence")
}

func (s *Service) removeProviderLimitProbeOwner(ctx context.Context, taskID, identity string, expected models.ProviderLimitProbeOwner) error {
	for range deferredLaunchCASRetryBudget {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return err
		}
		owners, err := models.ReadProviderLimitProbeOwners(record)
		if err != nil {
			return err
		}
		current, exists := owners[identity]
		if !exists || current.LaunchID != expected.LaunchID || current.ExecutionID != expected.ExecutionID || current.Generation != expected.Generation || current.Lease != expected.Lease {
			return nil
		}
		updated, err := models.RemoveProviderLimitProbeOwner(record, identity)
		if err != nil {
			return err
		}
		stored, lost, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
		if err != nil || stored {
			return err
		}
		if !lost {
			return errors.New("provider probe owner removal returned no outcome")
		}
	}
	return errors.New("provider probe owner removal lost repeated compare-and-set races")
}

func (s *Service) completeProviderLimitProbeOwnerSuccess(ctx context.Context, data watcher.AgentEventData, session *models.TaskSession, subject providerlimit.Subject, model string) (bool, error) {
	record, _, err := s.repo.GetTaskDeferredLaunch(ctx, session.TaskID)
	if err != nil {
		return false, err
	}
	owners, err := models.ReadProviderLimitProbeOwners(record)
	if err != nil {
		return false, err
	}
	generation := data.PromptGeneration
	if generation == 0 {
		generation = s.promptGenerationForSession(ctx, session.ID)
	}
	identity := s.successfulProviderLimitProbeOwner(ctx, session.ID, data.AgentExecutionID, generation, owners)
	if identity == "" {
		return false, nil
	}
	owner := owners[identity]
	if owner.ExecutionID != data.AgentExecutionID || owner.Generation != generation || owner.Model != model {
		return true, errors.New("successful provider completion does not own the observed probe")
	}
	owner, recorded, err := s.recordProviderLimitProbeOwnerSuccess(ctx, session.TaskID, identity, owner)
	if err != nil || !recorded {
		return true, err
	}
	return true, s.releaseSucceededProviderLimitProbe(ctx, session.TaskID, identity, subject, owner)
}

// successfulProviderLimitProbeOwner finds the probe owner for the completed
// execution generation, preferring the owner of the active turn.
func (s *Service) successfulProviderLimitProbeOwner(ctx context.Context, sessionID, executionID string, generation uint64, owners map[string]models.ProviderLimitProbeOwner) string {
	if turnID, err := s.peekActiveTurnID(ctx, sessionID); err == nil && turnID != "" {
		candidate := models.ProviderLimitWaitIdentity(sessionID, turnID)
		if owner, exists := owners[candidate]; exists && owner.ExecutionID == executionID && owner.Generation == generation {
			return candidate
		}
	}
	for candidate, owner := range owners {
		if owner.SessionID == sessionID && owner.ExecutionID == executionID && owner.Generation == generation {
			return candidate
		}
	}
	return ""
}

// recordProviderLimitProbeOwnerSuccess durably marks owner as succeeded unless
// it already is; recorded is false when a newer owner replaced it.
func (s *Service) recordProviderLimitProbeOwnerSuccess(ctx context.Context, taskID, identity string, owner models.ProviderLimitProbeOwner) (models.ProviderLimitProbeOwner, bool, error) {
	if owner.Succeeded {
		return owner, true, nil
	}
	changed, err := s.updateProviderLimitProbeOwner(ctx, taskID, identity, func(current models.ProviderLimitProbeOwner) (models.ProviderLimitProbeOwner, bool) {
		if current.LaunchID != owner.LaunchID || current.ExecutionID != owner.ExecutionID || current.Generation != owner.Generation || current.Lease != owner.Lease {
			return current, false
		}
		current.Succeeded = true
		return current, true
	})
	if err != nil || !changed {
		return owner, false, err
	}
	owner.Succeeded = true
	return owner, true, nil
}

// releaseSucceededProviderLimitProbe clears the mark, releases the owner's own
// probe lease, and removes the owner record.
func (s *Service) releaseSucceededProviderLimitProbe(ctx context.Context, taskID, identity string, subject providerlimit.Subject, owner models.ProviderLimitProbeOwner) error {
	if _, err := s.providerLimits.ClearOnSuccess(ctx, subject, owner.Model); err != nil {
		return err
	}
	if mark, exists := s.providerLimits.Get(owner.Lease.Key); exists && !mark.ProbeUntil.IsZero() && !mark.ProbeUntil.Equal(owner.Lease.Until) {
		return errors.New("successful provider completion cannot release a successor probe")
	}
	if _, err := s.providerLimits.ReleaseProbe(ctx, dynamic.ProbeLease{Key: owner.Lease.Key, ExpiresAt: owner.Lease.Until}, true); err != nil {
		return err
	}
	return s.removeProviderLimitProbeOwner(ctx, taskID, identity, owner)
}

func (s *Service) retireFailedProviderLimitProbeOwner(ctx context.Context, data watcher.AgentEventData) {
	if data.SessionID == "" || data.AgentExecutionID == "" {
		return
	}
	turnID, err := s.peekActiveTurnID(ctx, data.SessionID)
	if err != nil || turnID == "" {
		return
	}
	identity := models.ProviderLimitWaitIdentity(data.SessionID, turnID)
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil {
		return
	}
	record, _, err := s.repo.GetTaskDeferredLaunch(ctx, session.TaskID)
	if err != nil {
		return
	}
	owners, err := models.ReadProviderLimitProbeOwners(record)
	if err != nil {
		return
	}
	owner, exists := owners[identity]
	if !exists || owner.ExecutionID != data.AgentExecutionID || owner.Generation != data.PromptGeneration {
		return
	}
	_, _ = s.providerLimits.ReleaseProbe(ctx, dynamic.ProbeLease{Key: owner.Lease.Key, ExpiresAt: owner.Lease.Until}, false)
	if err := s.removeProviderLimitProbeOwner(ctx, session.TaskID, identity, owner); err != nil {
		s.logger.Warn("could not retire failed provider probe owner", zap.String("session_id", data.SessionID), zap.Error(err))
	}
}

func (s *Service) reconcileProviderLimitProbeOwners(ctx context.Context, tasks []*models.Task) time.Time {
	now := s.providerLimitNow()
	var next time.Time
	for _, task := range tasks {
		if task == nil {
			continue
		}
		record, _, err := s.repo.GetTaskDeferredLaunch(ctx, task.ID)
		if err != nil {
			continue
		}
		owners, err := models.ReadProviderLimitProbeOwners(record)
		if err != nil {
			continue
		}
		for identity, owner := range owners {
			next = earlierDeadline(next, s.reconcileProviderLimitProbeOwner(ctx, task, identity, owner, now))
		}
	}
	return next
}

// reconcileProviderLimitProbeOwner finishes a succeeded owner, keeps a live
// owner until its lease ends, and retires an orphaned one. It returns the next
// time the owner needs attention, or zero when none is needed.
func (s *Service) reconcileProviderLimitProbeOwner(ctx context.Context, task *models.Task, identity string, owner models.ProviderLimitProbeOwner, now time.Time) time.Time {
	if owner.Succeeded {
		if s.finishSucceededProviderLimitProbeOwner(ctx, owner) {
			return time.Time{}
		}
		return now.Add(time.Minute)
	}
	if s.providerLimitProbeOwnerLive(ctx, task, owner) {
		return owner.Lease.Until
	}
	_, _ = s.providerLimits.ReleaseProbe(ctx, dynamic.ProbeLease{Key: owner.Lease.Key, ExpiresAt: owner.Lease.Until}, false)
	if err := s.removeProviderLimitProbeOwner(ctx, task.ID, identity, owner); err != nil {
		return now.Add(time.Minute)
	}
	return time.Time{}
}

func (s *Service) finishSucceededProviderLimitProbeOwner(ctx context.Context, owner models.ProviderLimitProbeOwner) bool {
	session, err := s.repo.GetTaskSession(ctx, owner.SessionID)
	if err != nil || session == nil {
		return false
	}
	subject, _, err := s.providerLimitSubjectForSession(ctx, session)
	if err != nil || subject.Dynamic {
		return false
	}
	data := watcher.AgentEventData{SessionID: owner.SessionID, AgentExecutionID: owner.ExecutionID, PromptGeneration: owner.Generation}
	_, err = s.completeProviderLimitProbeOwnerSuccess(ctx, data, session, subject, owner.Model)
	return err == nil
}

// providerLimitProbeOwnerLive reports whether the owner's session is still
// active on the execution that holds the probe.
func (s *Service) providerLimitProbeOwnerLive(ctx context.Context, task *models.Task, owner models.ProviderLimitProbeOwner) bool {
	if task.ArchivedAt != nil {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, owner.SessionID)
	if err != nil || session == nil || session.AgentExecutionID != owner.ExecutionID {
		return false
	}
	switch session.State {
	case models.TaskSessionStateCancelled, models.TaskSessionStateCompleted, models.TaskSessionStateFailed:
		return false
	default:
		return true
	}
}

// earlierDeadline returns the earlier non-zero deadline; a zero candidate
// means no deadline and leaves current unchanged.
func earlierDeadline(current, candidate time.Time) time.Time {
	if candidate.IsZero() {
		return current
	}
	if current.IsZero() || candidate.Before(current) {
		return candidate
	}
	return current
}
