package automation

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReconcileUnboundLegacyRetryRetainsVisibleUncertainty(t *testing.T) {
	for _, taskID := range []string{"", "task-before-session"} {
		t.Run(taskID, func(t *testing.T) {
			ctx := context.Background()
			svc := newTestService(t)
			a := &Automation{WorkspaceID: "workspace", Name: "unbound legacy", Enabled: true}
			require.NoError(t, svc.store.CreateAutomation(ctx, a))
			group := &RetryGroup{ID: "legacy-group", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
			require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
			run := &AutomationRun{ID: "unbound-legacy", AutomationID: a.ID, TaskID: taskID, Status: RunStatusTriggered, RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
			require.NoError(t, svc.store.CreateRun(ctx, run))
			require.NoError(t, svc.ReconcileOpenRuns(ctx))
			retained, err := svc.store.GetRun(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, RunStatusTriggered, retained.Status)
			require.Equal(t, ErrRetryLaunchUncertain.Error(), retained.ErrorMessage)
			require.Equal(t, taskID, retained.TaskID)
			require.Empty(t, retained.SessionID)
			require.Empty(t, retained.TurnID)
			require.ErrorIs(t, svc.DeleteRun(ctx, run.ID), ErrRetryLaunchUncertain)
		})
	}
}
