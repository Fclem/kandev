package messagequeue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/testutil"
)

// newTestPostgresRepo opens an isolated Postgres schema (skipping unless
// KANDEV_TEST_POSTGRES_DSN is set) and constructs the repository over it. The
// queue repository's SQL is driver-agnostic (Rebind'd ? placeholders, no
// SQLite-only constructs), so the same implementation exercises the merge/drain
// ordering under a real shared-database server.
func newTestPostgresRepo(t *testing.T) Repository {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository(postgres): %v", err)
	}
	return repo
}

// TestPostgresRepository_MergeDrainOrdering_DrainWins asserts the drain-wins
// ordering of the merge/drain race: after the source drains, a merge reports
// ErrEntryNotFound and the target is untouched, so no content is lost.
// newTestPostgresRepoPair opens one isolated Postgres schema (skipping unless
// KANDEV_TEST_POSTGRES_DSN is set) and constructs two repository instances over
// it. Two instances simulate two backend processes sharing one queue database:
// their per-session admission locks are process-local, so cross-instance races
// are guarded only by the repository's affected-row checks.
func newTestPostgresRepoPair(t *testing.T) (Repository, Repository) {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// Create the shared schema exactly once; each handle below only selects it
	// via search_path so both repository instances see the same tables.
	setupDB, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if _, err := setupDB.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = setupDB.Close()
		t.Fatalf("create postgres schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_, _ = setupDB.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = setupDB.Close()
	})
	open := func() *sqlx.DB {
		db, err := sqlx.Open("pgx", dsn)
		if err != nil {
			t.Fatalf("open postgres: %v", err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		if _, err := db.Exec("SET search_path TO " + schema); err != nil {
			t.Fatalf("set postgres search_path %s: %v", schema, err)
		}
		return db
	}
	dbA, dbB := open(), open()
	repoA, err := NewSQLiteRepository(dbA, dbA)
	if err != nil {
		t.Fatalf("NewSQLiteRepository(postgres A): %v", err)
	}
	repoB, err := NewSQLiteRepository(dbB, dbB)
	if err != nil {
		t.Fatalf("NewSQLiteRepository(postgres B): %v", err)
	}
	return repoA, repoB
}

// TestPostgresRepository_AutoMergeCandidateIntoAbove_ConcurrentAdmissionCannotLoseContent
// proves the compare-and-swap guard in AutoMergeCandidateIntoAbove: two
// repository instances (two backend processes) that both read the same tail
// must not both fold successfully, silently dropping one accepted message.
//
// The dangerous interleaving is forced deterministically with a row lock.
// Instance A holds a FOR UPDATE lock on the tail while instance B runs the
// fold: B reads the original tail, then its CAS UPDATE blocks on A's lock.
// Only once the poll observes B waiting is B's read snapshot guaranteed to
// predate A's competing fold. A then commits its own fold, B's UPDATE
// re-evaluates its CAS condition against the new row version, affects zero
// rows, and reports a skip — the winner's content survives intact and the
// loser surfaces a failed admission instead of overwriting it.
func TestPostgresRepository_AutoMergeCandidateIntoAbove_ConcurrentAdmissionCannotLoseContent(t *testing.T) {
	repoA, repoB := newTestPostgresRepoPair(t)
	ctx := context.Background()

	tail := insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry("race", "first"))
	dbA := repoA.(*sqliteRepository).db

	// Instance A holds a row lock on the tail, simulating a competing fold
	// that is about to commit.
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `SELECT id FROM queued_messages WHERE id = $1 FOR UPDATE`, tail.ID); err != nil {
		t.Fatalf("lock tail row: %v", err)
	}

	// Instance B starts a fold while A holds the lock: B reads the original
	// tail, then its CAS UPDATE blocks on A's row lock.
	foldDone := make(chan error, 1)
	candidate := defaultAutoMergeEntry("race", "second")
	go func() {
		merged, didMerge, err := repoB.(automaticMergeCandidateRepository).AutoMergeCandidateIntoAbove(ctx, &candidate)
		if err != nil {
			foldDone <- err
			return
		}
		if didMerge || merged != nil {
			foldDone <- fmt.Errorf("concurrent fold succeeded (didMerge=%v merged=%+v), want stale-write skip", didMerge, merged)
			return
		}
		foldDone <- nil
	}()

	// Wait until B's UPDATE is queued on the row lock. Only then is B's read
	// snapshot guaranteed to predate A's competing fold below.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			-- A blocked row-lock waiter is observable as an ungranted
			-- transactionid lock on the blocking transaction; PostgreSQL has
			-- no locktype='row' in pg_locks (tuple/transactionid are the
			-- real ones).
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("instance B fold never blocked on the tail row lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Instance A's competing fold commits first.
	if _, err := lockTx.ExecContext(ctx, `
		UPDATE queued_messages SET content = $1 WHERE id = $2
	`, "first\n\ncompeting", tail.ID); err != nil {
		t.Fatalf("competing fold update: %v", err)
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit competing fold: %v", err)
	}

	if err := <-foldDone; err != nil {
		t.Fatalf("instance B fold: %v", err)
	}

	// The winner's content is intact and the loser's candidate is absent:
	// no silent content loss, and the loser's admission failed loudly.
	entries, err := repoA.ListBySession(ctx, "race")
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != tail.ID {
		t.Fatalf("entries after concurrent admission = %+v, want one surviving tail", entries)
	}
	if entries[0].Content != "first\n\ncompeting" {
		t.Fatalf("tail content = %q, want %q (loser must not overwrite the winner)", entries[0].Content, "first\n\ncompeting")
	}
}

// TestPostgresRepository_AutoMergeCandidateIntoAbove_RaceWithReorderSkips
// proves the position component of the CAS guard: a concurrent reorder that
// only changes positions must invalidate an in-flight fold, otherwise the fold
// lands on a row that is no longer the tail (e.g. it became the head) and
// drains in the wrong order. The interleaving is forced deterministically with
// a row lock, mirroring the concurrent-admission test.
func TestPostgresRepository_AutoMergeCandidateIntoAbove_RaceWithReorderSkips(t *testing.T) {
	repoA, repoB := newTestPostgresRepoPair(t)
	ctx := context.Background()

	first := insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry("race-reorder", "first"))
	second := insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry("race-reorder", "second"))
	dbA := repoA.(*sqliteRepository).db

	// Instance A holds a row lock on the tail (the "second" entry), simulating
	// a concurrent reorder that is about to commit.
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `SELECT id FROM queued_messages WHERE id = $1 FOR UPDATE`, second.ID); err != nil {
		t.Fatalf("lock tail row: %v", err)
	}

	// Instance B starts a fold while A holds the lock: B scans the tail
	// ("second", position 2), then its CAS UPDATE blocks on A's row lock.
	foldDone := make(chan error, 1)
	candidate := defaultAutoMergeEntry("race-reorder", "third")
	go func() {
		merged, didMerge, err := repoB.(automaticMergeCandidateRepository).AutoMergeCandidateIntoAbove(ctx, &candidate)
		if err != nil {
			foldDone <- err
			return
		}
		if didMerge || merged != nil {
			foldDone <- fmt.Errorf("fold survived a concurrent reorder (didMerge=%v merged=%+v), want stale-tail skip", didMerge, merged)
			return
		}
		foldDone <- nil
	}()

	// Wait until B's UPDATE is queued on the row lock, guaranteeing B's scan
	// predates the reorder below.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			-- A blocked row-lock waiter is observable as an ungranted
			-- transactionid lock on the blocking transaction; PostgreSQL has
			-- no locktype='row' in pg_locks (tuple/transactionid are the
			-- real ones).
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("instance B fold never blocked on the tail row lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The concurrent reorder swaps only positions: "second" becomes the head.
	if _, err := lockTx.ExecContext(ctx, `UPDATE queued_messages SET position = 1 WHERE id = $1`, second.ID); err != nil {
		t.Fatalf("reorder second: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `UPDATE queued_messages SET position = 2 WHERE id = $1`, first.ID); err != nil {
		t.Fatalf("reorder first: %v", err)
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit reorder: %v", err)
	}

	if err := <-foldDone; err != nil {
		t.Fatalf("instance B fold: %v", err)
	}

	// The reordered queue is intact and the fold content is absent: the tail
	// is no longer the tail, so the fold must not have landed on it.
	entries, err := repoA.ListBySession(ctx, "race-reorder")
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(entries) != 2 || entries[0].ID != second.ID || entries[1].ID != first.ID {
		t.Fatalf("entries after reorder = %+v, want second first", entries)
	}
	if entries[0].Content != "second" {
		t.Fatalf("head content = %q, want %q (fold must not land on a non-tail row)", entries[0].Content, "second")
	}
}

// TestPostgresRepository_AutoMergeCandidateIntoAbove_RaceWithInsertAbove proves
// the per-session cross-process lock: a fold must never land on a row that
// stopped being the tail. The interleaving is forced deterministically — the
// competing backend holds the queue_session_locks row, the fold blocks on it
// before scanning, and only after the competing drain+insert commits does the
// fold run — so the fold always scans the current tail, never a stale one.
func TestPostgresRepository_AutoMergeCandidateIntoAbove_RaceWithInsertAbove(t *testing.T) {
	repoA, repoB := newTestPostgresRepoPair(t)
	ctx := context.Background()

	first := insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry("race-tail", "first"))
	second := insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry("race-tail", "second"))
	dbA := repoA.(*sqliteRepository).db

	// Instance A holds the per-session cross-process lock, simulating a
	// concurrent drain+insert that is about to commit.
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('race-tail')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'race-tail' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	// Instance B's fold blocks on the session lock before it can scan.
	foldDone := make(chan error, 1)
	candidate := defaultAutoMergeEntry("race-tail", "third")
	go func() {
		merged, didMerge, err := repoB.(automaticMergeCandidateRepository).AutoMergeCandidateIntoAbove(ctx, &candidate)
		if err != nil {
			foldDone <- err
			return
		}
		if !didMerge || merged == nil {
			foldDone <- fmt.Errorf("fold skipped (didMerge=%v merged=%+v), want fold onto the new tail", didMerge, merged)
			return
		}
		if merged.ID == second.ID {
			foldDone <- errors.New("fold landed on the stale tail")
			return
		}
		foldDone <- nil
	}()

	// Wait until B's fold is queued on the session lock row.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			-- A blocked row-lock waiter is observable as an ungranted
			-- transactionid lock on the blocking transaction; PostgreSQL has
			-- no locktype='row' in pg_locks (tuple/transactionid are the
			-- real ones).
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("instance B fold never blocked on the session lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The competing backend drains the head and inserts a new row above the
	// old tail, then commits before the fold can run.
	if _, err := lockTx.ExecContext(ctx, `DELETE FROM queued_messages WHERE id = $1`, first.ID); err != nil {
		t.Fatalf("drain head: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queued_messages
			(id, session_id, task_id, position, content, model, plan_mode,
			 attachments_json, metadata_json, queued_at, queued_by)
		VALUES ($1, 'race-tail', 'task', 3, 'drained-in', 'model', 0, '[]', '{}', now(), 'user')
	`, uuid.NewString()); err != nil {
		t.Fatalf("insert above tail: %v", err)
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit competing drain+insert: %v", err)
	}

	if err := <-foldDone; err != nil {
		t.Fatalf("instance B fold: %v", err)
	}

	// The fold landed on the current tail (the newly inserted row), never on
	// the stale "second" row, and the queue order is intact.
	entries, err := repoA.ListBySession(ctx, "race-tail")
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want two rows", entries)
	}
	if entries[0].ID != second.ID || entries[0].Content != "second" {
		t.Fatalf("stale tail = %s %q, want untouched second entry", entries[0].ID, entries[0].Content)
	}
	if entries[1].Content != "drained-in\n\nthird" {
		t.Fatalf("new tail content = %q, want candidate folded into the current tail", entries[1].Content)
	}
}

// TestPostgresRepository_UpdateContentAndMetadata_RaceWithMerge proves the
// per-session lock also covers content edits: a merge's scan-to-write window
// must not silently overwrite a concurrent UpdateContentAndMetadata. The
// interleaving is forced deterministically — the competing backend holds the
// session lock (simulating a merge mid-transaction), the edit blocks on it,
// the merge commits, and only then does the edit apply on top.
func TestPostgresRepository_UpdateContentAndMetadata_RaceWithMerge(t *testing.T) {
	repoA, repoB := newTestPostgresRepoPair(t)
	ctx := context.Background()

	target := insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry("merge-update", "first"))
	source := insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry("merge-update", "second"))
	dbA := repoA.(*sqliteRepository).db

	// Instance A holds the per-session lock, simulating a merge that has
	// scanned its target/source and is about to write.
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('merge-update')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'merge-update' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	// Instance B's content edit blocks on the session lock until the merge
	// commits.
	updateDone := make(chan error, 1)
	go func() {
		err := repoB.UpdateContentAndMetadata(ctx, "merge-update", target.ID, "edited", nil,
			map[string]interface{}{"edited": true}, "user")
		updateDone <- err
	}()

	// Wait until B's edit is queued on the session lock row.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			-- A blocked row-lock waiter is observable as an ungranted
			-- transactionid lock on the blocking transaction; PostgreSQL has
			-- no locktype='row' in pg_locks (tuple/transactionid are the
			-- real ones).
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("instance B edit never blocked on the session lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The merge commits its stale-snapshot write (fold target + delete
	// source) while the edit is still waiting.
	if _, err := lockTx.ExecContext(ctx, `
		UPDATE queued_messages SET content = $1 WHERE id = $2 AND session_id = 'merge-update'
	`, "first\n\nsecond", target.ID); err != nil {
		t.Fatalf("merge target write: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		DELETE FROM queued_messages WHERE id = $1 AND session_id = 'merge-update'
	`, source.ID); err != nil {
		t.Fatalf("merge source delete: %v", err)
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit merge: %v", err)
	}

	if err := <-updateDone; err != nil {
		t.Fatalf("instance B edit: %v", err)
	}

	// The edit applied after the merge and survives: nothing was silently
	// overwritten by the merge's stale write.
	entries, err := repoA.ListBySession(ctx, "merge-update")
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != target.ID {
		t.Fatalf("entries = %+v, want the merged target only", entries)
	}
	if entries[0].Content != "edited" {
		t.Fatalf("target content = %q, want %q (concurrent edit must survive the merge)", entries[0].Content, "edited")
	}
	if edited := entries[0].Metadata["edited"]; edited != true {
		t.Fatalf("target metadata = %+v, want the edit's metadata applied", entries[0].Metadata)
	}
}

// TestPostgresRepository_TransferSession_RaceWithSourceInsert proves the real
// TransferSession holds the SOURCE session lock too: a source-side admission
// must serialize with the transfer instead of interleaving with its READ
// COMMITTED move. The test drives the actual TransferSession method and forces
// the interleaving deterministically:
//
//   - session ids are chosen so the transfer's stable-sorted acquisition locks
//     the source first and the destination second ("a-source" < "b-dest");
//   - the competing backend holds the DESTINATION lock, so TransferSession
//     acquires the source lock and then blocks on the destination;
//   - a source-side admission is started only after the transfer is blocked,
//     and must itself block on the source lock;
//   - releasing the destination lets the transfer commit, and only then does
//     the admission land on the (now empty) source session.
//
// If TransferSession regressed to destination-only locking, the admission
// would never block on the source and the second barrier would time out.
func TestPostgresRepository_TransferSession_RaceWithSourceInsert(t *testing.T) {
	repoA, repoB := newTestPostgresRepoPair(t)
	ctx := context.Background()

	const (
		oldSession = "a-source"
		newSession = "b-dest"
	)
	insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry(oldSession, "first"))
	dbA := repoA.(*sqliteRepository).db

	// Instance A holds the DESTINATION lock. TransferSession locks the source
	// first (stable sorted order), so it will block here on the destination.
	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('b-dest')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'b-dest' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock destination session: %v", err)
	}

	// The real transfer starts: it acquires the source lock, then blocks on
	// the destination.
	transferDone := make(chan error, 1)
	go func() {
		transferDone <- repoB.TransferSession(ctx, oldSession, newSession)
	}()

	waitForWaitingLocks := func(want int, what string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			var waiting int
			if err := lockTx.QueryRowContext(ctx, `
				-- A blocked row-lock waiter is observable as an ungranted
				-- transactionid lock on the blocking transaction.
				SELECT count(*) FROM pg_locks
				WHERE NOT granted AND locktype = 'transactionid'
			`).Scan(&waiting); err != nil {
				t.Fatalf("query pg_locks: %v", err)
			}
			if waiting >= want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s never reached %d waiting lock(s)", what, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Transfer blocked on the destination (holding the source lock).
	waitForWaitingLocks(1, "transfer")

	// The source-side admission must now block on the source lock held by the
	// transfer.
	insertDone := make(chan error, 1)
	admission := defaultAutoMergeEntry(oldSession, "inserted")
	go func() {
		insertDone <- repoB.Insert(ctx, &admission, 0)
	}()
	waitForWaitingLocks(2, "source admission")

	// Release the destination: the transfer commits its move before the
	// admission proceeds.
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-transferDone; err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if err := <-insertDone; err != nil {
		t.Fatalf("source admission: %v", err)
	}

	// The transfer moved every pre-existing row; the admission that waited
	// landed after it, on the source session it targeted. Nothing is lost or
	// stranded mid-move.
	oldEntries, err := repoA.ListBySession(ctx, oldSession)
	if err != nil {
		t.Fatalf("list old: %v", err)
	}
	if len(oldEntries) != 1 || oldEntries[0].Content != "inserted" {
		t.Fatalf("old entries = %+v, want the post-transfer admission only", oldEntries)
	}
	newEntries, err := repoA.ListBySession(ctx, newSession)
	if err != nil {
		t.Fatalf("list new: %v", err)
	}
	if len(newEntries) != 1 || newEntries[0].Content != "first" {
		t.Fatalf("new entries = %+v, want the transferred row", newEntries)
	}
}

// TestPostgresRepository_AcknowledgeByID_RaceWithRestore proves the ack is
// cross-process serialized on the session lock: an autocommit DELETE could
// otherwise remove a row a concurrent backend just restored/reinserted,
// losing durable retry state. The competing backend holds the session lock,
// the ack blocks on it, and only after release does the ack delete the row.
func TestPostgresRepository_AcknowledgeByID_RaceWithSessionLock(t *testing.T) {
	repoA, repoB := newTestPostgresRepoPair(t)
	ctx := context.Background()
	entry := insertAutoMergeEntry(t, repoA, defaultAutoMergeEntry("ack", "first"))
	dbA := repoA.(*sqliteRepository).db

	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('ack')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'ack' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	ackDone := make(chan error, 1)
	go func() {
		ackDone <- repoB.AcknowledgeByID(ctx, "ack", entry.ID)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ack never blocked on the session lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-ackDone; err != nil {
		t.Fatalf("ack: %v", err)
	}
	entries, err := repoA.ListBySession(ctx, "ack")
	if err != nil {
		t.Fatalf("list after ack: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries after ack = %+v, want removed", entries)
	}
}

// TestPostgresRepository_SetPendingMove_RaceWithTransferLock proves pending
// moves serialize on the session lock so a transfer cannot orphan one.
func TestPostgresRepository_SetPendingMove_RaceWithSessionLock(t *testing.T) {
	repoA, repoB := newTestPostgresRepoPair(t)
	ctx := context.Background()
	dbA := repoA.(*sqliteRepository).db

	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('pm')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'pm' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	setDone := make(chan error, 1)
	go func() {
		setDone <- repoB.SetPendingMove(ctx, "pm", &PendingMove{
			TaskID: "t", WorkflowID: "w", WorkflowStepID: "s", Position: 1, Actor: "test",
		})
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("set pending move never blocked on the session lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-setDone; err != nil {
		t.Fatalf("set pending move: %v", err)
	}
	move, err := repoA.GetPendingMove(ctx, "pm")
	if err != nil || move == nil || move.TaskID != "t" {
		t.Fatalf("pending move = %+v err=%v, want the set move", move, err)
	}
}

// TestPostgresRepository_TakePendingMove_TwoTakersSerialized proves two
// backends cannot both return the same pending move: the second taker blocks
// on the session lock until the first commits, then sees no move.
func TestPostgresRepository_TakePendingMove_TwoTakersSerialized(t *testing.T) {
	repoA, repoB := newTestPostgresRepoPair(t)
	ctx := context.Background()
	if err := repoA.SetPendingMove(ctx, "pm-take", &PendingMove{
		TaskID: "t", WorkflowID: "w", WorkflowStepID: "s", Position: 1, Actor: "test",
	}); err != nil {
		t.Fatalf("seed pending move: %v", err)
	}
	dbA := repoA.(*sqliteRepository).db

	lockTx, err := dbA.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `
		INSERT INTO queue_session_locks (session_id) VALUES ('pm-take')
		ON CONFLICT(session_id) DO NOTHING
	`); err != nil {
		t.Fatalf("ensure session lock row: %v", err)
	}
	if _, err := lockTx.ExecContext(ctx, `
		SELECT 1 FROM queue_session_locks WHERE session_id = 'pm-take' FOR UPDATE
	`); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	takeDone := make(chan error, 1)
	go func() {
		move, err := repoB.TakePendingMove(ctx, "pm-take")
		if err != nil {
			takeDone <- err
			return
		}
		if move == nil || move.TaskID != "t" {
			takeDone <- fmt.Errorf("take returned %+v, want the seeded move", move)
			return
		}
		takeDone <- nil
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := lockTx.QueryRowContext(ctx, `
			SELECT count(*) FROM pg_locks
			WHERE NOT granted AND locktype = 'transactionid'
		`).Scan(&waiting); err != nil {
			t.Fatalf("query pg_locks: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("take pending move never blocked on the session lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := lockTx.Commit(); err != nil {
		t.Fatalf("commit lock tx: %v", err)
	}
	if err := <-takeDone; err != nil {
		t.Fatalf("take pending move: %v", err)
	}
	if move, err := repoA.GetPendingMove(ctx, "pm-take"); err != nil || move != nil {
		t.Fatalf("pending move after take = %+v err=%v, want gone", move, err)
	}
}

func TestPostgresRepository_MergeDrainOrdering_DrainWins(t *testing.T) {
	repo := newTestPostgresRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil, nil)

	if _, err := repo.TakeByID(ctx, "s1", source.ID); err != nil {
		t.Fatalf("drain source: %v", err)
	}
	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user"); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("merge after drain error = %v, want ErrEntryNotFound", err)
	}
	kept, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept entries = %d, want 1", len(kept))
	}
	if kept[0].ID != target.ID || kept[0].Content != "first" {
		t.Errorf("kept entry = %s %q, want target %q", kept[0].ID, kept[0].Content, "first")
	}
}

// TestPostgresRepository_MergeDrainOrdering_MergeWins asserts the merge-wins
// ordering: the drain that follows picks up the merged entry with combined
// content, not the source.
func TestPostgresRepository_MergeDrainOrdering_MergeWins(t *testing.T) {
	repo := newTestPostgresRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil, nil)

	merged, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	drained, err := repo.TakeHead(ctx, "s1")
	if err != nil {
		t.Fatalf("drain after merge: %v", err)
	}
	if drained.ID != target.ID || drained.ID != merged.ID {
		t.Errorf("drained id = %s, want merged target %s", drained.ID, merged.ID)
	}
	if drained.Content != "first\n\nsecond" {
		t.Errorf("drained content = %q, want combined content", drained.Content)
	}
}

// TestPostgresRepository_MergeIntoAbove_ReferenceOverflow asserts the overflow
// rejection is atomic under Postgres too: neither row is touched and the queue
// count stays unchanged.
func TestPostgresRepository_MergeIntoAbove_ReferenceOverflow(t *testing.T) {
	repo := newTestPostgresRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil,
		map[string]interface{}{MetadataEntityReferences: manyEntityRefsFrom(1, entityrefs.MaxReferencesPerMessage)})
	_ = target
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil,
		map[string]interface{}{MetadataEntityReferences: manyEntityRefsFrom(entityrefs.MaxReferencesPerMessage+1, entityrefs.MaxReferencesPerMessage)})

	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user"); !errors.Is(err, ErrMergeReferenceOverflow) {
		t.Fatalf("merge error = %v, want ErrMergeReferenceOverflow", err)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 2 {
		t.Errorf("count after rejected overflow merge = %d, want 2", count)
	}
}

func TestPostgresRepository_CancellationIncludesAllOriginsAndPreservesReservation(t *testing.T) {
	repo := newTestPostgresRepo(t)
	ctx := context.Background()

	reservedEntry := insertDurableLifecycleEntry(t, repo, "s1")
	reserved, err := repo.ReserveHead(ctx, "s1")
	if err != nil || reserved == nil {
		t.Fatalf("reserve lifecycle entry: msg=%+v err=%v", reserved, err)
	}
	for _, queuedBy := range []string{QueuedByUser, QueuedByAgent, QueuedByWorkflow, QueuedByServer} {
		if err := repo.Insert(ctx, &QueuedMessage{
			SessionID: "s1", TaskID: "t1", Content: queuedBy, QueuedBy: queuedBy,
		}, 0); err != nil {
			t.Fatalf("insert %s entry: %v", queuedBy, err)
		}
	}

	removed, err := repo.DeleteAllBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("clear queue: %v", err)
	}
	if removed != 4 {
		t.Fatalf("removed = %d, want 4", removed)
	}
	if err := repo.AcknowledgeByID(ctx, "s1", reservedEntry.ID); err != nil {
		t.Fatalf("reserved entry did not survive clear: %v", err)
	}
}
