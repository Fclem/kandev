package automation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func (s *Store) SetRetryTaskIntentID(ctx context.Context, runID, intentID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET retry_task_intent_id = ? WHERE id = ?`), intentID, runID)
	return err
}
func (s *Store) bindRetryIntentTask(ctx context.Context, runID, taskID, state string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE automation_run_task_intents
		SET task_id = ?, state = ?, updated_at = CURRENT_TIMESTAMP
		WHERE run_id = ? AND (
			EXISTS (
				SELECT 1 FROM automation_runs ar
				JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id
				JOIN automations a ON a.id = ar.automation_id
				WHERE ar.id = ? AND rg.generation = ar.retry_group_generation
					AND rg.state = ? AND a.enabled = TRUE
			) OR EXISTS (
				SELECT 1 FROM automation_runs ar
				WHERE ar.id = ? AND ar.retry_group_id = ''
			)
		)`), taskID, state, runID, runID, RetryGroupLive, runID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		var groupID string
		if lookupErr := s.db.Get(&groupID, `SELECT retry_group_id FROM automation_runs WHERE id = ?`, runID); lookupErr == nil && groupID != "" {
			return ErrRetryGenerationMismatch
		}
	}
	return nil
}

func (s *Service) persistRetryAdmission(ctx context.Context, run *AutomationRun) error {
	if run == nil || run.RetryGroupID == "" {
		return nil
	}
	intent := &RetryTaskIntent{ID: uuid.NewString(), RunID: run.ID, State: retryIntentAdmitted, GroupGeneration: run.RetryGroupGeneration}
	if err := s.store.CreateRetryIntent(ctx, intent); err != nil {
		return err
	}
	if err := s.store.SetRetryTaskIntentID(ctx, run.ID, intent.ID); err != nil {
		return err
	}
	operation := &RetryOperation{IntentID: intent.ID, RunID: run.ID, GroupGeneration: run.RetryGroupGeneration, Kind: retryTaskOperationKind}
	if err := s.store.CreateRetryOperation(ctx, operation); err != nil {
		return err
	}
	payload, err := json.Marshal(&AutomationTriggeredEvent{
		RunID: run.ID, SnapshotVersion: run.RetryLaunchConfigVersion,
		RetryExternalID: RetryTaskExternalID(run.ID, run.RetryGroupGeneration),
	})
	if err != nil {
		return err
	}
	return s.store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID:         fmt.Sprintf("%s:%d", run.ID, run.RetryLaunchConfigVersion),
		RunID:           run.ID,
		SnapshotVersion: run.RetryLaunchConfigVersion,
		PayloadHash:     retryPayloadHash(payload),
		State:           retryOutboxPending,
	})
}
