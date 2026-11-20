package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

// A NOWAIT relation-lock conflict is transient: it must surface as a typed busy
// conflict, write no terminal audit barrier, and let the same idempotency key
// commit once the holder releases the relation (AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.6).
func TestTransferRelationsBusyDoesNotPoisonSameKeyRetry(t *testing.T) {
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 0)
	repo.db.SetMaxOpenConns(4)
	holder, err := repo.db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.Exec(`LOCK TABLE tasks IN ROW SHARE MODE`)
	require.NoError(t, err)

	command := postgresRaceTransferCommand(tasks[0], "relations-busy-retry")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = repo.TransferTask(ctx, command)
	require.ErrorIs(t, err, repoerrors.ErrTaskTransferConflict)
	reason, ok := repoerrors.TaskTransferConflictReasonOf(err)
	require.True(t, ok)
	require.Equal(t, repoerrors.TaskTransferRelationsBusy, reason)

	var audited int
	require.NoError(t, repo.db.QueryRow(`SELECT count(*) FROM task_transfer_audit`).Scan(&audited))
	require.Zero(t, audited, "a busy relation lock must not write a terminal barrier row")

	require.NoError(t, holder.Rollback())
	receipt, err := repo.TransferTask(context.Background(), command)
	require.NoError(t, err, "same-key retry must succeed after the relation lock is released")
	require.NotEmpty(t, receipt.OperationID)
}
