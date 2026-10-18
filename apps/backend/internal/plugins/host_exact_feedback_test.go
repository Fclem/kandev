package plugins

import (
	"context"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"testing"
)

func feedbackTestHost(t *testing.T) *pluginHost {
	data := &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {{ID: "task-one", WorkspaceID: "workspace-one"}}},
	}
	host := newExactReadHost(t, data, []string{"change_requests"}, []string{"host.v2.read:change_requests"})
	host.taskPRsDep, host.taskMRsDep = nil, nil
	host.taskPRs = &stubPRSource{}
	host.taskMRs = &summaryMRSource{}
	return host
}

// @covers AC-PLUGINS-EFFORT-SIGNALS-004.1 AC-PLUGINS-EFFORT-SIGNALS-004.4
func TestListChangeRequestFeedbackExact(t *testing.T) {
	host := feedbackTestHost(t)
	data := host.taskData.(*fakeTaskDataSource)
	data.tasksByWorkspace["workspace-one"] = append(data.tasksByWorkspace["workspace-one"], &taskmodels.Task{ID: "task-two", WorkspaceID: "workspace-one"})
	wire := dialPluginHostOverWire(t, host)
	manager, ok := pluginsdk.HostExactQueries(wire)
	require.True(t, ok)
	q := feedbackQuery()
	q.TaskIDs = nil
	q.Page.Limit = 1
	page, err := manager.ListChangeRequestFeedback(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "task-one", page.Items[0].TaskID)
	require.Len(t, page.Items[0].Providers, 2)
	for _, provider := range page.Items[0].Providers {
		require.Equal(t, "available", provider.ProviderState)
	}
	require.Empty(t, page.Items[0].Items)
	require.True(t, page.PageInfo.HasMore)
	require.NotEmpty(t, page.PageInfo.NextCursor)
	host.taskPRs = &stubPRSource{err: context.DeadlineExceeded}
	q.Page.Cursor = page.PageInfo.NextCursor
	q.Page.SnapshotVersion = page.PageInfo.SnapshotVersion
	next, err := manager.ListChangeRequestFeedback(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, next.Items, 1)
	require.Equal(t, "task-two", next.Items[0].TaskID)
	for _, provider := range next.Items[0].Providers {
		require.Equal(t, "available", provider.ProviderState, "continuation must retain the provider snapshot")
	}
}
