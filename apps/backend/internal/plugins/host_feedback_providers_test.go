package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type feedbackGitHubStub struct {
	reviews  []githubsvc.PRReview
	comments []githubsvc.PRComment
	err      error
}

func (s *feedbackGitHubStub) ListPRReviews(context.Context, string, string, int) ([]githubsvc.PRReview, error) {
	return s.reviews, s.err
}
func (s *feedbackGitHubStub) ListPRComments(context.Context, string, string, int, *time.Time) ([]githubsvc.PRComment, error) {
	return s.comments, s.err
}

type feedbackGitLabStub struct {
	feedback *gitlab.MRFeedback
	err      error
}

func (s *feedbackGitLabStub) GetMRFeedbackForWorkspace(_ context.Context, workspace, host, project string, number int) (*gitlab.MRFeedback, error) {
	if workspace != "workspace-one" || host != "https://gitlab.example" || project != "group/repo" || number != 7 {
		return nil, errors.New("incorrect scoped association")
	}
	return s.feedback, s.err
}
func linkedFeedbackHost(t *testing.T) (*pluginHost, *feedbackGitHubStub, *feedbackGitLabStub) {
	host := feedbackTestHost(t)
	host.taskPRs = &stubPRSource{byTask: map[string][]*githubsvc.TaskPR{"task-one": {{RepositoryID: "repo-gh", Owner: "acme", Repo: "repo", PRNumber: 7}}}}
	host.taskMRs = &summaryMRSource{rows: map[string][]*gitlab.TaskMR{"task-one": {{RepositoryID: "repo-gl", Host: "https://gitlab.example", ProjectPath: "group/repo", MRIID: 7}}}}
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	gh := &feedbackGitHubStub{reviews: []githubsvc.PRReview{{ID: 9, Author: "private-login", Body: "private-body", State: "APPROVED", CreatedAt: at}}}
	gl := &feedbackGitLabStub{feedback: &gitlab.MRFeedback{Approvals: []gitlab.MRApproval{{UserID: 42, Username: "before-rename", CreatedAt: at}}}}
	host.service.SetChangeRequestFeedbackReaders(gh, gl)
	return host, gh, gl
}
func feedbackQuery() pluginsdk.ExactChangeRequestFeedbackQuery {
	return pluginsdk.ExactChangeRequestFeedbackQuery{RequestID: "feedback", WorkspaceID: "workspace-one", TaskIDs: []string{"task-one"}, Page: pluginsdk.ExactReadPage{Limit: 20}}
}

// @covers AC-PLUGINS-EFFORT-SIGNALS-004.4 AC-PLUGINS-EFFORT-SIGNALS-004.6
func TestFeedbackProviderFailureIsPerProvider(t *testing.T) {
	for _, failed := range []string{"github", "gitlab"} {
		for _, phase := range []string{"association", "feedback"} {
			t.Run(failed+"/"+phase, func(t *testing.T) {
				host, gh, gl := linkedFeedbackHost(t)
				core, logs := observer.New(zap.DebugLevel)
				log, err := logger.NewFromZap(zap.New(core))
				require.NoError(t, err)
				host.log = log
				host.service.log = log
				failure := errors.New("private-token private-login private-body")
				failFeedbackProvider(host, gh, gl, failed, phase, failure)
				page, err := host.ListChangeRequestFeedback(context.Background(), feedbackQuery())
				require.NoError(t, err)
				require.Len(t, page.Items, 1)
				require.Len(t, page.Items[0].Items, 1)
				for _, provider := range page.Items[0].Providers {
					if provider.Provider == failed {
						require.Equal(t, "unavailable", provider.ProviderState)
						require.Equal(t, "provider_read_failed", *provider.ProviderError)
					} else {
						require.Equal(t, "available", provider.ProviderState)
						require.Equal(t, provider.Provider, page.Items[0].Items[0].Provider)
					}
				}
				for _, entry := range logs.All() {
					encoded, err := json.Marshal(entry.ContextMap())
					require.NoError(t, err)
					require.NotContains(t, entry.Message, "private-")
					require.NotContains(t, string(encoded), "private-")
				}
			})
		}
	}
}
func TestListChangeRequestFeedbackExactApprovalIdentity(t *testing.T) {
	host, _, gl := linkedFeedbackHost(t)
	first, err := host.ListChangeRequestFeedback(context.Background(), feedbackQuery())
	require.NoError(t, err)
	original := first.Items[0].Items[1]
	require.Equal(t, "approval:42", original.ItemID)
	gl.feedback.Approvals[0].Username = "after-rename"
	gl.feedback.Approvals = append(gl.feedback.Approvals, gl.feedback.Approvals[0])
	again, err := host.ListChangeRequestFeedback(context.Background(), feedbackQuery())
	require.NoError(t, err)
	require.Len(t, again.Items[0].Items, 2, "reapproval must not create an extra active approval")
	renamed := again.Items[0].Items[1]
	require.Equal(t, original.ItemID, renamed.ItemID)
	require.Equal(t, "after-rename", renamed.AuthorLogin)
	require.Equal(t, "review", renamed.Kind)
	require.Equal(t, "APPROVED", renamed.ReviewState)
	require.Equal(t, "unknown", renamed.AuthorKind)
	require.Empty(t, renamed.Body)
}
func TestListChangeRequestFeedbackExactUnsupportedAdapter(t *testing.T) {
	host, _, _ := linkedFeedbackHost(t)
	host.service.SetChangeRequestFeedbackReaders(nil, nil)
	page, err := host.ListChangeRequestFeedback(context.Background(), feedbackQuery())
	require.NoError(t, err)
	require.Empty(t, page.Items[0].Items)
	for _, p := range page.Items[0].Providers {
		require.Equal(t, "unsupported", p.ProviderState)
	}
}

func TestListChangeRequestFeedbackExactCommentKinds(t *testing.T) {
	host, gh, gl := linkedFeedbackHost(t)
	gh.reviews = nil
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	gh.comments = []githubsvc.PRComment{{ID: 3, CommentType: "review", Author: "comment-bot", AuthorIsBot: true, Body: "inline reply", CreatedAt: at}}
	gl.feedback.Approvals = nil
	gl.feedback.Discussions = []gitlab.MRDiscussion{{Resolvable: true, Resolved: false, Notes: []gitlab.MRNote{{ID: 3, Author: "human", Body: "discussion", CreatedAt: at, System: true}}}}
	page, err := host.ListChangeRequestFeedback(context.Background(), feedbackQuery())
	require.NoError(t, err)
	require.Len(t, page.Items[0].Items, 2)
	comment := page.Items[0].Items[0]
	require.Equal(t, "review_comment", comment.Kind)
	require.Equal(t, "bot", comment.AuthorKind)
	require.Equal(t, comment.CreatedAt, comment.UpdatedAt)
	require.Nil(t, comment.Resolved)
	note := page.Items[0].Items[1]
	require.Equal(t, "discussion_note", note.Kind)
	require.NotNil(t, note.Resolved)
	require.False(t, *note.Resolved)
	require.Equal(t, note.CreatedAt, note.UpdatedAt)
}

func failFeedbackProvider(host *pluginHost, gh *feedbackGitHubStub, gl *feedbackGitLabStub, provider, phase string, failure error) {
	if phase == "association" {
		if provider == "github" {
			host.taskPRs = &stubPRSource{err: failure}
		} else {
			host.taskMRs = &summaryMRSource{err: failure}
		}
		return
	}
	if provider == "github" {
		gh.err = failure
	} else {
		gl.err = failure
	}
}
