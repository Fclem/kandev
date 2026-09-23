package github

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
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

// TestPersonalStarReconnectRequiredNeedsACommittedConsent pins the documented
// invariant: reconnect_required stays false until a consent actually commits a
// usable connection. Every start bumps the fence, so a started-then-abandoned
// (or declined, or failed, or reduced-scope) first consent must still offer
// Connect rather than Reconnect.
func TestPersonalStarReconnectRequiredNeedsACommittedConsent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
		Available: true,
		Settings:  PersonalStarOAuthSettings{ClientID: "id", ClientSecret: "s", CallbackURL: "https://example/x"},
	})
	status := func() PersonalStarConnectionStatus {
		t.Helper()
		s, err := resolver.ConnectionStatus(ctx, "ws-1", "user-1")
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	if got := status(); got.ReconnectRequired {
		t.Fatalf("pristine status = %+v", got)
	}

	// Started and abandoned.
	abandoned := startPersonalStarConsent(t, service, "/settings/plugins")
	_ = abandoned
	if got := status(); got.ReconnectRequired {
		t.Fatalf("abandoned first consent asked for a reconnect: %+v", got)
	}

	// Declined first consent.
	declined := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: declined, Error: "access_denied"}, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	if got := status(); got.ReconnectRequired {
		t.Fatalf("declined first consent asked for a reconnect: %+v", got)
	}

	// Reduced scope first consent.
	client.tokens = GitHubOAuthTokens{AccessToken: "access-1", GrantedScopes: []string{"read:user"}}
	reduced := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: reduced, Code: "code"}, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	if got := status(); got.ReconnectRequired {
		t.Fatalf("reduced-scope first consent asked for a reconnect: %+v", got)
	}

	// A consent that commits flips the invariant, and a disconnect keeps it:
	// a previously valid generation now exists.
	client.tokens = GitHubOAuthTokens{AccessToken: "access-2", RefreshToken: "refresh-2", GrantedScopes: []string{"public_repo"}}
	connected := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: connected, Code: "code"}, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	if got := status(); !got.Active || !got.Capable || got.ReconnectRequired {
		t.Fatalf("connected status = %+v", got)
	}
	if err := service.DisconnectPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if got := status(); got.Active || got.Capable || !got.ReconnectRequired {
		t.Fatalf("post-disconnect status = %+v", got)
	}
}

// TestStarSecretCleanupTerminalFailureIsVisible pins the operator-visible half
// of the cleanup contract: a deletion that keeps failing must record the
// attempt count that gave up, keep its diagnostic last_error, stay out of the
// retry loop, and be counted so the failure is not silent.
func TestStarSecretCleanupTerminalFailureIsVisible(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	secrets.values["stuck-secret"] = "token"
	secrets.failOn = "stuck-secret"
	if err := store.EnqueuePersonalStarSecretCleanup(ctx, "ws-1", "user-1", "stuck-secret",
		personalStarSecretKindAccess, 3); err != nil {
		t.Fatal(err)
	}
	runner := NewStarSecretCleanupRunner(store, secrets, nil)
	now := time.Now()
	runner.now = func() time.Time { return now }

	terminalBefore := personalStarCleanupAttemptCount(personalStarCleanupOutcomeTerminal)
	for attempt := 0; attempt < personalStarCleanupMaxAttempts; attempt++ {
		now = now.Add(24 * time.Hour)
		if deleted, err := runner.RunOnce(ctx); err != nil || deleted != 0 {
			t.Fatalf("RunOnce() = %d, %v", deleted, err)
		}
	}

	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %d, %v", len(records), err)
	}
	record := records[0]
	if record.Status != personalStarCleanupTerminal {
		t.Fatalf("record = %+v, want terminal", record)
	}
	if record.Attempts != personalStarCleanupMaxAttempts {
		t.Fatalf("terminal attempts = %d, want %d", record.Attempts, personalStarCleanupMaxAttempts)
	}
	if record.LastError == "" {
		t.Fatal("terminal record carries no diagnostic")
	}

	// A terminal row leaves the retry loop: further passes neither delete nor
	// reschedule it.
	now = now.Add(24 * time.Hour)
	if deleted, err := runner.RunOnce(ctx); err != nil || deleted != 0 {
		t.Fatalf("RunOnce() after terminal = %d, %v", deleted, err)
	}
	if after, err := store.GetPersonalStarSecretCleanup(ctx, record.ID); err != nil || after == nil || after.Status != personalStarCleanupTerminal {
		t.Fatalf("record after terminal = %+v, %v", after, err)
	}

	terminalAfter := personalStarCleanupAttemptCount(personalStarCleanupOutcomeTerminal)
	if terminalAfter == terminalBefore {
		t.Fatal("a terminal cleanup failure was not counted")
	}
}

// failPersonalStarMarkerWrite installs a trigger that makes the durable
// generation marker unwritable, so the first-consent commit contract can be
// exercised deterministically. It returns a restore function.
func failPersonalStarMarkerWrite(t *testing.T, store *Store) func() {
	t.Helper()
	const trigger = `CREATE TRIGGER fail_star_connected_generation
		BEFORE UPDATE ON github_personal_star_lifecycle
		BEGIN SELECT RAISE(ABORT, 'lifecycle marker unavailable'); END`
	if _, err := store.db.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	return func() {
		if _, err := store.db.Exec(`DROP TRIGGER fail_star_connected_generation`); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPersonalStarFirstConsentRollsBackWhenMarkerWriteFails pins that the
// connection row and its durable generation marker commit together. A row that
// commits while the marker fails would read as active and capable, but the
// failed callback compensates the staged tokens the row points at, so every
// star call would fail while no state offered the user a reconnect path.
func TestPersonalStarFirstConsentRollsBackWhenMarkerWriteFails(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
		Available: true,
		Settings:  PersonalStarOAuthSettings{ClientID: "id", ClientSecret: "s", CallbackURL: "https://example/x"},
	})

	restore := failPersonalStarMarkerWrite(t, store)
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); err == nil {
		t.Fatal("first consent committed while the generation marker was unwritable")
	}
	restore()

	connection, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if connection != nil {
		t.Fatalf("failed first consent left a committed row: %+v", connection)
	}
	lifecycle, err := store.GetPersonalStarLifecycle(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.ConnectedGeneration != 0 {
		t.Fatalf("connected generation = %d after a failed first consent", lifecycle.ConnectedGeneration)
	}
	status, err := resolver.ConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Active || status.Capable || status.ReconnectRequired {
		t.Fatalf("failed first consent left an unusable state: %+v", status)
	}

	// The user consents again and gets a usable, capable connection.
	state = startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	status, err = resolver.ConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || !status.Capable || status.ReconnectRequired {
		t.Fatalf("retried consent did not produce a usable connection: %+v", status)
	}
	if _, err := resolver.ResolvePersonalStarClient(ctx, "ws-1", "user-1"); err != nil {
		t.Fatalf("retried consent cannot resolve a star client: %v", err)
	}
}

// TestPersonalStarCleanupAttemptBucketsSumToAttempts pins the meaning of
// personal_star_cleanup_attempts_total: every failed attempt is counted exactly
// once, in the bucket that describes what happened to it, so the buckets sum to
// the attempt count an operator would read from the queue row.
func TestPersonalStarCleanupAttemptBucketsSumToAttempts(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	secrets.values["stuck-secret"] = "token"
	secrets.failOn = "stuck-secret"
	if err := store.EnqueuePersonalStarSecretCleanup(ctx, "ws-1", "user-1", "stuck-secret",
		personalStarSecretKindAccess, 3); err != nil {
		t.Fatal(err)
	}
	runner := NewStarSecretCleanupRunner(store, secrets, nil)
	now := time.Now()
	runner.now = func() time.Time { return now }

	before := map[string]int64{}
	for _, outcome := range []string{
		personalStarCleanupOutcomeDeleted,
		personalStarCleanupOutcomeRetry,
		personalStarCleanupOutcomeTerminal,
	} {
		before[outcome] = personalStarCleanupAttemptCount(outcome)
	}
	for attempt := 0; attempt < personalStarCleanupMaxAttempts; attempt++ {
		now = now.Add(24 * time.Hour)
		if deleted, err := runner.RunOnce(ctx); err != nil || deleted != 0 {
			t.Fatalf("RunOnce() = %d, %v", deleted, err)
		}
	}
	total := int64(0)
	for outcome := range before {
		total += personalStarCleanupAttemptCount(outcome) - before[outcome]
	}
	if total != personalStarCleanupMaxAttempts {
		t.Fatalf("attempt buckets sum to %d, want %d (deleted %d, retry %d, terminal %d)",
			total, personalStarCleanupMaxAttempts,
			personalStarCleanupAttemptCount(personalStarCleanupOutcomeDeleted)-before[personalStarCleanupOutcomeDeleted],
			personalStarCleanupAttemptCount(personalStarCleanupOutcomeRetry)-before[personalStarCleanupOutcomeRetry],
			personalStarCleanupAttemptCount(personalStarCleanupOutcomeTerminal)-before[personalStarCleanupOutcomeTerminal])
	}
	if terminal := personalStarCleanupAttemptCount(personalStarCleanupOutcomeTerminal) - before[personalStarCleanupOutcomeTerminal]; terminal != 1 {
		t.Fatalf("terminal attempts = %d, want 1", terminal)
	}
	if retry := personalStarCleanupAttemptCount(personalStarCleanupOutcomeRetry) - before[personalStarCleanupOutcomeRetry]; retry != 7 {
		t.Fatalf("scheduled retries = %d, want %d", retry, personalStarCleanupMaxAttempts-1)
	}
}

// personalStarCleanupAttemptCount reads one attempt bucket. expvar creates a
// bucket on first write, so an untouched outcome reads as nil and therefore as
// zero: the buckets belong to the process, not to a single test.
func personalStarCleanupAttemptCount(outcome string) int64 {
	value := personalStarCleanupAttempts.Get(outcome)
	if value == nil {
		return 0
	}
	parsed, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
