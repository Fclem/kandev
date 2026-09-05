package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func TestSendNowOrdinaryClaimRestoresAfterProcessRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/queue.db"
	queue, db := newWorkflowTransferQueue(t, dbPath)
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{*source}); err != nil {
		t.Fatal(err)
	}
	if queue.GetStatus(ctx, "session-1").Count != 0 {
		t.Fatal("claimed ordinary prompt remained visible before restart")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restartedQueue, restartedDB := newWorkflowTransferQueue(t, dbPath)
	t.Cleanup(func() { _ = restartedDB.Close() })
	restarted := &Service{logger: testLogger(), messageQueue: restartedQueue}
	if err := restarted.reconcilePendingSendNowClaimsOnStartup(ctx); err != nil {
		t.Fatal(err)
	}
	status := restartedQueue.GetStatus(ctx, "session-1")
	if len(status.Entries) != 1 || status.Entries[0].ID != source.ID {
		t.Fatalf("queue after restart recovery = %#v, want source %s", status.Entries, source.ID)
	}
}

func TestSendNowWorkerCancellationAfterClaimRestoresOrdinarySource(t *testing.T) {
	ctx := context.Background()
	queue, db := newWorkflowTransferQueue(t, t.TempDir()+"/queue.db")
	t.Cleanup(func() { _ = db.Close() })
	source, err := queue.QueueMessage(
		ctx, "session-1", "task-1", "ordinary prompt", "", messagequeue.QueuedByUser, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := queue.ClaimSendNow(ctx, "session-1", []messagequeue.QueuedMessage{*source})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	cancel()
	svc := &Service{logger: testLogger(), messageQueue: queue}

	svc.executeSendNowClaimWithContext(workerCtx, claim, nil)

	status := queue.GetStatus(ctx, "session-1")
	if len(status.Entries) != 1 || status.Entries[0].ID != source.ID {
		t.Fatalf("queue after cancelled claimed worker = %#v, want source %s", status.Entries, source.ID)
	}
}
