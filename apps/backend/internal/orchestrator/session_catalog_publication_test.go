package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type catalogWorkspaceContextKey struct{}

type catalogFanoutManager struct {
	*mockAgentManager
	evidence map[string]sessioncatalog.ExecutionEvidence
	observed func(string)
}

func (m *catalogFanoutManager) CatalogRefreshExecution(_ context.Context, sessionID string, _ sessioncatalog.LaunchContext) (sessioncatalog.ExecutionEvidence, error) {
	if m.observed != nil {
		m.observed(sessionID)
	}
	return m.evidence[sessionID], nil
}

type catalogFaultRepository struct {
	*sqliterepo.Repository
	failedSession string
	beforeWrite   func(context.Context, string)
}

func (r *catalogFaultRepository) PublishSessionCatalog(ctx context.Context, sessionID, executionID, profileID string, expected any, next sessioncatalog.Observation) (*sessioncatalog.Observation, error) {
	if sessionID == r.failedSession {
		return nil, errors.New("TOKEN=private-storage-detail")
	}
	if r.beforeWrite != nil {
		r.beforeWrite(ctx, sessionID)
	}
	return r.Repository.PublishSessionCatalog(ctx, sessionID, executionID, profileID, expected, next)
}

func catalogFanoutFixture(t *testing.T, count int) (*Service, *sqliterepo.Repository, *catalogFanoutManager) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Own"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Own"}))
	seedTaskAndSessionInWorkspace(t, repo, "ws1", "wf1", "catalog-own-task", "own-000", "")
	manager := &catalogFanoutManager{mockAgentManager: &mockAgentManager{}, evidence: map[string]sessioncatalog.ExecutionEvidence{}}
	for i := range count {
		id := fmt.Sprintf("own-%03d", i)
		if i > 0 {
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: id, TaskID: "catalog-own-task", State: models.TaskSessionStateIdle}))
		}
		seedCatalogFanoutExecution(t, repo, manager, id)
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	service.eventBus = bus.NewMemoryEventBus(testLogger())
	service.taskAccessCheck = func(ctx context.Context, taskID string) error {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			return err
		}
		if task.WorkspaceID != ctx.Value(catalogWorkspaceContextKey{}) {
			return errors.New("forbidden")
		}
		return nil
	}
	return service, repo, manager
}

func seedCatalogFanoutExecution(t *testing.T, repo *sqliterepo.Repository, manager *catalogFanoutManager, sessionID string) {
	t.Helper()
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID, session.ExecutionProfileID = "routing-parent", "catalog-profile"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, "runtime_config", map[string]any{"model": "selected-model", "mode": "selected-mode"}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, "acp_config_baseline", map[string]any{"effort": "low"}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, "composer_draft", "unsent input"))
	owner := sessioncatalog.Owner{ExecutionID: "execution-" + sessionID, AttemptID: "attempt-" + sessionID, SourceGeneration: 1}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "runtime-" + sessionID, TaskID: session.TaskID, SessionID: sessionID, ExecutionProfileID: "catalog-profile", AgentExecutionID: owner.ExecutionID, Status: models.ExecutorRunningStatusReady}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": owner.ExecutionID, "settings_source_generation": owner.SourceGeneration, "settings_attempt_id": owner.AttemptID}))
	manager.evidence[sessionID] = sessioncatalog.ExecutionEvidence{Owner: owner, ProfileID: "catalog-profile", AgentID: "claude-acp", ExecutorType: "worktree", Initialized: true, Compatible: true}
}

func catalogFanoutCaps() hostutility.AgentCapabilities {
	return hostutility.AgentCapabilities{Status: hostutility.StatusOK, Models: []hostutility.Model{{ID: "candidate-model", Name: "Candidate model"}}, Modes: []hostutility.Mode{{ID: "candidate-mode", Name: "Candidate mode"}}}
}

func catalogFanoutGuard(ctx context.Context, write func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return write()
}

func TestSessionCatalogRefreshBoundedAuthorizedPartialPublication(t *testing.T) {
	service, repo, manager := catalogFanoutFixture(t, 130)
	ctx := context.WithValue(context.Background(), catalogWorkspaceContextKey{}, "ws1")
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "foreign-workspace", Name: "Foreign"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "foreign-workflow", WorkspaceID: "foreign-workspace", Name: "Foreign"}))
	seedTaskAndSessionInWorkspace(t, repo, "foreign-workspace", "foreign-workflow", "foreign-task", "foreign-session", "")
	seedCatalogFanoutExecution(t, repo, manager, "foreign-session")
	faults := &catalogFaultRepository{Repository: repo, failedSession: "own-001"}
	faults.beforeWrite = func(ctx context.Context, sessionID string) {
		if sessionID != "own-002" && sessionID != "own-003" {
			return
		}
		running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		require.NoError(t, err)
		if sessionID == "own-002" {
			running.AgentExecutionID = "replacement-execution"
		} else {
			running.ExecutionProfileID = "other-concrete-profile"
		}
		require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	}
	service.repo = faults
	delivered := map[string]bool{}
	subscription, err := service.eventBus.Subscribe(events.BuildSessionCatalogRefreshWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		update := event.Data.(sessioncatalog.Update)
		current, readErr := repo.GetTaskSession(ctx, update.SessionID)
		require.NoError(t, readErr)
		record, ok := loadCatalogObservation(current.Metadata[sessioncatalog.MetadataKey])
		require.True(t, ok)
		require.Equal(t, record, update.Record)
		delivered[update.SessionID] = true
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	publicationErr := service.PublishProfileCatalog(ctx, "catalog-profile", "claude-acp", hostutility.ProfileProbeContext{}, catalogFanoutCaps(), catalogFanoutGuard)
	require.Error(t, publicationErr, "partial persistence failure must not claim all-session publication")
	require.NotContains(t, publicationErr.Error(), "TOKEN")
	require.NotContains(t, publicationErr.Error(), "own-001")
	for sessionID, evidence := range manager.evidence {
		current, readErr := repo.GetTaskSession(ctx, sessionID)
		require.NoError(t, readErr)
		require.Equal(t, map[string]any{"model": "selected-model", "mode": "selected-mode"}, current.Metadata["runtime_config"])
		require.Equal(t, map[string]any{"effort": "low"}, current.Metadata["acp_config_baseline"])
		require.Equal(t, "unsent input", current.Metadata["composer_draft"])
		blocked := sessionID == "foreign-session" || sessionID == "own-001" || sessionID == "own-002" || sessionID == "own-003"
		if blocked {
			require.Nil(t, current.Metadata[sessioncatalog.MetadataKey], sessionID)
			require.False(t, delivered[sessionID], sessionID)
			continue
		}
		record, ok := loadCatalogObservation(current.Metadata[sessioncatalog.MetadataKey])
		require.True(t, ok, sessionID)
		require.Equal(t, uint64(1), record.Revision)
		require.Equal(t, evidence.Owner, record.Owner)
		require.Equal(t, []sessioncatalog.Choice{{ID: "candidate-model", Name: "Candidate model"}}, record.Models)
		require.Equal(t, []sessioncatalog.Choice{{ID: "candidate-mode", Name: "Candidate mode"}}, record.Modes)
		require.True(t, delivered[sessionID], sessionID)
	}
}

func TestSessionCatalogRefreshReauthorizesObservedTaskBeforeCommit(t *testing.T) {
	service, repo, manager := catalogFanoutFixture(t, 1)
	ctx := context.WithValue(context.Background(), catalogWorkspaceContextKey{}, "ws1")
	authorized := true
	service.taskAccessCheck = func(context.Context, string) error {
		if !authorized {
			return errors.New("permission revoked")
		}
		return nil
	}
	manager.observed = func(string) { authorized = false }
	require.Error(t, service.PublishProfileCatalog(ctx, "catalog-profile", "claude-acp", hostutility.ProfileProbeContext{}, catalogFanoutCaps(), catalogFanoutGuard))
	current, err := repo.GetTaskSession(ctx, "own-000")
	require.NoError(t, err)
	require.Nil(t, current.Metadata[sessioncatalog.MetadataKey], "authorization observed before runtime inspection cannot authorize the later write")
}

func TestSessionCatalogRefreshCancellationPreservesCommittedDelivery(t *testing.T) {
	service, repo, _ := catalogFanoutFixture(t, 2)
	scoped := context.WithValue(context.Background(), catalogWorkspaceContextKey{}, "ws1")
	ctx, cancel := context.WithCancel(scoped)
	t.Cleanup(cancel)
	delivered := false
	subscription, err := service.eventBus.Subscribe(events.BuildSessionCatalogRefreshWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		require.NoError(t, ctx.Err(), "a committed observation still needs a usable post-commit delivery context")
		require.Equal(t, "ws1", ctx.Value(catalogWorkspaceContextKey{}))
		update := event.Data.(sessioncatalog.Update)
		current, readErr := repo.GetTaskSession(ctx, update.SessionID)
		require.NoError(t, readErr)
		require.NotNil(t, current.Metadata[sessioncatalog.MetadataKey])
		delivered = true
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	guard := func(ctx context.Context, write func() error) error {
		if err := write(); err != nil {
			return err
		}
		cancel()
		return nil
	}
	require.ErrorIs(t, service.PublishProfileCatalog(ctx, "catalog-profile", "claude-acp", hostutility.ProfileProbeContext{}, catalogFanoutCaps(), guard), context.Canceled)
	first, err := repo.GetTaskSession(scoped, "own-000")
	require.NoError(t, err)
	require.NotNil(t, first.Metadata[sessioncatalog.MetadataKey])
	second, err := repo.GetTaskSession(scoped, "own-001")
	require.NoError(t, err)
	require.Nil(t, second.Metadata[sessioncatalog.MetadataKey], "request cancellation stops the remaining fan-out")
	require.True(t, delivered)
}
