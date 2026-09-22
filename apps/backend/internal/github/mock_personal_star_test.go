package github

import (
	"context"
	"testing"
)

func TestPersonalStarE2EControlsRequireBothGates(t *testing.T) {
	mock := NewMockClient()
	if personalStarE2EControlsEnabled(nil, func(string) string { return "true" }) {
		t.Fatal("star controls were enabled without a MockClient")
	}
	if personalStarE2EControlsEnabled(mock, nil) {
		t.Fatal("star controls were enabled without an environment reader")
	}
	if personalStarE2EControlsEnabled(mock, func(string) string { return "false" }) {
		t.Fatal("star controls were enabled without the E2E selector")
	}
	if !personalStarE2EControlsEnabled(mock, func(key string) string {
		if key == "KANDEV_E2E_MOCK" {
			return "true"
		}
		return ""
	}) {
		t.Fatal("star controls were disabled with both gates satisfied")
	}
}

func TestMockPersonalStarClientReflectsPerUserStateAndFailures(t *testing.T) {
	ctx := context.Background()
	mock := NewMockClient()
	repository, err := ParseGitHubRepositoryURL("https://github.com/kdlbs/kandev")
	if err != nil {
		t.Fatal(err)
	}
	client := mock.PersonalStarClientFor("ws-1", "user-1")
	other := mock.PersonalStarClientFor("ws-1", "user-2")

	starred, err := client.IsStarred(ctx, repository)
	if err != nil || starred {
		t.Fatalf("IsStarred() = %v, %v; want false, nil", starred, err)
	}
	mock.SetPersonalStarState("ws-1", "user-1", repository.String(), true)
	if starred, err = client.IsStarred(ctx, repository); err != nil || !starred {
		t.Fatalf("IsStarred() = %v, %v; want true, nil", starred, err)
	}
	if starred, err = other.IsStarred(ctx, repository); err != nil || starred {
		t.Fatalf("another user's IsStarred() = %v, %v; want false, nil", starred, err)
	}

	mock.SetPersonalStarFailures(true, false)
	if _, err := client.IsStarred(ctx, repository); err == nil {
		t.Fatal("IsStarred() ignored the injected status failure")
	}
	mock.SetPersonalStarFailures(false, false)
	if starred, err = client.Unstar(ctx, repository); err != nil || starred {
		t.Fatalf("Unstar() = %v, %v; want false, nil", starred, err)
	}
	mock.SetPersonalStarFailures(false, true)
	if _, err := client.Star(ctx, repository); err == nil {
		t.Fatal("Star() ignored the injected mutation failure")
	}
	mock.SetPersonalStarFailures(false, false)
	if starred, err = client.IsStarred(ctx, repository); err != nil || starred {
		t.Fatalf("state after a failed mutation = %v, %v; want false, nil", starred, err)
	}
}

func TestSeededPersonalStarConnectionServesMockProviderState(t *testing.T) {
	ctx := context.Background()
	sqlxDB := newProviderTestDatabase(t)
	log := newTestLogger()
	secrets := newFakePersonalStarSecrets()
	mock := NewMockClient()

	service, cleanup, err := Provide(sqlxDB, sqlxDB, nil, secrets, nil, log, PersonalStarDeployment{
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
	service.SetPersonalStarClientBuilder(func(_ string, workspaceID, userID string) PersonalStarClient {
		return mock.PersonalStarClientFor(workspaceID, userID)
	})

	if err := service.SeedPersonalStarConnectionForE2E(ctx, "ws-1", "user-1", "octocat", true, personalStarOutcomeConnected); err != nil {
		t.Fatal(err)
	}
	status, err := service.PersonalStarConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available || !status.Active || !status.Capable || status.ReconnectRequired {
		t.Fatalf("seeded status = %+v", status)
	}

	states, err := service.RepositoryStarStatus(ctx, "ws-1", "user-1", []string{"https://github.com/kdlbs/kandev"})
	if err != nil {
		t.Fatal(err)
	}
	state, ok := states["kdlbs/kandev"]
	if !ok || state == nil || *state {
		t.Fatalf("initial status = %+v", states)
	}
	if _, err := service.StarRepository(ctx, "ws-1", "user-1", "https://github.com/kdlbs/kandev"); err != nil {
		t.Fatal(err)
	}
	states, err = service.RepositoryStarStatus(ctx, "ws-1", "user-1", []string{"https://github.com/kdlbs/kandev"})
	if err != nil {
		t.Fatal(err)
	}
	if state = states["kdlbs/kandev"]; state == nil || !*state {
		t.Fatalf("starred status = %+v", states)
	}

	mock.SetPersonalStarFailures(true, false)
	states, err = service.RepositoryStarStatus(ctx, "ws-1", "user-1", []string{"https://github.com/kdlbs/kandev"})
	if err != nil {
		t.Fatal(err)
	}
	if state, ok := states["kdlbs/kandev"]; !ok || state != nil {
		t.Fatalf("failed provider call status = %+v, want a nil (unknown) state", states)
	}
}

func TestSeedPersonalStarConnectionRequiresPermissionForStarCalls(t *testing.T) {
	ctx := context.Background()
	sqlxDB := newProviderTestDatabase(t)
	secrets := newFakePersonalStarSecrets()
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

	if err := service.SeedPersonalStarConnectionForE2E(ctx, "ws-1", "user-1", "octocat", false, personalStarOutcomeInsufficientScope); err != nil {
		t.Fatal(err)
	}
	status, err := service.PersonalStarConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || status.Capable || !status.ReconnectRequired {
		t.Fatalf("reduced-scope status = %+v", status)
	}
	if _, err := service.StarRepository(ctx, "ws-1", "user-1", "https://github.com/kdlbs/kandev"); err != ErrGitHubCapabilityDenied {
		t.Fatalf("StarRepository() error = %v, want capability denied", err)
	}

	if err := service.ClearPersonalStarConnectionForE2E(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	status, err = service.PersonalStarConnectionStatus(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Active || status.Capable || !status.ReconnectRequired {
		t.Fatalf("cleared status = %+v", status)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("cleared connection left secrets: %v", secrets.values)
	}
}
