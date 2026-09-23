package github

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakePersonalStarSecrets struct {
	values  map[string]string
	deleted []string
	failOn  string
}

func newFakePersonalStarSecrets() *fakePersonalStarSecrets {
	return &fakePersonalStarSecrets{values: map[string]string{}}
}

// ListIDs satisfies ConnectionSecretStore so the same fake can back Provide.
func (f *fakePersonalStarSecrets) ListIDs(context.Context) ([]string, error) {
	ids := make([]string, 0, len(f.values))
	for id := range f.values {
		ids = append(ids, id)
	}
	return ids, nil
}

// Exists satisfies ConnectionSecretStore so the same fake can back Provide.
func (f *fakePersonalStarSecrets) Exists(_ context.Context, id string) (bool, error) {
	_, ok := f.values[id]
	return ok, nil
}

func (f *fakePersonalStarSecrets) Reveal(_ context.Context, id string) (string, error) {
	value, ok := f.values[id]
	if !ok {
		return "", errors.New("secret not found")
	}
	return value, nil
}

func (f *fakePersonalStarSecrets) Set(_ context.Context, id, _, value string) error {
	if f.failOn == id {
		return errors.New("secret store unavailable")
	}
	f.values[id] = value
	return nil
}

func (f *fakePersonalStarSecrets) Delete(_ context.Context, id string) error {
	if f.failOn == id {
		return errors.New("secret store unavailable")
	}
	f.deleted = append(f.deleted, id)
	delete(f.values, id)
	return nil
}

type fakePersonalStarOAuth struct {
	tokens       GitHubOAuthTokens
	refreshed    GitHubOAuthTokens
	refreshErr   error
	refreshCalls int
	seenRefresh  string
	user         GitHubOAuthUser
	exchangeErr  error
	seenVerifier string
	seenCode     string
}

func (f *fakePersonalStarOAuth) RefreshUserToken(_ context.Context, refreshToken string) (GitHubOAuthTokens, error) {
	f.refreshCalls++
	f.seenRefresh = refreshToken
	if f.refreshErr != nil {
		return GitHubOAuthTokens{}, f.refreshErr
	}
	return f.refreshed, nil
}

func (f *fakePersonalStarOAuth) ExchangeUserCode(
	_ context.Context, code, pkceVerifier, _ string,
) (GitHubOAuthTokens, error) {
	f.seenCode, f.seenVerifier = code, pkceVerifier
	if f.exchangeErr != nil {
		return GitHubOAuthTokens{}, f.exchangeErr
	}
	return f.tokens, nil
}

func (f *fakePersonalStarOAuth) GetOAuthUser(_ context.Context, _ string) (GitHubOAuthUser, error) {
	return f.user, nil
}

func newPersonalStarTestService(
	t *testing.T,
	store *Store,
	secrets PersonalStarSecrets,
	client PersonalStarOAuthClient,
	available bool,
) *PersonalStarOAuthService {
	t.Helper()
	service := NewPersonalStarOAuthService(store, secrets, PersonalStarOAuthSettings{
		ClientID:     "star-client-id",
		ClientSecret: "star-client-secret",
		CallbackURL:  "https://kandev.example" + PersonalStarCallbackPath,
	}, available)
	service.client = client
	return service
}

func startPersonalStarConsent(t *testing.T, service *PersonalStarOAuthService, returnPath string) string {
	t.Helper()
	result, err := service.Start(context.Background(), PersonalStarStartRequest{
		WorkspaceID:    "ws-1",
		UserID:         "user-1",
		SessionSubject: "user-1",
		SessionID:      "session-1",
		ReturnPath:     returnPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(result.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("scope") != PersonalStarScope || query.Get("code_challenge_method") != PKCEChallengeMethodS256 {
		t.Fatalf("authorization query = %v", query)
	}
	if query.Get("redirect_uri") != "https://kandev.example"+PersonalStarCallbackPath {
		t.Fatalf("redirect_uri = %q", query.Get("redirect_uri"))
	}
	if !result.StateExpiresAt.After(time.Now()) {
		t.Fatalf("state expiry = %v", result.StateExpiresAt)
	}
	state := query.Get("state")
	if _, err := base64.RawURLEncoding.DecodeString(state); err != nil {
		t.Fatalf("state %q is not RawURLEncoding: %v", state, err)
	}
	return state
}

func TestPersonalStarConsentCommitsDedicatedConnection(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)

	state := startPersonalStarConsent(t, service, "/settings/plugins?canvas=1")
	outcome, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Outcome != personalStarOutcomeConnected || !outcome.Active || !outcome.Capable {
		t.Fatalf("callback outcome = %+v", outcome)
	}
	if outcome.ReturnPath != "/settings/plugins?canvas=1" {
		t.Fatalf("return path = %q", outcome.ReturnPath)
	}
	connection, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || connection == nil {
		t.Fatalf("connection = %+v, %v", connection, err)
	}
	if connection.CredentialGeneration < 1 || !connection.PublicRepo || connection.Login != "octocat" {
		t.Fatalf("connection = %+v", connection)
	}
	if !strings.Contains(connection.AccessSecretID, fmt.Sprint(connection.CredentialGeneration)) {
		t.Fatalf("access secret %q is not generation-specific", connection.AccessSecretID)
	}
	if secrets.values[connection.AccessSecretID] != "access-1" {
		t.Fatal("access token was not staged under the connection's secret id")
	}
	if client.seenVerifier == "" || client.seenCode != "code-1" {
		t.Fatalf("exchange received code %q verifier %q", client.seenCode, client.seenVerifier)
	}

	resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
		Available: true,
		Settings:  PersonalStarOAuthSettings{ClientID: "star-client-id", ClientSecret: "s", CallbackURL: "https://kandev.example/x"},
	})
	status, err := resolver.ConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available || !status.Active || !status.Capable || status.ReconnectRequired {
		t.Fatalf("resolver status = %+v", status)
	}
	if status.CredentialGeneration != connection.CredentialGeneration {
		t.Fatalf("resolver generation = %d", status.CredentialGeneration)
	}
	if _, err := resolver.ResolvePersonalStarClient(ctx, "ws-1", "user-1"); err != nil {
		t.Fatalf("ResolvePersonalStarClient() error = %v", err)
	}

	// A callback state is single use.
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); !errors.Is(err, ErrPersonalStarFlowUnavailable) {
		t.Fatalf("replayed callback error = %v", err)
	}
}

func TestPersonalStarConsentFailsClosedAfterDisconnect(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)

	state := startPersonalStarConsent(t, service, "/settings/plugins")
	if err := service.DisconnectPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); !errors.Is(err, ErrPersonalStarFence) {
		t.Fatalf("fenced callback error = %v", err)
	}
	connection, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if connection == nil || connection.Status != PersonalStarStatusInactive {
		t.Fatalf("connection after fence = %+v", connection)
	}

	resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
		Available: true,
		Settings:  PersonalStarOAuthSettings{ClientID: "id", ClientSecret: "s", CallbackURL: "https://example/x"},
	})
	status, err := resolver.ConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Active || status.Capable || !status.ReconnectRequired {
		t.Fatalf("post-disconnect status = %+v", status)
	}
	if _, err := resolver.ResolvePersonalStarClient(ctx, "ws-1", "user-1"); !errors.Is(err, ErrGitHubPersonalRequired) {
		t.Fatalf("resolve after disconnect error = %v", err)
	}
}

func TestPersonalStarConsentRequiresPublicRepoScope(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", GrantedScopes: []string{"read:user"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)

	state := startPersonalStarConsent(t, service, "/settings/plugins")
	outcome, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Outcome != personalStarOutcomeInsufficientScope || outcome.Active || outcome.Capable {
		t.Fatalf("reduced-scope outcome = %+v", outcome)
	}
	connection, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if connection != nil {
		t.Fatalf("reduced-scope consent committed %+v", connection)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("reduced-scope consent staged secrets: %v", secrets.values)
	}
}

func TestPersonalStarConsentKeepsPriorGrantOnFailedReplacement(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}

	// A declined re-consent must not invalidate the committed generation.
	state = startPersonalStarConsent(t, service, "/settings/plugins")
	outcome, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Error: "access_denied", ErrorDescription: "user declined"},
		"user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Outcome != personalStarOutcomeDeclined || !outcome.Active || !outcome.Capable {
		t.Fatalf("declined outcome = %+v", outcome)
	}
	after, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if after == nil || after.Status != PersonalStarStatusActive || after.AccessSecretID != before.AccessSecretID {
		t.Fatalf("declined re-consent replaced the grant: %+v", after)
	}
	if secrets.deleted != nil {
		t.Fatalf("declined re-consent deleted secrets: %v", secrets.deleted)
	}

	resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
		Available: true,
		Settings:  PersonalStarOAuthSettings{ClientID: "id", ClientSecret: "s", CallbackURL: "https://example/x"},
	})
	status, err := resolver.ConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || !status.Capable || !status.ReconnectRequired {
		t.Fatalf("active-with-failed-reconnect status = %+v", status)
	}
}

func TestPersonalStarConsentUnavailableDeploymentInventsNoState(t *testing.T) {
	store := newTestStore(t)
	service := newPersonalStarTestService(t, store, newFakePersonalStarSecrets(), &fakePersonalStarOAuth{}, false)
	if _, err := service.Start(context.Background(), PersonalStarStartRequest{
		WorkspaceID: "ws-1", UserID: "user-1", SessionSubject: "user-1", SessionID: "session-1",
		ReturnPath: "/settings/plugins",
	}); !errors.Is(err, ErrGitHubNotConfigured) {
		t.Fatalf("Start() error = %v", err)
	}
	lifecycle, err := store.GetPersonalStarLifecycle(context.Background(), "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.Revision != 0 {
		t.Fatalf("unavailable start bumped the lifecycle: %+v", lifecycle)
	}
}

func TestParseGitHubStarCallbackRawQueryVectors(t *testing.T) {
	canonicalState := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("s", personalStarStateBytes)))
	valid, err := ParseGitHubStarCallbackRawQuery("state=" + canonicalState + "&code=%252F")
	if err != nil {
		t.Fatal(err)
	}
	if valid.Code != "%2F" {
		t.Fatalf("one-decode code = %q, want %%2F", valid.Code)
	}
	if valid.State != canonicalState {
		t.Fatalf("state = %q", valid.State)
	}

	denied, err := ParseGitHubStarCallbackRawQuery(
		"error=access_denied&error_description=user%20declined&error_uri=https%3A%2F%2Fexample&state=" + canonicalState)
	if err != nil {
		t.Fatal(err)
	}
	if !denied.Denied() || denied.ErrorDescription != "user declined" || denied.ErrorURI != "https://example" {
		t.Fatalf("denied query = %+v", denied)
	}

	for name, rawQuery := range map[string]string{
		"percent-encoded state":  fmt.Sprintf("state=%%%02X%s&code=1", canonicalState[0], canonicalState[1:]),
		"plus in state":          "state=" + canonicalState + "+&code=1",
		"padded state":           "state=" + canonicalState + "=&code=1",
		"duplicate state":        "state=" + canonicalState + "&state=" + canonicalState + "&code=1",
		"unknown key":            "state=" + canonicalState + "&code=1&extra=1",
		"mixed code and error":   "state=" + canonicalState + "&code=1&error=access_denied",
		"neither code nor error": "state=" + canonicalState,
		"empty code":             "state=" + canonicalState + "&code=",
		"missing state":          "code=1",
		"short state":            "state=AAAA&code=1",
		"malformed escape":       "state=" + canonicalState + "&code=%zz",
	} {
		if _, err := ParseGitHubStarCallbackRawQuery(rawQuery); err == nil {
			t.Fatalf("ParseGitHubStarCallbackRawQuery(%s) succeeded, want rejection", name)
		}
	}
}

func TestPersonalStarPKCEPairValidation(t *testing.T) {
	verifier, err := randomPersonalStarValue(strings.NewReader(strings.Repeat("a", 64)), personalStarVerifierBytes, base64.RawURLEncoding)
	if err != nil {
		t.Fatal(err)
	}
	challenge := personalStarPKCEChallenge(verifier)
	if !ValidPersonalStarPKCEPair(verifier, challenge) {
		t.Fatal("ValidPersonalStarPKCEPair() rejected a derived pair")
	}
	if ValidPersonalStarPKCEPair(verifier, personalStarPKCEChallenge(verifier+"x")) {
		t.Fatal("ValidPersonalStarPKCEPair() accepted a substituted challenge")
	}
	if ValidPersonalStarPKCEPair(strings.Repeat("a", 42), challenge) {
		t.Fatal("ValidPersonalStarPKCEPair() accepted a short verifier")
	}
	if ValidPersonalStarPKCEPair(verifier, challenge+"=") {
		t.Fatal("ValidPersonalStarPKCEPair() accepted a padded challenge")
	}
}

func TestPersonalStarReturnPathValidation(t *testing.T) {
	if _, err := validatePersonalStarReturnPath("/settings/plugins?tab=installed"); err != nil {
		t.Fatalf("valid return path rejected: %v", err)
	}
	for name, returnPath := range map[string]string{
		"relative":  "settings/plugins",
		"protocol":  "//evil.example/settings/plugins",
		"scheme":    "https://evil.example/settings/plugins",
		"fragment":  "/settings/plugins#x",
		"outside":   "/settings/general",
		"traversal": "/settings/plugins/../../admin",
		"empty":     "",
	} {
		if _, err := validatePersonalStarReturnPath(returnPath); err == nil {
			t.Fatalf("validatePersonalStarReturnPath(%s) accepted %q", name, returnPath)
		}
	}
}

func TestStarSecretCleanupRunnerRetriesThenTerminates(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	secrets.values["stale-secret"] = "token"
	secrets.failOn = "stale-secret"
	if err := store.EnqueuePersonalStarSecretCleanup(ctx, "ws-1", "user-1", "stale-secret", "github-personal-star", 1); err != nil {
		t.Fatal(err)
	}
	runner := NewStarSecretCleanupRunner(store, secrets, nil)
	now := time.Now()
	runner.now = func() time.Time { return now }

	if deleted, err := runner.RunOnce(ctx); err != nil || deleted != 0 {
		t.Fatalf("RunOnce() = %d, %v", deleted, err)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil || len(records) != 1 {
		t.Fatalf("cleanup records = %d, %v", len(records), err)
	}
	if records[0].Attempts != 1 || records[0].Status != personalStarCleanupPending {
		t.Fatalf("record after failure = %+v", records[0])
	}

	secrets.failOn = ""
	now = now.Add(2 * time.Hour)
	if deleted, err := runner.RunOnce(ctx); err != nil || deleted != 1 {
		t.Fatalf("RunOnce() after recovery = %d, %v", deleted, err)
	}
	records, err = store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil || len(records) != 0 {
		t.Fatalf("cleanup records after success = %d, %v", len(records), err)
	}
	if len(secrets.deleted) != 1 {
		t.Fatalf("deleted secrets = %v", secrets.deleted)
	}

	secrets.failOn = "second-secret"
	secrets.values["second-secret"] = "token"
	if err := store.EnqueuePersonalStarSecretCleanup(ctx, "ws-1", "user-1", "second-secret", "github-personal-star", 2); err != nil {
		t.Fatal(err)
	}
	runner.store = store
	record, err := store.GetPersonalStarSecretCleanup(ctx, cleanupRecordID(t, store))
	if err != nil || record == nil {
		t.Fatalf("record = %+v, %v", record, err)
	}
	for attempt := 0; attempt < personalStarCleanupMaxAttempts; attempt++ {
		now = now.Add(24 * time.Hour)
		if _, err := runner.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	record, err = store.GetPersonalStarSecretCleanup(ctx, cleanupRecordID(t, store))
	if err != nil || record == nil {
		t.Fatalf("record after retries = %+v, %v", record, err)
	}
	if record.Status != personalStarCleanupTerminal {
		t.Fatalf("record after retries = %+v, want terminal", record)
	}
}

func cleanupRecordID(t *testing.T, store *Store) string {
	t.Helper()
	records, err := store.ListPersonalStarSecretCleanups(context.Background(), "ws-1")
	if err != nil || len(records) == 0 {
		t.Fatalf("cleanup records = %d, %v", len(records), err)
	}
	return records[0].ID
}

// scriptedStarClient returns a provider failure on the first call so the
// unauthorized refresh path is exercised end to end.
type scriptedStarClient struct {
	calls int
	err   error
	value bool
}

func (c *scriptedStarClient) IsStarred(context.Context, GitHubRepositoryRef) (bool, error) {
	c.calls++
	if c.err != nil {
		failure := c.err
		c.err = nil
		return false, failure
	}
	return c.value, nil
}

func (c *scriptedStarClient) Star(context.Context, GitHubRepositoryRef) (bool, error) {
	return c.IsStarred(context.Background(), GitHubRepositoryRef{})
}

func (c *scriptedStarClient) Unstar(context.Context, GitHubRepositoryRef) (bool, error) {
	_, err := c.IsStarred(context.Background(), GitHubRepositoryRef{})
	return false, err
}

func TestPersonalStarRefreshRotatesSecretsAndPreservesScopes(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}

	client.refreshed = GitHubOAuthTokens{
		AccessToken: "access-2", RefreshToken: "refresh-2", GrantedScopes: []string{"public_repo"},
	}
	if err := service.RefreshPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	after, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if after.CredentialGeneration <= before.CredentialGeneration {
		t.Fatalf("generation did not advance: %d -> %d", before.CredentialGeneration, after.CredentialGeneration)
	}
	if client.seenRefresh != "refresh-1" {
		t.Fatalf("refresh used %q, want the stored refresh token", client.seenRefresh)
	}
	if !after.PublicRepo || !strings.Contains(after.GrantedScopes, PersonalStarScope) {
		t.Fatalf("refresh dropped the grant: %+v", after)
	}
	if secrets.values[after.AccessSecretID] != "access-2" || secrets.values[after.RefreshSecretID] != "refresh-2" {
		t.Fatalf("refreshed tokens were not staged: %v", secrets.values)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	supersededIDs := make([]string, 0, len(records))
	for _, record := range records {
		supersededIDs = append(supersededIDs, record.SecretID)
	}
	if len(records) != 2 ||
		!slices.Contains(supersededIDs, before.AccessSecretID) ||
		!slices.Contains(supersededIDs, before.RefreshSecretID) {
		t.Fatalf("superseded secret cleanup rows = %+v", supersededIDs)
	}

	// A refresh that loses public_repo disables the capability instead of
	// falling back to another credential source.
	client.refreshed = GitHubOAuthTokens{AccessToken: "access-3", GrantedScopes: []string{"read:user"}}
	if err := service.RefreshPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
		Available: true,
		Settings:  PersonalStarOAuthSettings{ClientID: "id", ClientSecret: "s", CallbackURL: "https://example/x"},
	})
	status, err := resolver.ConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || status.Capable || !status.ReconnectRequired {
		t.Fatalf("reduced-scope refresh status = %+v", status)
	}
	if _, err := resolver.ResolvePersonalStarClient(ctx, "ws-1", "user-1"); !errors.Is(err, ErrGitHubCapabilityDenied) {
		t.Fatalf("resolve after reduced-scope refresh error = %v", err)
	}
}

func TestPersonalStarRepositoryStatusRefreshesOnceAfterUnauthorized(t *testing.T) {
	ctx := context.Background()
	sqlxDB := newProviderTestDatabase(t)
	secrets := newFakePersonalStarSecrets()
	oauthClient := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service, cleanup, err := Provide(sqlxDB, sqlxDB, nil, secrets, nil, newTestLogger(), PersonalStarDeployment{
		Available: true,
		Settings: PersonalStarOAuthSettings{
			ClientID:    "client-id",
			CallbackURL: "https://kandev.example/api/v1/github/user/starred/oauth/callback",
		},
		ReadClientSecret: func(context.Context) (string, error) { return "client-secret", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	starOAuth := NewPersonalStarOAuthService(service.TestStore(), secrets, PersonalStarOAuthSettings{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		CallbackURL:  "https://kandev.example/api/v1/github/user/starred/oauth/callback",
	}, true)
	starOAuth.client = oauthClient
	service.SetPersonalStarOAuthCoordinator(starOAuth)
	if err := service.SeedPersonalStarConnectionForE2E(ctx, "ws-1", "user-1", "octocat", true, personalStarOutcomeConnected); err != nil {
		t.Fatal(err)
	}
	secrets.values[personalStarRefreshSecretID("ws-1", "user-1", 1)] = "refresh-1"

	scripted := &scriptedStarClient{err: &GitHubAPIError{StatusCode: 401}, value: true}
	service.SetPersonalStarClientBuilder(func(_ string, _, _ string) PersonalStarClient { return scripted })
	oauthClient.refreshed = GitHubOAuthTokens{AccessToken: "access-2", GrantedScopes: []string{"public_repo"}}

	states, err := service.RepositoryStarStatus(ctx, "ws-1", "user-1", []string{"https://github.com/kdlbs/kandev"})
	if err != nil {
		t.Fatal(err)
	}
	state := states["kdlbs/kandev"]
	if state == nil || !*state {
		t.Fatalf("state after refresh retry = %+v", states)
	}
	if scripted.calls != 2 || oauthClient.refreshCalls != 1 {
		t.Fatalf("provider calls = %d, refresh calls = %d; want one rejected attempt plus one retry", scripted.calls, oauthClient.refreshCalls)
	}
}

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
