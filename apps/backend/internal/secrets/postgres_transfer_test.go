package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/testutil"
)

// openPGWithConns opens an isolated Postgres schema with a pool of conns
// connections and search_path set per connection (via DSN options), so race
// tests can hold two real connections at once. A one-connection pool trivially
// serializes and proves nothing about the advisory locks.
func openPGWithConns(t *testing.T, conns int) *sqlx.DB {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)

	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	_ = admin.Close()
	t.Cleanup(func() {
		cleanup, cerr := sqlx.Open("pgx", dsn)
		if cerr == nil {
			_, _ = cleanup.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
			_ = cleanup.Close()
		}
	})

	db, err := sqlx.Open("pgx", dsn+" options=-csearch_path="+schema)
	if err != nil {
		t.Fatalf("open postgres pool: %v", err)
	}
	db.SetMaxOpenConns(conns)
	db.SetMaxIdleConns(conns)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newPGStore(t *testing.T, db *sqlx.DB) *sqliteStore {
	t.Helper()
	crypto, err := NewMasterKeyProvider(t.TempDir())
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	store, cleanup, err := Provide(db, db, crypto)
	if err != nil {
		t.Fatalf("provide store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	return store
}

func TestPostgresTransfer_CopyMoveConflictAndOwnership(t *testing.T) {
	db := openPGWithConns(t, 2)
	store := newPGStore(t, db)
	ctx := context.Background()

	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	// Copy global -> workspace with target identity.
	got, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", "copied", nil)
	if err != nil {
		t.Fatalf("CopyScoped: %v", err)
	}
	if got.Scope != ScopeWorkspace || got.WorkspaceID != "workspace-a" || got.Name != "copied" {
		t.Fatalf("copy metadata = %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("copy timestamps zero: %+v", got)
	}
	if value, err := store.RevealForWorkspace(ctx, got.ID, "workspace-a"); err != nil || value != "v" {
		t.Fatalf("reveal = %q, %v", value, err)
	}

	// Conflict on a second copy of the same target name.
	if _, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", "copied", nil); !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("conflict err = %v", err)
	}

	// Move workspace -> global removes the source.
	moved, err := store.MoveScoped(ctx, "g1", "", ScopeGlobal, "", "global-copy", nil)
	if err != nil {
		t.Fatalf("MoveScoped: %v", err)
	}
	if moved.Scope != ScopeGlobal {
		t.Fatalf("moved scope = %q", moved.Scope)
	}
	if _, err := store.Get(ctx, "g1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source still present: %v", err)
	}

	// Legacy empty-scope Global row conflicts with a Global target.
	ciphertext, nonce, err := Encrypt([]byte("legacy"), store.crypto.Key())
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO secrets (id, name, user_id, scope, workspace_id, encrypted_value, nonce, created_at, updated_at)
		VALUES (?, ?, '', '', '', ?, ?, ?, ?)`),
		"legacy-1", "legacy-name", ciphertext, nonce, now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g2", Name: "src2"}, Value: "v2"})
	if _, err := store.CopyScoped(ctx, "g2", "", ScopeGlobal, "", "legacy-name", nil); !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("legacy conflict err = %v", err)
	}
}

func TestPostgresTransfer_MoveRollbackAfterInsert(t *testing.T) {
	db := openPGWithConns(t, 2)
	store := newPGStore(t, db)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	store.failAfterInsert = func() error { return errors.New("injected delete failure") }
	if _, err := store.MoveScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", "copied", nil); err == nil {
		t.Fatal("MoveScoped succeeded despite failpoint")
	}
	if _, err := store.Get(ctx, "g1"); err != nil {
		t.Fatalf("source lost after rollback: %v", err)
	}
	items, err := store.ListScoped(ctx, SecretListOptions{Scope: ScopeWorkspace, WorkspaceID: "workspace-a"})
	if err != nil || len(items) != 0 {
		t.Fatalf("workspace rows = %+v, %v; want none", items, err)
	}
}

// TestPostgresTransfer_ConcurrentSameTarget proves exactly one winner for
// concurrent same-name transfers to a Global target, serialized by the Global
// advisory lock.
func TestPostgresTransfer_ConcurrentSameTarget(t *testing.T) {
	db := openPGWithConns(t, 3)
	storeA := newPGStore(t, db)
	storeB := newPGStore(t, db)
	ctx := context.Background()
	mustCreate(t, storeA, &SecretWithValue{Secret: Secret{ID: "src", Name: "orig"}, Value: "v"})

	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := storeA.CopyScoped(ctx, "src", "", ScopeGlobal, "", "dup", func(context.Context) error {
			close(locked)
			<-release
			return nil
		})
		done <- err
	}()
	<-locked

	bErr := make(chan error, 1)
	go func() {
		_, err := storeB.CopyScoped(ctx, "src", "", ScopeGlobal, "", "dup", nil)
		bErr <- err
	}()
	select {
	case err := <-bErr:
		t.Fatalf("B completed while A held the advisory lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("A: %v", err)
	}
	if err := <-bErr; !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("B error = %v, want ErrSecretNameConflict", err)
	}
}

// TestPostgresTransfer_DeleteVsTransferRace pins both interleavings of the
// shared workspace advisory lock: a transfer that inserts before a deletion
// commits is cleaned up by that deletion, and a deletion that commits first
// leaves nothing for a later transfer to attach to.
func TestPostgresTransfer_DeleteVsTransferRace(t *testing.T) {
	db := openPGWithConns(t, 3)
	store := newPGStore(t, db)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "src", Name: "orig"}, Value: "v"})

	t.Run("transfer locked before deletion cleanup", func(t *testing.T) {
		locked := make(chan struct{})
		release := make(chan struct{})
		transferDone := make(chan error, 1)
		go func() {
			_, err := store.CopyScoped(ctx, "src", "", ScopeWorkspace, "workspace-a", "copied", func(context.Context) error {
				close(locked)
				<-release
				return nil
			})
			transferDone <- err
		}()
		<-locked

		delDone := make(chan error, 1)
		go func() {
			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				delDone <- err
				return
			}
			defer func() { _ = tx.Rollback() }()
			if err := store.DeleteWorkspaceSecretsTx(ctx, tx, "workspace-a"); err != nil {
				delDone <- err
				return
			}
			delDone <- tx.Commit()
		}()
		select {
		case err := <-delDone:
			t.Fatalf("deletion completed while the transfer held the lock: %v", err)
		case <-time.After(300 * time.Millisecond):
		}

		close(release)
		if err := <-transferDone; err != nil {
			t.Fatalf("transfer: %v", err)
		}
		if err := <-delDone; err != nil {
			t.Fatalf("deletion: %v", err)
		}
		// The deletion ran after the transfer inserted and removed the copy:
		// no secret may remain attached to the deleted workspace.
		items, err := store.ListScoped(ctx, SecretListOptions{Scope: ScopeWorkspace, WorkspaceID: "workspace-a"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(items) != 0 {
			t.Fatalf("workspace rows = %+v, want none after deletion", items)
		}
	})

	t.Run("deletion committed before transfer existence check", func(t *testing.T) {
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("begin deletion: %v", err)
		}
		if err := store.DeleteWorkspaceSecretsTx(ctx, tx, "workspace-b"); err != nil {
			_ = tx.Rollback()
			t.Fatalf("delete: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit deletion: %v", err)
		}

		// The transfer's existence check (the callback, run under the lock)
		// denies a workspace that no longer exists; nothing is inserted.
		denied := errors.New("workspace gone")
		_, err = store.CopyScoped(ctx, "src", "", ScopeWorkspace, "workspace-b", "copied", func(context.Context) error {
			return denied
		})
		if !errors.Is(err, denied) {
			t.Fatalf("err = %v, want the denied callback error", err)
		}
		items, err := store.ListScoped(ctx, SecretListOptions{Scope: ScopeWorkspace, WorkspaceID: "workspace-b"})
		if err != nil || len(items) != 0 {
			t.Fatalf("workspace rows = %+v, %v; want none", items, err)
		}
	})
}
