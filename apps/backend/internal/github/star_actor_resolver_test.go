package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type starActorClientStub struct {
	calls int
	err   error
}

func (c *starActorClientStub) IsStarred(context.Context, GitHubRepositoryRef) (bool, error) {
	c.calls++
	return false, c.err
}

func (c *starActorClientStub) Star(context.Context, GitHubRepositoryRef) (bool, error) {
	c.calls++
	return c.err == nil, c.err
}

func (c *starActorClientStub) Unstar(context.Context, GitHubRepositoryRef) (bool, error) {
	c.calls++
	return false, c.err
}

// installationProviderStub resolves an App installation principal and records
// that it was asked.
type installationProviderStub func()

func (f installationProviderStub) ResolveInstallation(
	context.Context, *WorkspaceConnection, ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	f()
	return &ResolvedCredential{
		Client:     NewTokenClient("installation-token", TokenPrincipal{Kind: TokenCredentialInstallation}),
		Principal:  AuthPrincipal{Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation},
		credential: "installation-token",
	}, nil
}

func TestClassifyOAuthScopes(t *testing.T) {
	cases := []struct {
		name    string
		header  string
		present bool
		capable bool
		remedy  string
	}{
		{"repo scope", "gist, repo, workflow", true, true, ""},
		{"public_repo scope", "public_repo", true, true, ""},
		{"classic token without star scope", "read:org, gist", true, false, StarRemediationTokenScope},
		{"classic token with no scopes", "", true, false, StarRemediationTokenScope},
		{"fine-grained token", "", false, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyOAuthScopes(tc.header, tc.present)
			if got.capable != tc.capable || got.remedy != tc.remedy {
				t.Fatalf("classifyOAuthScopes(%q, %v) = %+v", tc.header, tc.present, got)
			}
		})
	}
}

// newCLIStarService builds a service whose workspace authenticates through a
// gh CLI account, with the dedicated OAuth App unavailable.
func newCLIStarService(t *testing.T, client PersonalStarClient) *Service {
	t.Helper()
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	resolver := NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"ws": {
			WorkspaceID: "ws", Source: ConnectionSourceGHCLI, GitHubHost: "github.com",
			Login: "octocat", Status: ConnectionStatusActive, CredentialGeneration: 3,
		},
	}}, nil)
	resolver.ghToken = func(context.Context, string, string) (string, error) { return "cli-token", nil }
	service.resolver = resolver
	dedicated := NewPersonalStarResolverService(nil, nil, PersonalStarDeploymentStatus{})
	dedicated.SetClientBuilder(func(token, _, _ string) PersonalStarClient {
		if token != "cli-token" {
			t.Fatalf("star client built with token %q, want the gh account token", token)
		}
		return client
	})
	service.SetPersonalStarResolver(dedicated)
	return service
}

func seedStarProbe(service *Service, entry starScopeProbe) {
	service.starCapabilities.storeProbe(starActorKey("ws", string(ConnectionSourceGHCLI), "octocat", 3), entry)
}

func TestStarActorUsesWorkspaceCLIAccountWhenDedicatedUnavailable(t *testing.T) {
	client := &starActorClientStub{}
	service := newCLIStarService(t, client)
	seedStarProbe(service, starScopeProbe{capable: true})

	status, err := service.PersonalStarConnectionStatus(context.Background(), "ws", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available || !status.Active || !status.Capable || status.CredentialGeneration != 3 {
		t.Fatalf("status = %+v, want an available capable actor", status)
	}
	if status.Actor == nil || status.Actor.Source != string(ConnectionSourceGHCLI) ||
		status.Actor.Login != "octocat" || !status.Actor.Shared {
		t.Fatalf("actor = %+v, want shared gh_cli octocat", status.Actor)
	}

	starred, err := service.StarRepository(context.Background(), "ws", "user-1", "https://github.com/kdlbs/kandev")
	if err != nil || !starred || client.calls != 1 {
		t.Fatalf("StarRepository = %v, %v (calls %d)", starred, err, client.calls)
	}
}

func TestStarActorWithoutStarScopeNeverCallsGitHub(t *testing.T) {
	client := &starActorClientStub{}
	service := newCLIStarService(t, client)
	seedStarProbe(service, starScopeProbe{remedy: StarRemediationTokenScope})

	status, err := service.PersonalStarConnectionStatus(context.Background(), "ws", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Capable || status.Remediation != StarRemediationTokenScope || status.Actor == nil {
		t.Fatalf("status = %+v, want incapable actor with token_scope remediation", status)
	}
	if _, err := service.StarRepository(context.Background(), "ws", "user-1", "https://github.com/kdlbs/kandev"); !errors.Is(err, ErrGitHubCapabilityDenied) {
		t.Fatalf("StarRepository error = %v, want capability denied", err)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want none", client.calls)
	}
}

func TestStarActorDenialMarksActorIncapable(t *testing.T) {
	client := &starActorClientStub{err: &GitHubAPIError{StatusCode: http.StatusForbidden}}
	service := newCLIStarService(t, client)
	seedStarProbe(service, starScopeProbe{capable: true})

	if _, err := service.StarRepository(context.Background(), "ws", "user-1", "https://github.com/kdlbs/kandev"); err == nil {
		t.Fatal("StarRepository succeeded, want the provider denial")
	}
	status, err := service.PersonalStarConnectionStatus(context.Background(), "ws", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Capable || status.Remediation != StarRemediationTokenScope {
		t.Fatalf("status = %+v, want denial recorded", status)
	}
	if _, err := service.StarRepository(context.Background(), "ws", "user-1", "https://github.com/kdlbs/kandev"); !errors.Is(err, ErrGitHubCapabilityDenied) {
		t.Fatalf("second StarRepository error = %v, want capability denied", err)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want the denied call only", client.calls)
	}
}

func TestStarActorNeverResolvesAppInstallation(t *testing.T) {
	service := NewService(nil, AuthMethodNone, nil, nil, nil, nil)
	resolver := NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		"ws": {
			WorkspaceID: "ws", Source: ConnectionSourceGitHubAppInstallation, AppRegistrationID: "app-1",
			Status: ConnectionStatusActive, CredentialGeneration: 1,
		},
	}}, nil)
	installationResolved := false
	resolver.SetInstallationProvider(installationProviderStub(func() { installationResolved = true }))
	service.resolver = resolver

	status, err := service.PersonalStarConnectionStatus(context.Background(), "ws", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Actor != nil || status.Available {
		t.Fatalf("status = %+v, want no actor for an App installation", status)
	}
	if installationResolved {
		t.Fatal("star resolution asked for an App installation credential")
	}
}
