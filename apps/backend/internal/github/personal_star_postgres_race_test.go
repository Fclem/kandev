package github

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"

	"github.com/kandev/kandev/internal/testutil"
)

const personalStarTestAdvisoryLock int64 = 991234567890

func openPersonalStarPostgresStore(t *testing.T) (*sqlx.DB, *Store) {
	t.Helper()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`CREATE TABLE workspaces (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO workspaces (id) VALUES ('ws-1')`); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("new GitHub store: %v", err)
	}
	return database, store
}

func installPersonalStarUpdateGate(t *testing.T, database *sqlx.DB) func() {
	t.Helper()
	for _, statement := range []string{
		`CREATE SEQUENCE personal_star_gate_sequence`,
		`CREATE FUNCTION personal_star_gate_update() RETURNS trigger AS $$
			BEGIN
				PERFORM nextval('personal_star_gate_sequence');
				PERFORM pg_advisory_xact_lock(991234567890);
				RETURN NEW;
			END;
		$$ LANGUAGE plpgsql`,
		`CREATE TRIGGER personal_star_gate_update
			BEFORE UPDATE ON github_personal_star_connection
			FOR EACH ROW EXECUTE FUNCTION personal_star_gate_update()`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("install PostgreSQL race gate: %v", err)
		}
	}
	connection, err := database.Connx(t.Context())
	if err != nil {
		t.Fatalf("reserve PostgreSQL gate connection: %v", err)
	}
	var locked bool
	if err := connection.GetContext(t.Context(), &locked,
		`SELECT pg_try_advisory_lock($1)`, personalStarTestAdvisoryLock); err != nil || !locked {
		_ = connection.Close()
		t.Fatalf("acquire PostgreSQL advisory gate = %v, %v", locked, err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			var unlocked bool
			if err := connection.GetContext(context.Background(), &unlocked,
				`SELECT pg_advisory_unlock($1)`, personalStarTestAdvisoryLock); err != nil || !unlocked {
				t.Errorf("release PostgreSQL advisory gate = %v, %v", unlocked, err)
			}
			if err := connection.Close(); err != nil {
				t.Errorf("close PostgreSQL gate connection: %v", err)
			}
		})
	}
}

func waitForPersonalStarGate(t *testing.T, database *sqlx.DB) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var called bool
		if err := database.GetContext(t.Context(), &called,
			`SELECT is_called FROM personal_star_gate_sequence`); err != nil {
			t.Fatalf("read PostgreSQL gate sequence: %v", err)
		}
		if called {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("personal-star replacement did not enter the gated update")
}

func waitForPersonalStarLockWait(t *testing.T, database *sqlx.DB, queryFragment string, settled <-chan struct{}) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiters int
		if err := database.GetContext(t.Context(), &waiters, `
			SELECT COUNT(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
				AND query LIKE '%' || $1 || '%'`, queryFragment); err != nil {
			t.Fatalf("inspect PostgreSQL lock wait: %v", err)
		}
		if waiters > 0 {
			return
		}
		select {
		case <-settled:
			t.Fatalf("operation completed instead of waiting on %q", queryFragment)
		default:
		}
		runtime.Gosched()
	}
	t.Fatalf("no PostgreSQL lock wait observed for %q", queryFragment)
}

func TestWorkspaceDeletionSerializesWithPersonalStarRefreshOnPostgres(t *testing.T) {
	database, store := openPersonalStarPostgresStore(t)
	ctx := t.Context()
	workspaceID, userID := "ws-1", "user-1"
	revision, generation, err := store.BumpPersonalStarLifecycle(ctx, workspaceID, userID)
	if err != nil {
		t.Fatal(err)
	}
	previous := &PersonalStarConnection{
		WorkspaceID: workspaceID, UserID: userID, Login: "octocat", GitHubUserID: 42,
		GrantedScopes: PersonalStarScope, PublicRepo: true, CredentialGeneration: generation,
		LifecycleRevision: revision, AccessSecretID: personalStarAccessSecretID(workspaceID, userID, generation),
		RefreshSecretID:        personalStarRefreshSecretID(workspaceID, userID, generation),
		AccessSecretGeneration: generation, RefreshSecretGeneration: generation,
	}
	if err := store.InsertPersonalStarConnection(ctx, previous, revision, generation); err != nil {
		t.Fatal(err)
	}
	secrets := newFakePersonalStarSecrets()
	secrets.values[previous.AccessSecretID] = "access-1"
	secrets.values[previous.RefreshSecretID] = "refresh-1"
	oauth := newPersonalStarTestService(t, store, secrets, &fakePersonalStarOAuth{
		refreshed: GitHubOAuthTokens{
			AccessToken: "access-2", RefreshToken: "refresh-2", GrantedScopes: []string{PersonalStarScope},
		},
	}, true)
	gateRelease := installPersonalStarUpdateGate(t, database)
	var workers sync.WaitGroup
	defer func() {
		gateRelease()
		finished := make(chan struct{})
		go func() { workers.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("PostgreSQL race-test operations did not finish")
		}
	}()

	refreshDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		refreshDone <- oauth.RefreshPersonalStarConnection(ctx, workspaceID, userID)
	}()
	waitForPersonalStarGate(t, database)

	service := &Service{store: store}
	deleteDone := make(chan error, 1)
	deleteSettled := make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer close(deleteSettled)
		deleteDone <- service.handleWorkspaceDeleted(ctx,
			bus.NewEvent(events.WorkspaceDeleted, "test", map[string]interface{}{"id": workspaceID}))
	}()
	waitForPersonalStarLockWait(t, database, "INSERT INTO github_personal_star_workspace_lifecycle", deleteSettled)
	gateRelease()
	if err := <-refreshDone; err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatalf("workspace teardown: %v", err)
	}
	connection, err := store.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil || connection != nil {
		t.Fatalf("connection after teardown = %+v, %v", connection, err)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	queued := make(map[string]bool, len(records))
	for _, record := range records {
		queued[record.SecretID] = true
	}
	for _, secretID := range []string{
		previous.AccessSecretID, previous.RefreshSecretID,
		personalStarAccessSecretID(workspaceID, userID, generation+1),
		personalStarRefreshSecretID(workspaceID, userID, generation+1),
	} {
		if !queued[secretID] {
			t.Errorf("token secret %q is not queued after refresh and teardown", secretID)
		}
	}
}

func TestPersonalStarReplacementHoldsLifecycleFenceThroughCommitOnPostgres(t *testing.T) {
	database, store := openPersonalStarPostgresStore(t)
	ctx := t.Context()
	workspaceID, userID := "ws-1", "user-1"
	revision, generation, err := store.BumpPersonalStarLifecycle(ctx, workspaceID, userID)
	if err != nil {
		t.Fatal(err)
	}
	previous := &PersonalStarConnection{
		WorkspaceID: workspaceID, UserID: userID, Login: "octocat", GitHubUserID: 42,
		GrantedScopes: PersonalStarScope, PublicRepo: true, CredentialGeneration: generation,
		LifecycleRevision: revision, AccessSecretID: personalStarAccessSecretID(workspaceID, userID, generation),
		RefreshSecretID:        personalStarRefreshSecretID(workspaceID, userID, generation),
		AccessSecretGeneration: generation, RefreshSecretGeneration: generation,
	}
	if err := store.InsertPersonalStarConnection(ctx, previous, revision, generation); err != nil {
		t.Fatal(err)
	}
	revision, generation, err = store.BumpPersonalStarLifecycle(ctx, workspaceID, userID)
	if err != nil {
		t.Fatal(err)
	}
	replacement := *previous
	replacement.CredentialGeneration = generation
	replacement.LifecycleRevision = revision
	replacement.AccessSecretID = personalStarAccessSecretID(workspaceID, userID, generation)
	replacement.RefreshSecretID = personalStarRefreshSecretID(workspaceID, userID, generation)
	replacement.AccessSecretGeneration = generation
	replacement.RefreshSecretGeneration = generation
	gateRelease := installPersonalStarUpdateGate(t, database)
	var workers sync.WaitGroup
	defer func() {
		gateRelease()
		finished := make(chan struct{})
		go func() { workers.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("PostgreSQL race-test operations did not finish")
		}
	}()

	replaceDone := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := store.ReplacePersonalStarConnection(ctx, &replacement,
			PersonalStarFence{Revision: previous.LifecycleRevision, Generation: previous.CredentialGeneration},
			PersonalStarFence{Revision: revision, Generation: generation})
		replaceDone <- err
	}()
	waitForPersonalStarGate(t, database)

	bumpDone := make(chan struct {
		revision, generation int64
		err                  error
	}, 1)
	bumpSettled := make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		defer close(bumpSettled)
		nextRevision, nextGeneration, err := store.BumpPersonalStarLifecycle(ctx, workspaceID, userID)
		bumpDone <- struct {
			revision, generation int64
			err                  error
		}{nextRevision, nextGeneration, err}
	}()
	waitForPersonalStarLockWait(t, database, "INSERT INTO github_personal_star_workspace_lifecycle", bumpSettled)
	gateRelease()
	if err := <-replaceDone; err != nil {
		t.Fatalf("replace connection: %v", err)
	}
	next := <-bumpDone
	if next.err != nil {
		t.Fatalf("advance lifecycle: %v", next.err)
	}
	if next.revision <= replacement.LifecycleRevision || next.generation <= replacement.CredentialGeneration {
		t.Fatalf("new lifecycle fence = %d/%d, want newer than committed connection %+v", next.revision, next.generation, replacement)
	}
	committed, err := store.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil || committed == nil {
		t.Fatalf("committed connection = %+v, %v", committed, err)
	}
	if committed.AccessSecretID != replacement.AccessSecretID || committed.LifecycleRevision != replacement.LifecycleRevision {
		t.Fatalf("connection write did not commit before the later lifecycle start: %+v", committed)
	}
}
