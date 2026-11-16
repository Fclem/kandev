package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) validateTransferCleanup(ctx context.Context, tx *sqlx.Tx, taskID string, automationCleanupJobs bool) error {
	var cleanupCount int
	if err := tx.GetContext(ctx, &cleanupCount, r.db.Rebind(`
		SELECT COUNT(*) FROM task_resource_cleanup_jobs
		WHERE task_id = ? AND state NOT IN ('completed', 'failed')`), taskID); err != nil {
		return err
	}
	if cleanupCount != 0 {
		return repoerrors.NewTaskTransferConflict(repoerrors.TaskTransferCleanupActive, "incompatible lifecycle mutation is active")
	}
	if automationCleanupJobs {
		if err := tx.GetContext(ctx, &cleanupCount, r.db.Rebind(`
			SELECT COUNT(*) FROM automation_task_cleanup_jobs
			WHERE task_id = ? AND state NOT IN ('completed', 'failed')`), taskID); err != nil {
			return err
		}
		if cleanupCount != 0 {
			return repoerrors.NewTaskTransferConflict(repoerrors.TaskTransferCleanupActive, "incompatible automation cleanup mutation is active")
		}
	}
	return nil
}
