package secrets

import (
	"context"
	"errors"
	"testing"
)

func TestUserVisibleStoreHidesInternalGitHubSecrets(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	internal := &SecretWithValue{
		Secret: Secret{ID: "github:user:workspace-1:user-1:access", Name: "personal token"},
		Value:  "personal-secret",
	}
	visible := &SecretWithValue{Secret: Secret{ID: "user-secret", Name: "user token"}, Value: "user-value"}
	if err := store.Create(ctx, internal); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, visible); err != nil {
		t.Fatal(err)
	}

	wrapped := NewUserVisibleStore(store)
	items, err := wrapped.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != visible.ID {
		t.Fatalf("visible secrets = %+v, want only %q", items, visible.ID)
	}
	for _, operation := range []func() error{
		func() error { _, err := wrapped.Get(ctx, internal.ID); return err },
		func() error { _, err := wrapped.Reveal(ctx, internal.ID); return err },
		func() error { return wrapped.Update(ctx, internal.ID, &UpdateSecretRequest{}) },
		func() error { return wrapped.Delete(ctx, internal.ID) },
	} {
		if err := operation(); !errors.Is(err, ErrNotFound) {
			t.Fatalf("internal operation error = %v, want not found", err)
		}
	}
	if got, err := store.Reveal(ctx, internal.ID); err != nil || got != "personal-secret" {
		t.Fatalf("raw internal reveal = %q, %v", got, err)
	}
}

func TestUserVisibleStoreRejectsInternalCreate(t *testing.T) {
	store := newTestSQLiteStore(t)
	err := NewUserVisibleStore(store).Create(context.Background(), &SecretWithValue{
		Secret: Secret{ID: "github:workspace:workspace-1:pat", Name: "PAT"}, Value: "secret",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Create() error = %v, want not found", err)
	}
}

func TestUserVisibleStoreHidesAutomationWebhookSecretsFromCredentials(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	internal := &SecretWithValue{
		Secret: Secret{ID: "automation-webhook:binding-1", Name: "webhook signing secret"},
		Value:  "webhook-secret",
	}
	visible := &SecretWithValue{Secret: Secret{ID: "user-secret", Name: "user token"}, Value: "user-value"}
	for _, secret := range []*SecretWithValue{internal, visible} {
		if err := store.Create(ctx, secret); err != nil {
			t.Fatalf("create %s: %v", secret.ID, err)
		}
	}

	wrapped := NewUserVisibleStore(store)
	items, err := wrapped.List(ctx)
	if err != nil {
		t.Fatalf("list visible secrets: %v", err)
	}
	if len(items) != 1 || items[0].ID != visible.ID {
		t.Fatalf("visible secrets = %+v, want only %q", items, visible.ID)
	}

	for _, operation := range []func() error{
		func() error { _, err := wrapped.Reveal(ctx, internal.ID); return err },
		func() error { return wrapped.Update(ctx, internal.ID, &UpdateSecretRequest{}) },
		func() error { return wrapped.Delete(ctx, internal.ID) },
	} {
		if err := operation(); !errors.Is(err, ErrNotFound) {
			t.Fatalf("internal operation error = %v, want not found", err)
		}
	}

	provider := NewSecretStoreProvider(wrapped)
	available, err := provider.ListAvailable(ctx)
	if err != nil {
		t.Fatalf("list credential choices: %v", err)
	}
	if len(available) != 1 || available[0] != visible.ID {
		t.Fatalf("available credentials = %v, want only %q", available, visible.ID)
	}
	if _, err := provider.GetCredential(ctx, internal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("internal credential selection error = %v, want not found", err)
	}
}

func TestIsInternalIDRecognizesKubernetesRuntimeSecrets(t *testing.T) {
	t.Parallel()

	if !IsInternalID("kandev-runtime:execution-1:agentctl-auth-token") {
		t.Fatal("Kubernetes runtime secret ID must be internal")
	}
	if IsInternalID("legacy-random-runtime-secret-id") {
		t.Fatal("legacy random secret ID must remain user-visible")
	}
	if !IsInternalID("  AUTOMATION-WEBHOOK:binding-1 ") {
		t.Fatal("automation webhook secret ID must be internal")
	}
}

func TestUserVisibleStoreWorkspaceOperationsFilterInternalSecrets(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	global := &SecretWithValue{Secret: Secret{ID: "global-visible", Name: "global"}, Value: "global-value"}
	workspace := &SecretWithValue{Secret: Secret{
		ID: "workspace-visible", Name: "workspace", Scope: ScopeWorkspace, WorkspaceID: "workspace-a",
	}, Value: "workspace-value"}
	internal := &SecretWithValue{Secret: Secret{
		ID: "github:internal-workspace", Name: "internal", Scope: ScopeWorkspace, WorkspaceID: "workspace-a",
	}, Value: "internal-value"}
	for _, secret := range []*SecretWithValue{global, workspace, internal} {
		if err := store.Create(ctx, secret); err != nil {
			t.Fatalf("create %s: %v", secret.ID, err)
		}
	}

	wrapped := NewUserVisibleStore(store)
	scoped, ok := wrapped.(ScopedSecretStore)
	if !ok {
		t.Fatal("user-visible store is not scoped")
	}
	items, err := scoped.ListScoped(ctx, SecretListOptions{
		Scope: ScopeWorkspace, WorkspaceID: "workspace-a", IncludeGlobal: true,
	})
	if err != nil {
		t.Fatalf("list workspace options: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("visible workspace items = %#v, want global and workspace entries", items)
	}
	if _, err := scoped.GetForWorkspace(ctx, workspace.ID, "workspace-a"); err != nil {
		t.Fatalf("get workspace secret: %v", err)
	}
	if _, err := scoped.GetForWorkspace(ctx, internal.ID, "workspace-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get internal workspace secret = %v, want not found", err)
	}
	if got, err := scoped.RevealForWorkspace(ctx, workspace.ID, "workspace-a"); err != nil || got != workspace.Value {
		t.Fatalf("reveal workspace secret = %q, %v", got, err)
	}

	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin cleanup transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	transactional, ok := wrapped.(WorkspaceSecretTransactionalDeleter)
	if !ok {
		t.Fatal("user-visible store is not transactional")
	}
	if err := transactional.DeleteWorkspaceSecretsTx(ctx, tx, "workspace-a"); err != nil {
		t.Fatalf("transactional workspace cleanup: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit cleanup transaction: %v", err)
	}
	if _, err := store.Get(ctx, workspace.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("workspace secret after cleanup = %v, want not found", err)
	}
	if _, err := store.Get(ctx, internal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("internal workspace secret after cleanup = %v, want not found", err)
	}
}

// TestUserVisibleStoreHidesDedicatedPersonalStarTokens pins the invariant that
// the dedicated OAuth App token namespace is backend-owned: the star feature
// stores access and refresh tokens as `kandev-runtime:github-personal-star:*`
// rows, and no user-visible surface may list, read, mutate, transfer, or select
// them as an agent credential.
func TestUserVisibleStoreHidesDedicatedPersonalStarTokens(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	access := &SecretWithValue{
		Secret: Secret{ID: "kandev-runtime:github-personal-star:access:ws-1:user-1:1", Name: "github personal star access token"},
		Value:  "oauth-access-token",
	}
	refresh := &SecretWithValue{
		Secret: Secret{ID: "kandev-runtime:github-personal-star:refresh:ws-1:user-1:1", Name: "github personal star refresh token"},
		Value:  "oauth-refresh-token",
	}
	visible := &SecretWithValue{Secret: Secret{ID: "user-secret", Name: "user token"}, Value: "user-value"}
	for _, secret := range []*SecretWithValue{access, refresh, visible} {
		if err := store.Create(ctx, secret); err != nil {
			t.Fatalf("create %s: %v", secret.ID, err)
		}
	}

	wrapped := NewUserVisibleStore(store)
	items, err := wrapped.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != visible.ID {
		t.Fatalf("visible secrets = %+v, want only %q", items, visible.ID)
	}

	for name, operation := range map[string]func() error{
		"Get":    func() error { _, err := wrapped.Get(ctx, access.ID); return err },
		"Reveal": func() error { _, err := wrapped.Reveal(ctx, access.ID); return err },
		"Update": func() error { return wrapped.Update(ctx, access.ID, &UpdateSecretRequest{}) },
		"Delete": func() error { return wrapped.Delete(ctx, access.ID) },
		"Create": func() error {
			return wrapped.Create(ctx, &SecretWithValue{
				Secret: Secret{ID: "kandev-runtime:github-personal-star:access:ws-1:user-1:2", Name: "star token"},
				Value:  "x",
			})
		},
	} {
		if err := operation(); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s error = %v, want not found", name, err)
		}
	}

	scoped, ok := wrapped.(SecretTransferStore)
	if !ok {
		t.Fatal("wrapped store does not expose scope transfers")
	}
	if _, err := scoped.CopyScoped(ctx, access.ID, "", ScopeGlobal, "", nil, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CopyScoped error = %v, want not found", err)
	}
	if _, err := scoped.MoveScoped(ctx, refresh.ID, "", ScopeGlobal, "", nil, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("MoveScoped error = %v, want not found", err)
	}

	// The provider used for agent credential selection must reject the same ids.
	provider := NewSecretStoreProvider(wrapped)
	for _, id := range []string{access.ID, refresh.ID} {
		if _, err := provider.GetCredential(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("credential selection error for %s = %v, want not found", id, err)
		}
	}
	available, err := provider.ListAvailable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range available {
		if id == access.ID || id == refresh.ID {
			t.Fatalf("credential selection listed %s", id)
		}
	}

	// Integration services keep direct access to the rows they own.
	if got, err := store.Reveal(ctx, access.ID); err != nil || got != "oauth-access-token" {
		t.Fatalf("raw reveal = %q, %v", got, err)
	}
	if !IsInternalID(access.ID) || !IsInternalID(refresh.ID) {
		t.Fatal("dedicated star token ids are not classified as internal")
	}
}
