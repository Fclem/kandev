package github

import (
	"context"
	"errors"
	"testing"
)

type blockingPersonalStarRefresh struct {
	refreshStarted chan struct{}
	releaseRefresh chan struct{}
	refreshTokens  GitHubOAuthTokens
	consentTokens  GitHubOAuthTokens
	user           GitHubOAuthUser
}

func (b *blockingPersonalStarRefresh) RefreshUserToken(context.Context, string) (GitHubOAuthTokens, error) {
	close(b.refreshStarted)
	<-b.releaseRefresh
	return b.refreshTokens, nil
}

func (b *blockingPersonalStarRefresh) ExchangeUserCode(context.Context, string, string, string) (GitHubOAuthTokens, error) {
	return b.consentTokens, nil
}

func (b *blockingPersonalStarRefresh) GetOAuthUser(context.Context, string) (GitHubOAuthUser, error) {
	return b.user, nil
}

func TestPersonalStarRefreshDoesNotInvalidateConsentStartedDuringExchange(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	secrets := newFakePersonalStarSecrets()
	client := &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{
			AccessToken: "initial-access", RefreshToken: "initial-refresh", GrantedScopes: []string{PersonalStarScope},
		},
		user: GitHubOAuthUser{ID: 42, Login: "octocat"},
	}
	service := newPersonalStarTestService(t, store, secrets, client, true)
	seedConnectedPersonalStar(t, service, client)
	service.client = &blockingPersonalStarRefresh{
		refreshStarted: make(chan struct{}),
		releaseRefresh: make(chan struct{}),
		refreshTokens: GitHubOAuthTokens{
			AccessToken: "refreshed-access", RefreshToken: "refreshed-token", GrantedScopes: []string{PersonalStarScope},
		},
		consentTokens: GitHubOAuthTokens{
			AccessToken: "consented-access", RefreshToken: "consented-refresh", GrantedScopes: []string{PersonalStarScope},
		},
		user: GitHubOAuthUser{ID: 43, Login: "new-octocat"},
	}
	blocking := service.client.(*blockingPersonalStarRefresh)
	refreshResult := make(chan error, 1)
	go func() {
		refreshResult <- service.RefreshPersonalStarConnection(ctx, "ws-1", "user-1")
	}()
	<-blocking.refreshStarted

	state := startPersonalStarConsent(t, service, "/settings/plugins")
	if _, err := service.CompleteCallback(ctx,
		GitHubStarCallbackQuery{State: state, Code: "consent-code"}, "user-1", "session-1"); err != nil {
		t.Fatalf("consent started during refresh did not complete: %v", err)
	}
	close(blocking.releaseRefresh)
	if err := <-refreshResult; !errors.Is(err, ErrPersonalStarFence) {
		t.Fatalf("refresh during a newer consent = %v, want lifecycle fence error", err)
	}

	connection, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || connection == nil {
		t.Fatalf("committed connection = %+v, %v", connection, err)
	}
	if connection.Login != "new-octocat" || secrets.values[connection.AccessSecretID] != "consented-access" ||
		secrets.values[connection.RefreshSecretID] != "consented-refresh" {
		t.Fatalf("new consent was not preserved: connection=%+v, secrets=%v", connection, secrets.values)
	}
}
