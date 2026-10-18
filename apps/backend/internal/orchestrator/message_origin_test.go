package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// AC-PLUGINS-EFFORT-SIGNALS-001.6: queued prompt metadata survives queue drain.
func TestQueuedPromptKeepsOrigin(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-origin", "session-origin", "step-origin")
	session, err := repo.GetTaskSession(ctx, "session-origin")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "execution-origin"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set waiting session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, session.AgentExecutionID)
	agentManager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentManager)
	svc.executor = executor.NewExecutor(agentManager, repo, testLogger(), executor.ExecutorConfig{})
	creator := &mockMessageCreator{}
	svc.messageCreator = creator
	queued := &messagequeue.QueuedMessage{
		ID: "queued-origin", TaskID: session.TaskID, SessionID: session.ID, Content: "peer prompt",
		Metadata: map[string]interface{}{
			"sender_task_id": "sender-task", "sender_session_id": "sender-session",
			messagequeue.MetadataStepHandoff: "already folded",
		},
	}
	svc.markQueuedDispatchInFlight(session.ID, queued.ID)
	svc.executeQueuedMessage(session.ID, queued)
	if len(creator.userMessages) != 1 {
		t.Fatalf("recorded user messages = %d, want 1", len(creator.userMessages))
	}
	metadata := creator.userMessages[0].metadata
	if metadata["sender_task_id"] != "sender-task" || metadata["sender_session_id"] != "sender-session" {
		t.Fatalf("queued origin markers = %#v, want the original peer-task markers", metadata)
	}
	if _, exists := metadata[messagequeue.MetadataStepHandoff]; exists {
		t.Fatalf("queue-only handoff persisted into chat metadata: %#v", metadata)
	}
	if got := models.ClassifyMessageOrigin(&models.Message{AuthorType: models.MessageAuthorUser, Metadata: metadata}); got != models.MessageOriginPeerTask {
		t.Fatalf("queued message origin = %q, want %q", got, models.MessageOriginPeerTask)
	}
}

// AC-PLUGINS-EFFORT-SIGNALS-001.6: workflow's pre-recorded prompt counts once.
func TestWorkflowAutoStartPreRecordedMessageCountsOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-workflow-origin", "session-workflow-origin", "step-workflow-origin")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	creator := &mockMessageCreator{}
	svc.messageCreator = creator
	origin := workflowMessageOrigin{StepID: "step-workflow-origin", StepName: "Implement", StepColor: "blue"}
	if !svc.recordAutoStartMessage(ctx, "task-workflow-origin", "session-workflow-origin", "workflow prompt", false, origin, nil, nil) {
		t.Fatal("recordAutoStartMessage returned false")
	}
	queued := &messagequeue.QueuedMessage{
		ID: "workflow-origin-queue", TaskID: "task-workflow-origin", SessionID: "session-workflow-origin",
		Metadata: workflowMessageMetadata(false, origin, nil),
	}
	queued.Metadata[metaKeyUserMessageRecorded] = true
	if err := svc.recordQueuedUserMessage(ctx, queued, nil); err != nil {
		t.Fatalf("record queued workflow prompt: %v", err)
	}
	if len(creator.userMessages) != 1 {
		t.Fatalf("recorded user messages = %d, want the pre-recorded row exactly once", len(creator.userMessages))
	}
	metadata := creator.userMessages[0].metadata
	if metadata["workflow_message"] != true || metadata["auto_start"] != true {
		t.Fatalf("pre-recorded workflow markers = %#v", metadata)
	}

	if got := models.ClassifyMessageOrigin(&models.Message{AuthorType: models.MessageAuthorUser, Metadata: metadata}); got != models.MessageOriginWorkflow {
		t.Fatalf("pre-recorded workflow message origin = %q, want %q", got, models.MessageOriginWorkflow)
	}
}

// AC-PLUGINS-EFFORT-SIGNALS-001.2 and .6: creator markers determine origin.
func TestUserMessageCreatorsHaveClassifiedOrigin(t *testing.T) {
	initialService := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	initialCreator := &mockMessageCreator{}
	initialService.messageCreator = initialCreator
	initialService.recordInitialMessage(context.Background(), "task-watcher", "session-watcher", "watcher prompt", false, true, nil)
	if len(initialCreator.userMessages) != 1 {
		t.Fatalf("recorded initial messages = %d, want 1", len(initialCreator.userMessages))
	}
	initialMetadata := initialCreator.userMessages[0].metadata
	prMetadata := taskPRAgentPromptMetadata(&github.TaskPR{
		RepositoryID: "repo-1", Owner: "owner", Repo: "repo", PRNumber: 1,
	}, "opened", "https://example.test/pr/1")
	mrMetadata := taskMRAgentPromptMetadata(&gitlab.TaskMR{
		MRIID: 1, ProjectPath: "group/project", RepositoryID: "repo-1",
	}, "opened", "https://example.test/mr/1")
	workflowMetadata := workflowMessageMetadata(false, workflowMessageOrigin{
		StepID: "step-1", StepName: "Implement", StepColor: "blue",
	}, nil)
	peerMetadata := NewUserMessageMeta().WithSenderTask("sender-task", "Sender", "sender-session").ToMap()
	tests := []struct {
		name    string
		message models.Message
		want    models.MessageOrigin
	}{
		{name: "human composer", message: models.Message{AuthorType: models.MessageAuthorUser}, want: models.MessageOriginHuman},
		{name: "peer task", message: models.Message{AuthorType: models.MessageAuthorUser, Metadata: peerMetadata}, want: models.MessageOriginPeerTask},
		{name: "plugin send", message: models.Message{AuthorType: models.MessageAuthorUser, Metadata: map[string]interface{}{"source": "plugin:story-points"}}, want: models.MessageOriginPlugin},
		{name: "GitHub PR automation", message: models.Message{AuthorType: models.MessageAuthorUser, Metadata: prMetadata}, want: models.MessageOriginAutomation},
		{name: "CI automation", message: models.Message{AuthorType: models.MessageAuthorUser, Metadata: ciAutomationMessageMetadata()}, want: models.MessageOriginAutomation},
		{name: "GitLab MR automation", message: models.Message{AuthorType: models.MessageAuthorUser, Metadata: mrMetadata}, want: models.MessageOriginAutomation},
		{name: "workflow auto start", message: models.Message{AuthorType: models.MessageAuthorUser, Metadata: workflowMetadata}, want: models.MessageOriginWorkflow},
		{name: "watcher initial auto start", message: models.Message{AuthorType: models.MessageAuthorUser, Metadata: initialMetadata}, want: models.MessageOriginAutomation},
		{name: "agent row", message: models.Message{AuthorType: models.MessageAuthorAgent}, want: models.MessageOriginAgent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := models.ClassifyMessageOrigin(&tt.message); got != tt.want {
				t.Fatalf("ClassifyMessageOrigin() = %q, want %q", got, tt.want)
			}
		})
	}
}
