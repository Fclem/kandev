package github

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestListActivePRWatches_ExcludesArchivedTasks(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t-active", false)
	seedTask(t, store, "t-archived", true)

	mustCreateWatch(t, store, "s-active", "t-active")
	mustCreateWatch(t, store, "s-archived", "t-archived")

	watches, err := svc.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("expected 1 watch (active only), got %d", len(watches))
	}
	if watches[0].TaskID != "t-active" {
		t.Errorf("expected watch for t-active, got %q", watches[0].TaskID)
	}
}

func TestListActivePRWatches_ExcludesOrphanedWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	// Orphan: watch exists but no matching task row (task was hard-deleted).
	mustCreateWatch(t, store, "s-orphan", "t-gone")

	watches, err := svc.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list watches: %v", err)
	}
	if len(watches) != 0 {
		t.Fatalf("expected orphaned watch to be excluded, got %d watches", len(watches))
	}
}

func TestHandleTaskUpdated_ArchiveDeletesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")
	if err := store.CreateTaskPR(ctx, &TaskPR{TaskID: "t1", WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: 1}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Simulate task-service publishing task.updated with archived_at set.
	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id":     "t1",
		"archived_at": "2026-04-19T12:00:00Z",
	})
	if err := svc.handleTaskUpdated(ctx, event); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted after archive event, got %+v", got)
	}
	// AC4: archive must not delete the task/PR association, only the watch.
	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "t1"); len(got) != 1 {
		t.Errorf("task PRs after archive event = %d, want 1 (association retained)", len(got))
	}
}

func TestHandleTaskUpdated_NonArchiveUpdateLeavesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	// Regular edit: no archived_at in payload.
	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id": "t1",
		"title":   "Edited title",
	})
	if err := svc.handleTaskUpdated(ctx, event); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got == nil {
		t.Error("expected watch to persist after non-archive update")
	}
}

func TestHandleTaskDeleted_DeletesWatches(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{
		"task_id": "t1",
	})
	if err := svc.handleTaskDeleted(ctx, event); err != nil {
		t.Fatalf("handleTaskDeleted: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted after delete event, got %+v", got)
	}
}

func TestHandleTaskDeletedRevokesCredentialLeases(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	broker := revocationTestBroker("workspace-1", "t1", "s1")
	svc.SetCredentialBroker(broker)

	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{
		"task_id": "t1",
	})
	if err := svc.handleTaskDeleted(context.Background(), event); err != nil {
		t.Fatalf("handleTaskDeleted: %v", err)
	}
	if got := broker.ActiveLeaseCount(); got != 0 {
		t.Fatalf("lease records = %d, want 0", got)
	}
}

func TestHandleTaskEvents_MalformedPayloadIsNoop(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")
	if err := store.CreateTaskPR(ctx, &TaskPR{TaskID: "t1", WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: 1}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}

	// Nil event — should be ignored, not crash.
	if err := svc.handleTaskDeleted(ctx, nil); err != nil {
		t.Fatalf("handleTaskDeleted nil event: %v", err)
	}

	// Wrong payload type — should be ignored, not crash.
	bad := bus.NewEvent(events.TaskUpdated, "task-service", "not-a-map")
	if err := svc.handleTaskUpdated(ctx, bad); err != nil {
		t.Fatalf("handleTaskUpdated: %v", err)
	}

	// Missing task_id — should be ignored.
	empty := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{})
	if err := svc.handleTaskDeleted(ctx, empty); err != nil {
		t.Fatalf("handleTaskDeleted: %v", err)
	}

	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got == nil {
		t.Error("expected watch to persist when payload is malformed")
	}
	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "t1"); len(got) != 1 {
		t.Errorf("task PRs after malformed events = %d, want 1", len(got))
	}
}

func TestHandleWorkspaceDeletedRemovesOnlyOwnedConnectionSecrets(t *testing.T) {
	svc, secrets := newWorkspaceConnectionService(t, "octocat")
	ctx := context.Background()
	if err := svc.store.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID:            "ws-1",
		TaskGitCredentialsMode: TaskGitCredentialsModeExecutor,
	}); err != nil {
		t.Fatalf("seed workspace settings: %v", err)
	}
	secrets.values[WorkspacePATSecretKey("ws-1")] = "pat"
	secrets.values[UserAccessTokenSecretKey("ws-1", "user-1")] = "access"
	secrets.values[UserRefreshTokenSecretKey("ws-1", "user-1")] = "refresh"
	secrets.values[WorkspacePATSecretKey("ws-2")] = "other"

	event := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-1"})
	if err := svc.handleWorkspaceDeleted(ctx, event); err != nil {
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}
	var settingsCount int
	if err := svc.store.db.GetContext(ctx, &settingsCount,
		`SELECT COUNT(1) FROM github_workspace_settings WHERE workspace_id = ?`, "ws-1"); err != nil {
		t.Fatalf("count workspace settings: %v", err)
	}
	if settingsCount != 0 {
		t.Fatalf("workspace settings rows = %d, want 0", settingsCount)
	}
	for _, id := range []string{
		WorkspacePATSecretKey("ws-1"),
		UserAccessTokenSecretKey("ws-1", "user-1"),
		UserRefreshTokenSecretKey("ws-1", "user-1"),
	} {
		if _, ok := secrets.values[id]; ok {
			t.Fatalf("workspace-owned secret %q was not deleted", id)
		}
	}
	if got := secrets.values[WorkspacePATSecretKey("ws-2")]; got != "other" {
		t.Fatalf("unrelated workspace secret changed: %q", got)
	}
}

func TestHandleWorkspaceDeletedRevokesCredentialLeasesWithoutSecretStore(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	broker := revocationTestBroker("workspace-1", "t1", "s1")
	svc.SetCredentialBroker(broker)
	svc.connectionSecrets = nil

	event := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "workspace-1"})
	if err := svc.handleWorkspaceDeleted(context.Background(), event); err != nil {
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}
	if got := broker.ActiveLeaseCount(); got != 0 {
		t.Fatalf("lease records = %d, want 0", got)
	}
}

func TestSubscribeTaskEventsTerminalSessionRevokesCredentialLease(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	broker := revocationTestBroker("workspace-1", "t1", "s1")
	svc.SetCredentialBroker(broker)
	svc.subscribeTaskEvents()
	t.Cleanup(svc.unsubscribeTaskEvents)

	event := bus.NewEvent(events.TaskSessionStateChanged, "orchestrator", map[string]interface{}{
		"task_id":    "t1",
		"session_id": "s1",
		"new_state":  string(taskmodels.TaskSessionStateCompleted),
	})
	if err := svc.eventBus.Publish(context.Background(), events.TaskSessionStateChanged, event); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := broker.ActiveLeaseCount(); got != 0 {
		t.Fatalf("lease records = %d, want 0", got)
	}
}

func TestSubscribeTaskEvents_EndToEnd(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	svc.subscribeTaskEvents()

	seedTask(t, store, "t1", false)
	mustCreateWatch(t, store, "s1", "t1")

	event := bus.NewEvent(events.TaskUpdated, "task-service", map[string]interface{}{
		"task_id":     "t1",
		"archived_at": "2026-04-19T12:00:00Z",
	})
	if err := svc.eventBus.Publish(ctx, events.TaskUpdated, event); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// MemoryEventBus delivers synchronously, so the watch should already be gone.
	if got, _ := store.GetPRWatchBySession(ctx, "s1"); got != nil {
		t.Errorf("expected watch to be deleted by subscribed handler, got %+v", got)
	}
}

// mustCreateWatch is a test helper for creating a PR watch with minimal boilerplate.
func mustCreateWatch(t *testing.T, store *Store, sessionID, taskID string) {
	t.Helper()
	w := &PRWatch{
		SessionID: sessionID,
		TaskID:    taskID,
		Owner:     "owner",
		Repo:      "repo",
		PRNumber:  0,
		Branch:    "main",
	}
	if err := store.CreatePRWatch(context.Background(), w); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
}

func revocationTestBroker(workspaceID, taskID, sessionID string) *CredentialBroker {
	connection := &WorkspaceConnection{
		WorkspaceID: workspaceID, Source: ConnectionSourcePAT, Status: ConnectionStatusActive,
	}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{workspaceID: connection}}
	broker := NewCredentialBroker(connections, NewCredentialResolver(connections, fakeAuthSecrets{
		WorkspacePATSecretKey(workspaceID): "transient",
	}), &fakeBrokerAuthorizer{})
	if _, err := broker.Issue(context.Background(), CredentialLeaseRequest{
		WorkspaceID: workspaceID, TaskID: taskID, SessionID: sessionID, RepositoryID: "repository-1",
		Owner: "owner", Repo: "repo", Host: defaultGitHubHost,
	}); err != nil {
		panic(err)
	}
	return broker
}

// TestHandleWorkspaceDeletedQueuesDedicatedStarTokenCleanup pins the workspace
// teardown contract for the dedicated OAuth App connection: the Global-scope
// access and refresh token rows must be recorded in the durable cleanup queue
// before the connection rows that name them are deleted, so the tokens are
// still deleted after the workspace is gone.
func TestHandleWorkspaceDeletedQueuesDedicatedStarTokenCleanup(t *testing.T) {
	svc, secrets := newWorkspaceConnectionService(t, "octocat")
	ctx := context.Background()
	if err := svc.SeedPersonalStarConnectionForE2E(ctx, "ws-1", "user-1", "octocat", true, personalStarOutcomeConnected); err != nil {
		t.Fatal(err)
	}
	connection, err := svc.store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || connection == nil {
		t.Fatalf("connection = %+v, %v", connection, err)
	}
	if _, ok := secrets.values[connection.AccessSecretID]; !ok {
		t.Fatalf("seeded access secret %q is missing", connection.AccessSecretID)
	}

	event := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-1"})
	if err := svc.handleWorkspaceDeleted(ctx, event); err != nil {
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}

	if connection, err := svc.store.GetPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil || connection != nil {
		t.Fatalf("connection after deletion = %+v, %v", connection, err)
	}
	records, err := svc.store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	queued := make(map[string]bool, len(records))
	for _, record := range records {
		queued[record.SecretID] = true
		if record.Status != personalStarCleanupPending {
			t.Fatalf("queued record = %+v, want pending", record)
		}
	}
	if !queued[connection.AccessSecretID] {
		t.Fatalf("access secret %q was not queued: %v", connection.AccessSecretID, queued)
	}
	if connection.RefreshSecretID != "" && !queued[connection.RefreshSecretID] {
		t.Fatalf("refresh secret %q was not queued: %v", connection.RefreshSecretID, queued)
	}

	// The seed writes only the access secret; cleanup must also complete the
	// queued refresh ID without counting it as a deletion.
	runner := NewStarSecretCleanupRunner(svc.store, secrets, nil)
	if deleted, err := runner.RunOnce(ctx); err != nil || deleted != 1 {
		t.Fatalf("RunOnce() = %d, %v; want one stored secret deleted", deleted, err)
	}
	if remaining, err := svc.store.ListPersonalStarSecretCleanups(ctx, "ws-1"); err != nil || len(remaining) != 0 {
		t.Fatalf("cleanup records after workspace deletion = %+v, %v", remaining, err)
	}
	if _, ok := secrets.values[connection.AccessSecretID]; ok {
		t.Fatal("access token survived workspace deletion")
	}
}

// TestWorkspaceDeletionKeepsPerSecretCleanupMetadata pins that the teardown
// queue labels each token with its own kind and generation. A refresh that
// returns no new refresh token keeps the earlier refresh generation while the
// credential generation advances, so a single shared generation would be wrong.
func TestWorkspaceDeletionKeepsPerSecretCleanupMetadata(t *testing.T) {
	svc, secrets := newWorkspaceConnectionService(t, "octocat")
	ctx := context.Background()
	if err := svc.SeedPersonalStarConnectionForE2E(ctx, "ws-1", "user-1", "octocat", true, personalStarOutcomeConnected); err != nil {
		t.Fatal(err)
	}
	connection, err := svc.store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || connection == nil {
		t.Fatalf("connection = %+v, %v", connection, err)
	}
	// The seed stages the access token only; the refresh token is what this
	// test needs to exercise the refresh path.
	secrets.values[connection.RefreshSecretID] = "refresh-1"

	// A refresh that returns no new refresh token: the access token rotates,
	// the stored refresh token and its generation stay put.
	starOAuth := NewPersonalStarOAuthService(svc.store, secrets, PersonalStarOAuthSettings{
		ClientID: "client-id", ClientSecret: "client-secret", CallbackURL: "https://kandev.example/cb",
	}, true)
	starOAuth.client = &fakePersonalStarOAuth{
		refreshed: GitHubOAuthTokens{AccessToken: "access-2", GrantedScopes: []string{"public_repo"}},
	}
	svc.SetPersonalStarOAuthCoordinator(starOAuth)
	if err := svc.RefreshPersonalStarConnection(ctx, "ws-1", "user-1"); err != nil {
		t.Fatal(err)
	}
	renewed, err := svc.store.GetPersonalStarConnection(ctx, "ws-1", "user-1")
	if err != nil || renewed == nil {
		t.Fatalf("connection = %+v, %v", renewed, err)
	}
	if renewed.AccessSecretGeneration == connection.AccessSecretGeneration {
		t.Fatalf("access generation did not advance: %+v", renewed)
	}
	if renewed.RefreshSecretGeneration != connection.RefreshSecretGeneration {
		t.Fatalf("refresh generation changed without a new token: %+v", renewed)
	}

	event := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-1"})
	if err := svc.handleWorkspaceDeleted(ctx, event); err != nil {
		t.Fatalf("handleWorkspaceDeleted: %v", err)
	}
	records, err := svc.store.ListPersonalStarSecretCleanups(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]PersonalStarSecretCleanup, len(records))
	for _, record := range records {
		byID[record.SecretID] = *record
	}
	liveAccess := byID[renewed.AccessSecretID]
	if liveAccess.SecretKind != personalStarSecretKindAccess || liveAccess.CredentialGeneration != renewed.AccessSecretGeneration {
		t.Fatalf("live access queue row = %+v", liveAccess)
	}
	liveRefresh := byID[renewed.RefreshSecretID]
	if liveRefresh.SecretKind != personalStarSecretKindRefresh || liveRefresh.CredentialGeneration != renewed.RefreshSecretGeneration {
		t.Fatalf("live refresh queue row = %+v", liveRefresh)
	}
	if liveAccess.CredentialGeneration == liveRefresh.CredentialGeneration {
		t.Fatal("the two live secrets were labelled with one shared generation")
	}
}
