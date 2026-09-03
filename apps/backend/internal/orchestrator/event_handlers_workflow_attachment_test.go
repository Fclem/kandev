package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

type workflowAttachmentTransferStub struct {
	calls []struct {
		taskID, oldSessionID, newSessionID string
	}
	started chan struct{}
	release chan struct{}
}

func (s *workflowAttachmentTransferStub) TransferSessionMessageAttachments(_ context.Context, taskID, oldSessionID, newSessionID string) error {
	s.calls = append(s.calls, struct {
		taskID, oldSessionID, newSessionID string
	}{taskID, oldSessionID, newSessionID})
	if s.started != nil {
		close(s.started)
		<-s.release
	}
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

func TestTransferQueuedSessionStateSerializesAttachmentTransferWithQueueMutation(t *testing.T) {
	ctx := context.Background()
	transfer := &workflowAttachmentTransferStub{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := &Service{
		logger:                      testLogger(),
		messageQueue:                messagequeue.NewServiceMemory(testLogger()),
		sessionAttachmentTransferer: transfer,
	}
	queued, err := svc.messageQueue.QueueMessage(ctx, "session-old", "task-transfer", "handoff", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	transferDone := make(chan error, 1)
	go func() {
		transferDone <- svc.transferQueuedSessionState(ctx, "task-transfer", "session-old", "session-new")
	}()
	select {
	case <-transfer.started:
	case <-time.After(time.Second):
		t.Fatal("attachment transfer did not start")
	}

	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- svc.messageQueue.UpdateMessageWithMetadata(
			ctx, "session-old", queued.ID, "changed", nil, nil, messagequeue.QueuedByUser,
		)
	}()
	select {
	case err := <-mutationDone:
		t.Fatalf("queue mutation completed during attachment transfer: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(transfer.release)

	if err := <-transferDone; err != nil {
		t.Fatal(err)
	}
	if err := <-mutationDone; err == nil {
		t.Fatal("queue mutation unexpectedly succeeded after transfer")
	}
}
