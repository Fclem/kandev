package github

import (
	"context"
	"testing"
)

func TestDeletePersonalStarWorkspaceStateQueuesPointersCommittedAfterCleanupSnapshot(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	workspaceID, userID := "ws-1", "user-1"

	revision, generation, err := store.BumpPersonalStarLifecycle(ctx, workspaceID, userID)
	if err != nil {
		t.Fatal(err)
	}
	previous := &PersonalStarConnection{
		WorkspaceID: workspaceID, UserID: userID, Login: "octocat", GitHubUserID: 42,
		GrantedScopes: "public_repo", PublicRepo: true, CredentialGeneration: generation,
		LifecycleRevision: revision, AccessSecretID: personalStarAccessSecretID(workspaceID, userID, generation),
		RefreshSecretID:        personalStarRefreshSecretID(workspaceID, userID, generation),
		AccessSecretGeneration: generation, RefreshSecretGeneration: generation,
	}
	if err := store.InsertPersonalStarConnection(ctx, previous, revision, generation); err != nil {
		t.Fatal(err)
	}

	// Model teardown's initial token snapshot, then let a refresh commit before
	// the workspace rows are removed.
	for _, ref := range personalStarSecretIDsFor(previous) {
		if err := store.EnqueuePersonalStarSecretCleanup(ctx, workspaceID, userID, ref.SecretID, ref.Kind, ref.Generation); err != nil {
			t.Fatal(err)
		}
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
	if _, err := store.ReplacePersonalStarConnection(
		ctx, &replacement,
		PersonalStarFence{Revision: previous.LifecycleRevision, Generation: previous.CredentialGeneration},
		PersonalStarFence{Revision: revision, Generation: generation},
	); err != nil {
		t.Fatal(err)
	}

	if err := store.DeletePersonalStarWorkspaceState(ctx, workspaceID); err != nil {
		t.Fatal(err)
	}
	records, err := store.ListPersonalStarSecretCleanups(ctx, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	queued := make(map[string]bool, len(records))
	for _, record := range records {
		queued[record.SecretID] = true
	}
	for _, ref := range personalStarSecretIDsFor(&replacement) {
		if !queued[ref.SecretID] {
			t.Errorf("committed %s secret %q is not queued for cleanup", ref.Kind, ref.SecretID)
		}
	}
}

func TestReplacePersonalStarConnectionRejectsLifecycleAdvanceBeforeCommit(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	workspaceID, userID := "ws-1", "user-1"
	revision, generation, err := store.BumpPersonalStarLifecycle(ctx, workspaceID, userID)
	if err != nil {
		t.Fatal(err)
	}
	previous := &PersonalStarConnection{
		WorkspaceID: workspaceID, UserID: userID, Login: "octocat", GitHubUserID: 42,
		GrantedScopes: "public_repo", PublicRepo: true, CredentialGeneration: generation,
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
	if _, err := store.db.Exec(`
		CREATE TRIGGER advance_personal_star_lifecycle_after_replace
		AFTER UPDATE ON github_personal_star_connection
		WHEN NEW.workspace_id = 'ws-1' AND NEW.user_id = 'user-1'
		BEGIN
			UPDATE github_personal_star_lifecycle
			SET revision = revision + 1, credential_generation = credential_generation + 1
			WHERE workspace_id = NEW.workspace_id AND user_id = NEW.user_id;
		END`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.ReplacePersonalStarConnection(
		ctx, &replacement,
		PersonalStarFence{Revision: previous.LifecycleRevision, Generation: previous.CredentialGeneration},
		PersonalStarFence{Revision: revision, Generation: generation},
	); err != ErrPersonalStarFence {
		t.Fatalf("replacement error = %v, want lifecycle fence rejection", err)
	}
	committed, err := store.GetPersonalStarConnection(ctx, workspaceID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.AccessSecretID != previous.AccessSecretID || committed.CredentialGeneration != previous.CredentialGeneration {
		t.Fatalf("replacement committed after its fence advanced: %+v", committed)
	}
}
