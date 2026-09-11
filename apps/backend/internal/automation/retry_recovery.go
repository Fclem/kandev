package automation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func (s *Store) ListPendingRetryOutbox(ctx context.Context, now time.Time) ([]RetryOutbox, error) {
	var rows []RetryOutbox
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT o.* FROM automation_retry_outbox o
		LEFT JOIN automation_retry_event_receipts r ON r.event_id = o.event_id
		WHERE r.event_id IS NULL AND o.state IN (?, ?) AND
			(o.lease_expires_at IS NULL OR o.lease_expires_at <= ?)
		ORDER BY o.created_at ASC LIMIT 32`), retryOutboxPending, retryOutboxLeased, now)
	return rows, err
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
		event := retryRecoveryEvent(run, row.SnapshotVersion)
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

func retryRecoveryEvent(run *AutomationRun, snapshotVersion int64) *AutomationTriggeredEvent {
	event := &AutomationTriggeredEvent{
		RunID: run.ID, RetryExternalID: RetryTaskExternalID(run.ID, run.RetryGroupGeneration),
		SnapshotVersion: snapshotVersion,
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
