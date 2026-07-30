package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Regression coverage for issue #2063 on the WS chat path: a task parked on a
// review step must not be reported IN_PROGRESS while the board still shows it
// on that step. The workflow step owns the column, so the REVIEW → IN_PROGRESS
// reactivation only applies once on_turn_start moved the task off the step.

// reviewStepRepo persists task-state writes so the handler's reactivation is
// observable; messageAddSwitchRepo's embedded mock swallows them.
type reviewStepRepo struct {
	messageAddSwitchRepo
}

func (r *reviewStepRepo) UpdateTaskState(_ context.Context, id string, state v1.TaskState) error {
	if task, ok := r.tasks[id]; ok {
		task.State = state
	}
	return nil
}

// reviewStepOrchestrator models a workflow step's on_turn_start behaviour:
// moveToStepID is empty when the step declares no transition.
type reviewStepOrchestrator struct {
	repo         *reviewStepRepo
	moveToStepID string
	prompted     chan struct{}
}

func (o *reviewStepOrchestrator) PromptTask(
	context.Context, string, string, string, string, bool, []v1.MessageAttachment, bool,
) (*orchestrator.PromptResult, error) {
	close(o.prompted)
	return &orchestrator.PromptResult{}, nil
}

func (o *reviewStepOrchestrator) ResumeTaskSession(context.Context, string, string) error {
	return nil
}

func (o *reviewStepOrchestrator) StartCreatedSession(
	context.Context, string, string, string, string, bool, bool, bool,
	[]v1.MessageAttachment, []v1.EntityReference,
) error {
	return nil
}

func (o *reviewStepOrchestrator) ProcessOnTurnStart(_ context.Context, taskID, _ string) error {
	if o.moveToStepID == "" {
		return nil
	}
	if task, ok := o.repo.tasks[taskID]; ok {
		task.WorkflowStepID = o.moveToStepID
	}
	return nil
}

func (o *reviewStepOrchestrator) StepRequiresCompletionSignal(context.Context, string) bool {
	return false
}

func (*reviewStepOrchestrator) ForegroundActivity(string) v1.ForegroundActivity {
	return ""
}

// runParkedReviewMessage sends one chat message to a task parked in REVIEW on
// stepID and returns the task as it stands once the prompt was dispatched.
func runParkedReviewMessage(t *testing.T, stepID, moveToStepID string) *models.Task {
	t.Helper()
	now := time.Now().UTC()
	repo := &reviewStepRepo{messageAddSwitchRepo: messageAddSwitchRepo{
		tasks: map[string]*models.Task{"t1": {
			ID: "t1", WorkspaceID: "ws1", State: v1.TaskStateReview,
			WorkflowStepID: stepID, UpdatedAt: now,
		}},
		sessions: map[string]*models.TaskSession{
			"s1": {
				ID: "s1", TaskID: "t1", State: models.TaskSessionStateWaitingForInput,
				AgentProfileID: "profile-1", UpdatedAt: now,
			},
		},
		primaryID: "s1",
	}}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Tasks: repo, TaskRepos: repo, Messages: repo, Turns: repo, Sessions: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &reviewStepOrchestrator{repo: repo, moveToStepID: moveToStepID, prompted: make(chan struct{})}
	h := NewMessageHandlers(svc, orch, log)

	req, err := ws.NewRequest("req-review", ws.ActionMessageAdd, map[string]any{
		"task_id": "t1", "session_id": "s1", "content": "another round please",
	})
	require.NoError(t, err)
	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	select {
	case <-orch.prompted:
	case <-time.After(time.Second):
		t.Fatal("message was not dispatched to the agent")
	}
	return repo.tasks["t1"]
}

func TestWSAddMessage_ParkedReviewStepWithoutMoveKeepsReviewState(t *testing.T) {
	task := runParkedReviewMessage(t, "step-review", "")
	assert.Equal(t, "step-review", task.WorkflowStepID)
	assert.Equal(t, v1.TaskStateReview, task.State,
		"state must not drift to IN_PROGRESS while the task stays parked on the review step")
}

func TestWSAddMessage_ParkedReviewStepMoveReactivatesTask(t *testing.T) {
	task := runParkedReviewMessage(t, "step-review", "step-in-progress")
	assert.Equal(t, "step-in-progress", task.WorkflowStepID)
	assert.Equal(t, v1.TaskStateInProgress, task.State)
}

func TestWSAddMessage_ParkedReviewTaskWithoutWorkflowStepReactivates(t *testing.T) {
	task := runParkedReviewMessage(t, "", "")
	assert.Equal(t, v1.TaskStateInProgress, task.State)
}
