package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeleteSessionPublishesOneQueueStatusNotification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-session-delete-once", "session-delete-once", models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(func() { eventBus.Close() })
	svc.eventBus = eventBus
	repo.SetTaskSessionQueuePurgeNotifier(func(ctx context.Context, taskID, sessionID string) {
		svc.messageQueue.InvalidateEditLeasesForSession(sessionID)
		_, _ = svc.messageQueue.PurgeSession(ctx, sessionID)
		svc.publishTaskQueueStatusEvent(ctx, taskID, sessionID)
	})
	svc.sessionQueuePurgeNotifierRegistered = true

	var statusEvents atomic.Int32
	if _, err := eventBus.Subscribe(events.MessageQueueStatusChanged, func(context.Context, *bus.Event) error {
		statusEvents.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("subscribe queue status: %v", err)
	}

	if err := svc.DeleteSession(ctx, "session-delete-once"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if got := statusEvents.Load(); got != 1 {
		t.Fatalf("queue status notifications = %d, want 1", got)
	}
}

func TestDeletedSessionQueueCleanupIgnoresCancelledContext(t *testing.T) {
	queue := messagequeue.NewServiceMemory(testLogger())
	entry, err := queue.QueueMessage(
		context.Background(),
		"session-delete-cancelled",
		"task-delete-cancelled",
		"queued",
		"",
		messagequeue.QueuedByUser,
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("queue message: %v", err)
	}

	svc := &Service{messageQueue: queue, logger: testLogger()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.purgeDeletedSessionQueue(ctx, entry.TaskID, entry.SessionID)

	if got := queue.GetStatus(context.Background(), entry.SessionID).Count; got != 0 {
		t.Fatalf("deleted session queue count = %d, want 0", got)
	}
}
