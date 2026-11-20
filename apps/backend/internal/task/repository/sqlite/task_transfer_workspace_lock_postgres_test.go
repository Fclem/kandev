package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The durable Web path must take the same workspace-row locks as TransferTask before
// the relation census. A holder row lock on the source workspace (the lock Office
// takes for scope-sensitive writes) must stop the durable transfer from committing,
// and the interruption must leave no terminal barrier for the same key.
func TestDurableTransferWaitsForSourceWorkspaceRowHolder(t *testing.T) {
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 0)
	repo.db.SetMaxOpenConns(4)
	command := postgresRaceTransferCommand(tasks[0], "workspace-holder-durable")
	command.DurableRejection = true

	holder, err := repo.db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.Exec(`SELECT id FROM workspaces WHERE id = ? FOR UPDATE`, command.ExpectedSourceWorkspaceID)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = repo.TransferTask(ctx, command)
	require.Error(t, err, "durable transfer must wait on the source workspace row, not commit past it")

	var barriers int
	require.NoError(t, repo.db.QueryRow(
		`SELECT count(*) FROM task_transfer_audit WHERE idempotency_key = 'workspace-holder-durable' AND result IN ('failed', 'denied')`,
	).Scan(&barriers))
	require.Zero(t, barriers, "an interrupted workspace-lock wait must not write a terminal barrier row")

	require.NoError(t, holder.Rollback())
	receipt, err := repo.TransferTask(context.Background(), command)
	require.NoError(t, err, "same-key retry must succeed after the workspace row is released")
	require.NotEmpty(t, receipt.OperationID)
}
