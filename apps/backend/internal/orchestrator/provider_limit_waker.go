package orchestrator

import (
	"context"
	"errors"
	"maps"
	"sort"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/providerlimit"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

type providerLimitProbeLaunchTransfer interface {
	TransferProviderLimitLaunchProbe(context.Context, string, string, models.ProviderLimitProbeOwner) (bool, error)
}

type providerLimitTimer interface{ Stop() bool }
type providerLimitClock interface {
	Now() time.Time
	AfterFunc(time.Duration, func()) providerLimitTimer
}
type realProviderLimitClock struct{}

func (realProviderLimitClock) Now() time.Time { return time.Now().UTC() }
func (realProviderLimitClock) AfterFunc(delay time.Duration, callback func()) providerLimitTimer {
	return time.AfterFunc(delay, callback)
}

func (s *Service) providerLimitNow() time.Time {
	if s.providerLimitClock != nil {
		return s.providerLimitClock.Now().UTC()
	}
	return time.Now().UTC()
}

type providerLimitWaitTaskLister interface {
	ListTasksWithProviderLimitWaits(context.Context) ([]*models.Task, error)
}
type providerLimitWaitCandidate struct {
	task     *models.Task
	identity string
	wait     models.ProviderLimitWait
}

// ReconcileProviderLimitWaits is also called by the task-session safety sweep.
func (s *Service) ReconcileProviderLimitWaits(ctx context.Context) {
	s.startAgentFailureRecovery(s.reconcileProviderLimitWaits)
}

func (s *Service) reconcileProviderLimitWaits(ctx context.Context) {
	s.providerLimitRecoveryPassMu.Lock()
	defer s.providerLimitRecoveryPassMu.Unlock()
	if ctx.Err() != nil || s.providerLimits == nil {
		return
	}
	lister, capable := s.repo.(providerLimitWaitTaskLister)
	if !capable {
		return
	}
	tasks, err := lister.ListTasksWithProviderLimitWaits(ctx)
	if err != nil {
		s.logger.Warn("list provider limit waits failed", zap.Error(err))
		s.scheduleProviderLimitWake(s.providerLimitNow().Add(time.Minute))
		return
	}
	var next time.Time
	for key, candidates := range s.collectProviderLimitWaitGroups(ctx, tasks) {
		next = earlierDeadline(next, s.reconcileProviderLimitWaitGroup(ctx, key, candidates))
	}
	next = earlierDeadline(next, s.reconcileProviderLimitProbeOwners(ctx, tasks))
	next = earlierDeadline(next, s.reconcileProviderLimitLaunches(ctx, tasks))
	s.scheduleProviderLimitWake(next)
}

// collectProviderLimitWaitGroups groups valid waits by mark key, discarding
// waits whose task step or owning session no longer matches.
func (s *Service) collectProviderLimitWaitGroups(ctx context.Context, tasks []*models.Task) map[string][]providerLimitWaitCandidate {
	groups := make(map[string][]providerLimitWaitCandidate)
	for _, task := range tasks {
		record, _, readErr := s.repo.GetTaskDeferredLaunch(ctx, task.ID)
		if readErr != nil {
			continue
		}
		waits, readErr := models.ReadProviderLimitWaits(record)
		if readErr != nil {
			s.logger.Warn("read provider limit waits failed", zap.String("task_id", task.ID), zap.Error(readErr))
			continue
		}
		for identity, wait := range waits {
			valid, known := s.providerLimitWaitValid(ctx, task, identity, wait)
			if !known {
				continue
			}
			if !valid {
				s.discardInvalidProviderLimitWait(ctx, task.ID, identity, wait)
				continue
			}
			groups[wait.MarkKey] = append(groups[wait.MarkKey], providerLimitWaitCandidate{task: task, identity: identity, wait: wait})
		}
	}
	return groups
}

// providerLimitWaitValid reports whether a wait still belongs to its task step
// and session; known is false when the session could not be read.
func (s *Service) providerLimitWaitValid(ctx context.Context, task *models.Task, identity string, wait models.ProviderLimitWait) (valid, known bool) {
	if task.ArchivedAt != nil || task.WorkflowStepID != wait.WorkflowStepID {
		return false, true
	}
	session, err := s.repo.GetTaskSession(ctx, wait.SessionID)
	if err != nil {
		return false, false
	}
	if wait.ProbeSucceeded {
		return true, true
	}
	if session == nil || session.Metadata[providerLimitWaitIdentityKey] != identity {
		return false, true
	}
	switch session.State {
	case models.TaskSessionStateCancelled, models.TaskSessionStateCompleted, models.TaskSessionStateFailed:
		return false, true
	default:
		return true, true
	}
}

func (s *Service) reconcileProviderLimitWaitGroup(ctx context.Context, key string, candidates []providerLimitWaitCandidate) time.Time {
	now := s.providerLimitNow()
	if deadline, finished := s.finishSucceededProviderLimitWait(ctx, candidates, now); finished {
		return deadline
	}
	mark, marked := s.providerLimits.Get(key)
	if marked && mark.ProbeUntil.After(now) {
		s.redispatchLeasedProviderLimitWaits(ctx, candidates, mark)
		return mark.ProbeUntil
	}
	sortProviderLimitWaitCandidates(candidates)
	var next time.Time
	for _, candidate := range candidates {
		session, ready := s.providerLimitWaitReadySession(ctx, candidate)
		if !ready {
			continue
		}
		deadline := candidate.wait.NotBefore
		if marked && mark.Until.After(deadline) {
			deadline = mark.Until
		}
		if deadline.After(now) {
			next = earlierDeadline(next, deadline)
			continue
		}
		if !s.providerLimitWaitResumable(ctx, session, mark, marked) {
			continue
		}
		selected, err := s.acquireProviderLimitWaitProbe(ctx, candidate, mark, marked)
		if err != nil {
			s.logger.Warn("select provider limit probe failed", zap.String("session_id", candidate.wait.SessionID), zap.Error(err))
			return now.Add(time.Minute)
		}
		if !selected {
			continue
		}
		s.dispatchProviderLimitWait(candidate)
		if marked {
			return now.Add(providerlimit.ProbeLifetime)
		}
	}
	return next
}

// finishSucceededProviderLimitWait completes the first wait whose probe has
// already succeeded; finished reports whether one was found.
func (s *Service) finishSucceededProviderLimitWait(ctx context.Context, candidates []providerLimitWaitCandidate, now time.Time) (time.Time, bool) {
	for _, candidate := range candidates {
		if !candidate.wait.ProbeSucceeded {
			continue
		}
		if err := s.finishSuccessfulProviderLimitWait(ctx, candidate); err != nil {
			s.logger.Warn("finish successful provider limit owner failed", zap.String("session_id", candidate.wait.SessionID), zap.Error(err))
			return now.Add(time.Minute), true
		}
		return now, true
	}
	return time.Time{}, false
}

// redispatchLeasedProviderLimitWaits replays the wait that holds the live probe
// lease but has not yet started its probe turn.
func (s *Service) redispatchLeasedProviderLimitWaits(ctx context.Context, candidates []providerLimitWaitCandidate, mark providerlimit.Mark) {
	for _, candidate := range candidates {
		wait := candidate.wait
		if wait.ProbeLease == nil || wait.ProbeLease.Key != mark.Key || !wait.ProbeLease.Until.Equal(mark.ProbeUntil) || wait.ProbeTurnID != "" {
			continue
		}
		session, err := s.repo.GetTaskSession(ctx, wait.SessionID)
		if err == nil && session != nil && session.State == models.TaskSessionStateWaitingForInput && session.Metadata[providerLimitWaitIdentityKey] == candidate.identity {
			s.dispatchProviderLimitWait(candidate)
		}
	}
}

func sortProviderLimitWaitCandidates(candidates []providerLimitWaitCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].wait.QueuedAt.Equal(candidates[j].wait.QueuedAt) {
			return candidates[i].identity < candidates[j].identity
		}
		return candidates[i].wait.QueuedAt.Before(candidates[j].wait.QueuedAt)
	})
}

// providerLimitWaitReadySession returns the session of a wait that is idle,
// still owned by the wait, and not already running a probe turn.
func (s *Service) providerLimitWaitReadySession(ctx context.Context, candidate providerLimitWaitCandidate) (*models.TaskSession, bool) {
	wait := candidate.wait
	if candidate.task.ArchivedAt != nil || candidate.task.WorkflowStepID != wait.WorkflowStepID || wait.ProbeTurnID != "" {
		return nil, false
	}
	session, err := s.repo.GetTaskSession(ctx, wait.SessionID)
	if err != nil || session == nil || session.Metadata[providerLimitWaitIdentityKey] != candidate.identity {
		return nil, false
	}
	return session, session.State == models.TaskSessionStateWaitingForInput
}

// providerLimitWaitResumable reports whether the session's profile still opts
// into resuming after reset and the mark's reset time is trusted.
func (s *Service) providerLimitWaitResumable(ctx context.Context, session *models.TaskSession, mark providerlimit.Mark, marked bool) bool {
	subject, _, err := s.providerLimitSubjectForSession(ctx, session)
	if err != nil || subject.Dynamic || !subject.Profile.ResumeAfterReset {
		return false
	}
	return !marked || mark.ResetKnown
}

func (s *Service) dispatchProviderLimitWait(candidate providerLimitWaitCandidate) {
	if _, active := s.providerLimitReplays.LoadOrStore(candidate.identity, struct{}{}); active {
		return
	}
	s.startAgentFailureRecovery(func(runCtx context.Context) {
		defer s.providerLimitReplays.Delete(candidate.identity)
		payload := maps.Clone(candidate.wait.Payload)
		payload[providerLimitWaitIdentityKey] = candidate.identity
		s.replayCeilingLaunchPromptEnsure(runCtx, candidate.task, payload)
	})
}

func (s *Service) acquireProviderLimitWaitProbe(ctx context.Context, candidate providerLimitWaitCandidate, mark providerlimit.Mark, marked bool) (bool, error) {
	var token *models.ProviderLimitProbeLease
	if marked {
		lease, acquired, err := s.providerLimits.AcquireProbe(ctx, mark.Key)
		if err != nil || !acquired {
			return false, err
		}
		token = &models.ProviderLimitProbeLease{Key: lease.Key, Until: lease.ExpiresAt}
	}
	return s.updateProviderLimitWait(ctx, candidate.task.ID, candidate.identity, func(wait models.ProviderLimitWait) (models.ProviderLimitWait, bool) {
		if wait.ProbeTurnID != "" || wait.MarkKey != candidate.wait.MarkKey {
			return wait, false
		}
		wait.ProbeLease = token
		return wait, true
	})
}

func (s *Service) updateProviderLimitWait(ctx context.Context, taskID, identity string, mutate func(models.ProviderLimitWait) (models.ProviderLimitWait, bool)) (bool, error) {
	for range deferredLaunchCASRetryBudget {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return false, err
		}
		waits, err := models.ReadProviderLimitWaits(record)
		if err != nil {
			return false, err
		}
		wait, exists := waits[identity]
		if !exists {
			return false, nil
		}
		wait, changed := mutate(wait)
		if !changed {
			return false, nil
		}
		updated, err := models.PutProviderLimitWait(record, wait)
		if err != nil {
			return false, err
		}
		changed, _, err = s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
		if err != nil || changed {
			return changed, err
		}
	}
	return false, errors.New("provider limit wait ownership changed during persistence")
}

func (s *Service) bindProviderLimitProbe(ctx context.Context, taskID, sessionID, identity string) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || session.Metadata[providerLimitWaitIdentityKey] != identity {
		return ErrCeilingLaunchSuperseded
	}
	turnID, err := s.peekActiveTurnID(ctx, sessionID)
	if err != nil || turnID == "" {
		return ErrCeilingLaunchSuperseded
	}
	generation := s.nextPromptGeneration(ctx, sessionID)
	changed, err := s.updateProviderLimitWait(ctx, taskID, identity, func(wait models.ProviderLimitWait) (models.ProviderLimitWait, bool) {
		if wait.SessionID != sessionID || wait.ProbeTurnID != "" || (wait.ProbeLease != nil && !wait.ProbeLease.Until.After(s.providerLimitNow())) {
			return wait, false
		}
		wait.ProbeTurnID, wait.ProbeExecutionID, wait.ProbeGeneration = turnID, session.AgentExecutionID, generation
		return wait, true
	})
	if err != nil {
		return err
	}
	if !changed {
		return ErrCeilingLaunchSuperseded
	}
	return nil
}

func (s *Service) scheduleProviderLimitWake(deadline time.Time) {
	s.providerLimitRecoveryMu.Lock()
	defer s.providerLimitRecoveryMu.Unlock()
	if s.providerLimitRecoveryTimer != nil {
		s.providerLimitRecoveryTimer.Stop()
		s.providerLimitRecoveryTimer = nil
	}
	if deadline.IsZero() || s.providerLimitRecoveryCtx == nil || s.providerLimitRecoveryCtx.Err() != nil {
		return
	}
	clock := s.providerLimitClock
	if clock == nil {
		clock = realProviderLimitClock{}
	}
	delay := deadline.Sub(clock.Now())
	if delay < 0 {
		delay = 0
	}
	ctx := s.providerLimitRecoveryCtx
	s.providerLimitRecoveryTimer = clock.AfterFunc(delay, func() { s.ReconcileProviderLimitWaits(ctx) })
}

func (s *Service) startProviderLimitRecovery(ctx context.Context) {
	s.providerLimitRecoveryMu.Lock()
	s.providerLimitRecoveryCtx, s.providerLimitRecoveryCancel = context.WithCancel(ctx)
	s.providerLimitRecoveryMu.Unlock()
	s.ReconcileProviderLimitWaits(ctx)
}

func (s *Service) stopProviderLimitRecovery() {
	s.providerLimitRecoveryMu.Lock()
	defer s.providerLimitRecoveryMu.Unlock()
	if s.providerLimitRecoveryCancel != nil {
		s.providerLimitRecoveryCancel()
		s.providerLimitRecoveryCancel = nil
	}
	if s.providerLimitRecoveryTimer != nil {
		s.providerLimitRecoveryTimer.Stop()
		s.providerLimitRecoveryTimer = nil
	}
}

func (s *Service) reconcileProviderLimitLaunches(ctx context.Context, tasks []*models.Task) time.Time {
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
		launch, err := models.ReadProviderLimitLaunch(record)
		if err != nil || launch == nil {
			continue
		}
		next = earlierDeadline(next, s.reconcileProviderLimitLaunch(ctx, task, launch, now))
	}
	return next
}

// reconcileProviderLimitLaunch replays one deferred launch once its mark has
// expired. It returns the next time the launch needs attention, or zero.
func (s *Service) reconcileProviderLimitLaunch(ctx context.Context, task *models.Task, launch *models.ProviderLimitLaunch, now time.Time) time.Time {
	if task.ArchivedAt != nil || task.WorkflowStepID != launch.WorkflowStepID {
		s.removeProviderLimitLaunch(ctx, task.ID, launch)
		return time.Time{}
	}
	if mark, marked := s.providerLimits.Get(launch.MarkKey); marked && mark.Until.After(now) {
		return mark.Until
	}
	launch, ready, err := s.acquireProviderLimitLaunchProbe(ctx, task.ID, launch)
	if err != nil || !ready {
		return providerLimitLaunchRetryAt(launch, now)
	}
	replayCtx, ok := s.providerLimitLaunchReplayContext(ctx, task.ID, launch)
	if !ok {
		return launch.ProbeLease.Until
	}
	deferral := models.CeilingDeferral{Kind: launch.Kind, Payload: launch.Payload, Origin: launch.Origin, QueuedAt: launch.QueuedAt}
	switch s.replayCeilingDeferral(replayCtx, task, deferral) {
	case ceilingReplaySucceeded:
		providerlimit.ObserveWait(s.logger.Zap(), providerlimit.MetricContextKanban, providerlimit.MetricWaitResumed)
		s.removeProviderLimitLaunch(ctx, task.ID, launch)
		s.publishTaskUpdatedByID(ctx, task.ID)
	case ceilingReplayStillDeferred, ceilingReplayFailed:
		return providerLimitLaunchRetryAt(launch, now)
	case ceilingReplaySuperseded, ceilingReplayRunClosed:
		s.removeProviderLimitLaunch(ctx, task.ID, launch)
		s.publishTaskUpdatedByID(ctx, task.ID)
	}
	return time.Time{}
}

// providerLimitLaunchRetryAt waits for a live probe lease to expire, otherwise
// retries after one minute.
func providerLimitLaunchRetryAt(launch *models.ProviderLimitLaunch, now time.Time) time.Time {
	if launch.ProbeLease != nil && launch.ProbeLease.Until.After(now) {
		return launch.ProbeLease.Until
	}
	return now.Add(time.Minute)
}

// providerLimitLaunchReplayContext attaches the hook that transfers a probe
// lease to the execution admitting the replayed prompt. ok is false when the
// repository cannot transfer leases.
func (s *Service) providerLimitLaunchReplayContext(ctx context.Context, taskID string, launch *models.ProviderLimitLaunch) (context.Context, bool) {
	if launch.ProbeLease == nil {
		return ctx, true
	}
	transfer, ok := s.repo.(providerLimitProbeLaunchTransfer)
	if !ok {
		return ctx, false
	}
	return agentruntime.WithPromptAdmissionHook(ctx, func(executionID string, generation uint64) error {
		owned, err := transfer.TransferProviderLimitLaunchProbe(ctx, taskID, launch.ID, models.ProviderLimitProbeOwner{
			LaunchID: launch.ID, SessionID: launch.SessionID, WorkflowStepID: launch.WorkflowStepID,
			Model: launch.Model, ExecutionID: executionID, Generation: generation, Lease: *launch.ProbeLease,
		})
		if err != nil {
			return err
		}
		if !owned {
			return ErrCeilingLaunchSuperseded
		}
		return nil
	}), true
}

func (s *Service) acquireProviderLimitLaunchProbe(ctx context.Context, taskID string, launch *models.ProviderLimitLaunch) (*models.ProviderLimitLaunch, bool, error) {
	now := s.providerLimitNow()
	expiredProbe := launch.ProbeLease != nil && !launch.ProbeLease.Until.After(now)
	if launch.ProbeLease != nil && launch.ProbeLease.Until.After(now) {
		return launch, true, nil
	}
	mark, marked := s.providerLimits.Get(launch.MarkKey)
	if !marked {
		return launch, true, nil
	}
	if mark.ProbeUntil.After(now) {
		return launch, false, nil
	}
	lease, acquired, err := s.providerLimits.AcquireProbe(ctx, launch.MarkKey)
	if err != nil || !acquired {
		return launch, false, err
	}
	launch.ProbeLease = &models.ProviderLimitProbeLease{Key: lease.Key, Until: lease.ExpiresAt}
	if err := s.persistProviderLimitLaunchProbe(ctx, taskID, launch); err != nil {
		_, _ = s.providerLimits.ReleaseProbe(ctx, lease, false)
		return launch, false, err
	}
	if expiredProbe {
		providerlimit.ObserveWait(s.logger.Zap(), providerlimit.MetricContextKanban, providerlimit.MetricWaitProbeFailed)
	}
	return launch, true, nil
}

// errProviderLimitLaunchReplaced reports that the deferred launch changed
// before its probe lease could be recorded.
var errProviderLimitLaunchReplaced = errors.New("provider limit launch was replaced before its probe lease was recorded")

// persistProviderLimitLaunchProbe records launch's probe lease while the same
// launch is still deferred for the task.
func (s *Service) persistProviderLimitLaunchProbe(ctx context.Context, taskID string, launch *models.ProviderLimitLaunch) error {
	for range deferredLaunchCASRetryBudget {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return err
		}
		current, err := models.ReadProviderLimitLaunch(record)
		if err != nil {
			return err
		}
		if current == nil || current.ID != launch.ID {
			return errProviderLimitLaunchReplaced
		}
		updated, err := models.PutProviderLimitLaunch(record, *launch)
		if err != nil {
			return err
		}
		stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
		if err != nil {
			return err
		}
		if stored {
			return nil
		}
		if !lostCompare {
			return errors.New("provider limit launch lease was not persisted")
		}
	}
	return errors.New("provider limit launch lease lost repeated compare-and-set races")
}

func (s *Service) removeProviderLimitLaunch(ctx context.Context, taskID string, expected *models.ProviderLimitLaunch) {
	for range deferredLaunchCASRetryBudget {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return
		}
		current, err := models.ReadProviderLimitLaunch(record)
		if err != nil || current == nil || current.ID != expected.ID {
			return
		}
		models.ClearProviderLimitLaunch(record)
		stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, record)
		if err != nil || stored || !lostCompare {
			return
		}
	}
}
