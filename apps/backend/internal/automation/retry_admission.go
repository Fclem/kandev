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
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_run_task_intents SET task_id = ?, state = ?, updated_at = CURRENT_TIMESTAMP WHERE run_id = ?`), taskID, state, runID)
	return err
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
	operation := &RetryOperation{IntentID: intent.ID, RunID: run.ID, GroupGeneration: run.RetryGroupGeneration, Kind: "create_task"}
	if err := s.store.CreateRetryOperation(ctx, operation); err != nil {
		return err
	}
	payload, err := json.Marshal(&AutomationTriggeredEvent{RunID: run.ID, SnapshotVersion: run.RetryLaunchConfigVersion})
	if err != nil {
		return err
	}
	return s.store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID:         fmt.Sprintf("%s:%d", run.ID, run.RetryLaunchConfigVersion),
		RunID:           run.ID,
		SnapshotVersion: run.RetryLaunchConfigVersion,
		PayloadHash:     retryPayloadHash(payload),
		State:           "pending",
	})
}
