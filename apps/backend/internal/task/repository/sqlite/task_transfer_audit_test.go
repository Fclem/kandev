package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRecordTaskTransferAttemptPersistsRedactedSchemaRejectedFields(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	command := models.TaskTransferCommand{
		ExpectedSourceWorkspaceID: "ws-source",
		Actor: models.TaskTransferActor{
			Kind: models.TaskTransferActorRejected,
			ID:   "human-1",
		},
	}

	if err := repo.RecordTaskTransferAttempt(context.Background(), command, "failed"); err != nil {
		t.Fatalf("RecordTaskTransferAttempt: %v", err)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_transfer_audit
		WHERE task_id = '' AND source_workspace_id = ? AND result = 'failed'`, "ws-source"); err != nil {
		t.Fatalf("read task transfer audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("task transfer audit rows = %d, want 1", count)
	}
}

func TestRecordTaskTransferAttemptDeduplicatesStableAttemptID(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	command := models.TaskTransferCommand{
		AuditAttemptID: "attempt-1",
		Actor:          models.TaskTransferActor{Kind: models.TaskTransferActorRejected, ID: "human-1"},
	}

	for range 2 {
		if err := repo.RecordTaskTransferAttempt(context.Background(), command, "failed"); err != nil {
			t.Fatalf("RecordTaskTransferAttempt: %v", err)
		}
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_transfer_audit WHERE id = ?`, command.AuditAttemptID); err != nil {
		t.Fatalf("read task transfer audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("task transfer audit rows = %d, want 1", count)
	}
}

// An unknown result must not be coerced into a failed row: failed is a durable
// Web barrier, so a typo would otherwise terminalize a retryable key.
func TestRecordTaskTransferAttemptRejectsUnknownResult(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	command := taskTransferCommand(task)
	command.DurableRejection = true

	if err := repo.RecordTaskTransferAttempt(context.Background(), command, "denyed"); err == nil {
		t.Fatal("RecordTaskTransferAttempt accepted an unknown result")
	}
	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_transfer_audit WHERE idempotency_key = ?`,
		command.IdempotencyKey); err != nil {
		t.Fatalf("read task transfer audit: %v", err)
	}
	if count != 0 {
		t.Fatalf("task transfer audit rows = %d, want 0", count)
	}
	if _, err := repo.TransferTask(context.Background(), command); err != nil {
		t.Fatalf("durable transfer after a refused audit write: %v", err)
	}
}
