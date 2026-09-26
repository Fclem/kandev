package github

import (
	"context"
	"errors"
	"testing"
)

func TestPersonalStarRefreshRejectionExposesReconnectOnlyForTerminalErrors(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		refreshErr       error
		wantReconnectCTA bool
	}{
		{name: "revoked refresh token", refreshErr: &GitHubOAuthError{Code: "invalid_grant"}, wantReconnectCTA: true},
		{name: "transient provider failure", refreshErr: errors.New("provider unavailable"), wantReconnectCTA: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			store := newTestStore(t)
			secrets := newFakePersonalStarSecrets()
			oauth := &fakePersonalStarOAuth{
				tokens: GitHubOAuthTokens{
					AccessToken: "access-1", RefreshToken: "refresh-1", GrantedScopes: []string{PersonalStarScope},
				},
				user: GitHubOAuthUser{ID: 42, Login: "octocat"},
			}
			service := newPersonalStarTestService(t, store, secrets, oauth, true)
			state := startPersonalStarConsent(t, service, "/settings/plugins")
			if _, err := service.CompleteCallback(
				ctx, GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1",
			); err != nil {
				t.Fatal(err)
			}
			before, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
			if err != nil || before == nil {
				t.Fatalf("connection before refresh = %+v, %v", before, err)
			}
			oauth.refreshErr = testCase.refreshErr

			if err := service.RefreshPersonalStarConnection(ctx, "ws-1", "user-1"); !errors.Is(err, ErrGitHubPersonalRequired) {
				t.Fatalf("refresh error = %v, want ErrGitHubPersonalRequired", err)
			}
			after, err := store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
			if err != nil || after == nil {
				t.Fatalf("connection after refresh = %+v, %v", after, err)
			}
			if after.AccessSecretID != before.AccessSecretID || after.RefreshSecretID != before.RefreshSecretID ||
				after.CredentialGeneration != before.CredentialGeneration {
				t.Fatalf("failed refresh replaced the committed grant: before=%+v after=%+v", before, after)
			}
			resolver := NewPersonalStarResolverService(store, secrets, PersonalStarDeploymentStatus{
				Available: true,
				Settings:  PersonalStarOAuthSettings{ClientID: "id", ClientSecret: "secret", CallbackURL: "https://example/cb"},
			})
			status, err := resolver.ConnectionStatus(ctx, "ws-1", "user-1")
			if err != nil {
				t.Fatal(err)
			}
			if status.ReconnectRequired != testCase.wantReconnectCTA {
				t.Fatalf("reconnect_required = %v, want %v (status=%+v)", status.ReconnectRequired, testCase.wantReconnectCTA, status)
			}
			if !status.Active || !status.Capable {
				t.Fatalf("connection status = %+v, want active and capable", status)
			}
		})
	}
}
