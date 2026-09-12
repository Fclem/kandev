package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestRetryAdmissionPersistsCompleteLaunchSnapshot(t *testing.T) {
	store := setupTestStore(t)
	log, _ := logger.NewFromZap(zap.NewNop())
	svc := NewService(store, bus.NewMemoryEventBus(log), log)
	ctx := context.Background()
	a := &Automation{
		WorkspaceID: "ws-snapshot-complete", Name: "Snapshot automation",
		WorkflowID: "workflow-1", WorkflowStepID: "step-1",
		AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
		Prompt: "Review {{pr.title}}", TaskTitleTemplate: "Review {{pr.title}}",
		TaskMode: TaskModeNormalTask, RepositoryMode: RepositoryModeSelected,
		Repositories:       []AutomationRepository{{RepositoryID: "repo-1", BaseBranch: "release"}},
		ContinuationPolicy: ContinuationPolicyReuseThread, Enabled: true,
		MaxConcurrentRuns: 1,
		RetryPolicy:       RetryPolicy{Mode: RetryModeFinite, MaxRetries: "2", DelaySeconds: "30", Backoff: RetryBackoffFixed, HistoryMode: RetryHistoryTimeline},
	}
	require.NoError(t, store.CreateAutomation(ctx, a))
	trigger := &AutomationTrigger{ID: "trigger-complete", AutomationID: a.ID, Type: TriggerTypeGitHubPR, Enabled: true}
	require.NoError(t, store.CreateTrigger(ctx, trigger))

	result, err := svc.FireTrigger(ctx, a.ID, trigger.ID, trigger.Type,
		json.RawMessage(`{"number":7,"title":"Snapshot PR","timestamp":"2026-09-12T01:02:03Z"}`), "delivery-7")
	require.NoError(t, err)
	run, err := store.GetRun(ctx, result.RunID)
	require.NoError(t, err)

	var snapshot map[string]any
	require.NoError(t, json.Unmarshal([]byte(run.RetryLaunchConfigSnapshot), &snapshot))
	require.Equal(t, float64(1), snapshot["version"])
	for _, key := range []string{
		"automation_id", "workspace_id", "name", "workflow_id", "workflow_step_id",
		"agent_profile_id", "executor_profile_id", "prompt", "task_title_template",
		"task_mode", "repository_mode", "repositories", "continuation_policy", "max_concurrent_runs",
		"trigger_id", "trigger_type", "trigger_data", "resolved_trigger_at", "retry_policy",
	} {
		require.Contains(t, snapshot, key, "missing launch snapshot field %s", key)
	}
	require.Equal(t, "github_pr", snapshot["trigger_data"].(map[string]any)["trigger_type"])
	require.NotContains(t, snapshot["trigger_data"], "title")
}

func TestRetryAdmissionSupersedesEquivalentTriggerSets(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-trigger-set", Name: "Trigger set automation", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	now := time.Now().UTC()
	oldGroup := &RetryGroup{
		ID: "old-trigger-set", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerIDsJSON: `["trigger-b","trigger-a"]`, Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, oldGroup))
	oldRun := &AutomationRun{
		ID: "old-trigger-run", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerType: TriggerTypeManual, Status: RunStatusScheduledRetry,
		RetryGroupID: oldGroup.ID, RetryGroupGeneration: 1, RetryState: RetryStateScheduled,
		RetryScheduledAt: &now, CreatedAt: now,
	}
	require.NoError(t, store.CreateRun(ctx, oldRun))

	newRun := &AutomationRun{
		ID: "new-trigger-run", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: "new-trigger-set", RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
		RetryLaunchConfigVersion: RetryLaunchConfigVersion, RetryLaunchConfigSnapshot: `{}`,
		RetryPolicySnapshot: `{}`, RetryTriggerSnapshot: `{}`, RetryContinuationSnapshot: `{}`,
		TriggerData: json.RawMessage(`{"trigger_type":"manual"}`),
	}
	newGroup := &RetryGroup{
		ID: newRun.RetryGroupID, AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerIDsJSON: `["trigger-a","trigger-b"]`, Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryAdmission(ctx, newRun, newGroup))
	reloadedGroup, err := store.GetRetryGroup(ctx, oldGroup.ID)
	require.NoError(t, err)
	require.Equal(t, RetryGroupSuperseded, reloadedGroup.State)
	reloadedRun, err := store.GetRun(ctx, oldRun.ID)
	require.NoError(t, err)
	require.Equal(t, RetryStateSuperseded, reloadedRun.RetryState)
}
