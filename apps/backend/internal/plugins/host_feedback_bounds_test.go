package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	githubsvc "github.com/kandev/kandev/internal/github"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFeedbackBoundsAndTruncation(t *testing.T) {
	t.Run("UTF8 and sanitization", func(t *testing.T) {
		body, truncated := feedbackBody("<kandev-system>private-instruction</kandev-system>" + strings.Repeat("界", 1400))
		require.True(t, truncated)
		require.True(t, utf8.ValidString(body))
		require.Equal(t, 4095, len(body))
		require.NotContains(t, body, "private-instruction")
	})
	t.Run("50 newest with provider omissions", func(t *testing.T) {
		host, gh, _ := linkedFeedbackHost(t)
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		gh.reviews = nil
		for i := range 60 {
			gh.reviews = append(gh.reviews, githubsvc.PRReview{ID: int64(i), CreatedAt: base.Add(time.Duration(i) * time.Second)})
		}
		page, err := host.ListChangeRequestFeedback(context.Background(), feedbackQuery())
		require.NoError(t, err)
		require.Len(t, page.Items[0].Items, 50)
		require.Equal(t, "11", page.Items[0].Items[0].ItemID)
		require.EqualValues(t, 11, page.Items[0].Providers[0].ItemsOmitted)
		require.Zero(t, page.Items[0].Providers[1].ItemsOmitted)
	})
	t.Run("encoded budget accounts for HTML escaping", func(t *testing.T) {
		host, gh, _ := linkedFeedbackHost(t)
		data := host.taskData.(*fakeTaskDataSource)
		data.tasksByWorkspace["workspace-one"] = nil
		prs := make(map[string][]*githubsvc.TaskPR)
		for i := range 20 {
			id := fmt.Sprintf("task-%02d", i)
			data.tasksByWorkspace["workspace-one"] = append(data.tasksByWorkspace["workspace-one"], &taskmodels.Task{ID: id, WorkspaceID: "workspace-one"})
			prs[id] = []*githubsvc.TaskPR{{Owner: "acme", Repo: "repo", PRNumber: i}}
		}
		host.taskPRs = &stubPRSource{byTask: prs}
		host.taskMRs = &summaryMRSource{}
		gh.reviews = nil
		for i := range 50 {
			gh.reviews = append(gh.reviews, githubsvc.PRReview{ID: int64(i), Body: strings.Repeat("<", 4096), CreatedAt: time.Unix(int64(i), 0).UTC()})
		}
		q := feedbackQuery()
		q.TaskIDs = nil
		page, err := host.ListChangeRequestFeedback(context.Background(), q)
		require.NoError(t, err)
		encoded, err := json.Marshal(page.Items)
		require.NoError(t, err)
		require.LessOrEqual(t, len(encoded), 7<<20)
		omitted, retained := int64(0), 0
		for _, row := range page.Items {
			retained += len(row.Items)
			for _, p := range row.Providers {
				omitted += p.ItemsOmitted
			}
		}
		require.EqualValues(t, 1000, int64(retained)+omitted)
		require.Positive(t, omitted)
	})
	t.Run("21 tasks refused", func(t *testing.T) {
		host := feedbackTestHost(t)
		q := feedbackQuery()
		q.TaskIDs = make([]string, 21)
		_, err := host.ListChangeRequestFeedback(context.Background(), q)
		require.Equal(t, codes.ResourceExhausted, status.Code(err))
	})
}
func TestFeedbackOrderingUsesProviderTieBreakers(t *testing.T) {
	at := "2026-01-02T03:04:05.000000000Z"
	items := []pluginsdk.ChangeRequestFeedbackItem{
		{CreatedAt: at, Provider: "gitlab", RepositoryID: "a", Number: 1, Kind: "review", ItemID: "1"},
		{CreatedAt: at, Provider: "github", RepositoryID: "b", Number: 1, Kind: "review", ItemID: "1"},
		{CreatedAt: at, Provider: "github", RepositoryID: "a", Number: 2, Kind: "review", ItemID: "1"},
		{CreatedAt: at, Provider: "github", RepositoryID: "a", Number: 1, Kind: "review", ItemID: "2"},
		{CreatedAt: at, Provider: "github", RepositoryID: "a", Number: 1, Kind: "comment", ItemID: "1"},
		{CreatedAt: at, Provider: "github", RepositoryID: "a", Number: 1, Kind: "review", ItemID: "1"},
	}
	sort.Slice(items, func(i, j int) bool { return feedbackItemLess(items[i], items[j]) })
	got := make([]string, len(items))
	for i, item := range items {
		got[i] = fmt.Sprintf("%s/%s/%d/%s/%s", item.Provider, item.RepositoryID, item.Number, item.Kind, item.ItemID)
	}
	require.Equal(t, []string{"github/a/1/comment/1", "github/a/1/review/1", "github/a/1/review/2", "github/a/2/review/1", "github/b/1/review/1", "gitlab/a/1/review/1"}, got)
}
func TestListChangeRequestFeedbackExactAuthorization(t *testing.T) {
	data := &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "workspace-one"}}}
	denied := newExactReadHost(t, data, []string{"change_requests"}, nil)
	_, err := denied.ListChangeRequestFeedback(context.Background(), feedbackQuery())
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	host := feedbackTestHost(t)
	q := feedbackQuery()
	q.TaskIDs = []string{"foreign-task"}
	_, foreign := host.ListChangeRequestFeedback(context.Background(), q)
	q.TaskIDs = []string{"missing-task"}
	_, missing := host.ListChangeRequestFeedback(context.Background(), q)
	require.Equal(t, codes.InvalidArgument, status.Code(foreign))
	require.Equal(t, foreign.Error(), missing.Error())
}
