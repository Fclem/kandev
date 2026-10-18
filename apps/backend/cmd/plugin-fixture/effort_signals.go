package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const effortEstimateAction = "estimate"
const effortSummaryAction = "summary"

// fixtureEffortSummary is test support, not an estimation algorithm.
type fixtureEffortSummary struct {
	TaskID       string                                `json:"task_id"`
	ActiveMS     int64                                 `json:"active_ms"`
	HumanPrompts int                                   `json:"human_prompts"`
	Activity     []pluginsdk.SessionUserActivity       `json:"activity"`
	Turns        []pluginsdk.SessionTurn               `json:"turns"`
	Sessions     []pluginsdk.Session                   `json:"sessions"`
	Code         []pluginsdk.SessionCodeStats          `json:"code"`
	Task         *pluginsdk.Task                       `json:"task"`
	Feedback     []pluginsdk.ChangeRequestFeedbackTask `json:"feedback"`
	Usage        []pluginsdk.TaskUsageObservation      `json:"usage"`
}

func readFixturePages[T any](read func(pluginsdk.Page) ([]T, *pluginsdk.PageInfo, error)) ([]T, error) {
	var result []T
	page := pluginsdk.Page{Limit: 100}
	for {
		rows, info, err := read(page)
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
		if info == nil || !info.HasMore {
			return result, nil
		}
		page.Cursor = info.NextCursor
	}
}

func (p *fixturePlugin) effortSignals(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	host := p.Host()
	if host == nil {
		return nil, fmt.Errorf("plugin-fixture: host unavailable")
	}
	input := struct {
		TaskID string `json:"task_id"`
	}{TaskID: req.Context.TaskID}
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &input); err != nil {
			return nil, err
		}
	}
	if input.TaskID == "" {
		return nil, fmt.Errorf("plugin-fixture: task_id required")
	}
	summary, err := readFixtureEffort(ctx, host, req.Context.WorkspaceID, input.TaskID)
	if err != nil {
		return nil, err
	}
	if req.ActionKey == effortEstimateAction {
		if err := host.SetState(ctx, "task", input.TaskID, "effort", map[string]any{"features": map[string]any{"active_ms": summary.ActiveMS, "human_prompts": summary.HumanPrompts}, effortEstimateAction: summary.HumanPrompts, "formula_version": "fixture-count-v1", "computed_at": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
			return nil, err
		}
	}
	body, err := json.Marshal(summary)
	if err != nil {
		return nil, err
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}

func readFixtureEffort(ctx context.Context, host pluginsdk.Host, workspaceID, taskID string) (fixtureEffortSummary, error) {
	result := fixtureEffortSummary{TaskID: taskID}
	filter := pluginsdk.SessionFilter{TaskIDs: []string{taskID}}
	activityFilter := pluginsdk.SessionActivityFilter{TaskIDs: []string{taskID}}
	var err error
	result.Task, err = host.Tasks().Get(ctx, taskID)
	if err != nil {
		return result, err
	}
	result.Sessions, err = readFixturePages(func(page pluginsdk.Page) ([]pluginsdk.Session, *pluginsdk.PageInfo, error) {
		return host.Sessions().List(ctx, filter, page)
	})
	if err != nil {
		return result, err
	}
	result.Activity, err = readFixturePages(func(page pluginsdk.Page) ([]pluginsdk.SessionUserActivity, *pluginsdk.PageInfo, error) {
		return host.Sessions().UserActivity(ctx, activityFilter, page)
	})
	if err != nil {
		return result, err
	}
	for _, row := range result.Activity {
		result.ActiveMS += row.ActiveMS
	}
	result.Turns, err = readFixturePages(func(page pluginsdk.Page) ([]pluginsdk.SessionTurn, *pluginsdk.PageInfo, error) {
		return host.Sessions().Turns(ctx, activityFilter, page)
	})
	if err != nil {
		return result, err
	}
	result.Code, err = readFixturePages(func(page pluginsdk.Page) ([]pluginsdk.SessionCodeStats, *pluginsdk.PageInfo, error) {
		return host.Sessions().CodeStats(ctx, filter, page)
	})
	if err != nil {
		return result, err
	}
	messages, err := readFixturePages(func(page pluginsdk.Page) ([]pluginsdk.Message, *pluginsdk.PageInfo, error) {
		return host.Messages().List(ctx, pluginsdk.MessageFilter{TaskIDs: []string{taskID}, AuthorTypes: []string{"user"}}, page)
	})
	if err != nil {
		return result, err
	}
	for _, message := range messages {
		if message.Origin == "human" {
			result.HumanPrompts++
		}
	}
	return readFixtureExactEffort(ctx, host, workspaceID, taskID, result)
}

func readFixtureExactEffort(ctx context.Context, host pluginsdk.Host, workspaceID, taskID string, result fixtureEffortSummary) (fixtureEffortSummary, error) {
	exact, ok := pluginsdk.HostExactQueries(host)
	if !ok {
		return result, fmt.Errorf("plugin-fixture: exact reads unavailable")
	}
	page := pluginsdk.ExactReadPage{Limit: 100}
	for {
		response, err := exact.ListChangeRequestFeedback(ctx, pluginsdk.ExactChangeRequestFeedbackQuery{RequestID: uuid.NewString(), WorkspaceID: workspaceID, TaskIDs: []string{taskID}, Page: page})
		if err != nil {
			return result, err
		}
		result.Feedback = append(result.Feedback, response.Items...)
		if !response.PageInfo.HasMore {
			break
		}
		page.Cursor = response.PageInfo.NextCursor
		page.SnapshotVersion = response.PageInfo.SnapshotVersion
	}
	page = pluginsdk.ExactReadPage{Limit: 100}
	for {
		response, err := exact.ListTaskUsage(ctx, pluginsdk.ExactTaskUsageQuery{RequestID: uuid.NewString(), WorkspaceID: workspaceID, TaskIDs: []string{taskID}, Page: page})
		if err != nil {
			return result, err
		}
		result.Usage = append(result.Usage, response.Items...)
		if !response.PageInfo.HasMore {
			break
		}
		page.Cursor = response.PageInfo.NextCursor
		page.SnapshotVersion = response.PageInfo.SnapshotVersion
	}
	return result, nil
}
