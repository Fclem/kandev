package plugins

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	feedbackMaxTasks         = 20
	feedbackMaxItems         = 50
	feedbackMaxBytes         = 7 << 20
	feedbackStateAvailable   = "available"
	feedbackStateUnavailable = "unavailable"
	feedbackStateUnsupported = "unsupported"
)

func (h *pluginHost) ListChangeRequestFeedback(ctx context.Context, q pluginsdk.ExactChangeRequestFeedbackQuery) (pluginsdk.ExactChangeRequestFeedbackPage, error) {
	identity, unlock, err := h.exactReadAdmission(ctx, q.RequestID, q.WorkspaceID, exactReadChangeFeedback, "host.v2.read:change_requests", q.TaskIDs)
	if err != nil {
		return pluginsdk.ExactChangeRequestFeedbackPage{}, err
	}
	defer unlock()
	if len(q.TaskIDs) > feedbackMaxTasks {
		return pluginsdk.ExactChangeRequestFeedbackPage{}, status.Error(codes.ResourceExhausted, "feedback query exceeds 20 tasks")
	}
	rows, info, err := exactPage(h, identity, q.RequestID, q.Page, func() ([]pluginsdk.ChangeRequestFeedbackTask, error) {
		ids := append([]string(nil), q.TaskIDs...)
		tasks, err := h.fetchTasksForWorkspaces(ctx, []string{q.WorkspaceID}, true, true)
		if err != nil {
			return nil, err
		}
		allowed := make(map[string]bool, len(tasks))
		for _, task := range tasks {
			if task != nil {
				allowed[task.ID] = true
			}
		}
		if len(ids) == 0 {
			for id := range allowed {
				ids = append(ids, id)
			}
		}
		if len(ids) > feedbackMaxTasks {
			return nil, status.Error(codes.ResourceExhausted, "feedback query exceeds 20 tasks")
		}
		sort.Strings(ids)
		unique := ids[:0]
		for _, id := range ids {
			if !allowed[id] {
				return nil, status.Error(codes.InvalidArgument, "feedback task is outside the workspace")
			}
			if len(unique) == 0 || unique[len(unique)-1] != id {
				unique = append(unique, id)
			}
		}
		result := make([]pluginsdk.ChangeRequestFeedbackTask, 0, len(unique))
		for _, id := range unique {
			row := pluginsdk.ChangeRequestFeedbackTask{TaskID: id, Items: []pluginsdk.ChangeRequestFeedbackItem{}}
			gh, items := h.githubFeedback(ctx, id)
			row.Providers = append(row.Providers, gh)
			row.Items = append(row.Items, items...)
			gl, items := h.gitlabFeedback(ctx, q.WorkspaceID, id)
			row.Providers = append(row.Providers, gl)
			row.Items = append(row.Items, items...)
			sort.Slice(row.Items, func(i, j int) bool { return feedbackItemLess(row.Items[i], row.Items[j]) })
			if len(row.Items) > feedbackMaxItems {
				omitFeedbackItems(&row, len(row.Items)-feedbackMaxItems)
			}
			result = append(result, row)
		}
		boundFeedbackBytes(result)
		return result, nil
	})
	return pluginsdk.ExactChangeRequestFeedbackPage{Items: rows, PageInfo: info}, err
}

func (h *pluginHost) githubFeedback(ctx context.Context, taskID string) (pluginsdk.ChangeRequestFeedbackProviderResult, []pluginsdk.ChangeRequestFeedbackItem) {
	result := pluginsdk.ChangeRequestFeedbackProviderResult{Provider: taskmodels.RemoteContributionProviderGitHub, ProviderState: feedbackStateAvailable}
	source := h.taskPRs
	if h.taskPRsDep != nil {
		source = h.taskPRsDep()
	}
	if source == nil {
		return feedbackUnavailable(result), nil
	}
	rows, err := source.ListTaskPRsByTaskIDs(ctx, []string{taskID})
	if err != nil {
		return feedbackUnavailable(result), nil
	}
	reader := h.feedbackReaders().github
	var items []pluginsdk.ChangeRequestFeedbackItem
	for _, pr := range rows[taskID] {
		if pr == nil {
			continue
		}
		if reader == nil {
			result.ProviderState = feedbackStateUnsupported
			return result, nil
		}
		next, err := githubFeedbackItems(ctx, reader, pr)
		if err != nil {
			return feedbackUnavailable(result), nil
		}
		items = append(items, next...)
	}
	return result, items
}

func (h *pluginHost) gitlabFeedback(ctx context.Context, workspaceID, taskID string) (pluginsdk.ChangeRequestFeedbackProviderResult, []pluginsdk.ChangeRequestFeedbackItem) {
	result := pluginsdk.ChangeRequestFeedbackProviderResult{Provider: taskmodels.RemoteContributionProviderGitLab, ProviderState: feedbackStateAvailable}
	source := h.taskMRs
	if h.taskMRsDep != nil {
		source = h.taskMRsDep()
	}
	if source == nil {
		return feedbackUnavailable(result), nil
	}
	rows, err := source.ListTaskMRsByTaskIDs(ctx, []string{taskID})
	if err != nil {
		return feedbackUnavailable(result), nil
	}
	reader := h.feedbackReaders().gitlab
	var items []pluginsdk.ChangeRequestFeedbackItem
	for _, mr := range rows[taskID] {
		if mr == nil {
			continue
		}
		if reader == nil {
			result.ProviderState = feedbackStateUnsupported
			return result, nil
		}
		feedback, err := reader.GetMRFeedbackForWorkspace(ctx, workspaceID, mr.Host, mr.ProjectPath, mr.MRIID)
		if err != nil || feedback == nil {
			return feedbackUnavailable(result), nil
		}
		next, err := gitlabFeedbackItems(mr, feedback)
		if err != nil {
			return feedbackUnavailable(result), nil
		}
		items = append(items, next...)
	}
	return result, items
}

func feedbackUnavailable(result pluginsdk.ChangeRequestFeedbackProviderResult) pluginsdk.ChangeRequestFeedbackProviderResult {
	reason := "provider_read_failed"
	result.ProviderState = feedbackStateUnavailable
	result.ProviderError = &reason
	return result
}

func feedbackItemLess(a, b pluginsdk.ChangeRequestFeedbackItem) bool {
	// Timestamps are normalized to a fixed-width UTC representation by adapters.
	if a.CreatedAt != b.CreatedAt {
		return a.CreatedAt < b.CreatedAt
	}
	if a.Provider != b.Provider {
		return a.Provider < b.Provider
	}
	if a.RepositoryID != b.RepositoryID {
		return a.RepositoryID < b.RepositoryID
	}
	if a.Number != b.Number {
		return a.Number < b.Number
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.ItemID < b.ItemID
}

func omitFeedbackItems(row *pluginsdk.ChangeRequestFeedbackTask, count int) {
	for _, item := range row.Items[:count] {
		for i := range row.Providers {
			if row.Providers[i].Provider == item.Provider {
				row.Providers[i].ItemsOmitted++
			}
		}
	}
	row.Items = row.Items[count:]
}

func boundFeedbackBytes(rows []pluginsdk.ChangeRequestFeedbackTask) {
	sizes := make([][]int, len(rows))
	total := 2
	if len(rows) > 0 {
		total += len(rows) - 1
	}
	for i, row := range rows {
		empty := row
		empty.Items = []pluginsdk.ChangeRequestFeedbackItem{}
		encoded, _ := json.Marshal(empty)
		total += len(encoded)
		sizes[i] = make([]int, len(row.Items))
		for j, item := range row.Items {
			encoded, _ := json.Marshal(item)
			sizes[i][j] = len(encoded)
			total += len(encoded)
			if j > 0 {
				total++
			}
		}
	}
	for total > feedbackMaxBytes {
		oldest := -1
		for i := range rows {
			if len(rows[i].Items) > 0 && (oldest < 0 || feedbackItemLess(rows[i].Items[0], rows[oldest].Items[0])) {
				oldest = i
			}
		}
		if oldest < 0 {
			return
		}
		row := &rows[oldest]
		total -= sizes[oldest][0]
		sizes[oldest] = sizes[oldest][1:]
		if len(row.Items) > 1 {
			total--
		}
		for _, provider := range row.Providers {
			if provider.Provider == row.Items[0].Provider {
				total += len(strconv.FormatInt(provider.ItemsOmitted+1, 10)) - len(strconv.FormatInt(provider.ItemsOmitted, 10))
			}
		}
		omitFeedbackItems(row, 1)
	}
}

// Narrow provider contracts retain native credential and host resolution.
type githubFeedbackReader interface {
	ListPRReviews(context.Context, string, string, int) ([]githubsvc.PRReview, error)
	ListPRComments(context.Context, string, string, int, *time.Time) ([]githubsvc.PRComment, error)
}
type gitlabFeedbackReader interface {
	GetMRFeedbackForWorkspace(context.Context, string, string, string, int) (*gitlab.MRFeedback, error)
}
type feedbackReaders struct {
	github githubFeedbackReader
	gitlab gitlabFeedbackReader
}

func (s *Service) SetChangeRequestFeedbackReaders(gh githubFeedbackReader, gl gitlabFeedbackReader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changeFeedback = feedbackReaders{github: gh, gitlab: gl}
}
func (h *pluginHost) feedbackReaders() feedbackReaders {
	if h.service == nil {
		return feedbackReaders{}
	}
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	return h.service.changeFeedback
}
