package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestDispatchTakenQueuedMessageRestoresEmptyOrdinaryEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	_, err := svc.messageQueue.QueueMessage(ctx, "session-empty", "task-empty", "", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue empty ordinary entry: %v", err)
	}
	queued, ok := svc.messageQueue.TakeQueued(ctx, "session-empty")
	if !ok || queued == nil {
		t.Fatal("take empty ordinary entry")
	}

	if svc.dispatchTakenQueuedMessage(ctx, "session-empty", queued, true) {
		t.Fatal("empty ordinary entry was dispatched")
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-empty").Count; got != 1 {
		t.Fatalf("empty ordinary entry count = %d, want restored entry", got)
	}
}

func TestDispatchTakenQueuedMessageAcknowledgesEmptyLifecycleEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	_, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "session-empty-lifecycle", "task-empty-lifecycle", "", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin}, "empty-lifecycle", true,
	)
	if err != nil || !accepted {
		t.Fatalf("queue empty lifecycle entry: accepted=%v err=%v", accepted, err)
	}
	queued, ok := svc.messageQueue.ReserveQueued(ctx, "session-empty-lifecycle")
	if !ok || queued == nil {
		t.Fatal("reserve empty lifecycle entry")
	}

	if svc.dispatchTakenQueuedMessage(ctx, "session-empty-lifecycle", queued, true) {
		t.Fatal("empty lifecycle entry was dispatched")
	}
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, "session-empty-lifecycle")
	if err != nil {
		t.Fatalf("snapshot empty lifecycle queue: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("empty lifecycle entries after discard = %d, want 0", len(entries))
	}
}

func TestTakeAndMergeHandoffMessageRestoresEmptyEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	_, err := svc.messageQueue.QueueMessage(ctx, "session-empty-handoff", "task-empty-handoff", "", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue empty handoff: %v", err)
	}

	msg, prompt, attachments, references := svc.takeAndMergeHandoffMessage(ctx, "session-empty-handoff", "base")
	if msg != nil || prompt != "base" || attachments != nil || references != nil {
		t.Fatalf("empty handoff result = msg:%v prompt:%q attachments:%v references:%v", msg, prompt, attachments, references)
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-empty-handoff").Count; got != 1 {
		t.Fatalf("empty handoff count = %d, want restored entry", got)
	}
}

func TestHandleAgentReadyPassthroughAttachmentFailureRestoresEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-attachment-failure", "session-attachment-failure", "step1")
	session, err := repo.GetTaskSession(ctx, "session-attachment-failure")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.WorkspacePath = t.TempDir()
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session workspace: %v", err)
	}
	seedExecutorRunning(t, repo, "session-attachment-failure", "task-attachment-failure", "execution-attachment-failure")
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-attachment-failure", v1.TaskStateReview)
	agentMgr := &mockAgentManager{
		isPassthrough:          true,
		isAgentRunning:         true,
		passthroughStdinErr:    errors.New("PTY write failed"),
		repoForExecutionLookup: repo,
	}
	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "workflow1"}
	svc := createTestServiceWithAgent(repo, steps, taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.messageQueue.QueueMessage(
		ctx, "session-attachment-failure", "task-attachment-failure", "read attached file", "",
		messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{Type: "resource", Data: "AQ==", MimeType: "text/plain", Name: "note.txt"}},
	)
	if err != nil {
		t.Fatalf("queue attachment entry: %v", err)
	}

	svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "task-attachment-failure", SessionID: "session-attachment-failure"})
	if got := svc.messageQueue.GetStatus(ctx, "session-attachment-failure").Count; got != 1 {
		t.Fatalf("attachment entry count after PTY failure = %d, want restored entry", got)
	}
}
