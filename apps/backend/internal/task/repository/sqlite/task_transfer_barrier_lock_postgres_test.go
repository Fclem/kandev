package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

// transferBarrierHoldClass and the isolated schema name form the advisory key that
// parks failed-barrier inserts, so concurrent runs on a shared cluster never see
// each other's waiters. Every query runs under the test schema's search_path.
const (
	transferBarrierHoldClass   = 75130
	transferBarrierHoldKeySQL  = "hashtext(current_schema())"
	transferBarrierWaiterQuery = `SELECT pid FROM pg_locks
		WHERE locktype = 'advisory' AND NOT granted AND objsubid = 2
			AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			AND classid = $1::int4::oid AND objid = hashtext(current_schema())::oid`
)

// A durable domain rejection must keep its workspace-row locks until its failed
// barrier commits. ROLLBACK TO SAVEPOINT releases locks taken after the savepoint,
// so the savepoint must follow the workspace-row locks, which key every same-key
// barrier lookup: a retry that starts while the barrier insert is in flight waits
// on the rejecting transaction and then observes the committed barrier.
func TestDurableRejectionHoldsSerializationUntilBarrierCommits(t *testing.T) {
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 1)
	repo.db.SetMaxOpenConns(8)
	ctx := context.Background()
	_, err := repo.db.Exec(`UPDATE tasks SET workspace_id = 'pg-race-destination',
		workflow_id = 'pg-race-wf-destination', workflow_step_id = 'pg-race-step-destination',
		wip_admitted = 1, queued_for_step_id = '' WHERE id = $1`, tasks[1].ID)
	require.NoError(t, err)
	holder := holdTransferBarrierInserts(t, repo.db)

	command := postgresRaceTransferCommand(tasks[0], "barrier-hold-durable")
	command.DurableRejection = true
	first := make(chan error, 1)
	go func() { _, transferErr := repo.TransferTask(ctx, command); first <- transferErr }()
	rejectingPID := waitForTransferBarrierWaiter(t, repo.db, first)

	retry := make(chan error, 1)
	go func() { _, transferErr := repo.TransferTask(ctx, command); retry <- transferErr }()
	waitForBackendBlockedBy(t, repo.db, rejectingPID, retry)

	_, err = holder.ExecContext(ctx, fmt.Sprintf(`SELECT pg_advisory_unlock(%d, %s)`,
		transferBarrierHoldClass, transferBarrierHoldKeySQL))
	require.NoError(t, err)
	requireTransferConflictReason(t, first, repoerrors.TaskTransferDestinationFull)
	requireTransferConflictReason(t, retry, repoerrors.TaskTransferOperationRejected)

	var barriers int
	require.NoError(t, repo.db.QueryRow(
		`SELECT count(*) FROM task_transfer_audit WHERE idempotency_key = 'barrier-hold-durable' AND result = 'failed'`,
	).Scan(&barriers))
	require.Equal(t, 1, barriers, "the retry must observe the committed barrier, not write a second one")
	assertPostgresTransferSourceUnchanged(t, repo, tasks[0])
}

// holdTransferBarrierInserts installs a schema-local trigger that makes every
// failed-barrier insert wait on this schema's advisory key, then takes that key on
// a pinned connection the caller releases.
func holdTransferBarrierInserts(t *testing.T, db *sqlx.DB) *sqlx.Conn {
	t.Helper()
	_, err := db.Exec(fmt.Sprintf(`CREATE FUNCTION hold_transfer_barrier() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN PERFORM pg_advisory_xact_lock(%d, %s); RETURN NEW; END $$`,
		transferBarrierHoldClass, transferBarrierHoldKeySQL))
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER hold_transfer_barrier BEFORE INSERT ON task_transfer_audit
		FOR EACH ROW WHEN (NEW.result = 'failed') EXECUTE FUNCTION hold_transfer_barrier()`)
	require.NoError(t, err)
	holder, err := db.Connx(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = holder.ExecContext(context.Background(), `SELECT pg_advisory_unlock_all()`)
		_ = holder.Close()
	})
	var acquired bool
	require.NoError(t, holder.GetContext(context.Background(), &acquired, fmt.Sprintf(
		`SELECT pg_try_advisory_lock(%d, %s)`, transferBarrierHoldClass, transferBarrierHoldKeySQL)))
	require.True(t, acquired, "this schema's barrier hold key is already taken")
	return holder
}

// waitForTransferBarrierWaiter returns the backend parked in the barrier insert.
func waitForTransferBarrierWaiter(t *testing.T, db *sqlx.DB, done <-chan error) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case transferErr := <-done:
			t.Fatalf("rejecting transfer finished before its barrier insert was held: %v", transferErr)
		default:
		}
		var pids []int
		require.NoError(t, db.Select(&pids, transferBarrierWaiterQuery, transferBarrierHoldClass))
		if len(pids) == 1 {
			return pids[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("rejecting transfer never reached its barrier insert")
	return 0
}

// waitForBackendBlockedBy requires another backend to wait on the rejecting
// transaction while no second transfer reaches the barrier insert.
func waitForBackendBlockedBy(t *testing.T, db *sqlx.DB, rejectingPID int, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case transferErr := <-done:
			t.Fatalf("same-key retry bypassed the rejecting transaction's serialization: %v", transferErr)
		default:
		}
		var blocked, barrierWaiters int
		require.NoError(t, db.Get(&blocked, `SELECT count(*) FROM pg_stat_activity
			WHERE pid <> $1 AND $1 = ANY(pg_blocking_pids(pid))`, rejectingPID))
		require.NoError(t, db.Get(&barrierWaiters, `SELECT count(*) FROM (`+transferBarrierWaiterQuery+`) AS waiters`,
			transferBarrierHoldClass))
		require.LessOrEqual(t, barrierWaiters, 1, "same-key retry reached the barrier insert past the rejecting transaction's locks")
		if blocked > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("same-key retry never waited on the rejecting transaction")
}

func requireTransferConflictReason(t *testing.T, done <-chan error, want repoerrors.TaskTransferConflictReason) {
	t.Helper()
	select {
	case transferErr := <-done:
		require.ErrorIs(t, transferErr, repoerrors.ErrTaskTransferConflict)
		reason, ok := repoerrors.TaskTransferConflictReasonOf(transferErr)
		require.True(t, ok, "conflict must carry a reason: %v", transferErr)
		require.Equal(t, want, reason)
	case <-time.After(10 * time.Second):
		t.Fatalf("transfer did not finish; want %s", want)
	}
}
