package plugins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/sysprompt"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const feedbackTimestampFormat = "2006-01-02T15:04:05.000000000Z"

const (
	feedbackKindReview    = "review"
	feedbackKindComment   = "comment"
	feedbackAuthorUnknown = "unknown"
	feedbackAuthorHuman   = "human"
	feedbackAuthorBot     = "bot"
)

func feedbackBody(body string) (string, bool) {
	body = sysprompt.StripSystemContent(body)
	if len(body) <= 4096 {
		return body, false
	}
	end := 4096
	for !utf8.RuneStart(body[end]) {
		end--
	}
	return body[:end], true
}
func githubFeedbackItems(ctx context.Context, reader githubFeedbackReader, workspaceID string, pr *githubsvc.TaskPR) ([]pluginsdk.ChangeRequestFeedbackItem, error) {
	feedback, err := reader.GetPRFeedbackItemsForWorkspace(ctx, workspaceID, pr.Owner, pr.Repo, pr.PRNumber)
	if err != nil {
		return nil, err
	}
	if feedback == nil {
		return nil, fmt.Errorf("GitHub feedback unavailable")
	}
	reviews, comments := feedback.Reviews, feedback.Comments
	items := make([]pluginsdk.ChangeRequestFeedbackItem, 0, len(reviews)+len(comments))
	base := pluginsdk.ChangeRequestFeedbackItem{Provider: taskmodels.RemoteContributionProviderGitHub, RepositoryID: pr.RepositoryID, RepositoryOwner: pr.Owner, RepositoryName: pr.Repo, Number: int64(pr.PRNumber)}
	for _, review := range reviews {
		item := base
		item.ItemID = strconv.FormatInt(review.ID, 10)
		item.Kind = feedbackKindReview
		item.AuthorLogin = review.Author
		item.AuthorKind = feedbackAuthorUnknown
		if strings.HasSuffix(review.Author, "[bot]") {
			item.AuthorKind = feedbackAuthorBot
		}
		item.ReviewState = review.State
		item.CreatedAt = review.CreatedAt.UTC().Format(feedbackTimestampFormat)
		item.UpdatedAt = item.CreatedAt
		item.Body, item.BodyTruncated = feedbackBody(review.Body)
		items = append(items, item)
	}
	for _, comment := range comments {
		item := base
		item.ItemID = strconv.FormatInt(comment.ID, 10)
		item.Kind = feedbackKindComment
		if comment.Path != "" || comment.CommentType == feedbackKindReview {
			item.Kind = "review_comment"
		}
		item.AuthorLogin = comment.Author
		item.AuthorKind = feedbackAuthorHuman
		if comment.AuthorIsBot {
			item.AuthorKind = feedbackAuthorBot
		}
		item.CreatedAt = comment.CreatedAt.UTC().Format(feedbackTimestampFormat)
		updated := comment.UpdatedAt
		if updated.IsZero() {
			updated = comment.CreatedAt
		}
		item.UpdatedAt = updated.UTC().Format(feedbackTimestampFormat)
		item.Body, item.BodyTruncated = feedbackBody(comment.Body)
		items = append(items, item)
	}
	return items, nil
}
func gitlabFeedbackItems(mr *gitlab.TaskMR, feedback *gitlab.MRFeedback) ([]pluginsdk.ChangeRequestFeedbackItem, error) {
	owner, name, _ := strings.Cut(mr.ProjectPath, "/")
	if at := strings.LastIndex(mr.ProjectPath, "/"); at >= 0 {
		owner = mr.ProjectPath[:at]
		name = mr.ProjectPath[at+1:]
	}
	base := pluginsdk.ChangeRequestFeedbackItem{Provider: taskmodels.RemoteContributionProviderGitLab, RepositoryID: mr.RepositoryID, RepositoryOwner: owner, RepositoryName: name, Number: int64(mr.MRIID)}
	items := make([]pluginsdk.ChangeRequestFeedbackItem, 0, len(feedback.Approvals)+len(feedback.Discussions))
	seen := make(map[int64]bool, len(feedback.Approvals))
	for _, approval := range feedback.Approvals {
		if approval.UserID <= 0 {
			return nil, fmt.Errorf("approval user identity is unavailable")
		}
		if seen[approval.UserID] {
			continue
		}
		seen[approval.UserID] = true
		item := base
		item.ItemID = "approval:" + strconv.FormatInt(approval.UserID, 10)
		item.Kind = feedbackKindReview
		item.AuthorLogin = approval.Username
		item.AuthorKind = feedbackAuthorUnknown
		item.ReviewState = "APPROVED"
		item.CreatedAt = approval.CreatedAt.UTC().Format(feedbackTimestampFormat)
		item.UpdatedAt = item.CreatedAt
		items = append(items, item)
	}
	for _, discussion := range feedback.Discussions {
		for _, note := range discussion.Notes {
			if note.System {
				continue
			}
			item := base
			item.ItemID = strconv.FormatInt(note.ID, 10)
			item.Kind = "discussion_note"
			item.AuthorLogin = note.Author
			item.AuthorKind = feedbackAuthorHuman
			if note.AuthorIsBot {
				item.AuthorKind = feedbackAuthorBot
			}
			if discussion.Resolvable {
				resolved := discussion.Resolved
				item.Resolved = &resolved
			}
			item.CreatedAt = note.CreatedAt.UTC().Format(feedbackTimestampFormat)
			updated := note.UpdatedAt
			if updated.IsZero() {
				updated = note.CreatedAt
			}
			item.UpdatedAt = updated.UTC().Format(feedbackTimestampFormat)
			item.Body, item.BodyTruncated = feedbackBody(note.Body)
			items = append(items, item)
		}
	}
	return items, nil
}
