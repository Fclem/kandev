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
	first, err := svc.GetPRFeedbackItemsForWorkspace(ctx, testWorkspaceID, "", feedbackSyncOwner, feedbackSyncRepo, 2408)
	require.NoError(t, err)
	require.Len(t, first.Reviews, 1)
	require.Equal(t, "first review", first.Reviews[0].Body)
	client.AddReviews(feedbackSyncOwner, feedbackSyncRepo, 2408, []PRReview{{ID: 2, Body: "updated review"}})
	second, err := svc.GetPRFeedbackItemsForWorkspace(ctx, testWorkspaceID, "", feedbackSyncOwner, feedbackSyncRepo, 2408)
	require.NoError(t, err)
	require.Len(t, second.Reviews, 2)
	require.Equal(t, "updated review", second.Reviews[1].Body)
	row, err := store.GetTaskPRByRepoAndNumber(ctx, "feedback-read-only", "repo-1", 2408)
	require.NoError(t, err)
	require.Equal(t, prStateOpen, row.State)
	require.Equal(t, 9, row.ReviewCount)
}
