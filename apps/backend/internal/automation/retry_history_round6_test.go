package automation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListRetryHistoryReturnsCompleteAttemptTimelineAndCursor(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{ID: "history-automation", WorkspaceID: "history-workspace", Name: "history", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "history-group", AutomationID: a.ID, TriggerID: "trigger-a", TriggerIDsJSON: `["trigger-a","trigger-b"]`, Generation: 1, State: RetryGroupCompleted}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	for i, id := range []string{"attempt-1", "attempt-2"} {
		require.NoError(t, store.CreateRun(ctx, &AutomationRun{
			ID: id, AutomationID: a.ID, TriggerID: "trigger-a", TriggerType: TriggerTypeManual,
			RetryGroupID: group.ID, RetryGroupGeneration: 1, AttemptNumber: int64(i + 1),
			RetryState: RetryStateCompleted, Status: RunStatusSucceeded,
			TriggerData: json.RawMessage(`{"projection_version":1}`),
		}))
	}
	page, err := store.ListRetryHistory(ctx, a.ID, "", 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, []string{"trigger-a", "trigger-b"}, page.Items[0].TriggerIDs)
	require.Len(t, page.Items[0].Attempts, 2)
	require.NotEmpty(t, page.HighWaterMark)
}
