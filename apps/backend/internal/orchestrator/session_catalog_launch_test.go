package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SESSION-CATALOG-002.4
func TestSessionCatalogRefreshAcceptedLaunchOwner(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "catalog-launch-task", "catalog-launch-session", models.TaskSessionStateCancelled)
	session, err := repo.GetTaskSession(ctx, "catalog-launch-session")
	require.NoError(t, err)
	session.AgentProfileID = "profile1"
	session.ExecutionProfileID = "profile1"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "old-runtime", TaskID: session.TaskID, SessionID: session.ID, ExecutionProfileID: "profile1", AgentExecutionID: "old-execution", Status: models.ExecutorRunningStatusStopped, Resumable: true, ResumeToken: "native-conversation"}))
	acceptedAttempt := ""
	manager := &mockAgentManager{repoForExecutionLookup: repo, isAgentReadyFn: func(context.Context, string) bool { return true }, launchAgentFunc: func(ctx context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		acceptedAttempt = executor.ResumeAttemptIDFromContext(ctx)
		require.Equal(t, "native-conversation", request.ACPSessionID)
		running, readErr := repo.GetExecutorRunningBySessionID(ctx, request.SessionID)
		require.NoError(t, readErr)
		running.AgentExecutionID = "accepted-execution"
		running.Status = models.ExecutorRunningStatusReady
		require.NoError(t, repo.UpsertExecutorRunning(ctx, running))
		require.NoError(t, repo.UpdateTaskSessionState(ctx, request.SessionID, models.TaskSessionStateWaitingForInput, ""))
		return &executor.LaunchAgentResponse{AgentExecutionID: "accepted-execution"}, nil
	}}
	tasks := newMockTaskRepo()
	tasks.tasks[session.TaskID] = &v1.Task{ID: session.TaskID, Title: "Catalog", State: v1.TaskStateInProgress}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), tasks, manager)
	service.executor = withTestCapabilities(executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{}))
	response, err := service.LaunchSession(ctx, &LaunchSessionRequest{TaskID: session.TaskID, SessionID: session.ID, Intent: IntentResume})
	require.NoError(t, err)
	require.NotEmpty(t, acceptedAttempt)
	payload, err := json.Marshal(response)
	require.NoError(t, err)
	var wire struct {
		Owner *sessioncatalog.Owner `json:"accepted_launch_owner"`
	}
	require.NoError(t, json.Unmarshal(payload, &wire))
	require.NotNil(t, wire.Owner, "Resume ACK must carry its accepted launch owner rather than making the composer guess")
	require.Equal(t, "accepted-execution", wire.Owner.ExecutionID)
	require.Equal(t, acceptedAttempt, wire.Owner.AttemptID)
	require.Equal(t, session.ID, response.SessionID)
}
