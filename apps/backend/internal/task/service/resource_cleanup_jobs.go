package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/archivecascade"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	taskResourceCleanupRetryDelay       = time.Minute
	preparedCleanupTransitionRetryDelay = 50 * time.Millisecond
	taskResourceCleanupMaxAttempts      = 8
)

const taskResourceCleanupMutationOutcomeUnknown = "task mutation outcome requires reconciliation"

var ErrCleanupCancellationRace = errors.New("cleanup cancellation lost lifecycle race")

type taskResourceCleanupCancellationCAS interface {
	CancelTaskResourceCleanupJobIfPending(ctx context.Context, id string) (bool, error)
}

type taskResourceCleanupArchiveInspector interface {
	ListArchiveTaskResourceCleanupJobs(ctx context.Context, taskID string) ([]*models.TaskResourceCleanupJob, error)
}

var taskResourceCleanupRetryDelays = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
	3 * time.Hour,
	6 * time.Hour,
	12 * time.Hour,
}

type persistedTaskStopTarget struct {
	SessionID   string `json:"session_id"`
	ExecutionID string `json:"execution_id,omitempty"`
	Terminal    bool   `json:"terminal,omitempty"`
}

type taskResourceCleanupSnapshot struct {
	Sessions              []*models.TaskSession     `json:"sessions,omitempty"`
	Worktrees             []*worktree.Worktree      `json:"worktrees,omitempty"`
	WorktreeHeadOIDs      map[string]string         `json:"worktree_head_oids,omitempty"`
	StopTargets           []persistedTaskStopTarget `json:"stop_targets,omitempty"`
	TaskEnvironment       *models.TaskEnvironment   `json:"task_environment,omitempty"`
	DeleteEnvironmentRow  bool                      `json:"delete_environment_row,omitempty"`
	LegacyWorktreeCleanup bool                      `json:"legacy_worktree_cleanup,omitempty"`
	// SSHTaskDirs records the remote task directories this task launched into.
	// Additive and absent-tolerant: a job row written by an older backend
	// decodes with an empty list and reclaims nothing.
	SSHTaskDirs []sshReclaimTarget `json:"ssh_task_dirs,omitempty"`
}

type taskResourceCleanupRun struct {
	job    *models.TaskResourceCleanupJob
	cancel context.CancelFunc
	done   chan struct{}
}

func newTaskResourceCleanupOperationID(trigger models.TaskResourceCleanupTrigger, taskID string) string {
	return string(trigger) + ":" + taskID + ":" + uuid.NewString()
}

func (s *Service) persistTaskResourceCleanup(
	ctx context.Context,
	taskID string,
	trigger models.TaskResourceCleanupTrigger,
	operationID string,
	sessions []*models.TaskSession,
	worktrees []*worktree.Worktree,
	stopTargets []taskStopTarget,
	envCleanup taskEnvironmentCleanup,
	prepared bool,
	collectSSH bool,
) (*models.TaskResourceCleanupJob, error) {
	if s.resourceCleanups == nil {
		return nil, nil
	}
	if operationID == "" {
		operationID = newTaskResourceCleanupOperationID(trigger, taskID)
	}
	worktreeHeadOIDs, err := s.captureWorktreeCleanupHeadOIDs(ctx, worktrees)
	if err != nil {
		return nil, err
	}
	snapshot := taskResourceCleanupSnapshot{
		Sessions: sessions, Worktrees: worktrees, WorktreeHeadOIDs: worktreeHeadOIDs,
		StopTargets:           persistStopTargets(stopTargets),
		TaskEnvironment:       envCleanup.env,
		DeleteEnvironmentRow:  envCleanup.deleteRow,
		LegacyWorktreeCleanup: s.hasLegacyWorktreeCleanup(),
	}
	if collectSSH {
		sshTaskDirs, err := s.gatherSSHReclaimTargets(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("list remote task directories for cleanup snapshot: %w", err)
		}
		snapshot.SSHTaskDirs = sshTaskDirs
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode task resource cleanup snapshot: %w", err)
	}
	state := models.TaskResourceCleanupStatePending
	if prepared {
		state = models.TaskResourceCleanupStatePrepared
	}
	job := &models.TaskResourceCleanupJob{
		OperationID: operationID, TaskID: taskID, Trigger: trigger,
		State: state, ResourceSnapshot: string(encoded),
	}
	if err := s.resourceCleanups.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		return nil, fmt.Errorf("persist task resource cleanup intent: %w", err)
	}
	return s.resourceCleanups.GetTaskResourceCleanupJobByOperationID(ctx, operationID)
}

func (s *Service) captureWorktreeCleanupHeadOIDs(
	ctx context.Context, worktrees []*worktree.Worktree,
) (map[string]string, error) {
	if len(worktrees) == 0 || s.worktreeCleanup == nil {
		return nil, nil
	}
	provider, ok := s.worktreeCleanup.(WorktreeCleanupIdentityProvider)
	if !ok {
		return nil, nil
	}
	identities, err := provider.CaptureCleanupHeadOIDs(ctx, worktrees)
	if err != nil {
		return nil, fmt.Errorf("capture worktree cleanup identities: %w", err)
	}
	return identities, nil
}

func persistStopTargets(targets []taskStopTarget) []persistedTaskStopTarget {
	result := make([]persistedTaskStopTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, persistedTaskStopTarget{
			SessionID: target.sessionID, ExecutionID: target.executionID, Terminal: target.terminal,
		})
	}
	return result
}

func restoreStopTargets(targets []persistedTaskStopTarget) []taskStopTarget {
	result := make([]taskStopTarget, 0, len(targets))
	for _, target := range targets {
		result = append(result, taskStopTarget{
			sessionID: target.SessionID, executionID: target.ExecutionID, terminal: target.Terminal,
		})
	}
	return result
}

func (s *Service) startTaskResourceCleanup(job *models.TaskResourceCleanupJob) {
	if job == nil {
		return
	}
	s.cleanupWorkerMu.Lock()
	wake := s.cleanupWorkerWake
	s.cleanupWorkerMu.Unlock()
	if wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// StartTaskResourceCleanupWorker owns the install-wide durable task cleanup
// loop. StopTaskResourceCleanupWorker joins it during backend shutdown.
func (s *Service) StartTaskResourceCleanupWorker(ctx context.Context) error {
	if s.resourceCleanups == nil {
		return nil
	}
	startupPreparedCutoff := time.Now().UTC()
	s.cleanupWorkerMu.Lock()
	if s.cleanupWorkerCancel != nil {
		s.cleanupWorkerMu.Unlock()
		return nil
	}
	workerCtx, cancel := context.WithCancel(ctx)
	wake := make(chan struct{}, 1)
	s.cleanupWorkerCancel = cancel
	s.cleanupWorkerWake = wake
	s.cleanupWorkerWG.Add(1)
	s.cleanupWorkerMu.Unlock()
	resumeErr := s.resumeTaskResourceCleanupJobs(workerCtx, startupPreparedCutoff)
	go s.runTaskResourceCleanupWorker(workerCtx, wake, resumeErr != nil, startupPreparedCutoff)
	return resumeErr
}

func (s *Service) runTaskResourceCleanupWorker(
	ctx context.Context,
	wake <-chan struct{},
	resumePending bool,
	startupPreparedCutoff time.Time,
) {
	defer s.cleanupWorkerWG.Done()
	ticker := time.NewTicker(taskResourceCleanupRetryDelay)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-wake:
		}
		if resumePending {
			if err := s.resumeTaskResourceCleanupJobs(ctx, startupPreparedCutoff); err != nil {
				if ctx.Err() == nil {
					s.logger.Warn("resume task resource cleanup jobs", zap.Error(err))
				}
				continue
			}
			resumePending = false
			continue
		}
		if err := s.processDueTaskResourceCleanupJobs(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("process due task resource cleanup jobs", zap.Error(err))
		}
	}
}

func (s *Service) StopTaskResourceCleanupWorker() {
	s.cleanupWorkerMu.Lock()
	cancel := s.cleanupWorkerCancel
	s.cleanupWorkerCancel = nil
	s.cleanupWorkerWake = nil
	s.cleanupWorkerMu.Unlock()
	if cancel != nil {
		cancel()
		s.cleanupWorkerWG.Wait()
	}
}

// ResumeTaskResourceCleanupJobs reconstructs interrupted task cleanup after a
// backend restart. It is independent of optional scheduled storage maintenance.
func (s *Service) ResumeTaskResourceCleanupJobs(ctx context.Context) error {
	return s.resumeTaskResourceCleanupJobs(ctx, time.Now().UTC())
}

func (s *Service) resumeTaskResourceCleanupJobs(ctx context.Context, startupPreparedCutoff time.Time) error {
	if s.resourceCleanups == nil {
		return nil
	}
	if err := s.resourceCleanups.ResetRunningTaskResourceCleanupJobs(ctx); err != nil {
		return fmt.Errorf("reset interrupted task cleanup jobs: %w", err)
	}
	if err := s.reconcilePreparedTaskResourceCleanupJobs(ctx, &startupPreparedCutoff); err != nil {
		return err
	}
	return s.processDueTaskResourceCleanupJobs(ctx)
}

func (s *Service) processDueTaskResourceCleanupJobs(ctx context.Context) error {
	reconcileErr := s.reconcilePreparedTaskResourceCleanupJobs(ctx, nil)
	jobs, err := s.resourceCleanups.ListDueTaskResourceCleanupJobs(ctx, time.Now().UTC(), 100)
	if err != nil {
		return errors.Join(reconcileErr, fmt.Errorf("list due task cleanup jobs: %w", err))
	}
	for _, job := range jobs {
		if err := s.processTaskResourceCleanupJob(ctx, job.ID); err != nil {
			s.logger.Warn("resumed task resource cleanup job failed",
				zap.String("job_id", job.ID), zap.String("task_id", job.TaskID), zap.Error(err))
		}
	}
	return reconcileErr
}

func (s *Service) reconcilePreparedTaskResourceCleanupJobs(
	ctx context.Context,
	cancelUncommittedBefore *time.Time,
) error {
	jobs, err := s.resourceCleanups.ListPreparedTaskResourceCleanupJobs(ctx)
	if err != nil {
		return fmt.Errorf("list prepared task cleanup jobs: %w", err)
	}
	var errs []error
	for _, job := range jobs {
		committed, commitErr := s.preparedTaskCleanupMutationCommitted(ctx, job)
		if commitErr != nil {
			errs = append(errs, fmt.Errorf("verify prepared cleanup %s: %w", job.ID, commitErr))
			continue
		}
		if !committed {
			shouldCancel := job.LastError == taskResourceCleanupMutationOutcomeUnknown
			if cancelUncommittedBefore != nil && job.CreatedAt.Before(*cancelUncommittedBefore) {
				shouldCancel = true
			}
			if !shouldCancel {
				continue
			}
			if cas, ok := s.resourceCleanups.(taskResourceCleanupCancellationCAS); ok {
				cancelled, cancelErr := cas.CancelTaskResourceCleanupJobIfPending(ctx, job.ID)
				if cancelErr != nil {
					errs = append(errs, fmt.Errorf("cancel uncommitted prepared cleanup %s: %w", job.ID, cancelErr))
				} else if !cancelled {
					current, reloadErr := s.resourceCleanups.GetTaskResourceCleanupJob(ctx, job.ID)
					if reloadErr != nil {
						errs = append(errs, fmt.Errorf("reload prepared cleanup %s: %w", job.ID, reloadErr))
					} else if current != nil && current.State == models.TaskResourceCleanupStatePrepared {
						errs = append(errs, fmt.Errorf("%w: prepared cleanup %s changed concurrently", ErrCleanupCancellationRace, job.ID))
					}
				}
			} else if err := s.resourceCleanups.CompleteTaskResourceCleanupJob(
				ctx, job.ID, models.TaskResourceCleanupStateCancelled, "", nil,
			); err != nil {
				errs = append(errs, fmt.Errorf("cancel uncommitted prepared cleanup %s: %w", job.ID, err))
			}
			continue
		}
		if err := s.activatePreparedTaskResourceCleanupJob(ctx, job); err != nil {
			errs = append(errs, fmt.Errorf("start committed prepared cleanup %s: %w", job.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) preparedTaskCleanupMutationCommitted(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
) (bool, error) {
	if job == nil || s.tasks == nil {
		return false, errors.New("task repository is unavailable")
	}
	task, err := s.tasks.GetTask(ctx, job.TaskID)
	if err != nil && !errors.Is(err, taskrepo.ErrTaskNotFound) {
		return false, err
	}
	taskExists := err == nil && task != nil
	if taskResourceCleanupDeletesTask(job.Trigger) {
		return !taskExists, nil
	}
	switch job.Trigger {
	case models.TaskResourceCleanupTriggerArchive, models.TaskResourceCleanupTriggerCascadeArchive:
		return taskExists && task.ArchivedAt != nil, nil
	default:
		return false, nil
	}
}
func (s *Service) processTaskResourceCleanupJob(ctx context.Context, id string) error {
	operationCtx, cancel := context.WithDeadline(ctx, archivecascade.ArchiveDeadline(ctx))
	defer cancel()
	candidate, err := s.resourceCleanups.GetTaskResourceCleanupJob(operationCtx, id)
	if err != nil {
		return err
	}
	runCtx, run := s.registerTaskResourceCleanupRun(operationCtx, candidate)
	defer s.finishTaskResourceCleanupRun(run)

	claimed, err := s.resourceCleanups.MarkTaskResourceCleanupJobRunning(operationCtx, id)
	if err != nil || !claimed {
		return err
	}
	job, err := s.resourceCleanups.GetTaskResourceCleanupJob(runCtx, id)
	if err != nil {
		claimedJob := *candidate
		claimedJob.Attempts++
		return s.retryTaskResourceCleanupJob(runCtx, &claimedJob,
			fmt.Errorf("reload claimed cleanup %s: %w", id, err))
	}
	if s.cleanupActivity != nil {
		lease, acquireErr := s.cleanupActivity.AcquireTaskResourceCleanup(runCtx)
		if acquireErr != nil {
			return s.retryTaskResourceCleanupJob(runCtx, job, acquireErr)
		}
		defer lease.Release()
	}
	if cancelled, cancelErr := s.cancelIfTaskUnarchived(runCtx, job); cancelErr != nil || cancelled {
		if cancelErr != nil {
			return s.retryTaskResourceCleanupJob(runCtx, job, cancelErr)
		}
		return nil
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot); err != nil {
		return s.retryTaskResourceCleanupJob(runCtx, job, fmt.Errorf("decode resource snapshot: %w", err))
	}
	// Archive snapshots written by older versions can request environment-row
	// deletion. The lifecycle trigger is authoritative, so normalize that
	// stale flag before any destructive step and persist the corrected snapshot.
	if job.IsArchive() {
		snapshot.DeleteEnvironmentRow = false
	}
	for _, wt := range snapshot.Worktrees {
		if wt != nil && snapshot.WorktreeHeadOIDs != nil {
			wt.CleanupHeadOID = snapshot.WorktreeHeadOIDs[wt.ID]
		}
	}
	defer s.signalCleanupDoneForTest()
	cleanupErr := s.executeTaskResourceCleanupJob(runCtx, job, &snapshot)
	if cleanupErr != nil {
		return s.retryTaskResourceCleanupJob(runCtx, job, cleanupErr)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return s.retryTaskResourceCleanupJob(runCtx, job, fmt.Errorf("encode resource snapshot outcomes: %w", err))
	}
	updated, err := s.resourceCleanups.UpdateClaimedTaskResourceCleanupSnapshot(
		runCtx, job.ID, job.Attempts, string(encoded),
	)
	if err != nil {
		return s.retryTaskResourceCleanupJob(runCtx, job, fmt.Errorf("persist resource snapshot outcomes: %w", err))
	}
	if !updated {
		return nil
	}
	_, err = s.resourceCleanups.CompleteClaimedTaskResourceCleanupJob(
		runCtx, job.ID, job.Attempts, models.TaskResourceCleanupStateSucceeded, "", nil,
	)
	return err
}

func (s *Service) registerTaskResourceCleanupRun(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
) (context.Context, *taskResourceCleanupRun) {
	runCtx, cancel := context.WithCancel(ctx)
	run := &taskResourceCleanupRun{job: job, cancel: cancel, done: make(chan struct{})}
	s.cleanupRunsMu.Lock()
	if s.cleanupRuns == nil {
		s.cleanupRuns = make(map[*taskResourceCleanupRun]struct{})
	}
	s.cleanupRuns[run] = struct{}{}
	s.cleanupRunsMu.Unlock()
	return runCtx, run
}

func (s *Service) finishTaskResourceCleanupRun(run *taskResourceCleanupRun) {
	run.cancel()
	s.cleanupRunsMu.Lock()
	delete(s.cleanupRuns, run)
	close(run.done)
	s.cleanupRunsMu.Unlock()
}

func (s *Service) executeTaskResourceCleanupJob(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
	snapshot *taskResourceCleanupSnapshot,
) error {
	if snapshot == nil {
		return errors.New("resource cleanup snapshot is nil")
	}
	targets, err := s.refreshTaskRuntimeStopTargets(
		ctx,
		job.TaskID,
		restoreStopTargets(snapshot.StopTargets),
	)
	if err != nil {
		return fmt.Errorf("refresh task cleanup runtime inventory: %w", err)
	}
	s.registerTaskRuntimeStopOwners(targets, true)
	stopOutcome := s.stopTaskRuntimeTargetsWithTaskDeleted(
		ctx,
		job.TaskID,
		targets,
		taskResourceCleanupStopReason(job.Trigger),
		"task cleanup runtime stop failed",
		taskResourceCleanupDeletesTask(job.Trigger),
		true,
	)
	failedStops := stopOutcome.failed
	if cancelled, err := s.cancelIfTaskUnarchived(ctx, job); err != nil || cancelled {
		return err
	}
	var errs []error
	if taskResourceCleanupDeletesTask(job.Trigger) && s.attachmentSvc != nil {
		if err := s.attachmentSvc.DeleteByTask(ctx, job.TaskID); err != nil {
			errs = append(errs, fmt.Errorf("delete task attachments: %w", err))
		}
	}
	errs = append(errs, s.performTaskCleanup(ctx, job.TaskID, snapshot.Sessions, snapshot.Worktrees, targets,
		taskEnvironmentCleanup{
			env: snapshot.TaskEnvironment, deleteRow: snapshot.DeleteEnvironmentRow,
			preserveBranches: job.IsArchive(),
		},
		taskCleanupPreserveRows(stopOutcome))...)
	if cause := context.Cause(ctx); cause != nil {
		return errors.Join(append(errs, cause)...)
	}
	if snapshot.LegacyWorktreeCleanup && !job.IsArchive() && len(failedStops) == 0 && s.worktreeCleanup != nil {
		if err := s.worktreeCleanup.OnTaskDeleted(ctx, job.TaskID); err != nil {
			errs = append(errs, fmt.Errorf("legacy worktree cleanup: %w", err))
		}
	}
	if len(failedStops) == 0 {
		errs = append(errs, s.reclaimSSHTaskDirs(ctx, job, snapshot)...)
	}
	if cause := context.Cause(ctx); cause != nil {
		return errors.Join(append(errs, cause)...)
	}
	if len(errs) == 0 && len(failedStops) == 0 {
		return nil
	}
	if len(failedStops) > 0 {
		errs = append(errs, fmt.Errorf("%d runtime stop operations failed", len(failedStops)))
	}
	return errors.Join(errs...)
}

func (s *Service) hasLegacyWorktreeCleanup() bool {
	if s.worktreeCleanup == nil {
		return false
	}
	_, isProvider := s.worktreeCleanup.(WorktreeProvider)
	return !isProvider
}

func taskResourceCleanupDeletesTask(trigger models.TaskResourceCleanupTrigger) bool {
	switch trigger {
	case models.TaskResourceCleanupTriggerDelete,
		models.TaskResourceCleanupTriggerCascadeDelete,
		models.TaskResourceCleanupTriggerWorkspaceDelete,
		models.TaskResourceCleanupTriggerQuickChatExpire:
		return true
	default:
		return false
	}
}

func taskResourceCleanupStopReason(trigger models.TaskResourceCleanupTrigger) string {
	switch trigger {
	case models.TaskResourceCleanupTriggerArchive:
		return "task archived"
	case models.TaskResourceCleanupTriggerCascadeArchive:
		return "cascade archive"
	case models.TaskResourceCleanupTriggerCascadeDelete:
		return "cascade delete"
	default:
		return "task deleted"
	}
}

func (s *Service) cancelIfTaskUnarchived(ctx context.Context, job *models.TaskResourceCleanupJob) (bool, error) {
	if !job.IsArchive() {
		return false, nil
	}
	current, err := s.resourceCleanups.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		return false, err
	}
	if current == nil {
		return false, nil
	}
	if current.State == models.TaskResourceCleanupStateCancelled {
		return true, nil
	}
	task, err := s.tasks.GetTask(ctx, job.TaskID)
	if err != nil && !errors.Is(err, taskrepo.ErrTaskNotFound) {
		return false, err
	}
	if errors.Is(err, taskrepo.ErrTaskNotFound) || task == nil || task.ArchivedAt == nil {
		if current.State == models.TaskResourceCleanupStateRunning {
			return false, ErrCleanupCancellationRace
		}
		cancelled, cancelErr := s.resourceCleanups.CancelTaskResourceCleanupJobIfPending(ctx, job.ID)
		if cancelErr != nil {
			return false, cancelErr
		}
		return cancelled, nil
	}
	return false, nil
}

func (s *Service) resolveTaskResourceCleanupAfterMutationError(ctx context.Context, job *models.TaskResourceCleanupJob) {
	if job == nil || s.resourceCleanups == nil {
		return
	}
	transitionCtx, cancel := detachedCleanupTransitionContext(ctx)
	defer cancel()
	committed, err := s.preparedTaskCleanupMutationCommitted(transitionCtx, job)
	if err != nil {
		if markErr := s.resourceCleanups.CompleteTaskResourceCleanupJob(
			transitionCtx, job.ID, models.TaskResourceCleanupStatePrepared,
			taskResourceCleanupMutationOutcomeUnknown, nil,
		); markErr != nil {
			s.logger.Warn("mark ambiguous task resource cleanup outcome failed",
				zap.String("job_id", job.ID), zap.String("task_id", job.TaskID), zap.Error(markErr))
		}
		s.startTaskResourceCleanup(job)
		s.logger.Warn("task resource cleanup mutation outcome is ambiguous; retaining prepared intent",
			zap.String("job_id", job.ID), zap.String("task_id", job.TaskID), zap.Error(err))
		return
	}
	if committed {
		if err := s.activatePreparedTaskResourceCleanupJob(transitionCtx, job); err != nil {
			s.logger.Warn("activate cleanup after committed task mutation error failed",
				zap.String("job_id", job.ID), zap.String("task_id", job.TaskID), zap.Error(err))
		}
		return
	}
	if err := s.resourceCleanups.CompleteTaskResourceCleanupJob(
		transitionCtx, job.ID, models.TaskResourceCleanupStateCancelled, "", nil,
	); err != nil {
		s.logger.Warn("cancel task resource cleanup job failed",
			zap.String("job_id", job.ID), zap.String("task_id", job.TaskID), zap.Error(err))
	}
}

func (s *Service) retryTaskResourceCleanupJob(ctx context.Context, job *models.TaskResourceCleanupJob, cleanupErr error) error {
	state := models.TaskResourceCleanupStateRetryWait
	var nextAttempt *time.Time
	if !isCascadeCriticalCleanupTrigger(job.Trigger) && job.Attempts >= taskResourceCleanupMaxAttempts {
		state = models.TaskResourceCleanupStateFailed
	} else {
		next := time.Now().UTC().Add(taskResourceCleanupRetryDelayForAttempt(job.Attempts))
		nextAttempt = &next
	}
	transitionCtx, cancel := detachedCleanupTransitionContext(context.WithoutCancel(ctx))
	defer cancel()
	_, err := s.resourceCleanups.CompleteClaimedTaskResourceCleanupJob(
		transitionCtx, job.ID, job.Attempts, state, cleanupErr.Error(), nextAttempt,
	)
	if err != nil {
		return errors.Join(cleanupErr, err)
	}
	return cleanupErr
}

func isCascadeCriticalCleanupTrigger(trigger models.TaskResourceCleanupTrigger) bool {
	return trigger == models.TaskResourceCleanupTriggerCascadeArchive ||
		trigger == models.TaskResourceCleanupTriggerCascadeDelete
}

func taskResourceCleanupRetryDelayForAttempt(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(taskResourceCleanupRetryDelays) {
		attempt = len(taskResourceCleanupRetryDelays)
	}
	return taskResourceCleanupRetryDelays[attempt-1]
}

func detachedCleanupTransitionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(5 * time.Second)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	return context.WithDeadline(context.WithoutCancel(ctx), deadline)
}

// CancelArchiveTaskResourceCleanup cancels retryable archive cleanup before an
// unarchive mutation makes the task active again.
func (s *Service) CancelArchiveTaskResourceCleanup(ctx context.Context, taskID string) error {
	if s.resourceCleanups == nil {
		return nil
	}
	inspector, inspectOK := s.resourceCleanups.(taskResourceCleanupArchiveInspector)
	cas, casOK := s.resourceCleanups.(taskResourceCleanupCancellationCAS)
	if !inspectOK || !casOK {
		return s.resourceCleanups.CancelArchiveTaskResourceCleanupJobs(ctx, taskID)
	}
	return s.cancelInspectedArchiveTaskResourceCleanup(ctx, taskID, inspector, cas)
}

func (s *Service) cancelInspectedArchiveTaskResourceCleanup(
	ctx context.Context,
	taskID string,
	inspector taskResourceCleanupArchiveInspector,
	cas taskResourceCleanupCancellationCAS,
) error {
	jobs, err := inspector.ListArchiveTaskResourceCleanupJobs(ctx, taskID)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job != nil && job.State == models.TaskResourceCleanupStateRunning {
			return fmt.Errorf("%w: task %s cleanup %s is running",
				ErrCleanupCancellationRace, taskID, job.OperationID)
		}
	}
	var errs []error
	for _, job := range jobs {
		if err := s.cancelInspectedArchiveCleanupJob(ctx, taskID, job, cas); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) cancelInspectedArchiveCleanupJob(
	ctx context.Context,
	taskID string,
	job *models.TaskResourceCleanupJob,
	cas taskResourceCleanupCancellationCAS,
) error {
	if job == nil {
		return nil
	}
	cancelled, err := cas.CancelTaskResourceCleanupJobIfPending(ctx, job.ID)
	if err != nil {
		return fmt.Errorf("cancel cleanup %s: %w", job.OperationID, err)
	}
	if cancelled {
		return nil
	}
	current, err := s.resourceCleanups.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		return fmt.Errorf("reload cleanup %s after cancellation race: %w", job.OperationID, err)
	}
	if current != nil &&
		(current.State == models.TaskResourceCleanupStateRunning ||
			current.LastError == taskResourceCleanupMutationOutcomeUnknown) {
		return fmt.Errorf("%w: task %s cleanup %s changed concurrently",
			ErrCleanupCancellationRace, taskID, job.OperationID)
	}
	return nil
}

// PrepareTaskResourceCleanup captures cleanup handles before a cascade mutates
// task rows. StartPreparedTaskResourceCleanup is called only after the matching
// lifecycle mutation commits.
func (s *Service) PrepareTaskResourceCleanup(
	ctx context.Context,
	taskID string,
	trigger models.TaskResourceCleanupTrigger,
	operationID string,
	deleteEnvironmentRow bool,
) error {
	// Reserve the durable lifecycle barrier BEFORE capturing the inventory.
	// Session and worktree creation serialize against the owning task row and
	// reject new ownership while this prepared barrier is active, so the
	// snapshot below cannot miss a resource admitted mid-preparation.
	job, err := s.persistTaskResourceCleanup(ctx, taskID, trigger, operationID,
		nil, nil, nil, taskEnvironmentCleanup{}, true, false)
	if err != nil {
		return err
	}
	if s.resourceCleanups == nil {
		return nil
	}
	barrierOperationID := job.OperationID
	cancelPrepared := func(cause error) error {
		return errors.Join(cause, s.CancelPreparedTaskResourceCleanup(ctx, barrierOperationID))
	}
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return cancelPrepared(fmt.Errorf("list task sessions for cleanup snapshot: %w", err))
	}
	stopTargets, err := s.buildStopTargets(ctx, taskID, sessions)
	if err != nil {
		return cancelPrepared(fmt.Errorf("list runtime cleanup inventory: %w", err))
	}
	worktrees, err := s.gatherWorktreesForDelete(ctx, taskID)
	if err != nil {
		return cancelPrepared(fmt.Errorf("list worktrees for cleanup snapshot: %w", err))
	}
	worktreeHeadOIDs, err := s.captureWorktreeCleanupHeadOIDs(ctx, worktrees)
	if err != nil {
		return cancelPrepared(err)
	}
	taskEnv, err := s.gatherTaskEnvironmentForCleanup(ctx, taskID)
	if err != nil {
		return cancelPrepared(fmt.Errorf("lookup task environment for cleanup: %w", err))
	}
	sshTaskDirs, err := s.gatherSSHReclaimTargets(ctx, taskID)
	if err != nil {
		return cancelPrepared(fmt.Errorf("list remote task directories for cleanup snapshot: %w", err))
	}
	snapshot := taskResourceCleanupSnapshot{
		Sessions: sessions, Worktrees: worktrees, WorktreeHeadOIDs: worktreeHeadOIDs,
		StopTargets:           persistStopTargets(stopTargets),
		TaskEnvironment:       taskEnv,
		DeleteEnvironmentRow:  deleteEnvironmentRow,
		LegacyWorktreeCleanup: s.hasLegacyWorktreeCleanup(),
		SSHTaskDirs:           sshTaskDirs,
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return cancelPrepared(fmt.Errorf("encode task resource cleanup snapshot: %w", err))
	}
	if err := s.resourceCleanups.UpdateTaskResourceCleanupSnapshot(ctx, barrierOperationID, string(encoded)); err != nil {
		return cancelPrepared(fmt.Errorf("persist task resource cleanup snapshot: %w", err))
	}
	return nil
}

func (s *Service) StartPreparedTaskResourceCleanup(ctx context.Context, operationID string) error {
	if s.resourceCleanups == nil {
		return nil
	}
	transitionCtx, cancel := detachedCleanupTransitionContext(ctx)
	defer cancel()
	var lastErr error
	for {
		job, err := s.resourceCleanups.GetTaskResourceCleanupJobByOperationID(transitionCtx, operationID)
		if err == nil {
			if job == nil {
				err = fmt.Errorf("prepared cleanup job %q not found", operationID)
			} else {
				err = s.activatePreparedTaskResourceCleanupJob(transitionCtx, job)
			}
			if err == nil {
				return nil
			}
		}
		lastErr = err
		timer := time.NewTimer(preparedCleanupTransitionRetryDelay)
		select {
		case <-transitionCtx.Done():
			timer.Stop()
			return errors.Join(lastErr, transitionCtx.Err())
		case <-timer.C:
		}
	}
}

func (s *Service) activatePreparedTaskResourceCleanupJob(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
) error {
	started, err := s.resourceCleanups.StartPreparedTaskResourceCleanupJob(ctx, job.ID)
	if err != nil || !started {
		return err
	}
	job.State = models.TaskResourceCleanupStatePending
	s.startTaskResourceCleanup(job)
	return nil
}
func (s *Service) CancelPreparedTaskResourceCleanup(ctx context.Context, operationID string) error {
	if s.resourceCleanups == nil {
		return nil
	}
	cas, ok := s.resourceCleanups.(taskResourceCleanupCancellationCAS)
	if !ok {
		return errors.New("cleanup repository lacks fenced cancellation")
	}
	transitionCtx, cancel := detachedCleanupTransitionContext(context.WithoutCancel(ctx))
	defer cancel()
	job, err := s.resourceCleanups.GetTaskResourceCleanupJobByOperationID(transitionCtx, operationID)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}
	cancelled, err := cas.CancelTaskResourceCleanupJobIfPending(transitionCtx, job.ID)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}
	current, err := s.resourceCleanups.GetTaskResourceCleanupJobByOperationID(transitionCtx, operationID)
	if err != nil {
		return err
	}
	if current == nil ||
		current.State == models.TaskResourceCleanupStateCancelled ||
		current.State == models.TaskResourceCleanupStateSucceeded ||
		current.State == models.TaskResourceCleanupStateFailed {
		return nil
	}
	return fmt.Errorf("%w: operation %s is in state %s", ErrCleanupCancellationRace, operationID, current.State)
}
