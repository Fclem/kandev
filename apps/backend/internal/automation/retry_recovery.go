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
		ORDER BY o.created_at ASC LIMIT 32`), "pending", "leased", now)
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
		run, getErr := s.store.GetRun(ctx, row.RunID)
		if errors.Is(getErr, sql.ErrNoRows) || run == nil {
			if revokeErr := s.store.FailRetryOutbox(ctx, row.EventID, errors.New("retry run is missing")); revokeErr != nil {
				return revokeErr
			}
			continue
		}
		if getErr != nil {
			return getErr
		}
		if run.RetryGroupID == "" || run.RetryState == RetryStateCompleted || run.RetryState == RetryStateExhausted || run.RetryState == RetryStateCancelled || run.RetryState == RetryStateSuperseded {
			if revokeErr := s.store.FailRetryOutbox(ctx, row.EventID, errors.New("retry run is no longer dispatchable")); revokeErr != nil {
				return revokeErr
			}
			continue
		}
		if run.Status == RunStatusScheduledRetry && run.RetryState == RetryStateScheduled {
			continue
		}
		event := &AutomationTriggeredEvent{RunID: run.ID, SnapshotVersion: row.SnapshotVersion}
		if run.RetryState == RetryStateClaimed {
			event.AutomationID = run.AutomationID
			event.TriggerID = run.TriggerID
			event.TriggerType = run.TriggerType
			event.RetryClaimToken = run.RetryClaimToken
			event.RetryGroupGeneration = run.RetryGroupGeneration
		}
		if err := s.eventBus.Publish(ctx, events.AutomationTriggered, bus.NewEvent(events.AutomationTriggered, "automation_retry_recovery", event)); err != nil {
			return err
		}
	}
	return nil
}
