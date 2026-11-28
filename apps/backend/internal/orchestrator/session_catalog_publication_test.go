package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type catalogWorkspaceContextKey struct{}

// catalogFanoutManager is a capability-complete catalogRefreshRuntime fake.
// Each session's probe returns its own task-owned catalog.
type catalogFanoutManager struct {
	*mockAgentManager
	mu          sync.Mutex
	evidence    map[string]sessioncatalog.ExecutionEvidence
	probes      map[string]int
	savedSource string
	observed    func(string)
	probe       func(context.Context, string) (sessioncatalog.TaskProbeReply, error)
}

func (m *catalogFanoutManager) CurrentCatalogSavedSource(context.Context, string) (string, error) {
	return m.savedSource, nil
}

func (m *catalogFanoutManager) CatalogRefreshExecution(_ context.Context, sessionID, savedSource string) (sessioncatalog.ExecutionEvidence, error) {
	if m.observed != nil {
		m.observed(sessionID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	evidence := m.evidence[sessionID]
	evidence.SavedSourceCurrent = evidence.SavedSourceCurrent && savedSource == m.savedSource
	return evidence, nil
}

func (m *catalogFanoutManager) DiscoverTaskCatalogCandidates(ctx context.Context, sessionID string, owner sessioncatalog.Owner) (sessioncatalog.TaskProbeReply, error) {
	m.mu.Lock()
	m.probes[sessionID]++
	expected := m.evidence[sessionID].Owner
	m.mu.Unlock()
	if owner != expected {
		return sessioncatalog.TaskProbeReply{}, sessioncatalog.ErrTaskProbeOwner
	}
	if m.probe != nil {
		return m.probe(ctx, sessionID)
	}
	return taskOwnedReply(sessionID), nil
}

func (m *catalogFanoutManager) probeCount(sessionID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.probes[sessionID]
}

func taskOwnedReply(sessionID string) sessioncatalog.TaskProbeReply {
	return sessioncatalog.TaskProbeReply{Capability: "fixture", Result: sessioncatalog.Discovery{
		Outcome: sessioncatalog.OutcomeComplete,
		Models: sessioncatalog.Surface{Present: true, Supported: true, Choices: []sessioncatalog.Choice{
			{ID: "model-of-" + sessionID, Name: "Model of " + sessionID},
		}},
	}}
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
	manager := &catalogFanoutManager{mockAgentManager: &mockAgentManager{}, evidence: map[string]sessioncatalog.ExecutionEvidence{}, probes: map[string]int{}, savedSource: "saved-source"}
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
	manager.mu.Lock()
	manager.evidence[sessionID] = sessioncatalog.ExecutionEvidence{Owner: owner, ProfileID: "catalog-profile", AgentID: "codex-app-server", ExecutorType: "worktree", Initialized: true, SavedSourceCurrent: true}
	manager.mu.Unlock()
}

func catalogFanoutGuard(ctx context.Context, write func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return write()
}

// @covers AC-AGENTS-SESSION-CATALOG-001.4
// @covers AC-AGENTS-SESSION-CATALOG-001.7
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
	var deliveredMu sync.Mutex
	delivered := map[string]bool{}
	subscription, err := service.eventBus.Subscribe(events.BuildSessionCatalogRefreshWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		update := event.Data.(sessioncatalog.Update)
		current, readErr := repo.GetTaskSession(ctx, update.SessionID)
		require.NoError(t, readErr)
		record, ok := loadCatalogObservation(current.Metadata[sessioncatalog.MetadataKey])
		require.True(t, ok)
		require.Equal(t, record, update.Record)
		deliveredMu.Lock()
		delivered[update.SessionID] = true
		deliveredMu.Unlock()
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	logCore, logs := observer.New(zap.WarnLevel)
	service.logger, err = logger.NewFromZap(zap.New(logCore))
	require.NoError(t, err)
	publicationErr := service.PublishProfileCatalog(ctx, "catalog-profile", "codex-app-server", catalogFanoutGuard)
	require.Error(t, publicationErr, "partial persistence failure must not claim all-session publication")
	require.NotContains(t, publicationErr.Error(), "TOKEN")
	require.NotContains(t, publicationErr.Error(), "own-001")
	failures := logs.FilterMessage("session catalog publication failed").All()
	require.Len(t, failures, 1, "each failed session is logged once")
	fields := failures[0].ContextMap()
	require.Equal(t, "commit_failed", fields["reason"])
	require.Equal(t, "catalog-profile", fields["profile_id"])
	require.Equal(t, "own-001", fields["session_id"])
	require.NotEmpty(t, fields["task_id"])
	require.NotEmpty(t, fields["execution_id"])
	for _, entry := range logs.All() {
		encoded, encodeErr := json.Marshal(entry.ContextMap())
		require.NoError(t, encodeErr)
		require.NotContains(t, entry.Message+string(encoded), "TOKEN", "logs never carry storage or provider error text")
	}
	require.Zero(t, manager.probeCount("foreign-session"), "an unauthorized task must not reach a task-side request")
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
		require.Equal(t, sessioncatalog.SourceTaskProbeV1, record.Source)
		require.Equal(t, []sessioncatalog.Choice{{ID: "model-of-" + sessionID, Name: "Model of " + sessionID}}, record.Models, "each session receives only its own task-owned catalog")
		require.Nil(t, record.Modes, "an unsupported mode surface is not an empty supported catalog")
		require.True(t, delivered[sessionID], sessionID)
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.4
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
	manager.probe = func(_ context.Context, sessionID string) (sessioncatalog.TaskProbeReply, error) {
		authorized = false
		return taskOwnedReply(sessionID), nil
	}
	require.NoError(t, service.PublishProfileCatalog(ctx, "catalog-profile", "codex-app-server", catalogFanoutGuard))
	current, err := repo.GetTaskSession(ctx, "own-000")
	require.NoError(t, err)
	require.Nil(t, current.Metadata[sessioncatalog.MetadataKey], "authorization observed before the task probe cannot authorize the later write")
}

// @covers AC-AGENTS-SESSION-CATALOG-001.6
func TestSessionCatalogRefreshCancellationPreservesCommittedDelivery(t *testing.T) {
	service, repo, manager := catalogFanoutFixture(t, 3)
	scoped := context.WithValue(context.Background(), catalogWorkspaceContextKey{}, "ws1")
	ctx, cancel := context.WithCancel(scoped)
	t.Cleanup(cancel)
	committed := make(chan struct{})
	manager.probe = func(probeCtx context.Context, sessionID string) (sessioncatalog.TaskProbeReply, error) {
		if sessionID == "own-000" {
			return taskOwnedReply(sessionID), nil
		}
		<-committed
		cancel()
		<-probeCtx.Done()
		return sessioncatalog.TaskProbeReply{}, probeCtx.Err()
	}
	delivered := false
	subscription, err := service.eventBus.Subscribe(events.BuildSessionCatalogRefreshWildcardSubject(), func(ctx context.Context, event *bus.Event) error {
		require.NoError(t, ctx.Err(), "a committed observation still needs a usable post-commit delivery context")
		require.Equal(t, "ws1", ctx.Value(catalogWorkspaceContextKey{}))
		update := event.Data.(sessioncatalog.Update)
		require.Equal(t, "own-000", update.SessionID)
		delivered = true
		close(committed)
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })
	require.ErrorIs(t, service.PublishProfileCatalog(ctx, "catalog-profile", "codex-app-server", catalogFanoutGuard), context.Canceled)
	first, err := repo.GetTaskSession(scoped, "own-000")
	require.NoError(t, err)
	require.NotNil(t, first.Metadata[sessioncatalog.MetadataKey])
	for _, sessionID := range []string{"own-001", "own-002"} {
		current, readErr := repo.GetTaskSession(scoped, sessionID)
		require.NoError(t, readErr)
		require.Nil(t, current.Metadata[sessioncatalog.MetadataKey], "request cancellation stops the remaining fan-out")
	}
	require.True(t, delivered)
	require.Zero(t, manager.probeCount("own-000")-1, "the committed session was probed exactly once")
}

type staleCatalogRefreshKey struct{}

// A superseded refresh can still hold an in-flight instance's single helper
// slot while agentctl reaps it. Its cancelled fan-out must start no further
// probe, and the current refresh must treat busy as contention: no write, the
// prior task-owned record intact, and an unpublished partial result.
// @covers AC-AGENTS-SESSION-CATALOG-001.6
// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogRefreshBusyHelperFromSupersededFanout(t *testing.T) {
	service, repo, manager := catalogFanoutFixture(t, 3)
	scoped := context.WithValue(context.Background(), catalogWorkspaceContextKey{}, "ws1")
	require.NoError(t, service.PublishProfileCatalog(scoped, "catalog-profile", "codex-app-server", catalogFanoutGuard))
	var reasonsMu sync.Mutex
	var committedReasons []string
	subscription, err := service.eventBus.Subscribe(events.BuildSessionCatalogRefreshWildcardSubject(), func(_ context.Context, event *bus.Event) error {
		reasonsMu.Lock()
		defer reasonsMu.Unlock()
		committedReasons = append(committedReasons, event.Data.(sessioncatalog.Update).Record.Reason)
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	var slotMu sync.Mutex
	staleHeld := map[string]bool{}
	staleStarted := make(chan struct{}, catalogRefreshConcurrency)
	var busyReplies atomic.Int32
	manager.probe = func(ctx context.Context, sessionID string) (sessioncatalog.TaskProbeReply, error) {
		slotMu.Lock()
		if staleHeld[sessionID] {
			slotMu.Unlock()
			busyReplies.Add(1)
			return sessioncatalog.TaskProbeReply{}, sessioncatalog.ErrTaskProbeBusy
		}
		if ctx.Value(staleCatalogRefreshKey{}) == nil {
			slotMu.Unlock()
			reply := taskOwnedReply(sessionID)
			reply.Result.Models.Choices = []sessioncatalog.Choice{{ID: "fresh-" + sessionID, Name: "Fresh " + sessionID}}
			return reply, nil
		}
		// The stale helper keeps the slot after its request ends, as agentctl
		// does while it reaps a cancelled helper.
		staleHeld[sessionID] = true
		slotMu.Unlock()
		staleStarted <- struct{}{}
		<-ctx.Done()
		return sessioncatalog.TaskProbeReply{}, ctx.Err()
	}

	staleCtx, supersede := context.WithCancel(context.WithValue(scoped, staleCatalogRefreshKey{}, true))
	defer supersede()
	staleErr := make(chan error, 1)
	go func() {
		staleErr <- service.PublishProfileCatalog(staleCtx, "catalog-profile", "codex-app-server", catalogFanoutGuard)
	}()
	for range catalogRefreshConcurrency {
		<-staleStarted
	}
	supersede()
	require.ErrorIs(t, <-staleErr, context.Canceled)
	require.Error(t, service.PublishProfileCatalog(scoped, "catalog-profile", "codex-app-server", catalogFanoutGuard),
		"sessions skipped as busy must not be reported as published")

	require.Equal(t, int32(catalogRefreshConcurrency), busyReplies.Load(), "busy is reported once per held slot, never retried")
	slotMu.Lock()
	require.Len(t, staleHeld, catalogRefreshConcurrency)
	require.False(t, staleHeld["own-002"], "a superseded refresh starts no further task probe")
	slotMu.Unlock()
	reasonsMu.Lock()
	require.NotContains(t, committedReasons, sessioncatalog.ReasonTaskDiscoveryFailed, "busy is never committed as a failed discovery")
	reasonsMu.Unlock()
	for _, sessionID := range []string{"own-000", "own-001", "own-002"} {
		current, readErr := repo.GetTaskSession(scoped, sessionID)
		require.NoError(t, readErr)
		record, ok := loadCatalogObservation(current.Metadata[sessioncatalog.MetadataKey])
		require.True(t, ok, sessionID)
		require.Equal(t, sessioncatalog.SourceTaskProbeV1, record.Source, sessionID)
		if staleHeld[sessionID] {
			require.Equal(t, uint64(1), record.Revision, sessionID)
			require.Equal(t, []sessioncatalog.Choice{{ID: "model-of-" + sessionID, Name: "Model of " + sessionID}}, record.Models, "a busy session keeps its prior task-owned record")
			continue
		}
		require.Equal(t, uint64(2), record.Revision, sessionID)
		require.Equal(t, []sessioncatalog.Choice{{ID: "fresh-" + sessionID, Name: "Fresh " + sessionID}}, record.Models, sessionID)
	}
}
