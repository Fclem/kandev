package github

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceFeedbackItemsStayLiveAndReadOnly(t *testing.T) {
	_, svc, client, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "feedback-read-only", false)
	seedFeedbackTaskPR(t, store, "feedback-read-only", 2408, func(row *TaskPR) { row.ReviewCount = 9 })
	addMergedFeedbackPR(client, 2408, time.Now().UTC())
	client.AddReviews(feedbackSyncOwner, feedbackSyncRepo, 2408, []PRReview{{ID: 1, Body: "first review"}})
	first, err := svc.GetPRFeedbackItemsForWorkspace(ctx, testWorkspaceID, feedbackSyncOwner, feedbackSyncRepo, 2408)
	require.NoError(t, err)
	require.Len(t, first.Reviews, 1)
	require.Equal(t, "first review", first.Reviews[0].Body)
	client.AddReviews(feedbackSyncOwner, feedbackSyncRepo, 2408, []PRReview{{ID: 2, Body: "updated review"}})
	second, err := svc.GetPRFeedbackItemsForWorkspace(ctx, testWorkspaceID, feedbackSyncOwner, feedbackSyncRepo, 2408)
	require.NoError(t, err)
	require.Len(t, second.Reviews, 2)
	require.Equal(t, "updated review", second.Reviews[1].Body)
	row, err := store.GetTaskPRByRepoAndNumber(ctx, "feedback-read-only", "repo-1", 2408)
	require.NoError(t, err)
	require.Equal(t, prStateOpen, row.State)
	require.Equal(t, 9, row.ReviewCount)
}

type feedbackBoundaryClient struct {
	*MockClient
	providerCalls int
}

func (c *feedbackBoundaryClient) HasRepositoryAccess(context.Context, string, string) (bool, error) {
	c.providerCalls++
	return true, nil
}

func (c *feedbackBoundaryClient) ListPRReviews(ctx context.Context, owner, repo string, number int) ([]PRReview, error) {
	c.providerCalls++
	return c.MockClient.ListPRReviews(ctx, owner, repo, number)
}

func TestWorkspaceFeedbackItemsRejectOutsideScopeWithoutProviderCall(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	seedConnectionWorkspaces(t, store, "workspace-a")
	require.NoError(t, store.UpsertWorkspaceSettings(ctx, &WorkspaceSettings{
		WorkspaceID: "workspace-a", RepoScopeMode: RepoScopeModeRepos,
		RepoScopeRepos: []RepoFilter{{Owner: "o", Name: "allowed"}},
	}))
	client := &feedbackBoundaryClient{MockClient: NewMockClient()}
	service := NewService(client, AuthMethodPAT, nil, store, nil, testLogger(t))
	configureTestWorkspaceAuth(t, service, client, "workspace-a")
	_, err := service.GetPRFeedbackItemsForWorkspace(ctx, "workspace-a", "o", "outside", 1)
	require.ErrorIs(t, err, ErrRepoNotResolvable)
	require.Zero(t, client.providerCalls)
}
