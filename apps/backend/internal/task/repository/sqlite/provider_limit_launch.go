package sqlite

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// TransferProviderLimitLaunchProbe commits the first-turn owner before the prompt can reach a provider.
func (r *Repository) TransferProviderLimitLaunchProbe(ctx context.Context, taskID, launchID string, owner models.ProviderLimitProbeOwner) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	taskMetadata, err := r.lockMetadataRow(ctx, tx, "tasks", "task", taskID)
	if err != nil {
		return false, err
	}
	onStep, err := r.taskActiveOnStep(ctx, tx, taskID, owner.WorkflowStepID)
	if err != nil || !onStep {
		return false, err
	}
	record, _, err := decodeDeferredLaunch(taskMetadata)
	if err != nil {
		return false, err
	}
	launch, err := models.ReadProviderLimitLaunch(record)
	if err != nil || !launchOwnsProbe(launch, launchID, owner) {
		return false, err
	}
	owned, err := r.resolveProbeOwnerTurn(ctx, tx, taskID, &owner)
	if err != nil || !owned {
		return false, err
	}
	record, err = models.PutProviderLimitProbeOwner(record, owner)
	if err != nil {
		return false, err
	}
	models.ClearProviderLimitLaunch(record)
	payload, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("tasks", r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), models.MetaKeyDeferredLaunch, string(payload), r.nowUTC(), taskID)...); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// taskActiveOnStep reports whether the unarchived task is still on stepID.
func (r *Repository) taskActiveOnStep(ctx context.Context, tx *sqlx.Tx, taskID, stepID string) (bool, error) {
	var currentStep string
	var archived bool
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT workflow_step_id, archived_at IS NOT NULL FROM tasks WHERE id = ?`), taskID).Scan(&currentStep, &archived); err != nil {
		return false, err
	}
	return !archived && currentStep == stepID, nil
}

// launchOwnsProbe reports whether launch is the identified launch holding the
// same probe lease the owner presents.
func launchOwnsProbe(launch *models.ProviderLimitLaunch, launchID string, owner models.ProviderLimitProbeOwner) bool {
	if launch == nil || launch.ID != launchID || launch.MarkKey != owner.Lease.Key {
		return false
	}
	if launch.SessionID != "" && launch.SessionID != owner.SessionID {
		return false
	}
	return launch.ProbeLease != nil && launch.ProbeLease.Key == owner.Lease.Key && launch.ProbeLease.Until.Equal(owner.Lease.Until)
}

// resolveProbeOwnerTurn fills the owner's session (from its execution when
// absent) and active turn. owned is false when the session belongs to another task.
func (r *Repository) resolveProbeOwnerTurn(ctx context.Context, tx *sqlx.Tx, taskID string, owner *models.ProviderLimitProbeOwner) (bool, error) {
	if owner.SessionID == "" {
		if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT ts.id FROM task_sessions ts JOIN executors_running er ON er.session_id = ts.id WHERE ts.task_id = ? AND er.agent_execution_id = ?`), taskID, owner.ExecutionID).Scan(&owner.SessionID); err != nil {
			return false, err
		}
	}
	var sessionTaskID string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT task_id FROM task_sessions WHERE id = ?`), owner.SessionID).Scan(&sessionTaskID); err != nil {
		return false, err
	}
	if sessionTaskID != taskID {
		return false, nil
	}
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT id FROM task_session_turns WHERE task_session_id = ? AND completed_at IS NULL ORDER BY started_at DESC, created_at DESC, id DESC LIMIT 1`), owner.SessionID).Scan(&owner.TurnID); err != nil {
		return false, err
	}
	return true, nil
}
