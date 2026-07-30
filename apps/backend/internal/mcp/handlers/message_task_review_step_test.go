package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// assertNoTaskStateChangedEvent fails when any task.state_changed event was
// published for taskID.
func assertNoTaskStateChangedEvent(t *testing.T, ch <-chan *bus.Event, taskID string) {
	t.Helper()
	for len(ch) > 0 {
		event := <-ch
		data, ok := event.Data.(map[string]interface{})
		require.True(t, ok)
		if data["task_id"] == taskID {
			t.Fatalf("unexpected task.state_changed event for task %s: %v", taskID, data["state"])
		}
	}
}

// Regression coverage for issue #2063: messaging a task parked on a review step
// must not report IN_PROGRESS while the board still shows the task on that
// step. The workflow step is authoritative for the column, so the REVIEW →
// IN_PROGRESS reactivation only applies once on_turn_start has actually moved
// the task off the step it was parked on.

// TestHandleMessageTask_ParkedReviewStepWithoutMoveKeepsReviewState covers a
// workflow whose review step declares no on_turn_start transition (the built-in
// PR Review workflow is shaped this way). The task stays on the step, so the
// state must stay REVIEW instead of drifting to IN_PROGRESS.
func TestHandleMessageTask_ParkedReviewStepWithoutMoveKeepsReviewState(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc)
	// No on_turn_start transition configured: the step never moves.
	orch.onTurnStart = func(context.Context, string, string) error { return nil }

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "review follow-up", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.turnStartCalls, 1)
	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, "step-review", updatedTask.WorkflowStepID)
	assert.Equal(t, v1.TaskStateReview, updatedTask.State,
		"state must not drift to IN_PROGRESS while the task stays parked on the review step")
}

// TestHandleMessageTask_ParkedReviewStepMoveReactivatesTask covers the built-in
// Kanban/Architecture shape, where the review step declares
// `on_turn_start: move_to_previous`. The task leaves the review step, so the
// reactivation to IN_PROGRESS is consistent with the board.
func TestHandleMessageTask_ParkedReviewStepMoveReactivatesTask(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, sess := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "step-review"
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc)
	orch.onTurnStart = func(ctx context.Context, taskID, _ string) error {
		movedTask, err := svc.GetTask(ctx, taskID)
		require.NoError(t, err)
		movedTask.WorkflowStepID = "step-in-progress"
		return repo.UpdateTask(ctx, movedTask)
	}

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "back to work", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	require.Len(t, orch.promptCalls, 1)
	assert.Equal(t, sess.ID, orch.promptCalls[0].sessionID)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, "step-in-progress", updatedTask.WorkflowStepID)
	assert.Equal(t, v1.TaskStateInProgress, updatedTask.State)
}

// TestHandleMessageTask_ParkedReviewTaskWithoutWorkflowStepReactivates keeps
// quick-chat/ephemeral tasks (no workflow step, so no board to contradict)
// on the immediate reactivation they had before.
func TestHandleMessageTask_ParkedReviewTaskWithoutWorkflowStepReactivates(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, _ := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = ""
	require.NoError(t, repo.UpdateTask(ctx, task))

	h, orch := newMessageTaskHandler(t, svc)
	orch.onTurnStart = func(context.Context, string, string) error { return nil }

	msg := makeWSMessage(t, ws.ActionMCPMessageTask, senderPayload(target.ID, "keep going", sender.ID))
	resp, err := h.handleMessageTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateInProgress, updatedTask.State)
}
