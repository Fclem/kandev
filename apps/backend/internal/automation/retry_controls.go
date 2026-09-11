package automation

import (
	"context"
	"time"
)

// CancelRetryGroupsByAutomation fences every live retry chain before a
// configuration disable or deletion can race a scheduler claim.
func (s *Store) CancelRetryGroupsByAutomation(ctx context.Context, automationID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_groups SET generation = generation + 1, state = ?, updated_at = ? WHERE automation_id = ? AND state = ?`), RetryGroupCancelled, now, automationID, RetryGroupLive); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET status = ?, retry_state = ?, retry_cancelled_at = ?, retry_claim_token = '', retry_claimed_at = NULL, retry_claim_expires_at = NULL WHERE automation_id = ? AND retry_state NOT IN (?, ?, ?, ?, ?)`), RunStatusFailed, RetryStateCancelled, now, automationID, RetryStateCompleted, RetryStateExhausted, RetryStateCancelled, RetryStateSuperseded, RetryStateSchedulingFailed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_run_task_intents SET state = ?, automation_deleted_at = ? WHERE run_id IN (SELECT id FROM automation_runs WHERE automation_id = ?) AND state != ?`), retryIntentAbandoned, now, automationID, retryIntentAbandoned); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_run_operations SET state = ?, updated_at = ? WHERE run_id IN (SELECT id FROM automation_runs WHERE automation_id = ?) AND state NOT IN (?, ?)`), retryOperationAbandoned, now, automationID, retryOperationCommitted, retryOperationAbandoned)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_outbox SET state = ?, updated_at = ? WHERE run_id IN (SELECT id FROM automation_runs WHERE automation_id = ?) AND state IN (?, ?)`), retryOutboxRevoked, now, automationID, retryOutboxPending, retryOutboxLeased); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) CancelAutomationRetries(ctx context.Context, automationID string) error {
	return s.store.CancelRetryGroupsByAutomation(ctx, automationID)
}
