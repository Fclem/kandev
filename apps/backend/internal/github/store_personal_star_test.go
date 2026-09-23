package github

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPersonalStarRowLockClauseMatchesThePostgresDriverName pins the dialect
// check: the PostgreSQL pool names its driver "pgx", so an equality test
// against "postgres" never matches and silently drops the row lock.
func TestPersonalStarRowLockClauseMatchesThePostgresDriverName(t *testing.T) {
	if clause := personalStarRowLockClause(dialect.PGX); clause == "" {
		t.Fatal("PostgreSQL row read takes no row lock")
	}
	if clause := personalStarRowLockClause(dialect.SQLite3); clause != "" {
		t.Fatalf("SQLite row read clause = %q, want none", clause)
	}
}

// TestPersonalStarConnectionLifecycleOnPostgres exercises the star store's
// transactions on PostgreSQL: the lifecycle upsert with RETURNING, the
// fence-checked replacement, the superseded-secret enqueue, the row-locked
// reads, and the disconnect tombstone. Skips without KANDEV_TEST_POSTGRES_DSN.
func TestPersonalStarConnectionLifecycleOnPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := t.Context()
	if _, err := database.Exec(`INSERT INTO workspaces (id) VALUES ('ws-1')`); err != nil {
		t.Fatal(err)
	}

	revision, generation, err := store.BumpPersonalStarLifecycle(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatalf("bump lifecycle: %v", err)
	}
	if revision != 1 || generation != 1 {
		t.Fatalf("first bump = %d/%d", revision, generation)
	}
	first := &PersonalStarConnection{
		WorkspaceID: "ws-1", UserID: "user-1", Login: "octocat", GitHubUserID: 42,
		GrantedScopes: "public_repo", PublicRepo: true,
		CredentialGeneration: generation, LifecycleRevision: revision,
		AccessSecretID:         personalStarAccessSecretID("ws-1", "user-1", generation),
		RefreshSecretID:        personalStarRefreshSecretID("ws-1", "user-1", generation),
		AccessSecretGeneration: generation, RefreshSecretGeneration: generation,
	}
	if err := store.InsertPersonalStarConnection(ctx, first, revision, generation); err != nil {
		t.Fatalf("insert connection: %v", err)
	}

	// A second consent (reconnect) must satisfy the committed row's fence.
	revision, generation, err = store.BumpPersonalStarLifecycle(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	second := *first
	second.CredentialGeneration = generation
	second.LifecycleRevision = revision
	second.AccessSecretID = personalStarAccessSecretID("ws-1", "user-1", generation)
	second.RefreshSecretID = personalStarRefreshSecretID("ws-1", "user-1", generation)
	second.AccessSecretGeneration = generation
	second.RefreshSecretGeneration = generation
	superseded, err := store.ReplacePersonalStarConnection(
		ctx, &second,
		PersonalStarFence{Revision: first.LifecycleRevision, Generation: first.CredentialGeneration},
		PersonalStarFence{Revision: revision, Generation: generation})
	if err != nil {
		t.Fatalf("replace connection: %v", err)
	}
	if len(superseded) != 2 {
		t.Fatalf("superseded = %v, want both previous secrets", superseded)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil || len(records) != 2 {
		t.Fatalf("queued cleanup rows = %d, %v", len(records), err)
	}
	for _, record := range records {
		if record.CredentialGeneration != first.CredentialGeneration {
			t.Fatalf("queued generation = %d, want %d", record.CredentialGeneration, first.CredentialGeneration)
		}
	}

	// A stale fence is still rejected.
	if _, err := store.ReplacePersonalStarConnection(
		ctx, &second,
		PersonalStarFence{Revision: revision, Generation: generation},
		PersonalStarFence{Revision: revision, Generation: generation}); err != ErrPersonalStarFence {
		t.Fatalf("stale replace error = %v, want fence mismatch", err)
	}

	// Disconnect tombstones the row and queues the live generation.
	disconnected, err := store.DisconnectPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if len(disconnected) != 2 {
		t.Fatalf("disconnect superseded = %v, want both live secrets", disconnected)
	}
	row, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || row == nil {
		t.Fatalf("connection = %+v, %v", row, err)
	}
	if row.Status != PersonalStarStatusInactive || row.AccessSecretID != "" {
		t.Fatalf("row after disconnect = %+v", row)
	}
	records, err = store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil || len(records) != 4 {
		t.Fatalf("queued cleanup rows after disconnect = %d, %v", len(records), err)
	}

	// The flow claim transaction takes its lock through the same helper.
	flow := &PersonalStarFlow{
		StateDigest: "digest", FlowKind: PersonalStarFlowKind,
		WorkspaceID: "ws-1", UserID: "user-1", SessionSubject: "user-1",
		ExpectedRevision: revision, ExpectedGeneration: generation,
		CodeVerifier: "verifier", CodeChallenge: personalStarPKCEChallenge("verifier"),
		RedirectURI: "https://kandev.example/cb", ReturnPath: "/settings/plugins",
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	if err := store.CreatePersonalStarFlow(ctx, flow); err != nil {
		t.Fatalf("create flow: %v", err)
	}
	// The claim reads the fence through the transaction's own executor (and
	// takes the row lock) before consuming the flow exactly once.
	claimed, err := store.ClaimPersonalStarFlowForExchange(ctx, "digest", time.Now().UTC(),
		func(ctx context.Context, exec sqlx.Ext, _ *PersonalStarFlow) error {
			lifecycle, err := getPersonalStarLifecycleOn(ctx, exec, "ws-1", "user-1")
			if err != nil {
				return err
			}
			if lifecycle.Revision == 0 {
				return ErrPersonalStarFence
			}
			return nil
		})
	if err != nil {
		t.Fatalf("claim flow: %v", err)
	}
	if claimed == nil || claimed.StateDigest != "digest" {
		t.Fatalf("claimed flow = %+v", claimed)
	}
	if _, err := store.ClaimPersonalStarFlowForExchange(ctx, "digest", time.Now().UTC(),
		func(context.Context, sqlx.Ext, *PersonalStarFlow) error { return nil }); err != ErrPersonalStarFlowUnavailable {
		t.Fatalf("second claim error = %v, want unavailable", err)
	}
}
