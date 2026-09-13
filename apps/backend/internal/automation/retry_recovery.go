package automation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// RecoverRetryLedger restores replayable durable work after an unclean stop.
// Noncommitted operation leases are reclaimed because they belong to the
// stopped process; committed identities and group tombstones remain
// authoritative and are never replaced by a fresh task identity.
func (s *Store) RecoverRetryLedger(ctx context.Context, now time.Time) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_runs
		SET retry_state = ?, retry_claimed_at = NULL, retry_claim_expires_at = NULL, retry_claim_token = ''
		WHERE retry_state = ? AND retry_claim_expires_at IS NOT NULL AND retry_claim_expires_at <= ?`),
		RetryStateScheduled, RetryStateClaimed, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_run_operations
		SET state = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE state = ?`),
		retryOperationRequested, now, retryOperationLeased); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_retry_outbox
		SET state = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE state = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?`),
		retryOutboxPending, now, retryOutboxLeased, now); err != nil {
		return err
	}
	// Admitted intents are already scheduler-visible. Only an unstarted
	// creation intent can be safely returned to the admitted claim state.
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_run_task_intents
		SET state = ?, updated_at = ?
		WHERE state = ? AND task_id IS NULL`),
		retryIntentAdmitted, now, retryIntentCreating); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_run_operations
		SET state = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE state IN (?, ?, ?) AND run_id IN (
			SELECT ar.id FROM automation_runs ar
			LEFT JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
			WHERE rg.id IS NULL OR rg.state != ?
		)`),
		retryOperationAbandoned, now, retryOperationRequested, retryOperationLeased, retryOperationAmbiguous, RetryGroupLive); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE automation_retry_outbox
		SET state = ?, lease_token = '', lease_expires_at = NULL, updated_at = ?
		WHERE state IN (?, ?) AND run_id IN (
			SELECT ar.id FROM automation_runs ar
			LEFT JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
			WHERE rg.id IS NULL OR rg.state != ?
		)`),
		retryOutboxRevoked, now, retryOutboxPending, retryOutboxLeased, RetryGroupLive); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListPendingRetryOutbox(ctx context.Context, now time.Time) ([]RetryOutbox, error) {
	var rows []RetryOutbox
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT o.* FROM automation_retry_outbox o
		LEFT JOIN automation_retry_event_receipts r ON r.event_id = o.event_id
		WHERE r.event_id IS NULL AND o.state IN (?, ?) AND
			(o.lease_expires_at IS NULL OR o.lease_expires_at <= ?)
		ORDER BY o.created_at ASC`), retryOutboxPending, retryOutboxLeased, now)
	return rows, err
}

// RetryRunHasReplayableOutbox reports whether an unbound retry run still has
// durable dispatch work that startup replay is responsible for delivering.
func (s *Store) RetryRunHasReplayableOutbox(ctx context.Context, runID string, generation int64, now time.Time) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`
		SELECT COUNT(*) FROM automation_retry_outbox o
		LEFT JOIN automation_retry_event_receipts r ON r.event_id = o.event_id
		JOIN automation_run_operations op ON op.run_id = o.run_id
			AND op.group_generation = ?
			AND op.operation_kind = ?
		WHERE o.run_id = ? AND r.event_id IS NULL
			AND o.state IN (?, ?)
			AND (o.lease_expires_at IS NULL OR o.lease_expires_at <= ?)
			AND op.state IN (?, ?, ?, ?)`),
		generation, retryTaskOperationKind, runID, retryOutboxPending, retryOutboxLeased, now,
		retryOperationRequested, retryOperationLeased, retryOperationCommitted, retryOperationAmbiguous)
	return count > 0, err
}

// ReplayPendingRetryEvents republishes only unacknowledged, immutable retry
// events. Missing or terminal runs are revoked instead of guessed into work.
//
//nolint:cyclop // Recovery branches explicitly distinguish replayable and terminal rows.
func (s *Service) ReplayPendingRetryEvents(ctx context.Context) error {
	rows, err := s.store.ListPendingRetryOutbox(ctx, time.Now().UTC())
	if s.eventBus == nil {
		return errors.New("retry recovery event bus is unavailable")
	}
	if err != nil {
		return err
	}
	for _, row := range rows {
		run, skip, recoveryErr := s.prepareRetryRecoveryRun(ctx, row)
		if recoveryErr != nil {
			return recoveryErr
		}
		if skip {
			continue
		}
		operation, operationErr := s.store.GetRetryTaskOperation(ctx, run.ID, run.RetryGroupGeneration)
		if operationErr != nil {
			return operationErr
		}
		event := retryRecoveryEvent(run, row.SnapshotVersion, operation.State == retryOperationAmbiguous)
		if err := s.eventBus.Publish(ctx, events.AutomationTriggered,
			bus.NewEvent(events.AutomationTriggered, "automation_retry_recovery", event)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) prepareRetryRecoveryRun(ctx context.Context, row RetryOutbox) (*AutomationRun, bool, error) {
	run, err := s.store.GetRun(ctx, row.RunID)
	if errors.Is(err, sql.ErrNoRows) || run == nil {
		return nil, true, s.store.FailRetryOutbox(ctx, row.EventID, errors.New("retry run is missing"))
	}
	if err != nil {
		return nil, false, err
	}
	if retryRunIsTerminal(run) {
		return nil, true, s.store.FailRetryOutbox(ctx, row.EventID, errors.New("retry run is no longer dispatchable"))
	}
	if run.Status == RunStatusScheduledRetry && run.RetryState == RetryStateScheduled {
		return nil, true, nil
	}
	operation, err := s.store.GetRetryTaskOperation(ctx, run.ID, run.RetryGroupGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, true, s.store.FailRetryOutbox(ctx, row.EventID, errors.New("retry task operation is no longer dispatchable"))
	}
	if err != nil {
		return nil, false, err
	}
	if operation == nil {
		return nil, false, errors.New("retry task operation lookup returned no operation")
	}
	if operation.State == retryOperationAmbiguous {
		if operation.ExternalTaskID == "" || operation.ExternalSessionID == "" || operation.ExternalTurnID == "" {
			return nil, true, s.store.FailRetryOutbox(ctx, row.EventID, errors.New("retry continuation identity is incomplete"))
		}
		return run, false, nil
	}
	if !retryOperationIsDispatchable(operation) {
		return nil, true, s.store.FailRetryOutbox(ctx, row.EventID, ErrRetryOperationUndispatchable)
	}
	return run, false, nil
}

func retryRunIsTerminal(run *AutomationRun) bool {
	if run.RetryGroupID == "" {
		return true
	}
	switch run.RetryState {
	case RetryStateCompleted, RetryStateExhausted, RetryStateCancelled,
		RetryStateSuperseded, RetryStateSchedulingFailed:
		return true
	default:
		return false
	}
}

func retryOperationIsDispatchable(operation *RetryOperation) bool {
	return operation != nil && (operation.State == retryOperationRequested ||
		operation.State == retryOperationLeased || operation.State == retryOperationCommitted)
}

func retryRecoveryEvent(run *AutomationRun, snapshotVersion int64, ambiguous bool) *AutomationTriggeredEvent {
	event := &AutomationTriggeredEvent{
		RunID: run.ID, RetryExternalID: RetryTaskExternalID(run.ID, run.RetryGroupGeneration),
		SnapshotVersion: snapshotVersion, RetryAmbiguousRecovery: ambiguous,
	}
	if run.RetryState == RetryStateClaimed {
		event.AutomationID = run.AutomationID
		event.TriggerID = run.TriggerID
		event.TriggerType = run.TriggerType
		event.RetryClaimToken = run.RetryClaimToken
		event.RetryGroupGeneration = run.RetryGroupGeneration
	}
	return event
}
