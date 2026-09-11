package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestRetryDelayAndTitleUseCheckedArithmetic(t *testing.T) {
	policy := RetryPolicy{Mode: RetryModeFinite, MaxRetries: "10", DelaySeconds: "2", Backoff: RetryBackoffExponential}
	delay, err := RetryDelay(policy, 3)
	require.NoError(t, err)
	require.Equal(t, 8*time.Second, delay)

	_, err = RetryDelay(RetryPolicy{Mode: RetryModeInfinite, DelaySeconds: "9223372036854775807", Backoff: RetryBackoffExponential}, 2)
	require.ErrorIs(t, err, ErrRetryDelayOverflow)

	title := FormatRetryTitle(strings.Repeat("x", 100), 12)
	require.Equal(t, 60, runeCount(title))
	require.True(t, strings.HasPrefix(title, "Retry 12: "))
}

func TestRetryFailureSanitizesSecretsAndCreatesOneChild(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-retry", Name: "retry", Enabled: true, MaxConcurrentRuns: 1,
		RetryPolicy: RetryPolicy{Mode: RetryModeFinite, MaxRetries: "2", DelaySeconds: "0"}}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-1", AutomationID: a.ID, TriggerID: "trigger-1", Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "run-1", AutomationID: a.ID, TriggerID: "trigger-1", TriggerType: TriggerTypeManual,
		Status: RunStatusTriggered, RetryGroupID: group.ID, AttemptNumber: 1, RetryState: RetryStateTriggered,
		RetryGroupGeneration: 1, RetryBaseTitle: "Root", RetryPolicySnapshot: `{"mode":"finite","max_retries":"2","delay_seconds":"0","backoff":"fixed","history_mode":"attempts"}`}
	require.NoError(t, store.CreateRun(ctx, run))

	child, err := store.FinalizeRetryFailure(ctx, run.ID, 1, errors.New("token=secret prompt=private /home/user"), "launch")
	require.NoError(t, err)
	require.NotNil(t, child)
	require.Equal(t, int64(2), child.AttemptNumber)
	require.Equal(t, RunStatusScheduledRetry, child.Status)
	require.Equal(t, RetryStateScheduled, child.RetryState)
	require.NotContains(t, child.ErrorMessage, "secret")

	replay, err := store.FinalizeRetryFailure(ctx, run.ID, 1, errors.New("different"), "launch")
	require.NoError(t, err)
	require.Equal(t, child.ID, replay.ID)
	var count int
	require.NoError(t, store.db.Get(&count, `SELECT COUNT(*) FROM automation_runs WHERE retry_group_id = ?`, group.ID))
	require.Equal(t, 2, count)
}

func TestRetryClaimLeaseIsSingleUseAndGenerationFenced(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-claim", Name: "claim", Enabled: true, MaxConcurrentRuns: 1}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "group-claim", AutomationID: a.ID, Generation: 4, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	due := time.Now().UTC().Add(-time.Second)
	run := &AutomationRun{ID: "run-claim", AutomationID: a.ID, Status: RunStatusScheduledRetry, RetryGroupID: group.ID,
		AttemptNumber: 2, RetryState: RetryStateScheduled, RetryScheduledAt: &due, RetryGroupGeneration: 4}
	require.NoError(t, store.CreateRun(ctx, run))

	claimed, token, err := store.ClaimDueRetry(ctx, time.Now().UTC(), time.Minute)
	require.NoError(t, err)
	require.Equal(t, run.ID, claimed.ID)
	require.NotEmpty(t, token)
	_, _, err = store.ClaimDueRetry(ctx, time.Now().UTC(), time.Minute)
	require.ErrorIs(t, err, ErrNoDueRetry)

	require.NoError(t, store.CancelRetryGroup(ctx, group.ID, 4))
	require.ErrorIs(t, store.ReleaseRetryClaim(ctx, run.ID, token, 4), ErrRetryGenerationMismatch)

	var state string
	require.NoError(t, store.db.Get(&state, `SELECT retry_state FROM automation_runs WHERE id = ?`, run.ID))
	require.Equal(t, string(RetryStateCancelled), state)
}
func TestRetryAdmissionPersistsImmutableIntentAndSafeEvent(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	eventsSeen := make(chan *AutomationTriggeredEvent, 1)
	_, err = eventBus.Subscribe(events.AutomationTriggered, func(_ context.Context, event *bus.Event) error {
		evt, ok := event.Data.(*AutomationTriggeredEvent)
		if ok {
			eventsSeen <- evt
		}
		return nil
	})
	require.NoError(t, err)
	svc := NewService(store, eventBus, log)
	a := &Automation{WorkspaceID: "ws-admission", Name: "safe", Enabled: true,
		RetryPolicy: RetryPolicy{Mode: RetryModeFinite, MaxRetries: "1", DelaySeconds: "0"}}
	require.NoError(t, store.CreateAutomation(ctx, a))
	trigger := &AutomationTrigger{ID: "trigger", AutomationID: a.ID, Type: TriggerTypeManual, Enabled: true}
	require.NoError(t, store.CreateTrigger(ctx, trigger))
	result, err := svc.FireTrigger(ctx, a.ID, "trigger", TriggerTypeManual, []byte(`{"secret":"do-not-publish"}`), "manual-1")
	require.NoError(t, err)
	require.NotEmpty(t, result.RunID)
	evt := <-eventsSeen
	require.Equal(t, result.RunID, evt.RunID)
	require.Empty(t, evt.TriggerData)
	require.Empty(t, evt.AutomationID)
	var intents, outbox int
	require.NoError(t, store.db.Get(&intents, `SELECT COUNT(*) FROM automation_run_task_intents WHERE run_id = ?`, result.RunID))
	require.NoError(t, store.db.Get(&outbox, `SELECT COUNT(*) FROM automation_retry_outbox WHERE run_id = ?`, result.RunID))
	require.Equal(t, 1, intents)
	run, err := store.GetRun(ctx, result.RunID)
	require.NoError(t, err)
	require.NotContains(t, run.RetryTriggerSnapshot, "do-not-publish")
	require.Equal(t, 1, outbox)
}
