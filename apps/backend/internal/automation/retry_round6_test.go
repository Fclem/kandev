package automation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"testing"
	"time"
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
	require.Equal(t, "Snapshot PR", snapshot["trigger_data"].(map[string]any)["title"])
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

func TestRetryAdmissionSupersedesPendingWithoutTerminalizingActiveRun(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-active-supersession", Name: "Active supersession", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	oldGroup := &RetryGroup{
		ID: "old-active-group", AutomationID: a.ID, TriggerID: "trigger-a",
		Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, oldGroup))
	activeRun := &AutomationRun{
		ID: "active-supersession-run", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		RetryGroupID: oldGroup.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, store.CreateRun(ctx, activeRun))
	newRun := &AutomationRun{
		ID: "new-active-supersession-run", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: "new-active-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
		RetryLaunchConfigVersion: RetryLaunchConfigVersion, RetryLaunchConfigSnapshot: `{}`,
		RetryPolicySnapshot: `{}`, RetryTriggerSnapshot: `{}`, RetryContinuationSnapshot: `{}`,
	}
	newGroup := &RetryGroup{
		ID: newRun.RetryGroupID, AutomationID: a.ID, TriggerID: "trigger-a",
		Generation: 1, State: RetryGroupLive,
	}

	require.NoError(t, store.CreateRetryAdmission(ctx, newRun, newGroup))

	reloaded, err := store.GetRun(ctx, activeRun.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusTaskCreated, reloaded.Status)
	require.Equal(t, RetryStateTriggered, reloaded.RetryState)
}

func TestSupersededActiveRetryCanCompleteWithoutReplacingLiveGroup(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-active-completion", WorkspaceID: "ws-active-completion", Name: "Active completion", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	oldGroup := &RetryGroup{
		ID: "old-completion-group", AutomationID: a.ID, TriggerID: "trigger-a",
		Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, oldGroup))
	activeRun := &AutomationRun{
		ID: "active-completion-run", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		RetryGroupID: oldGroup.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
		AttemptNumber:       1,
		RetryPolicySnapshot: `{"mode":"finite","max_retries":"2","delay_seconds":"0","backoff":"fixed"}`,
	}
	require.NoError(t, store.CreateRun(ctx, activeRun))
	replacement := &AutomationRun{
		ID: "replacement-completion-run", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: "replacement-completion-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
		RetryLaunchConfigVersion: RetryLaunchConfigVersion, RetryLaunchConfigSnapshot: `{}`,
		RetryPolicySnapshot: `{}`, RetryTriggerSnapshot: `{}`, RetryContinuationSnapshot: `{}`,
	}
	replacementGroup := &RetryGroup{
		ID: replacement.RetryGroupID, AutomationID: a.ID, TriggerID: "trigger-a",
		Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryAdmission(ctx, replacement, replacementGroup))

	child, err := store.FinalizeRetryFailure(ctx, activeRun.ID, 1, errors.New("provider failed"), "launch")
	require.NoError(t, err)
	require.Nil(t, child)

	reloadedRun, err := store.GetRun(ctx, activeRun.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusFailed, reloadedRun.Status)
	require.Equal(t, RetryStateCompleted, reloadedRun.RetryState)
	var childCount int
	require.NoError(t, store.db.Get(&childCount,
		`SELECT COUNT(*) FROM automation_runs WHERE retry_parent_run_id = ?`, activeRun.ID))
	require.Zero(t, childCount)
	reloadedReplacement, err := store.GetRetryGroup(ctx, replacementGroup.ID)
	require.NoError(t, err)
	require.Equal(t, RetryGroupLive, reloadedReplacement.State)
	require.Equal(t, int64(1), reloadedReplacement.Generation)
}

func TestSupersededActiveRetryCanSucceedWithoutReplacingLiveGroup(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "automation-active-success", WorkspaceID: "ws-active-success", Name: "Active success", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	oldGroup := &RetryGroup{
		ID: "old-success-group", AutomationID: a.ID, TriggerID: "trigger-a",
		Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryGroup(ctx, oldGroup))
	activeRun := &AutomationRun{
		ID: "active-success-run", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerType: TriggerTypeManual, Status: RunStatusTaskCreated,
		RetryGroupID: oldGroup.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
	}
	require.NoError(t, store.CreateRun(ctx, activeRun))
	replacement := &AutomationRun{
		ID: "replacement-success-run", AutomationID: a.ID, TriggerID: "trigger-a",
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: "replacement-success-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered,
		RetryLaunchConfigVersion: RetryLaunchConfigVersion, RetryLaunchConfigSnapshot: `{}`,
		RetryPolicySnapshot: `{}`, RetryTriggerSnapshot: `{}`, RetryContinuationSnapshot: `{}`,
	}
	replacementGroup := &RetryGroup{
		ID: replacement.RetryGroupID, AutomationID: a.ID, TriggerID: "trigger-a",
		Generation: 1, State: RetryGroupLive,
	}
	require.NoError(t, store.CreateRetryAdmission(ctx, replacement, replacementGroup))

	require.NoError(t, store.MarkRetrySucceeded(ctx, activeRun.ID, 1))

	reloadedRun, err := store.GetRun(ctx, activeRun.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusSucceeded, reloadedRun.Status)
	require.Equal(t, RetryStateCompleted, reloadedRun.RetryState)
	reloadedReplacement, err := store.GetRetryGroup(ctx, replacementGroup.ID)
	require.NoError(t, err)
	require.Equal(t, RetryGroupLive, reloadedReplacement.State)
	require.Equal(t, int64(1), reloadedReplacement.Generation)
}

func TestWebhookRetryAdmissionKeepsRawPayloadEphemeral(t *testing.T) {
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	eventsSeen := make(chan *AutomationTriggeredEvent, 1)
	_, err = eventBus.Subscribe(events.AutomationTriggered, func(_ context.Context, event *bus.Event) error {
		if evt, ok := event.Data.(*AutomationTriggeredEvent); ok {
			eventsSeen <- evt
		}
		return nil
	})
	require.NoError(t, err)
	svc := NewService(store, eventBus, log)
	ctx := context.Background()
	a := &Automation{
		WorkspaceID: "ws-webhook-raw", Name: "Webhook raw", Enabled: true,
		Prompt:      "Review {{payload}}",
		RetryPolicy: RetryPolicy{Mode: RetryModeFinite, MaxRetries: "1", DelaySeconds: "0"},
	}
	require.NoError(t, store.CreateAutomation(ctx, a))
	trigger := &AutomationTrigger{ID: "webhook-trigger", AutomationID: a.ID, Type: TriggerTypeWebhook, Enabled: true}
	require.NoError(t, store.CreateTrigger(ctx, trigger))
	raw := json.RawMessage(`{"payload":"private","secret":"never-persist"}`)
	safe, err := safeWebhookTriggerData(raw, []string{"/payload"}, trigger.ID, "delivery-1")
	require.NoError(t, err)

	result, err := svc.FireTriggerWithInitialData(
		ctx, a.ID, trigger.ID, trigger.Type, safe, raw, "webhook:"+a.ID+":delivery-1",
	)
	require.NoError(t, err)
	evt := <-eventsSeen
	require.Equal(t, raw, evt.TriggerData)
	require.Equal(t, safe, evt.SafeTriggerData)
	run, err := store.GetRun(ctx, result.RunID)
	require.NoError(t, err)
	require.NotContains(t, run.TriggerData, "never-persist")
	require.NotContains(t, run.RetryTriggerSnapshot, "never-persist")
}
