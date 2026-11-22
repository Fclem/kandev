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
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 0)
	repo.db.SetMaxOpenConns(8)
	ctx := context.Background()
	_, err := repo.db.Exec(`UPDATE workflow_steps SET prompt = 'different instructions'
		WHERE id = 'pg-race-step-destination'`)
	require.NoError(t, err)
	holder := holdTransferInserts(t, repo.db, "task_transfer_audit", "NEW.result = 'failed'")

	command := postgresRaceTransferCommand(tasks[0], "barrier-hold-durable")
	command.DurableRejection = true
	first := make(chan error, 1)
	go func() { _, transferErr := repo.TransferTask(ctx, command); first <- transferErr }()
	rejectingPID := waitForHeldTransferInsert(t, repo.db, first)

	retry := make(chan error, 1)
	go func() { _, transferErr := repo.TransferTask(ctx, command); retry <- transferErr }()
	waitForBackendBlockedBy(t, repo.db, rejectingPID, retry)

	_, err = holder.ExecContext(ctx, fmt.Sprintf(`SELECT pg_advisory_unlock(%d, %s)`,
		transferBarrierHoldClass, transferBarrierHoldKeySQL))
	require.NoError(t, err)
	requireTransferConflictReason(t, first, repoerrors.TaskTransferStepIncompatible)
	requireTransferConflictReason(t, retry, repoerrors.TaskTransferOperationRejected)

	var barriers int
	require.NoError(t, repo.db.QueryRow(
		`SELECT count(*) FROM task_transfer_audit WHERE idempotency_key = 'barrier-hold-durable' AND result = 'failed'`,
	).Scan(&barriers))
	require.Equal(t, 1, barriers, "the retry must observe the committed barrier, not write a second one")
	assertPostgresTransferSourceUnchanged(t, repo, tasks[0])
}

// A boundary denied row is written outside transfer serialization, but the
// relation reservation holds task_transfer_audit, so the denied insert waits for
// an admitted attempt to finish. Precheck therefore never reads a denied barrier
// beside an attempt that can still commit; once the attempt commits, its receipt
// wins over the denied row.
func TestDeniedAuditWaitsForAdmittedApply(t *testing.T) {
	repo, tasks := seedPostgresConcurrentTransferTasks(t, 0)
	repo.db.SetMaxOpenConns(8)
	ctx := context.Background()
	holder := holdTransferInserts(t, repo.db, "task_transfer_operations", "true")
	command := postgresRaceTransferCommand(tasks[0], "denied-during-apply")
	command.DurableRejection = true

	applying := make(chan error, 1)
	go func() { _, transferErr := repo.TransferTask(ctx, command); applying <- transferErr }()
	applyingPID := waitForHeldTransferInsert(t, repo.db, applying)
	denied := make(chan error, 1)
	go func() { denied <- repo.RecordTaskTransferAttempt(ctx, command, "denied") }()
	waitForBackendBlockedBy(t, repo.db, applyingPID, denied)
	receipt, err := repo.TaskTransferPrecheck(ctx, command)
	require.NoError(t, err, "no denied barrier may be visible while the admitted attempt applies")
	require.Nil(t, receipt)

	_, err = holder.ExecContext(ctx, fmt.Sprintf(`SELECT pg_advisory_unlock(%d, %s)`,
		transferBarrierHoldClass, transferBarrierHoldKeySQL))
	require.NoError(t, err)
	select {
	case transferErr := <-applying:
		require.NoError(t, transferErr)
	case <-time.After(10 * time.Second):
		t.Fatal("admitted transfer did not commit")
	}
	select {
	case auditErr := <-denied:
		// The denied write either lands after the commit or exhausts its bounded wait.
		if auditErr != nil {
			require.ErrorIs(t, auditErr, context.DeadlineExceeded)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("denied audit did not finish")
	}
	receipt, err = repo.TaskTransferPrecheck(ctx, command)
	require.NoError(t, err)
	require.NotNil(t, receipt, "the committed receipt wins over the later denied row")
}

// holdTransferInserts installs a schema-local trigger that makes every insert into
// table matching when wait on this schema's advisory key, then takes that key on a
// pinned connection the caller releases.
func holdTransferInserts(t *testing.T, db *sqlx.DB, table, when string) *sqlx.Conn {
	t.Helper()
	_, err := db.Exec(fmt.Sprintf(`CREATE FUNCTION hold_transfer_insert() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN PERFORM pg_advisory_xact_lock(%d, %s); RETURN NEW; END $$`,
		transferBarrierHoldClass, transferBarrierHoldKeySQL))
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TRIGGER hold_transfer_insert BEFORE INSERT ON ` + table +
		` FOR EACH ROW WHEN (` + when + `) EXECUTE FUNCTION hold_transfer_insert()`)
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

// waitForHeldTransferInsert returns the backend parked in the held insert.
func waitForHeldTransferInsert(t *testing.T, db *sqlx.DB, done <-chan error) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case transferErr := <-done:
			t.Fatalf("transfer finished before its held insert was reached: %v", transferErr)
		default:
		}
		var pids []int
		require.NoError(t, db.Select(&pids, transferBarrierWaiterQuery, transferBarrierHoldClass))
		if len(pids) == 1 {
			return pids[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("transfer never reached its held insert")
	return 0
}

// waitForBackendBlockedBy requires the operation behind done to wait on the
// holding transaction while no second backend reaches the held insert.
func waitForBackendBlockedBy(t *testing.T, db *sqlx.DB, holdingPID int, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case operationErr := <-done:
			t.Fatalf("operation bypassed the holding transaction's locks: %v", operationErr)
		default:
		}
		var blocked, barrierWaiters int
		require.NoError(t, db.Get(&blocked, `SELECT count(*) FROM pg_stat_activity
			WHERE pid <> $1 AND $1 = ANY(pg_blocking_pids(pid))`, holdingPID))
		require.NoError(t, db.Get(&barrierWaiters, `SELECT count(*) FROM (`+transferBarrierWaiterQuery+`) AS waiters`,
			transferBarrierHoldClass))
		require.LessOrEqual(t, barrierWaiters, 1, "a second backend reached the held insert past the holding transaction's locks")
		if blocked > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("operation never waited on the holding transaction")
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
