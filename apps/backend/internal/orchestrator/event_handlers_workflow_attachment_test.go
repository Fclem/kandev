package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

type workflowAttachmentTransferStub struct {
	calls []struct {
		taskID, oldSessionID, newSessionID string
	}
}

func (s *workflowAttachmentTransferStub) TransferSessionMessageAttachments(_ context.Context, taskID, oldSessionID, newSessionID string) error {
	s.calls = append(s.calls, struct {
		taskID, oldSessionID, newSessionID string
	}{taskID, oldSessionID, newSessionID})
	return nil
}

func TestTransferQueuedSessionStateRebindsAttachments(t *testing.T) {
	ctx := context.Background()
	transfer := &workflowAttachmentTransferStub{}
	svc := &Service{
		logger:                      testLogger(),
		messageQueue:                messagequeue.NewServiceMemory(testLogger()),
		sessionAttachmentTransferer: transfer,
	}
	queued, err := svc.messageQueue.QueueMessage(ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new"); err != nil {
		t.Fatal(err)
	}
	if len(transfer.calls) != 1 || transfer.calls[0].taskID != "task-transfer" || transfer.calls[0].oldSessionID != "session-old" || transfer.calls[0].newSessionID != "session-new" {
		t.Fatalf("attachment transfer calls = %+v", transfer.calls)
	}
	moved, ok := svc.messageQueue.TakeQueued(ctx, "session-new")
	if !ok || moved.ID != queued.ID {
		t.Fatalf("moved queue entry = %#v, ok=%t", moved, ok)
	}
}
