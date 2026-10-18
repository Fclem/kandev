package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type feedbackParityManager struct{ ExactQueryManager }

type feedbackParityHost struct{ *dataRecordingHost }

func (h *feedbackParityHost) ExactQueries() ExactQueryManager { return feedbackParityManager{} }
func feedbackParityRow() ChangeRequestFeedbackTask {
	reason := "provider_read_failed"
	resolved := false
	return ChangeRequestFeedbackTask{TaskID: "task-one", Providers: []ChangeRequestFeedbackProviderResult{{Provider: "github", ProviderState: "available", ItemsOmitted: 2}, {Provider: "gitlab", ProviderState: "unavailable", ProviderError: &reason}}, Items: []ChangeRequestFeedbackItem{{
		Provider: "github", RepositoryID: "repo-one", RepositoryOwner: "owner", RepositoryName: "repo", Number: 7, ItemID: "review-42", Kind: "review", AuthorLogin: "reviewer", AuthorKind: "unknown", ReviewState: "APPROVED", Resolved: &resolved, CreatedAt: "2026-01-02T03:04:05Z", UpdatedAt: "2026-01-02T03:04:05Z", Body: "bounded feedback", BodyTruncated: true,
	}}}
}
func (feedbackParityManager) ListChangeRequestFeedback(_ context.Context, q ExactChangeRequestFeedbackQuery) (ExactChangeRequestFeedbackPage, error) {
	return ExactChangeRequestFeedbackPage{Items: []ChangeRequestFeedbackTask{feedbackParityRow()}, PageInfo: ExactReadPageInfo{SnapshotVersion: "snapshot", Receipt: HostReadReceipt{RequestID: q.RequestID, WorkspaceID: q.WorkspaceID}}}, nil
}
func testFeedbackRoundTrip(t *testing.T) {
	expected := feedbackParityRow()
	require.Equal(t, expected, feedbackTaskFromProto(feedbackTaskToProto(expected)))
	*expected.Items[0].Resolved = true
	require.Equal(t, expected, feedbackTaskFromProto(feedbackTaskToProto(expected)))
	expected.Items[0].Resolved = nil
	require.Equal(t, expected, feedbackTaskFromProto(feedbackTaskToProto(expected)))
}
