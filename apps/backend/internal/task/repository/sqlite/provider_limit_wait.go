package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ArmProviderLimitWait commits the task intent, session state and consecutive
// wait counter together. A stale session or workflow entry cannot arm recovery.
func (r *Repository) ArmProviderLimitWait(ctx context.Context, session *models.TaskSession, expected models.TaskSessionState, expectedUpdatedAt time.Time, metadata map[string]interface{}, wait models.ProviderLimitWait) (bool, error) {
	metadataJSON, err := marshalSessionMetadata(metadata)
	if err != nil || expectedUpdatedAt.IsZero() {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	taskMetadata, err := r.lockMetadataRow(ctx, tx, "tasks", "task", session.TaskID)
	if err != nil {
		return false, err
	}
	onStep, err := r.taskActiveOnStep(ctx, tx, session.TaskID, wait.WorkflowStepID)
	if err != nil || !onStep {
		return false, err
	}
	record, _, err := decodeDeferredLaunch(taskMetadata)
	if err != nil {
		return false, err
	}
	record, err = models.PutProviderLimitWait(record, wait)
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	session.UpdatedAt = r.nowUTC()
	changed, err := r.updateTaskSessionWithRevisionGuard(ctx, tx, session, expected, expectedUpdatedAt)
	if err != nil || !changed {
		return false, err
	}
	if err := r.updateSessionMetadataJSON(ctx, tx, session.ID, metadataJSON, session.UpdatedAt); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("tasks", r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), models.MetaKeyDeferredLaunch, string(payload), r.nowUTC(), session.TaskID)...); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) ListTasksWithProviderLimitWaits(ctx context.Context) ([]*models.Task, error) {
	var predicate string
	var args []interface{}
	if dialect.IsPostgres(r.ro.DriverName()) {
		predicate = `jsonb_typeof(jsonb_extract_path(CASE WHEN t.metadata IS NULL OR t.metadata = 'null' OR t.metadata = '' THEN '{}'::jsonb ELSE t.metadata::jsonb END, ?, ?)) = 'object' OR jsonb_typeof(jsonb_extract_path(CASE WHEN t.metadata IS NULL OR t.metadata = 'null' OR t.metadata = '' THEN '{}'::jsonb ELSE t.metadata::jsonb END, ?, ?)) = 'object' OR jsonb_typeof(jsonb_extract_path(CASE WHEN t.metadata IS NULL OR t.metadata = 'null' OR t.metadata = '' THEN '{}'::jsonb ELSE t.metadata::jsonb END, ?, ?)) = 'object'`
		args = []interface{}{models.MetaKeyDeferredLaunch, models.ProviderLimitWaitsKey, models.MetaKeyDeferredLaunch, models.ProviderLimitLaunchKey, models.MetaKeyDeferredLaunch, models.ProviderLimitProbeOwnersKey}
	} else {
		waitPath := jsonPath(models.MetaKeyDeferredLaunch + "." + models.ProviderLimitWaitsKey)
		launchPath := jsonPath(models.MetaKeyDeferredLaunch + "." + models.ProviderLimitLaunchKey)
		ownerPath := jsonPath(models.MetaKeyDeferredLaunch + "." + models.ProviderLimitProbeOwnersKey)
		predicate = `json_type(CASE WHEN t.metadata IS NULL OR t.metadata = 'null' OR t.metadata = '' THEN '{}' ELSE t.metadata END, ?) = 'object' OR json_type(CASE WHEN t.metadata IS NULL OR t.metadata = 'null' OR t.metadata = '' THEN '{}' ELSE t.metadata END, ?) = 'object' OR json_type(CASE WHEN t.metadata IS NULL OR t.metadata = 'null' OR t.metadata = '' THEN '{}' ELSE t.metadata END, ?) = 'object'`
		args = []interface{}{waitPath, launchPath, ownerPath}
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE `+predicate+` ORDER BY t.id ASC`), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanTasks(rows)
}

// RemoveProviderLimitWait releases only the observed token and matching session
// identity. Success resets the consecutive budget in the same transaction.
func (r *Repository) RemoveProviderLimitWait(ctx context.Context, taskID, identity string, token *models.ProviderLimitProbeLease, resetBudget bool) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	taskMetadata, err := r.lockMetadataRow(ctx, tx, "tasks", "task", taskID)
	if err != nil {
		return false, err
	}
	record, _, err := decodeDeferredLaunch(taskMetadata)
	if err != nil {
		return false, err
	}
	waits, err := models.ReadProviderLimitWaits(record)
	if err != nil {
		return false, err
	}
	wait, exists := waits[identity]
	if !exists || !waitHoldsProbeToken(wait, token) {
		return false, nil
	}
	if err := r.releaseSessionProviderLimitWait(ctx, tx, wait.SessionID, identity, resetBudget); err != nil {
		return false, err
	}
	record, err = models.RemoveProviderLimitWait(record, wait.SessionID, wait.TurnID)
	if err != nil {
		return false, err
	}
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

// waitHoldsProbeToken reports whether the wait's probe lease matches token,
// where a nil token matches only a wait without a lease.
func waitHoldsProbeToken(wait models.ProviderLimitWait, token *models.ProviderLimitProbeLease) bool {
	if wait.ProbeLease == nil || token == nil {
		return wait.ProbeLease == nil && token == nil
	}
	return wait.ProbeLease.Key == token.Key && wait.ProbeLease.Until.Equal(token.Until)
}

// releaseSessionProviderLimitWait clears the session's wait identity when it
// still names identity, optionally resetting the consecutive wait budget.
func (r *Repository) releaseSessionProviderLimitWait(ctx context.Context, tx *sqlx.Tx, sessionID, identity string, resetBudget bool) error {
	sessionJSON, err := r.lockMetadataRow(ctx, tx, "task_sessions", "session", sessionID)
	if err != nil {
		return err
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(sessionJSON), &metadata); err != nil {
		return err
	}
	if metadata["provider_limit_wait_identity"] != identity {
		return nil
	}
	delete(metadata, "provider_limit_wait_identity")
	if resetBudget {
		delete(metadata, models.ProviderLimitWaitsKey)
	}
	encoded, err := marshalSessionMetadata(metadata)
	if err != nil {
		return err
	}
	return r.updateSessionMetadataJSON(ctx, tx, sessionID, encoded, r.nowUTC())
}
