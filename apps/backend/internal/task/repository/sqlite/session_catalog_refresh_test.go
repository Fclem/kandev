package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type catalogRefreshRepository interface {
	ListCatalogRefreshSessions(context.Context, string, string, int) ([]*models.TaskSession, error)
	PublishSessionCatalog(context.Context, string, string, string, any, sessioncatalog.Observation) (*sessioncatalog.Observation, error)
}

// @covers AC-AGENTS-SESSION-CATALOG-001.4
// @covers AC-AGENTS-SESSION-CATALOG-001.6
func TestSessionCatalogRefreshConditionalWrite(t *testing.T) {
	repo := newRepoForSessionTests(t)
	exerciseCatalogConditionalWrite(t, repo)
}

func exerciseCatalogConditionalWrite(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	writer, ok := any(repo).(catalogRefreshRepository)
	if !ok {
		t.Fatal("catalog writes cannot guard the current execution and prior observation atomically")
	}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "catalog-task", Title: "Catalog"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "catalog-session", TaskID: "catalog-task", AgentProfileID: "routing-parent", ExecutionProfileID: "concrete-profile", State: models.TaskSessionStateIdle, Metadata: map[string]any{"runtime_config": map[string]any{"model": "selected"}, "draft": "retained"}}))
	running := &models.ExecutorRunning{ID: "catalog-executor", SessionID: "catalog-session", TaskID: "catalog-task", ExecutionProfileID: "concrete-profile", AgentExecutionID: "execution-1", Runtime: agentruntime.Runtime("standalone"), Status: "running"}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, running.SessionID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": "execution-1", "settings_source_generation": 1, "settings_attempt_id": "attempt-1"}))
	sessions, err := writer.ListCatalogRefreshSessions(ctx, "concrete-profile", "", 1)
	require.NoError(t, err)
	require.Len(t, sessions, 1, "initialized idle executions must be enumerable by concrete profile")
	require.Equal(t, "catalog-session", sessions[0].ID)
	empty, err := writer.ListCatalogRefreshSessions(ctx, "routing-parent", "", 1)
	require.NoError(t, err)
	require.Empty(t, empty, "routing parent is not the running concrete profile")
	next := sessioncatalog.Observation{Version: 1, ProfileID: "concrete-profile", Digest: "catalog-1", Owner: sessioncatalog.Owner{ExecutionID: "execution-1", AttemptID: "attempt-1", SourceGeneration: 1}, Disposition: "rediscovery_required", Reason: "executor_scoped"}
	first, err := writer.PublishSessionCatalog(ctx, "catalog-session", "execution-1", "concrete-profile", nil, next)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, uint64(1), first.Revision)
	session, err := repo.GetTaskSession(ctx, "catalog-session")
	require.NoError(t, err)
	require.Equal(t, "retained", session.Metadata["draft"])
	require.Equal(t, map[string]any{"model": "selected"}, session.Metadata["runtime_config"])
	old := session.Metadata[sessioncatalog.MetadataKey]
	stale, err := writer.PublishSessionCatalog(ctx, session.ID, "execution-1", "concrete-profile", nil, next)
	require.NoError(t, err)
	require.Nil(t, stale, "stale prior-record CAS must not allocate a revision")
	running.AgentExecutionID = "execution-2"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	stale, err = writer.PublishSessionCatalog(ctx, session.ID, "execution-1", "concrete-profile", old, next)
	require.NoError(t, err)
	require.Nil(t, stale, "matching prior JSON must not authorize a replaced execution")
	next.Owner.ExecutionID = "execution-2"
	require.NoError(t, repo.SetSessionMetadataKey(ctx, running.SessionID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": "execution-2", "settings_source_generation": 1, "settings_attempt_id": "attempt-1"}))
	second, err := writer.PublishSessionCatalog(ctx, session.ID, "execution-2", "concrete-profile", old, next)
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, uint64(2), second.Revision)
	session, err = repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, running.SessionID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": "execution-2", "settings_source_generation": 2, "settings_attempt_id": "attempt-2", "settings_policy": "provider_restored"}))
	stale, err = writer.PublishSessionCatalog(ctx, session.ID, "execution-2", "concrete-profile", session.Metadata[sessioncatalog.MetadataKey], next)
	require.NoError(t, err)
	require.Nil(t, stale, "matching execution and prior observation cannot authorize an obsolete startup source")
	starter, ok := any(repo).(interface {
		BeginSessionCatalogDiscovery(context.Context, string, string, sessioncatalog.Owner, string) (*sessioncatalog.Launch, error)
	})
	if !ok {
		t.Fatal("replacement cannot atomically capture its notice revision and reserve its discovery source")
	}
	owner := sessioncatalog.Owner{ExecutionID: "execution-2", AttemptID: "attempt-3", SourceGeneration: 3}
	launch, err := starter.BeginSessionCatalogDiscovery(ctx, session.ID, "concrete-profile", owner, "strict")
	require.NoError(t, err)
	require.Equal(t, &sessioncatalog.Launch{Owner: owner, NoticeRevision: 2}, launch)
	session, err = repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, "strict", session.Metadata[models.SessionMetaKeyACPModelState].(map[string]any)["settings_policy"], "new ordinary resume cannot inherit the predecessor's report policy")
	next.Owner = owner
	next.Digest = "newer-catalog"
	newer, err := writer.PublishSessionCatalog(ctx, session.ID, owner.ExecutionID, "concrete-profile", session.Metadata[sessioncatalog.MetadataKey], next)
	require.NoError(t, err)
	require.NotNil(t, newer)
	require.Equal(t, uint64(3), newer.Revision)
	duplicateLaunch, err := starter.BeginSessionCatalogDiscovery(ctx, session.ID, "concrete-profile", owner, "strict")
	require.NoError(t, err)
	require.Equal(t, launch, duplicateLaunch, "re-entering the same launch must not capture a newer notice")
	oldLaunch, err := starter.BeginSessionCatalogDiscovery(ctx, session.ID, "concrete-profile", sessioncatalog.Owner{ExecutionID: "execution-2", AttemptID: "attempt-2", SourceGeneration: 2}, "strict")
	require.NoError(t, err)
	require.Nil(t, oldLaunch, "late old finalizer cannot reserve over an accepted newer source")
	exerciseCatalogFailedOwnerSettlement(t, repo)
	exerciseCatalogLifecycleExclusion(t, repo)
	exerciseCatalogNewOwnerReservation(t, repo)
}

// A new owner's reservation starts its own settings snapshot: the previous
// execution's settled options, current mode/model and lists must not be
// relabelled as the new source. Durable provider availability is retained,
// as the event projection retains it on owner change.
func exerciseCatalogNewOwnerReservation(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID, sessionID, profileID = "reserve-task", "reserve-session", "reserve-profile"
	previous := sessioncatalog.Owner{ExecutionID: "reserve-e1", AttemptID: "reserve-a1", SourceGeneration: 4}
	next := sessioncatalog.Owner{ExecutionID: "reserve-e2", AttemptID: "reserve-a2", SourceGeneration: 1}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Reserve"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, AgentProfileID: profileID, ExecutionProfileID: profileID, State: models.TaskSessionStateStarting}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "reserve-runtime", TaskID: taskID, SessionID: sessionID, ExecutionProfileID: profileID, AgentExecutionID: next.ExecutionID, Status: models.ExecutorRunningStatusStarting}))
	providerAvailability := map[string]any{"revision": 3, "profile_id": profileID}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyACPModelState, map[string]any{
		"settings_source_execution_id": previous.ExecutionID, "settings_attempt_id": previous.AttemptID,
		"settings_source_generation": previous.SourceGeneration, "settings_policy": "strict",
		"current_model_id": "old-model", "current_mode_id": "old-mode", "current_model_generation": 9, "current_mode_generation": 9,
		"models":                 []map[string]any{{"modelId": "old-model"}},
		"config_options":         []map[string]any{{"id": "effort", "currentValue": "high"}},
		"config_options_settled": true,
		"catalog_discovery":      providerAvailability,
	}))
	launch, err := repo.BeginSessionCatalogDiscovery(ctx, sessionID, profileID, next, "strict")
	require.NoError(t, err)
	require.NotNil(t, launch)
	stored, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	raw, err := json.Marshal(stored.Metadata[models.SessionMetaKeyACPModelState])
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, json.Unmarshal(raw, &state))
	require.Equal(t, next.ExecutionID, state["settings_source_execution_id"])
	require.Equal(t, next.AttemptID, state["settings_attempt_id"])
	require.EqualValues(t, next.SourceGeneration, state["settings_source_generation"])
	require.Equal(t, "strict", state["settings_policy"])
	for _, key := range []string{"current_model_id", "current_mode_id", "current_model_generation", "current_mode_generation", "models", "config_options", "config_options_settled"} {
		require.NotContains(t, state, key, "the previous owner's %s must not be relabelled as the new source", key)
	}
	require.Equal(t, map[string]any{"revision": float64(3), "profile_id": profileID}, state["catalog_discovery"])

	// Same-owner re-entry returns the existing launch before any reservation, so
	// the settings that owner has since reported are untouched. Every startup
	// path advances the source generation before Begin, so a reservation never
	// meets its own owner's snapshot.
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyACPModelState, map[string]any{
		"settings_source_execution_id": next.ExecutionID, "settings_attempt_id": next.AttemptID,
		"settings_source_generation": next.SourceGeneration, "settings_policy": "strict",
		"current_model_id": "selected-model", "config_options_settled": true,
	}))
	again, err := repo.BeginSessionCatalogDiscovery(ctx, sessionID, profileID, next, "strict")
	require.NoError(t, err)
	require.Equal(t, launch, again)
	stored, err = repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	raw, err = json.Marshal(stored.Metadata[models.SessionMetaKeyACPModelState])
	require.NoError(t, err)
	state = map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &state))
	require.Equal(t, "selected-model", state["current_model_id"])
	require.Equal(t, true, state["config_options_settled"])
}

func exerciseCatalogLifecycleExclusion(t *testing.T, repo *Repository) {
	t.Helper()
	for _, tc := range []struct {
		name           string
		sessionState   models.TaskSessionState
		taskState      v1.TaskState
		executorStatus string
		archived       bool
	}{
		{name: "stopped-execution", executorStatus: models.ExecutorRunningStatusStopped},
		{name: "failed-execution", executorStatus: models.ExecutorRunningStatusFailed},
		{name: "completed-execution", executorStatus: models.ExecutorRunningStatusComplete},
		{name: "prepared-execution", executorStatus: models.ExecutorRunningStatusPrepared},
		{name: "starting-session", sessionState: models.TaskSessionStateStarting},
		{name: "created-session", sessionState: models.TaskSessionStateCreated},
		{name: "terminal-session", sessionState: models.TaskSessionStateCancelled},
		{name: "completed-task", taskState: v1.TaskStateCompleted},
		{name: "failed-task", taskState: v1.TaskStateFailed},
		{name: "cancelled-task", taskState: v1.TaskStateCancelled},
		{name: "archived-task", archived: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			task := &models.Task{ID: "catalog-" + tc.name, Title: tc.name, State: tc.taskState}
			require.NoError(t, repo.CreateTask(ctx, task))
			state := tc.sessionState
			if state == "" {
				state = models.TaskSessionStateIdle
			}
			profileID := "profile-" + tc.name
			session := &models.TaskSession{ID: "session-" + tc.name, TaskID: task.ID, AgentProfileID: profileID, ExecutionProfileID: profileID, State: state}
			require.NoError(t, repo.CreateTaskSession(ctx, session))
			status := tc.executorStatus
			if status == "" {
				status = models.ExecutorRunningStatusReady
			}
			owner := sessioncatalog.Owner{ExecutionID: "execution-" + tc.name, AttemptID: "attempt", SourceGeneration: 1}
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "executor-" + tc.name, SessionID: session.ID, TaskID: task.ID, ExecutionProfileID: profileID, AgentExecutionID: owner.ExecutionID, Status: status}))
			require.NoError(t, repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": owner.ExecutionID, "settings_attempt_id": owner.AttemptID, "settings_source_generation": owner.SourceGeneration}))
			if tc.archived {
				require.NoError(t, repo.ArchiveTask(ctx, task.ID))
			}
			sessions, err := repo.ListCatalogRefreshSessions(ctx, profileID, "", 128)
			require.NoError(t, err)
			require.Empty(t, sessions, "only unarchived live tasks with a live execution are eligible")
			stored, err := repo.PublishSessionCatalog(ctx, session.ID, owner.ExecutionID, profileID, nil, sessioncatalog.Observation{Owner: owner, ProfileID: profileID, Disposition: "rediscovery_required", Digest: "host"})
			require.NoError(t, err)
			require.Nil(t, stored, "the write must repeat lifecycle checks even after enumeration")
			if tc.name == "completed-task" {
				require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateStarting, ""))
				running, readErr := repo.GetExecutorRunningBySessionID(ctx, session.ID)
				require.NoError(t, readErr)
				running.Status = models.ExecutorRunningStatusStarting
				require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
				owner.SourceGeneration++
				owner.AttemptID = "accepted-resume"
				launch, beginErr := repo.BeginSessionCatalogDiscovery(ctx, session.ID, profileID, owner, "strict")
				require.NoError(t, beginErr)
				require.NotNil(t, launch, "publication exclusions must not block ordinary resume of a completed task before task promotion")
			}
			if tc.name == "prepared-execution" {
				require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateStarting, ""))
				launch, beginErr := repo.BeginSessionCatalogDiscovery(ctx, session.ID, profileID, owner, "strict")
				require.NoError(t, beginErr)
				require.NotNil(t, launch, "the first launch of a prepare-only session starts from its prepared executor row")
			}
		})
	}
}

func exerciseCatalogFailedOwnerSettlement(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	const taskID, sessionID, profileID = "failed-catalog-task", "failed-catalog-session", "failed-catalog-profile"
	owner := sessioncatalog.Owner{ExecutionID: "failed-catalog-execution", AttemptID: "accepted-failed-attempt", SourceGeneration: 1}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Failed catalog report"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, AgentProfileID: profileID, ExecutionProfileID: profileID, State: models.TaskSessionStateRunning, Metadata: map[string]any{"runtime_config": map[string]any{"model": "keep-model"}, "draft": "keep-draft"}}))
	running := &models.ExecutorRunning{ID: "failed-catalog-runtime", TaskID: taskID, SessionID: sessionID, ExecutionProfileID: profileID, AgentExecutionID: owner.ExecutionID, Status: "running"}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyACPModelState, map[string]any{"settings_source_execution_id": owner.ExecutionID, "settings_attempt_id": owner.AttemptID, "settings_source_generation": owner.SourceGeneration}))
	observation := sessioncatalog.Observation{Version: 1, ProfileID: profileID, Owner: owner, Disposition: "rediscovery_required", Reason: "executor_scoped"}
	first, err := repo.PublishSessionCatalog(ctx, sessionID, owner.ExecutionID, profileID, nil, observation)
	require.NoError(t, err)
	require.NotNil(t, first)
	launch, err := repo.BeginSessionCatalogDiscovery(ctx, sessionID, profileID, owner, "strict")
	require.NoError(t, err)
	require.Equal(t, first.Revision, launch.NoticeRevision)
	current, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	prior := current.Metadata[sessioncatalog.MetadataKey]
	current.State = models.TaskSessionStateFailed
	require.NoError(t, repo.UpdateTaskSession(ctx, current))
	running.Status = "failed"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	result := sessioncatalog.Discovery{Outcome: "failed"}
	provider, err := repo.PersistSessionCatalogAvailability(ctx, sessionID, owner, result)
	require.NoError(t, err)
	require.NotNil(t, provider, "a current owned report delivered after startup failure remains authoritative availability evidence")
	observation.Result = &result
	observation.Reason = "failed"
	settled, err := repo.SettleSessionCatalogDiscovery(ctx, sessionID, owner.ExecutionID, profileID, prior, observation)
	require.NoError(t, err)
	require.NotNil(t, settled, "asynchronous event delivery must still settle its captured notice after the same owner fails")
	require.Equal(t, first.Revision+1, settled.Revision)
	running.AgentExecutionID = "replacement-catalog-execution"
	running.Status = "ready"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
	stale, err := repo.PersistSessionCatalogAvailability(ctx, sessionID, owner, sessioncatalog.Discovery{Outcome: "unsupported"})
	require.NoError(t, err)
	require.Nil(t, stale, "an old failed owner cannot publish after execution replacement")
	current, err = repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"model": "keep-model"}, current.Metadata["runtime_config"])
	require.Equal(t, "keep-draft", current.Metadata["draft"])
	stored := current.Metadata[models.SessionMetaKeyACPModelState].(map[string]any)["catalog_discovery"].(map[string]any)
	require.Equal(t, "failed", stored["result"].(map[string]any)["outcome"])
}
