package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestPersonalStarTokenSecretIDsStayInTheReservedNamespace pins where dedicated
// OAuth token rows live. The backend-owned credential policy
// (secrets.IsInternalID) protects the `kandev-runtime:` namespace, so a builder
// that produced a user-visible id would expose access and refresh tokens to the
// secrets API and to agent credential selection.
func TestPersonalStarTokenSecretIDsStayInTheReservedNamespace(t *testing.T) {
	access := personalStarAccessSecretID("ws-1", "user-1", 7)
	refresh := personalStarRefreshSecretID("ws-1", "user-1", 7)
	for name, id := range map[string]string{"access": access, "refresh": refresh} {
		if !strings.HasPrefix(id, "kandev-runtime:github-personal-star:") {
			t.Fatalf("%s secret id %q is outside the reserved runtime namespace", name, id)
		}
		if !strings.Contains(id, "ws-1") || !strings.Contains(id, "user-1") || !strings.HasSuffix(id, ":7") {
			t.Fatalf("%s secret id %q does not name its workspace/user/generation", name, id)
		}
	}
	if access == refresh {
		t.Fatal("access and refresh tokens share one secret id")
	}
	if !strings.HasPrefix(access, KandevRuntimePersonalStarAccessPrefix) ||
		!strings.HasPrefix(refresh, KandevRuntimePersonalStarRefreshPrefix) {
		t.Fatal("exported teardown prefixes do not match the token secret ids")
	}
}

// failStarCleanupEnqueue installs a trigger that makes every cleanup-queue
// insert fail, so the atomic swap/enqueue contract can be exercised
// deterministically. It returns a restore function.
func failStarCleanupEnqueue(t *testing.T, store *Store) func() {
	t.Helper()
	const trigger = `CREATE TRIGGER fail_star_cleanup_insert
		BEFORE INSERT ON github_personal_star_cleanup
		BEGIN SELECT RAISE(ABORT, 'cleanup queue unavailable'); END`
	if _, err := store.db.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	return func() {
		if _, err := store.db.Exec(`DROP TRIGGER fail_star_cleanup_insert`); err != nil {
			t.Fatal(err)
		}
	}
}

func seedConnectedPersonalStar(t *testing.T, service *PersonalStarOAuthService, client *fakePersonalStarOAuth) *PersonalStarConnection {
	t.Helper()
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(context.Background(),
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	connection, err := service.store.GetPersonalStarConnection(context.Background(), "ws-1", "user-1")
	if err != nil || connection == nil {
		t.Fatalf("connection = %+v, %v", connection, err)
	}
	_ = client
	return connection
}

func TestPersonalStarReplacementRollsBackWhenCleanupEnqueueFails(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	before := seedConnectedPersonalStar(t, service, client)

	restore := failStarCleanupEnqueue(t, store)
	client.refreshed = GitHubOAuthTokens{AccessToken: "access-2", GrantedScopes: []string{"public_repo"}}
	if err := service.RefreshPersonalStarConnection(ctx, "ws-1", "user-1"); err == nil {
		t.Fatal("refresh succeeded while the cleanup queue was unavailable")
	}

	// The committed row must still reference the tokens it had before: a
	// replacement that cannot record its superseded secrets must not orphan
	// them by committing.
	after, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || after == nil {
		t.Fatalf("connection = %+v, %v", after, err)
	}
	if after.AccessSecretID != before.AccessSecretID ||
		after.CredentialGeneration != before.CredentialGeneration ||
		after.LifecycleRevision != before.LifecycleRevision {
		t.Fatalf("failed refresh mutated the committed row: %+v -> %+v", before, after)
	}
	// The rejected refresh compensates the secret it staged, and the live
	// token stays untouched.
	if len(secrets.deleted) != 1 || !strings.Contains(secrets.deleted[0], "access") {
		t.Fatalf("staged secrets were not compensated: %v", secrets.deleted)
	}
	if _, ok := secrets.values[before.AccessSecretID]; !ok {
		t.Fatal("the live access token was deleted by a rejected refresh")
	}

	// Once the queue is writable the same refresh succeeds and queues the
	// superseded generation.
	restore()
	superseded := before.AccessSecretID
	if err := service.RefreshPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].SecretID != superseded {
		t.Fatalf("queued cleanup rows = %+v, want the superseded access secret", records)
	}
	if records[0].CredentialGeneration != before.AccessSecretGeneration {
		t.Fatalf("queued generation = %d, want %d", records[0].CredentialGeneration, before.AccessSecretGeneration)
	}
	if records[0].SecretKind != personalStarSecretKindAccess {
		t.Fatalf("queued kind = %q", records[0].SecretKind)
	}
}

func TestPersonalStarDisconnectRollsBackWhenCleanupEnqueueFails(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	before := seedConnectedPersonalStar(t, service, client)

	restore := failStarCleanupEnqueue(t, store)
	if err := service.DisconnectPersonalStarConnection(ctx, "ws-1", "user-1"); err == nil {
		t.Fatal("disconnect succeeded while the cleanup queue was unavailable")
	}
	after, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || after == nil {
		t.Fatalf("connection = %+v, %v", after, err)
	}
	if after.Status != PersonalStarStatusActive || after.AccessSecretID != before.AccessSecretID {
		t.Fatalf("failed disconnect tombstoned the row: %+v", after)
	}
	lifecycle, err := store.GetPersonalStarLifecycle(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.Revision != before.LifecycleRevision {
		t.Fatalf("failed disconnect bumped the fence: %d -> %d", before.LifecycleRevision, lifecycle.Revision)
	}

	restore()
	if err := service.DisconnectPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	queued := map[string]int64{}
	for _, record := range records {
		queued[record.SecretID] = record.CredentialGeneration
	}
	for _, expected := range []struct {
		id         string
		generation int64
	}{
		{before.AccessSecretID, before.AccessSecretGeneration},
		{before.RefreshSecretID, before.RefreshSecretGeneration},
	} {
		generation, ok := queued[expected.id]
		if !ok {
			t.Fatalf("disconnect did not queue %q: %v", expected.id, queued)
		}
		if generation != expected.generation {
			t.Fatalf("queued generation for %q = %d, want %d", expected.id, generation, expected.generation)
		}
	}
	runner := NewStarSecretCleanupRunner(store, secrets, nil)
	if deleted, err := runner.RunOnce(ctx); err != nil || deleted != len(records) {
		t.Fatalf("RunOnce() = %d, %v; want %d", deleted, err, len(records))
	}
	if len(secrets.values) != 0 {
		t.Fatalf("disconnect left token values behind: %v", secrets.values)
	}
}

// TestPersonalStarSecondConsentCommitsANewGeneration covers the reconnect and
// connect-after-disconnect paths: a consent that replaces a committed row must
// satisfy that row's fence, not the fence its own start already bumped past.
func TestPersonalStarSecondConsentCommitsANewGeneration(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	first := seedConnectedPersonalStar(t, service, client)

	// Second consent for the same workspace/user (reconnect).
	client.tokens = GitHubOAuthTokens{AccessToken: "access-2", RefreshToken: "refresh-2", GrantedScopes: []string{"public_repo"}}
	second := startPersonalStarConsent(t, service, "/settings/plugins")
	outcome, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: second, Code: "code-2"}, "user-1", "session-1")
	if err != nil {
		t.Fatalf("second consent: %v", err)
	}
	if !outcome.Active || !outcome.Capable {
		t.Fatalf("second consent outcome = %+v", outcome)
	}
	reconnected, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || reconnected == nil {
		t.Fatalf("connection = %+v, %v", reconnected, err)
	}
	if reconnected.CredentialGeneration <= first.CredentialGeneration {
		t.Fatalf("generation did not advance: %d -> %d", first.CredentialGeneration, reconnected.CredentialGeneration)
	}
	if secrets.values[reconnected.AccessSecretID] != "access-2" {
		t.Fatalf("reconnected token was not staged: %v", secrets.values)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Fatal("reconnect did not queue the superseded generation for cleanup")
	}

	// Connect after a disconnect: the tombstone fence is its own bump.
	if err := service.DisconnectPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	client.tokens = GitHubOAuthTokens{AccessToken: "access-3", GrantedScopes: []string{"public_repo"}}
	third := startPersonalStarConsent(t, service, "/settings/plugins")
	outcome, err = service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: third, Code: "code-3"}, "user-1", "session-1")
	if err != nil {
		t.Fatalf("connect after disconnect: %v", err)
	}
	if !outcome.Active || !outcome.Capable {
		t.Fatalf("connect-after-disconnect outcome = %+v", outcome)
	}
	connected, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || connected == nil {
		t.Fatalf("connection = %+v, %v", connected, err)
	}
	if connected.Status != PersonalStarStatusActive || secrets.values[connected.AccessSecretID] != "access-3" {
		t.Fatalf("connect after disconnect = %+v", connected)
	}
}

// blockingStarOAuth lets a test land a disconnect inside the token-exchange
// window, i.e. after the flow claim and before the commit.
type blockingStarOAuth struct {
	tokens  GitHubOAuthTokens
	user    GitHubOAuthUser
	entered chan struct{}
	release chan struct{}
}

func (b *blockingStarOAuth) ExchangeUserCode(
	ctx context.Context, _, _, _ string,
) (GitHubOAuthTokens, error) {
	close(b.entered)
	<-b.release
	return b.tokens, nil
}

func (b *blockingStarOAuth) RefreshUserToken(context.Context, string) (GitHubOAuthTokens, error) {
	return GitHubOAuthTokens{}, errors.New("unexpected refresh")
}

func (b *blockingStarOAuth) GetOAuthUser(context.Context, string) (GitHubOAuthUser, error) {
	return b.user, nil
}

// TestPersonalStarDisconnectDuringExchangeWins covers the window the claim
// check cannot see: the flow is claimed, the token exchange is in flight, and
// the user disconnects before the commit. The commit must not resurrect the
// tombstone or write back a fence older than the lifecycle row.
func TestPersonalStarDisconnectDuringExchangeWins(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	if _, _, err := store.BumpPersonalStarLifecycle(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := service.DisconnectPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}

	blocking := &blockingStarOAuth{
		tokens:  GitHubOAuthTokens{AccessToken: "access-9", GrantedScopes: []string{"public_repo"}},
		user:    GitHubOAuthUser{ID: 42, Login: "octocat"},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	service.client = blocking
	state := startPersonalStarConsent(t, service, "/settings/plugins")

	result := make(chan error, 1)
	go func() {
		_, err := service.CompleteCallback(ctx,
			GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1")
		result <- err
	}()
	<-blocking.entered
	// The disconnect lands while the exchange is in flight.
	if _, err := store.DisconnectPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	close(blocking.release)
	if err := <-result; err == nil {
		t.Fatal("a consent committed after the user disconnected during its exchange")
	}

	lifecycle, err := store.GetPersonalStarLifecycle(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || row == nil {
		t.Fatalf("connection = %+v, %v", row, err)
	}
	if row.Status != PersonalStarStatusInactive || row.AccessSecretID != "" {
		t.Fatalf("disconnected row was resurrected: %+v", row)
	}
	if row.LifecycleRevision != lifecycle.Revision || row.CredentialGeneration != lifecycle.CredentialGeneration {
		t.Fatalf("row fence %d/%d regressed behind lifecycle %d/%d",
			row.LifecycleRevision, row.CredentialGeneration, lifecycle.Revision, lifecycle.CredentialGeneration)
	}
}

// TestPersonalStarStaleCallbackOutcomeDoesNotRelabelANewerConsent covers a
// callback that lost the race: it claimed its flow, then a newer consent
// committed a healthy connection while this callback was still in flight (or
// simply failed later). The stale attempt must not relabel the newer row,
// because the resolver treats a failed outcome as "reconnect required" and the
// design promises a failed reconnect never invalidates a valid connection.
func TestPersonalStarStaleCallbackOutcomeDoesNotRelabelANewerConsent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)

	// Flow A claims its fence and parks inside the token exchange.
	blocking := &blockingStarOAuth{
		tokens:  GitHubOAuthTokens{AccessToken: "access-stale"},
		user:    GitHubOAuthUser{ID: 42, Login: "octocat"},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	service.client = blocking
	staleState := startPersonalStarConsent(t, service, "/settings/plugins")
	staleResult := make(chan error, 1)
	go func() {
		_, err := service.CompleteCallback(ctx,
			GitHubStarCallbackQuery{State: staleState, Code: "code-stale"}, "user-1", "session-1")
		staleResult <- err
	}()
	<-blocking.entered

	// Flow B starts after A, completes, and commits a healthy connection.
	service.client = client
	client.tokens = GitHubOAuthTokens{AccessToken: "access-2", RefreshToken: "refresh-2", GrantedScopes: []string{"public_repo"}}
	freshState := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: freshState, Code: "code-fresh"}, "user-1", "session-1"); err != nil {
		t.Fatalf("newer consent: %v", err)
	}
	connected, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || connected == nil {
		t.Fatalf("connection = %+v, %v", connected, err)
	}
	if connected.LastReconnectOutcome != personalStarOutcomeConnected {
		t.Fatalf("newer consent outcome = %q", connected.LastReconnectOutcome)
	}

	// A resumes and fails.
	close(blocking.release)
	<-staleResult

	after, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || after == nil {
		t.Fatalf("connection = %+v, %v", after, err)
	}
	if after.LastReconnectOutcome != personalStarOutcomeConnected {
		t.Fatalf("stale callback relabelled the newer row: %q", after.LastReconnectOutcome)
	}
	if after.AccessSecretID != connected.AccessSecretID {
		t.Fatalf("stale callback changed the committed secrets: %+v", after)
	}
	resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
		Available: true,
		Settings:  PersonalStarOAuthSettings{ClientID: "id", ClientSecret: "s", CallbackURL: "https://example/x"},
	})
	status, err := resolver.ConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.ReconnectRequired {
		t.Fatalf("stale failure made a valid connection need a reconnect: %+v", status)
	}
}
