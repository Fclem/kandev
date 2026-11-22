package sqlite

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const taskTransferApplySavepoint = "task_transfer_apply"

// taskTransferSettled marks an error whose outcome row is already committed by the
// transaction that returned it, so the durable wrapper must not audit it again.
type taskTransferSettled struct {
	err error
}

func (s taskTransferSettled) Error() string { return s.err.Error() }

func (s taskTransferSettled) Unwrap() error { return s.err }

// taskTransferAttemptIdentity is the request identity a failed-key barrier
// row was recorded for.
type taskTransferAttemptIdentity struct {
	TaskID                 string    `db:"task_id"`
	SourceWorkflowID       string    `db:"source_workflow_id"`
	SourceStepID           string    `db:"source_step_id"`
	DestinationWorkspaceID string    `db:"destination_workspace_id"`
	DestinationWorkflowID  string    `db:"destination_workflow_id"`
	DestinationStepID      string    `db:"destination_step_id"`
	TaskGeneration         time.Time `db:"task_generation"`
	PreservationPolicy     string    `db:"preservation_policy"`
}

func (identity taskTransferAttemptIdentity) matches(command models.TaskTransferCommand) bool {
	return identity.TaskID == command.TaskID && identity.SourceWorkflowID == command.ExpectedSourceWorkflowID &&
		identity.SourceStepID == command.ExpectedSourceStepID &&
		identity.DestinationWorkspaceID == command.DestinationWorkspaceID &&
		identity.DestinationWorkflowID == command.DestinationWorkflowID &&
		identity.DestinationStepID == command.DestinationStepID &&
		identity.TaskGeneration.Equal(command.ExpectedTaskUpdatedAt) &&
		identity.PreservationPolicy == command.PreservationPolicy
}

// transferTaskWithDurableRejection runs one reviewed Web transfer under the
// shared transfer serialization. Outcome precedence is: committed receipt,
// then this caller's terminal rejection, then the guarded mutation. A terminal
// domain guard rejection commits exactly one failed attempt row bound to this
// caller and request after rolling back any mutation effects (a source_changed
// found under the workspace-row locks takes no savepoint); that row is the barrier
// that keeps every later same-key attempt from committing. A retryable rejection
// (see taskTransferRejectionTerminal) commits a non-barrier conflict row instead.
func (r *Repository) transferTaskWithDurableRejection(
	ctx context.Context,
	command models.TaskTransferCommand,
	requestDigest string,
) (*models.TaskTransferReceipt, error) {
	receipt, err := r.runDurableTaskTransfer(ctx, command, requestDigest)
	var settled taskTransferSettled
	switch {
	case err == nil:
		return receipt, nil
	case errors.As(err, &settled):
		return nil, settled.err
	default:
		// Not a settled domain rejection: the transaction rolled back or never committed,
		// so no outcome is committed. Audit a non-barrier conflict so the key stays
		// retryable. A failed row here would turn cancellation, lock timeouts (including
		// NOWAIT relation conflicts), and infrastructure errors into a terminal rejection.
		return nil, errors.Join(err, r.RecordTaskTransferAttempt(context.WithoutCancel(ctx), command, models.TaskTransferAuditConflict))
	}
}

// runDurableTaskTransfer returns taskTransferSettled only for errors whose outcome
// row was committed by the transaction that produced them. Every other error leaves
// the transaction rolled back for transferTaskWithDurableRejection to audit once.
func (r *Repository) runDurableTaskTransfer(
	ctx context.Context,
	command models.TaskTransferCommand,
	requestDigest string,
) (*models.TaskTransferReceipt, error) {
	projections, counts, inventory, err := r.inspectTaskTransferRelations(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, r.hierarchyTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Same acquisition order as TransferTask: workspace rows, then the relation census.
	if err := r.lockTaskTransferWorkspaces(ctx, tx, command); err != nil {
		if !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
			return nil, err
		}
		return r.commitTaskTransferRejection(ctx, tx, command, requestDigest, err)
	}
	if err := r.lockTaskTransferRelations(ctx, tx, projections, counts, inventory); err != nil {
		return nil, err
	}
	if receipt, found, err := r.serializedTaskTransferOutcome(ctx, tx, command, requestDigest); err != nil || found {
		return receipt, err
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+taskTransferApplySavepoint); err != nil {
		return nil, err
	}
	receipt, applyErr := r.applyTaskTransfer(ctx, tx, command, requestDigest, projections, counts, inventory)
	if applyErr == nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return receipt, nil
	}
	if !errors.Is(applyErr, repoerrors.ErrTaskTransferConflict) {
		return nil, applyErr
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+taskTransferApplySavepoint); err != nil {
		return nil, err
	}
	return r.commitTaskTransferRejection(ctx, tx, command, requestDigest, applyErr)
}

// serializedTaskTransferOutcome resolves an exact receipt or this caller's
// recorded rejection. found reports that the attempt is already settled; it
// never runs the mutation path.
func (r *Repository) serializedTaskTransferOutcome(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	requestDigest string,
) (*models.TaskTransferReceipt, bool, error) {
	receipt, found, err := r.replayTaskTransfer(ctx, tx, command, requestDigest)
	switch {
	case err != nil && errors.Is(err, repoerrors.ErrTaskTransferConflict):
		return nil, true, r.commitTaskTransferAttemptConflict(ctx, tx, command, err)
	case err != nil:
		return nil, true, err
	case found:
		if err := r.insertTaskTransferAudit(ctx, tx, command, receipt, "idempotent_replay"); err != nil {
			return nil, true, err
		}
		if err := tx.Commit(); err != nil {
			return nil, true, err
		}
		return receipt, true, nil
	}
	if err := r.taskTransferTerminalRejection(ctx, tx, command); err != nil {
		if errors.Is(err, repoerrors.ErrTaskTransferConflict) {
			return nil, true, r.commitTaskTransferAttemptConflict(ctx, tx, command, err)
		}
		return nil, true, err
	}
	return nil, false, nil
}

// taskTransferTerminalRejection inspects this caller's failed-key barrier.
// A barrier recorded for exactly this request is an authoritative terminal
// rejection; a barrier for any other request under the key is an idempotency
// conflict that neither settles nor replaces that other operation.
func (r *Repository) taskTransferTerminalRejection(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
) error {
	var barriers []taskTransferAttemptIdentity
	if err := tx.SelectContext(ctx, &barriers, r.db.Rebind(`
		SELECT task_id, source_workflow_id, source_step_id, destination_workspace_id, destination_workflow_id,
			destination_step_id, task_generation, preservation_policy
		FROM task_transfer_audit
		WHERE source_workspace_id = ? AND idempotency_key = ? AND result IN ('denied', 'failed')
			AND actor_session_id = ? AND (actor_id = ? OR (actor_kind = 'rejected' AND actor_session_id <> ''))`),
		command.ExpectedSourceWorkspaceID, command.IdempotencyKey, command.Actor.SessionID, command.Actor.ID); err != nil {
		return err
	}
	if len(barriers) == 0 {
		return nil
	}
	for _, barrier := range barriers {
		if barrier.matches(command) {
			return repoerrors.NewTaskTransferConflict(repoerrors.TaskTransferOperationRejected,
				"operation was durably rejected")
		}
	}
	return repoerrors.NewTaskTransferConflict(repoerrors.TaskTransferIdempotencyConflict,
		"idempotency key belongs to another rejected request")
}

// TaskTransferPrecheck resolves this caller's exact request before the service
// runs its mutable boundary checks. Replay runs again after the barrier read:
// under READ COMMITTED each statement has its own snapshot, so a receipt that
// commits between the two reads still wins over a barrier read in between. A
// barrier read here never sits beside an attempt that can still commit: a failed
// row commits only while its writer holds the source workspace row lock that
// every same-key attempt takes first, and the relation reservation holds
// task_transfer_audit, so a boundary denied row waits for any admitted attempt.
func (r *Repository) TaskTransferPrecheck(
	ctx context.Context,
	command models.TaskTransferCommand,
) (*models.TaskTransferReceipt, error) {
	requestDigest, err := taskTransferRequestDigest(command)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	receipt, found, err := r.replayTaskTransfer(ctx, tx, command, requestDigest)
	if err != nil {
		return nil, err
	}
	if found {
		return r.commitTaskTransferReplay(ctx, tx, command, receipt)
	}
	rejection := r.taskTransferTerminalRejection(ctx, tx, command)
	receipt, found, err = r.replayTaskTransfer(ctx, tx, command, requestDigest)
	if err != nil {
		return nil, err
	}
	if found {
		return r.commitTaskTransferReplay(ctx, tx, command, receipt)
	}
	return nil, rejection
}

// commitTaskTransferReplay audits a receipt served by precheck and commits the
// transaction, so a replayed attempt is never left without its audit row.
func (r *Repository) commitTaskTransferReplay(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	receipt *models.TaskTransferReceipt,
) (*models.TaskTransferReceipt, error) {
	if err := r.insertTaskTransferAudit(ctx, tx, command, receipt, "idempotent_replay"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return receipt, nil
}

// commitTaskTransferRejection rechecks receipt precedence and commits one failed
// barrier row for a terminal domain guard rejection. A retryable rejection and an
// idempotency conflict commit a non-barrier conflict row instead. The caller has
// rolled back any mutation effects. A storage or commit failure stays an
// uncertain error.
func (r *Repository) commitTaskTransferRejection(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	requestDigest string,
	rejection error,
) (*models.TaskTransferReceipt, error) {
	if receipt, found, err := r.serializedTaskTransferOutcome(ctx, tx, command, requestDigest); err != nil || found {
		return receipt, err
	}
	if reason, _ := repoerrors.TaskTransferConflictReasonOf(rejection); !taskTransferRejectionTerminal(reason) {
		return nil, r.commitTaskTransferAttemptConflict(ctx, tx, command, rejection)
	}
	if err := r.insertTaskTransferAttempt(ctx, tx, command, models.TaskTransferAuditFailed); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return nil, taskTransferSettled{err: rejection}
}

// taskTransferRejectionTerminal reports whether a domain rejection bars every
// later commit of the same request. A cause that can clear without the request
// changing (destination availability, lane capacity, an active cleanup) stays
// retryable under the same key, and an idempotency conflict never replaces another
// request's barrier.
func taskTransferRejectionTerminal(reason repoerrors.TaskTransferConflictReason) bool {
	switch reason {
	case repoerrors.TaskTransferDestinationUnavailable, repoerrors.TaskTransferDestinationFull,
		repoerrors.TaskTransferCleanupActive, repoerrors.TaskTransferIdempotencyConflict:
		return false
	default:
		return true
	}
}

// commitTaskTransferAttemptConflict audits a non-terminal conflict inside the
// serialized transaction. A conflict row is never barrier evidence.
func (r *Repository) commitTaskTransferAttemptConflict(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	conflict error,
) error {
	if err := r.insertTaskTransferAttempt(ctx, tx, command, models.TaskTransferAuditConflict); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return taskTransferSettled{err: conflict}
}
