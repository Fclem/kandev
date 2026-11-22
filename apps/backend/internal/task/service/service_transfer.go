package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const taskTransferPostCommitTimeout = 10 * time.Second

// Audit results the service records for an attempt that reaches no repository mutation.
const (
	taskTransferAuditDenied   = "denied"
	taskTransferAuditConflict = "conflict"
	taskTransferAuditFailed   = "failed"
)

type taskTransferRepository interface {
	TransferTask(context.Context, models.TaskTransferCommand) (*models.TaskTransferReceipt, error)
}

type taskTransferAttemptRepository interface {
	RecordTaskTransferAttempt(context.Context, models.TaskTransferCommand, string) error
}

type taskTransferReplayRepository interface {
	ReplayTaskTransfer(context.Context, models.TaskTransferCommand) (*models.TaskTransferReceipt, bool, error)
	ResolveTaskTransferReplayActor(context.Context, models.TaskTransferCommand) (models.TaskTransferActor, bool, error)
}

type taskTransferPrecheckRepository interface {
	TaskTransferPrecheck(context.Context, models.TaskTransferCommand) (*models.TaskTransferReceipt, error)
}

// TransferTask preserves one task identity while atomically rebinding its
// workspace, workflow, and semantically equivalent lane. The caller needs
// task.write in the task's current workspace and in the destination, checked
// before replay so a committed receipt is never disclosed without it.
func (s *Service) TransferTask(
	ctx context.Context,
	command models.TaskTransferCommand,
) (*models.TaskTransferReceipt, error) {
	repository, ok := s.tasks.(taskTransferRepository)
	if !ok {
		err := fmt.Errorf("task transfer repository is unavailable")
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, taskTransferAuditFailed))
	}
	if err := s.authorizeTaskScope(ctx, command.TaskID, authz.ScopeTaskWrite); err != nil {
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, taskTransferBoundaryAuditResult(command, err)))
	}
	if err := s.AuthorizeWorkspaceScope(ctx, command.DestinationWorkspaceID, authz.ScopeTaskWrite); err != nil {
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, taskTransferBoundaryAuditResult(command, err)))
	}
	if replayRepository, ok := s.tasks.(taskTransferReplayRepository); ok {
		receipt, found, err := replayRepository.ReplayTaskTransfer(ctx, command)
		if err != nil {
			return nil, err
		}
		if found {
			return receipt, nil
		}
	}
	if precheck, ok := s.tasks.(taskTransferPrecheckRepository); ok && command.DurableRejection {
		receipt, err := precheck.TaskTransferPrecheck(ctx, command)
		if err != nil {
			return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, taskTransferAuditConflict))
		}
		if receipt != nil {
			return receipt, nil
		}
	}
	if err := s.authorizeWorkflowID(ctx, command.DestinationWorkflowID); err != nil {
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, taskTransferBoundaryAuditResult(command, err)))
	}
	authorizedOwnerID, err := s.validateTaskTransferBoundary(ctx, command)
	if err != nil {
		return nil, errors.Join(err, s.AuditTaskTransferAttempt(ctx, command, taskTransferBoundaryAuditResult(command, err)))
	}
	command.AuthorizedOwnerID = authorizedOwnerID
	command.OwnerPredicateSet = true
	receipt, err := repository.TransferTask(ctx, command)
	if err != nil {
		return nil, err
	}
	if !receipt.IdempotentReplay {
		postCommitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), taskTransferPostCommitTimeout)
		defer cancel()
		s.publishTaskTransferUpdate(postCommitCtx, receipt)
		s.pullNextTaskOnVacate(postCommitCtx, receipt.SourceStepID, receipt.TaskID)
	}
	return receipt, nil
}

// taskTransferBoundaryAuditResult classifies a failed authorization or boundary
// check. A durable Web attempt writes the terminal denied barrier only for a
// settled authorization outcome; cancellation, a lookup failure, or a denial
// produced by a failed access lookup is audited as a non-barrier conflict so the
// same key stays retryable.
func taskTransferBoundaryAuditResult(command models.TaskTransferCommand, err error) string {
	if command.DurableRejection && errors.Is(err, errAccessLookupFailed) {
		return taskTransferAuditConflict
	}
	if !command.DurableRejection || IsForbidden(err) || errors.Is(err, repoerrors.ErrTaskNotFound) ||
		errors.Is(err, repoerrors.ErrWorkspaceNotFound) || errors.Is(err, repoerrors.ErrWorkflowNotFound) {
		return taskTransferAuditDenied
	}
	return taskTransferAuditConflict
}

// AuditTaskTransferAttempt records a redacted denied attempt without invoking
// the placement mutation path.
func (s *Service) AuditTaskTransferAttempt(
	ctx context.Context,
	command models.TaskTransferCommand,
	result string,
) error {
	repository, ok := s.tasks.(taskTransferAttemptRepository)
	if !ok {
		return fmt.Errorf("task transfer audit repository is unavailable")
	}
	return repository.RecordTaskTransferAttempt(ctx, command, result)
}

// ResolveTaskTransferReplayActor authorizes only an exact committed replay;
// it cannot create a new operation.
func (s *Service) ResolveTaskTransferReplayActor(
	ctx context.Context,
	command models.TaskTransferCommand,
) (models.TaskTransferActor, bool, error) {
	if err := s.authorizeTaskID(ctx, command.TaskID); err != nil {
		return models.TaskTransferActor{}, false, err
	}
	if err := s.authorizeWorkspaceID(ctx, command.DestinationWorkspaceID); err != nil {
		return models.TaskTransferActor{}, false, err
	}
	repository, ok := s.tasks.(taskTransferReplayRepository)
	if !ok {
		return models.TaskTransferActor{}, false, fmt.Errorf("task transfer replay repository is unavailable")
	}
	return repository.ResolveTaskTransferReplayActor(ctx, command)
}

func (s *Service) validateTaskTransferBoundary(
	ctx context.Context,
	command models.TaskTransferCommand,
) (string, error) {
	source, err := s.workspaces.GetWorkspace(ctx, command.ExpectedSourceWorkspaceID)
	if err != nil {
		return "", err
	}
	destination, err := s.workspaces.GetWorkspace(ctx, command.DestinationWorkspaceID)
	if err != nil {
		return "", err
	}
	if source.OwnerID != destination.OwnerID {
		return "", repoerrors.ErrWorkspaceNotFound
	}
	workflow, err := s.workflows.GetWorkflow(ctx, command.DestinationWorkflowID)
	if err != nil {
		return "", err
	}
	if workflow.WorkspaceID != destination.ID {
		return "", repoerrors.ErrWorkspaceNotFound
	}
	return source.OwnerID, nil
}

func (s *Service) publishTaskTransferUpdate(ctx context.Context, receipt *models.TaskTransferReceipt) {
	task, err := s.tasks.GetTask(ctx, receipt.TaskID)
	if err != nil {
		s.logger.Error("load transferred task for event")
		return
	}
	extra := map[string]interface{}{
		"transfer_operation_id": receipt.OperationID,
		"source_workspace_id":   receipt.SourceWorkspaceID,
		"source_workflow_id":    receipt.SourceWorkflowID,
		"source_step_id":        receipt.SourceStepID,
		"preservation_digest":   receipt.PreservationDigest,
	}
	s.publishTaskEventWithExtra(ctx, events.TaskUpdated, task, nil, extra, receipt.SourceWorkflowID)
}
