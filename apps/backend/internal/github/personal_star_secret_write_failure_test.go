package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPersonalStarConsentCleansUpUnacknowledgedSecretWrites(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		failedValue string
	}{
		{name: "access token", failedValue: "consented-access"},
		{name: "renewal token", failedValue: "consented-refresh"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			store := newTestStore(t)
			secrets := newFakePersonalStarSecrets()
			client := &fakePersonalStarOAuth{
				tokens: GitHubOAuthTokens{
					AccessToken: "consented-access", RefreshToken: "consented-refresh",
					GrantedScopes: []string{PersonalStarScope},
				},
				user: GitHubOAuthUser{ID: 42, Login: "octocat"},
			}
			service := newPersonalStarTestService(t, store, secrets, client, true)
			state := startPersonalStarConsent(t, service, "/settings/plugins")
			secrets.writeThenFailValue = testCase.failedValue

			outcome, err := service.CompleteCallback(ctx,
				GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1")
			if err != nil || outcome.Outcome != personalStarOutcomeFailed || outcome.Active {
				t.Fatalf("callback outcome = %+v, %v", outcome, err)
			}
			assertNoPersonalStarTokenPrefix(t, secrets, "consented-")
		})
	}
}

func TestPersonalStarRefreshCleansUpUnacknowledgedSecretWrites(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		failedValue string
	}{
		{name: "access token", failedValue: "refreshed-access"},
		{name: "renewal token", failedValue: "refreshed-refresh"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			store := newTestStore(t)
			secrets := newFakePersonalStarSecrets()
			client := &fakePersonalStarOAuth{
				tokens: GitHubOAuthTokens{
					AccessToken: "consented-access", RefreshToken: "consented-refresh",
					GrantedScopes: []string{PersonalStarScope},
				},
				user: GitHubOAuthUser{ID: 42, Login: "octocat"},
			}
			service := newPersonalStarTestService(t, store, secrets, client, true)
			previous := seedConnectedPersonalStar(t, service, client)
			service.client = &fakePersonalStarOAuth{
				refreshed: GitHubOAuthTokens{
					AccessToken: "refreshed-access", RefreshToken: "refreshed-refresh",
					GrantedScopes: []string{PersonalStarScope},
				},
			}
			secrets.writeThenFailValue = testCase.failedValue

			if err := service.RefreshPersonalStarConnection(ctx, "ws-1", "user-1"); err == nil {
				t.Fatal("refresh succeeded after a secret write returned an error")
			}
			current, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
			if err != nil || current == nil || current.AccessSecretID != previous.AccessSecretID ||
				current.RefreshSecretID != previous.RefreshSecretID {
				t.Fatalf("connection after rejected refresh = %+v, %v", current, err)
			}
			if secrets.values[previous.AccessSecretID] != "consented-access" ||
				secrets.values[previous.RefreshSecretID] != "consented-refresh" {
				t.Fatalf("previous credentials changed: %v", secrets.values)
			}
			assertNoPersonalStarTokenPrefix(t, secrets, "refreshed-")
		})
	}
}

func assertNoPersonalStarTokenPrefix(t *testing.T, secrets *fakePersonalStarSecrets, prefix string) {
	t.Helper()
	for id, value := range secrets.values {
		if strings.HasPrefix(value, prefix) {
			t.Errorf("uncommitted token remains under %q: %q", id, value)
		}
	}
}

type failOncePersonalStarSecretExists struct {
	*fakePersonalStarSecrets
	failNext bool
}

func (f *failOncePersonalStarSecretExists) Exists(ctx context.Context, id string) (bool, error) {
	if f.failNext {
		f.failNext = false
		return false, errors.New("secret store unavailable")
	}
	return f.fakePersonalStarSecrets.Exists(ctx, id)
}

func TestPersonalStarUnacknowledgedWriteQueuesCleanupWhenExistenceIsUnavailable(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	stored := newFakePersonalStarSecrets()
	secrets := &failOncePersonalStarSecretExists{fakePersonalStarSecrets: stored}
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "consented-access", RefreshToken: "consented-refresh",
			GrantedScopes: []string{PersonalStarScope},
		},
		user: GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	state := startPersonalStarConsent(t, service, "/settings/plugins")
	stored.writeThenFailValue = "consented-access"
	secrets.failNext = true

	outcome, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1")
	if err != nil || outcome.Outcome != personalStarOutcomeFailed {
		t.Fatalf("callback outcome = %+v, %v", outcome, err)
	}
	accessID := personalStarAccessSecretID("ws-1", "user-1", 1)
	if stored.values[accessID] != "consented-access" {
		t.Fatalf("ambiguous write was not retained for durable cleanup: %v", stored.values)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil || len(records) != 1 || records[0].SecretID != accessID {
		t.Fatalf("cleanup records = %+v, %v", records, err)
	}

	runner := NewStarSecretCleanupRunner(store, secrets, nil)
	deleted, err := runner.RunOnce(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("cleanup RunOnce() = %d, %v", deleted, err)
	}
	if _, exists := stored.values[accessID]; exists {
		t.Fatalf("queued token survived cleanup: %v", stored.values)
	}
	if records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1"); err != nil || len(records) != 0 {
		t.Fatalf("cleanup records after recovery = %+v, %v", records, err)
	}
}
func TestPersonalStarUnacknowledgedWriteCleanupSurvivesRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	secrets.writeThenFailValue = "consented-access"
	secrets.cancelAfterWrite = cancel
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "consented-access", RefreshToken: "consented-refresh",
			GrantedScopes: []string{PersonalStarScope},
		},
		user: GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	state := startPersonalStarConsent(t, service, "/settings/plugins")

	_, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1")
	if err == nil {
		t.Fatal("callback unexpectedly succeeded after its context was cancelled during Set")
	}
	for id, value := range secrets.values {
		if value == "consented-access" {
			t.Fatalf("unacknowledged token survived request cancellation under %q", id)
		}
	}
}

func TestPersonalStarCleanupCompletesWhenSecretIsAlreadyAbsent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	if err := store.EnqueuePersonalStarSecretCleanup(
		ctx, "ws-1", "user-1", "missing-secret", personalStarSecretKindAccess, 1,
	); err != nil {
		t.Fatal(err)
	}

	runner := NewStarSecretCleanupRunner(store, secrets, nil)
	deleted, err := runner.RunOnce(ctx)
	if err != nil || deleted != 0 {
		t.Fatalf("cleanup RunOnce() = %d, %v; want no deletion", deleted, err)
	}
	if len(secrets.deleted) != 0 {
		t.Fatalf("cleanup attempted to delete an absent secret: %v", secrets.deleted)
	}
	if records, err := store.ListPersonalStarSecretCleanups(ctx, "ws-1"); err != nil || len(records) != 0 {
		t.Fatalf("cleanup records after absent secret = %+v, %v", records, err)
	}
}
