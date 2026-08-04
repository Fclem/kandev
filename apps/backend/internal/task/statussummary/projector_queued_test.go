package statussummary

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
)

func TestProjectorQueueEventUpdatesQueuedPromptCount(t *testing.T) {
	projector, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-queued-count"

	publishSessionState(t, eventBus, taskID, "session-1", nil)

	projector.countQueuedPrompts = func(_ context.Context, id string) (int, error) {
		if id != taskID {
			return 0, nil
		}
		return 3, nil
	}

	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":    taskID,
		"session_id": "session-1",
	})

	summary := store.summary(taskID)
	if summary == nil {
		t.Fatal("expected a summary after the queue event")
	}
	if summary.QueuedPromptCount != 3 {
		t.Fatalf("queued prompt count = %d, want 3", summary.QueuedPromptCount)
	}
}

func TestProjectorQueueEventWithUnchangedCountDoesNotRepublish(t *testing.T) {
	projector, store, eventBus, updates, _ := newProjectorTest(t)
	const taskID = "task-queued-unchanged"

	publishSessionState(t, eventBus, taskID, "session-1", nil)
	baseline := updates.Load()

	projector.countQueuedPrompts = func(_ context.Context, id string) (int, error) {
		return 2, nil
	}
	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":    taskID,
		"session_id": "session-1",
	})
	first := updates.Load()
	if first == baseline {
		t.Fatalf("first queue event should have published a summary update (baseline %d)", baseline)
	}

	// Same count again: the projector must not bump the revision or republish.
	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"task_id":    taskID,
		"session_id": "session-1",
	})
	if got := updates.Load(); got != first {
		t.Fatalf("unchanged queued count republished: updates %d -> %d", first, got)
	}
	if summary := store.summary(taskID); summary.QueuedPromptCount != 2 {
		t.Fatalf("queued prompt count = %d, want 2", summary.QueuedPromptCount)
	}
}

func TestProjectorRestoresQueuedPromptCountFromPersistedSummary(t *testing.T) {
	_, store, eventBus, _, _ := newProjectorTest(t)
	const taskID = "task-queued-restore"

	// Seed a persisted summary carrying a queued count from a previous run.
	store.rows[taskID] = &StoredTaskStatusSummary{
		TaskID:      taskID,
		WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{
			Revision:          7,
			QueuedPromptCount: 5,
		},
	}

	publishSessionState(t, eventBus, taskID, "session-1", nil)

	summary := store.summary(taskID)
	if summary == nil {
		t.Fatal("expected a summary")
	}
	if summary.QueuedPromptCount != 5 {
		t.Fatalf("restored queued prompt count = %d, want 5", summary.QueuedPromptCount)
	}
}

func TestProjectorQueueEventWithoutTaskIDIsIgnored(t *testing.T) {
	projector, store, eventBus, _, _ := newProjectorTest(t)

	projector.countQueuedPrompts = func(_ context.Context, id string) (int, error) { return 1, nil }

	publishProjectorEvent(t, eventBus, events.MessageQueueStatusChanged, events.MessageQueueStatusChanged, map[string]interface{}{
		"session_id": "session-1",
	})

	if summary := store.summary("any-task"); summary != nil {
		t.Fatalf("queue event without task_id created a summary: %+v", summary)
	}
}
