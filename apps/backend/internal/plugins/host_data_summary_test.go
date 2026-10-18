package plugins

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-EFFORT-SIGNALS-003.3 AC-PLUGINS-EFFORT-SIGNALS-003.4
func TestTaskPullRequestSummaryFields(t *testing.T) {
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	changed := 4
	for _, synced := range []bool{false, true} {
		t.Run(map[bool]string{false: "unobserved", true: "observed"}[synced], func(t *testing.T) {
			pr := &githubsvc.TaskPR{PRNumber: 12, CreatedAt: at, ChangedFiles: &changed, ReviewCount: 2, Additions: 9, Deletions: 3}
			if synced {
				pr.LastSyncedAt = &at
			}
			rows := mergeTaskPullRequests([]*githubsvc.TaskPR{pr}, nil)
			require.Len(t, rows, 1)
			values := summaryJSON(t, rows[0])
			require.Equal(t, at.Format(time.RFC3339), values["CreatedAt"])
			require.Equal(t, synced, values["SizeObserved"])
			if synced {
				require.Equal(t, float64(4), values["ChangedFiles"])
				require.Equal(t, float64(2), values["ApprovedReviewerCount"])
			} else {
				require.Contains(t, values, "ChangedFiles")
				require.Nil(t, values["ChangedFiles"])
				require.Nil(t, values["ApprovedReviewerCount"])
			}
		})
	}
}

func summaryJSON(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	bytes, err := json.Marshal(value)
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(bytes, &result))
	return result
}

type summaryMRSource struct {
	rows map[string][]*gitlab.TaskMR
	err  error
}

func (s *summaryMRSource) ListTaskMRsByTaskIDs(context.Context, []string) (map[string][]*gitlab.TaskMR, error) {
	return s.rows, s.err
}

func newSummaryHost(t *testing.T, githubSource *stubPRSource, gitlabSource *summaryMRSource) pluginsdk.Host {
	t.Helper()
	registry := NewRegistry()
	registry.Add(&store.Record{Manifest: manifest.Manifest{ID: "summary-reader", Capabilities: manifest.Capabilities{APIRead: []string{"tasks"}}}})
	svc := NewService(store.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	t.Cleanup(func() { require.NoError(t, svc.Close()) })
	svc.SetTaskPRSource(githubSource)
	data := &fakeTaskDataSource{tasksByID: map[string]*taskmodels.Task{"task-summary": {ID: "task-summary"}}}
	host := svc.hostForPlugin("summary-reader").(*pluginHost)
	host.taskData = data
	svc.SetTaskMRSource(gitlabSource)
	return host
}

// @covers AC-PLUGINS-EFFORT-SIGNALS-003.6
func TestTaskPullRequestsIncludeGitLab(t *testing.T) {
	base := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	mr := &gitlab.TaskMR{
		MRIID: 21, MRURL: "https://gitlab.example/group/repo/-/merge_requests/21", MRTitle: "MR summary", State: "locked",
		HeadBranch: "feature", BaseBranch: "main", Draft: true, AuthorUsername: "reviewer",
		ApprovalState: "approved", PipelineState: "success", MergeStatus: "can_be_merged",
		PipelineJobsTotal: 5, PipelineJobsPass: 4, UnresolvedDiscussions: 3,
		CreatedAt: base.Add(time.Hour), ClosedAt: &base, LastSyncedAt: &base, ApprovalCount: 2,
	}
	host := newSummaryHost(t, &stubPRSource{byTask: map[string][]*githubsvc.TaskPR{"task-summary": {{PRNumber: 12, CreatedAt: base}}}}, &summaryMRSource{rows: map[string][]*gitlab.TaskMR{"task-summary": {mr}}})
	task, err := host.Tasks().Get(context.Background(), "task-summary")
	require.NoError(t, err)
	require.Len(t, task.PullRequests, 2)
	require.Equal(t, "gitlab", task.PullRequests[0].Provider)
	require.Equal(t, "closed", task.PullRequests[0].State)
	require.Equal(t, "github", task.PullRequests[1].Provider)
	reviews := int32(2)
	closedAt := base.Format(time.RFC3339)
	require.Equal(t, pluginsdk.TaskPullRequest{
		Number: 21, URL: mr.MRURL, Title: "MR summary", State: "closed",
		HeadBranch: "feature", BaseBranch: "main", IsDraft: true, AuthorLogin: "reviewer", Provider: "gitlab",
		ReviewState: "approved", ChecksState: "success", MergeableState: "can_be_merged",
		ChecksTotal: 5, ChecksPassing: 4, UnresolvedReviewThreads: 3,
		CreatedAt: base.Add(time.Hour).Format(time.RFC3339), ClosedAt: &closedAt, ApprovedReviewerCount: &reviews,
	}, task.PullRequests[0])
	require.NotNil(t, task.PullRequestsPartial)
	require.False(t, *task.PullRequestsPartial)
}

func TestTaskPullRequestsPartialByProvider(t *testing.T) {
	for _, failed := range []string{"github", "gitlab"} {
		t.Run(failed, func(t *testing.T) {
			gh := &stubPRSource{byTask: map[string][]*githubsvc.TaskPR{"task-summary": {{PRNumber: 12}}}}
			gl := &summaryMRSource{rows: map[string][]*gitlab.TaskMR{"task-summary": {{MRIID: 21}}}}
			if failed == "github" {
				gh.err = context.DeadlineExceeded
			} else {
				gl.err = context.DeadlineExceeded
			}
			host := newSummaryHost(t, gh, gl)
			task, err := host.Tasks().Get(context.Background(), "task-summary")
			require.NoError(t, err)
			require.Len(t, task.PullRequests, 1)
			want := "github"
			if failed == "github" {
				want = "gitlab"
			}
			require.Equal(t, want, task.PullRequests[0].Provider)
			require.Equal(t, true, summaryJSON(t, task)["PullRequestsPartial"])
		})
	}
}

func TestTaskPullRequestSummaryFieldsUnobservedGitLab(t *testing.T) {
	row := taskMRToDTO(&gitlab.TaskMR{MRIID: 21, ApprovalCount: 9})
	require.Nil(t, row.ApprovedReviewerCount)
	require.Nil(t, row.ChangedFiles)
	require.False(t, row.SizeObserved)
	require.Zero(t, row.Additions)
	require.Zero(t, row.Deletions)
}

func TestTaskPullRequestsPartialByProviderExact(t *testing.T) {
	for _, failed := range []string{"github", "gitlab"} {
		t.Run(failed, func(t *testing.T) {
			data := &fakeTaskDataSource{tasksByID: map[string]*taskmodels.Task{"task-summary": {
				ID: "task-summary", WorkspaceID: "workspace-one",
			}}}
			data.workspaces = []*taskmodels.Workspace{{ID: "workspace-one"}}
			host := newExactTaskReadHost(t, data)
			gh := &stubPRSource{byTask: map[string][]*githubsvc.TaskPR{"task-summary": {{PRNumber: 12}}}}
			gl := &summaryMRSource{rows: map[string][]*gitlab.TaskMR{"task-summary": {{MRIID: 21}}}}
			if failed == "github" {
				gh.err = context.DeadlineExceeded
			} else {
				gl.err = context.DeadlineExceeded
			}
			host.taskPRs, host.taskMRs = gh, gl
			host.taskPRsDep, host.taskMRsDep = nil, nil
			row, _, err := host.GetTask(context.Background(), pluginsdk.ExactTaskGetQuery{
				RequestID: "summary-exact", WorkspaceID: "workspace-one", TaskID: "task-summary",
			})
			require.NoError(t, err)
			require.Len(t, row.Task.PullRequests, 1)
			require.NotNil(t, row.Task.PullRequestsPartial)
			require.True(t, *row.Task.PullRequestsPartial)
		})
	}
}
