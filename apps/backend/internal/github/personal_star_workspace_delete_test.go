package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
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

func TestPersonalStarConsentStartCannotRecreateDeletedWorkspaceState(t *testing.T) {
	ctx := context.Background()
	githubService, _ := newWorkspaceConnectionService(t, "octocat")
	starService := newPersonalStarTestService(t, githubService.store, newFakePersonalStarSecrets(), &fakePersonalStarOAuth{
		tokens: GitHubOAuthTokens{AccessToken: "access-1", GrantedScopes: []string{"public_repo"}},
		user:   GitHubOAuthUser{ID: 42, Login: "octocat"},
	}, true)
	request := PersonalStarStartRequest{
		WorkspaceID: "ws-1", UserID: "user-1", SessionSubject: "user-1",
		SessionID: "session-1", ReturnPath: "/settings/plugins",
	}

	event := bus.NewEvent(events.WorkspaceDeleted, "test", map[string]interface{}{"id": "ws-1"})
	if err := githubService.handleWorkspaceDeleted(ctx, event); err != nil {
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}
	if _, err := starService.Start(ctx, request); !errors.Is(err, errPersonalStarWorkspaceDeleted) {
		t.Fatalf("Start() after workspace deletion error = %v, want workspace-deleted fence", err)
	}
	var lifecycleCount, flowCount int
	if err := githubService.store.db.GetContext(ctx, &lifecycleCount,
		`SELECT COUNT(*) FROM github_personal_star_lifecycle WHERE workspace_id = 'ws-1'`); err != nil {
		t.Fatal(err)
	}
	if err := githubService.store.db.GetContext(ctx, &flowCount,
		`SELECT COUNT(*) FROM github_personal_star_flow WHERE workspace_id = 'ws-1'`); err != nil {
		t.Fatal(err)
	}
	if lifecycleCount != 0 || flowCount != 0 {
		t.Fatalf("deleted workspace state was recreated: lifecycles=%d flows=%d", lifecycleCount, flowCount)
	}
}

func TestPersonalStarFlowCreationCannotCrossWorkspaceDeletion(t *testing.T) {
	ctx := context.Background()
	githubService, _ := newWorkspaceConnectionService(t, "octocat")
	revision, generation, err := githubService.store.BumpPersonalStarLifecycle(ctx, "ws-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	event := bus.NewEvent(events.WorkspaceDeleted, "test", map[string]interface{}{"id": "ws-1"})
	if err := githubService.handleWorkspaceDeleted(ctx, event); err != nil {
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}
	now := time.Now().UTC()
	err = githubService.store.CreatePersonalStarFlow(ctx, &PersonalStarFlow{
		StateDigest: "pending-flow-digest", FlowKind: PersonalStarFlowKind,
		WorkspaceID: "ws-1", UserID: "user-1", SessionSubject: "user-1",
		ExpectedRevision: revision, ExpectedGeneration: generation,
		CodeVerifier: "verifier", CodeChallenge: "challenge",
		RedirectURI: "https://kandev.example" + PersonalStarCallbackPath,
		ReturnPath:  "/settings/plugins", CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	if !errors.Is(err, errPersonalStarWorkspaceDeleted) {
		t.Fatalf("CreatePersonalStarFlow() after deletion error = %v, want workspace-deleted fence", err)
	}
	var flowCount int
	if err := githubService.store.db.GetContext(ctx, &flowCount,
		`SELECT COUNT(*) FROM github_personal_star_flow WHERE workspace_id = 'ws-1'`); err != nil {
		t.Fatal(err)
	}
	if flowCount != 0 {
		t.Fatalf("flow rows after deletion = %d, want 0", flowCount)
	}
}

type blockingPersonalStarOAuth struct {
	entered chan struct{}
	release chan struct{}
}

func (c *blockingPersonalStarOAuth) ExchangeUserCode(
	context.Context, string, string, string,
) (GitHubOAuthTokens, error) {
	close(c.entered)
	<-c.release
	return GitHubOAuthTokens{AccessToken: "access-1", GrantedScopes: []string{"public_repo"}}, nil
}

func (c *blockingPersonalStarOAuth) GetOAuthUser(context.Context, string) (GitHubOAuthUser, error) {
	return GitHubOAuthUser{ID: 42, Login: "octocat"}, nil
}

func (c *blockingPersonalStarOAuth) RefreshUserToken(context.Context, string) (GitHubOAuthTokens, error) {
	return GitHubOAuthTokens{}, errors.New("refresh is not used in this test")
}

func TestPersonalStarCallbackCannotCommitAfterWorkspaceDeletion(t *testing.T) {
	ctx := context.Background()
	githubService, _ := newWorkspaceConnectionService(t, "octocat")
	secrets := newFakePersonalStarSecrets()
	client := &blockingPersonalStarOAuth{entered: make(chan struct{}), release: make(chan struct{})}
	starService := newPersonalStarTestService(t, githubService.store, secrets, client, true)
	state := startPersonalStarConsent(t, starService, "/settings/plugins")

	type callbackResult struct {
		outcome PersonalStarCallbackOutcome
		err     error
	}
	callbackDone := make(chan callbackResult, 1)
	go func() {
		outcome, err := starService.CompleteCallback(ctx,
			GitHubStarCallbackQuery{State: state, Code: "code-1"}, "user-1", "session-1")
		callbackDone <- callbackResult{outcome: outcome, err: err}
	}()
	select {
	case <-client.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("callback did not reach token exchange")
	}

	event := bus.NewEvent(events.WorkspaceDeleted, "test", map[string]interface{}{"id": "ws-1"})
	if err := githubService.handleWorkspaceDeleted(ctx, event); err != nil {
		close(client.release)
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}
	close(client.release)
	var result callbackResult
	select {
	case result = <-callbackDone:
	case <-time.After(5 * time.Second):
		t.Fatal("callback did not finish after token exchange resumed")
	}
	if !errors.Is(result.err, errPersonalStarWorkspaceDeleted) || result.outcome.Active {
		t.Fatalf("callback after workspace deletion = %+v, want workspace-deleted rejection", result)
	}
	if connection, err := githubService.store.GetPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil || connection != nil {
		t.Fatalf("connection after deletion = %+v, %v", connection, err)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("staged tokens remain after rejected callback: %v", secrets.values)
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
