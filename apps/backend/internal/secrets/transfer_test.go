package secrets

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
)

func testTransferContext(userID string) context.Context {
	if userID == "" {
		return context.Background()
	}
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID})
}

func mustCreate(t *testing.T, store SecretStore, secret *SecretWithValue) {
	t.Helper()
	if err := store.Create(context.Background(), secret); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
}

func TestTransferStore_CopyGlobalToWorkspace(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v1"})

	got, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", "copied", nil)
	if err != nil {
		t.Fatalf("CopyScoped: %v", err)
	}
	if got.Scope != ScopeWorkspace || got.WorkspaceID != "workspace-a" || got.Name != "copied" {
		t.Fatalf("copy metadata = %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("copy timestamps are zero: %+v", got)
	}
	// Source intact.
	if _, err := store.Get(ctx, "g1"); err != nil {
		t.Fatalf("source missing after copy: %v", err)
	}
	// Value carried server-side, verifiable only through reveal.
	value, err := store.RevealForWorkspace(ctx, got.ID, "workspace-a")
	if err != nil {
		t.Fatalf("reveal copy: %v", err)
	}
	if value != "v1" {
		t.Fatalf("copy value = %q, want v1", value)
	}
}

func TestTransferStore_CopyWorkspaceToGlobal(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v2"})

	got, err := store.CopyScoped(ctx, "w1", "workspace-a", ScopeGlobal, "", "global-copy", nil)
	if err != nil {
		t.Fatalf("CopyScoped: %v", err)
	}
	if got.Scope != ScopeGlobal || got.WorkspaceID != "" || got.Name != "global-copy" {
		t.Fatalf("copy metadata = %+v", got)
	}
	value, err := store.RevealGlobal(ctx, got.ID)
	if err != nil {
		t.Fatalf("reveal global copy: %v", err)
	}
	if value != "v2" {
		t.Fatalf("copy value = %q, want v2", value)
	}
	if _, err := store.Get(ctx, "w1"); err != nil {
		t.Fatalf("source missing after copy: %v", err)
	}
}

func TestTransferStore_CopyWorkspaceToWorkspace(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v3"})

	got, err := store.CopyScoped(ctx, "w1", "workspace-a", ScopeWorkspace, "workspace-b", "moved", nil)
	if err != nil {
		t.Fatalf("CopyScoped: %v", err)
	}
	if got.WorkspaceID != "workspace-b" {
		t.Fatalf("copy workspace = %q, want workspace-b", got.WorkspaceID)
	}
	value, err := store.RevealForWorkspace(ctx, got.ID, "workspace-b")
	if err != nil {
		t.Fatalf("reveal copy: %v", err)
	}
	if value != "v3" {
		t.Fatalf("copy value = %q, want v3", value)
	}
}

func TestTransferStore_MoveWorkspaceToGlobalRemovesSource(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v4"})

	got, err := store.MoveScoped(ctx, "w1", "workspace-a", ScopeGlobal, "", "global-copy", nil)
	if err != nil {
		t.Fatalf("MoveScoped: %v", err)
	}
	if got.Scope != ScopeGlobal {
		t.Fatalf("moved metadata scope = %q", got.Scope)
	}
	if _, err := store.Get(ctx, "w1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source still present after move: %v", err)
	}
	value, err := store.RevealGlobal(ctx, got.ID)
	if err != nil {
		t.Fatalf("reveal moved copy: %v", err)
	}
	if value != "v4" {
		t.Fatalf("moved value = %q, want v4", value)
	}
}

func TestTransferStore_MoveGlobalToWorkspaceRemovesSource(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v5"})

	got, err := store.MoveScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", "copied", nil)
	if err != nil {
		t.Fatalf("MoveScoped: %v", err)
	}
	if got.WorkspaceID != "workspace-a" {
		t.Fatalf("moved workspace = %q", got.WorkspaceID)
	}
	if _, err := store.Get(ctx, "g1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source still present after move: %v", err)
	}
}

func TestTransferStore_ConflictGlobal(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g2", Name: "taken"}, Value: "other"})

	_, err := store.CopyScoped(ctx, "g1", "", ScopeGlobal, "", "taken", nil)
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("error = %v, want ErrSecretNameConflict", err)
	}
}

func TestTransferStore_ConflictWorkspace(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "taken", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "other"})

	_, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", "taken", nil)
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("error = %v, want ErrSecretNameConflict", err)
	}
}

func TestTransferStore_ConflictLegacyEmptyScopeGlobal(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	// Seed a legacy row whose stored scope is empty (pre-scope migration state).
	ciphertext, nonce, err := Encrypt([]byte("legacy"), store.crypto.Key())
	if err != nil {
		t.Fatalf("encrypt legacy: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.db.ExecContext(ctx, store.db.Rebind(`
		INSERT INTO secrets (id, name, user_id, scope, workspace_id, encrypted_value, nonce, created_at, updated_at)
		VALUES (?, ?, '', '', '', ?, ?, ?, ?)`),
		"legacy-1", "taken", ciphertext, nonce, now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	_, err = store.CopyScoped(ctx, "g1", "", ScopeGlobal, "", "taken", nil)
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("error = %v, want ErrSecretNameConflict (legacy empty scope is Global)", err)
	}
}

func TestTransferStore_SourceMissing(t *testing.T) {
	store := newTestSQLiteStore(t)
	_, err := store.CopyScoped(context.Background(), "nope", "", ScopeWorkspace, "workspace-a", "x", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestTransferStore_VerifyDestinationErrorRollsBack(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	denied := errors.New("destination denied")
	_, err := store.CopyScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", "copied", func(context.Context) error {
		return denied
	})
	if !errors.Is(err, denied) {
		t.Fatalf("error = %v, want the callback error", err)
	}
	items, err := store.ListScoped(ctx, SecretListOptions{Scope: ScopeWorkspace, WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("destination rows = %d, want 0 after callback rollback", len(items))
	}
	if _, err := store.Get(ctx, "g1"); err != nil {
		t.Fatalf("source lost after callback rollback: %v", err)
	}
}

func TestTransferStore_MoveFailpointAfterInsertRollsBack(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "g1", Name: "orig"}, Value: "v"})

	store.failAfterInsert = func() error { return errors.New("injected delete failure") }
	_, err := store.MoveScoped(ctx, "g1", "", ScopeWorkspace, "workspace-a", "copied", nil)
	if err == nil {
		t.Fatal("MoveScoped succeeded despite injected failpoint")
	}
	// Source intact, no destination row: Move is transactional, not copy-then-commit-delete.
	if _, err := store.Get(ctx, "g1"); err != nil {
		t.Fatalf("source lost after failpoint rollback: %v", err)
	}
	items, err := store.ListScoped(ctx, SecretListOptions{Scope: ScopeWorkspace, WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("destination rows = %d, want 0 after failpoint rollback", len(items))
	}
}

func TestTransferStore_PerUserScoping(t *testing.T) {
	store := newTestSQLiteStore(t)
	alice := testTransferContext("user-a")
	bob := testTransferContext("user-b")

	// Alice owns a workspace secret; Bob must not see or move it.
	mustCreate(t, store, &SecretWithValue{Secret: Secret{ID: "w1", Name: "alice-ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v"})
	// The seed above ran unscoped (background ctx), so claim it for Alice by
	// re-creating under her identity: delete the unowned row first.
	if err := store.Delete(context.Background(), "w1"); err != nil {
		t.Fatalf("cleanup seed: %v", err)
	}
	if err := store.Create(alice, &SecretWithValue{Secret: Secret{ID: "w1", Name: "alice-ws", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"}, Value: "v"}); err != nil {
		t.Fatalf("seed as alice: %v", err)
	}

	if _, err := store.CopyScoped(bob, "w1", "workspace-a", ScopeGlobal, "", "x", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob copying alice's secret: err = %v, want ErrNotFound", err)
	}

	// Alice can move her own secret to Global.
	got, err := store.MoveScoped(alice, "w1", "workspace-a", ScopeGlobal, "", "alice-global", nil)
	if err != nil {
		t.Fatalf("alice move: %v", err)
	}
	// The moved copy is owned by Alice and invisible to Bob.
	if _, err := store.Get(bob, got.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob reading alice's copy: err = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(alice, got.ID); err != nil {
		t.Fatalf("alice reading her copy: %v", err)
	}
}

func TestTransferStore_UserVisibleStoreRejectsInternalSource(t *testing.T) {
	raw := newTestSQLiteStore(t)
	// Seed an internal backend-owned row through the RAW store (the wrapper
	// refuses to create it).
	mustCreate(t, raw, &SecretWithValue{Secret: Secret{ID: "github:user:workspace:user:access", Name: "internal"}, Value: "token"})

	wrapper, ok := NewUserVisibleStore(raw).(ScopedSecretStore)
	if !ok {
		t.Fatal("wrapper is not a ScopedSecretStore")
	}
	_, err := wrapper.CopyScoped(context.Background(), "github:user:workspace:user:access", "", ScopeGlobal, "", "x", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound for internal source", err)
	}
}

func TestTransferStore_ConcurrentSameTargetSQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "secrets.db")

	conn1, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db 1: %v", err)
	}
	t.Cleanup(func() { _ = conn1.Close() })
	conn2, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db 2: %v", err)
	}
	t.Cleanup(func() { _ = conn2.Close() })

	crypto, err := NewMasterKeyProvider(dir)
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	store1, cleanup1, err := Provide(sqlx.NewDb(conn1, "sqlite3"), sqlx.NewDb(conn1, "sqlite3"), crypto)
	if err != nil {
		t.Fatalf("provide store 1: %v", err)
	}
	t.Cleanup(func() { _ = cleanup1() })
	store2, cleanup2, err := Provide(sqlx.NewDb(conn2, "sqlite3"), sqlx.NewDb(conn2, "sqlite3"), crypto)
	if err != nil {
		t.Fatalf("provide store 2: %v", err)
	}
	t.Cleanup(func() { _ = cleanup2() })

	ctx := context.Background()
	mustCreate(t, store1, &SecretWithValue{Secret: Secret{ID: "src", Name: "orig"}, Value: "v"})

	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := store1.CopyScoped(ctx, "src", "", ScopeGlobal, "", "dup", func(context.Context) error {
			close(locked)
			<-release
			return nil
		})
		done <- err
	}()

	// A holds the SQLite write lock (BEGIN IMMEDIATE + callback in-flight).
	<-locked

	bErr := make(chan error, 1)
	go func() {
		_, err := store2.CopyScoped(ctx, "src", "", ScopeGlobal, "", "dup", nil)
		bErr <- err
	}()

	select {
	case err := <-bErr:
		t.Fatalf("B completed while A held the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
		// B is blocked at the SQLite transaction lock, not at a Go pool.
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("A: %v", err)
	}
	err = <-bErr
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("B error = %v, want ErrSecretNameConflict", err)
	}
}
