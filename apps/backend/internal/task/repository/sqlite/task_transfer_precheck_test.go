package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestTaskTransferPrecheckReturnsCommittedReceiptBeforeBarrier(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	ctx := context.Background()
	command := taskTransferCommand(task)
	receipt, err := repo.TransferTask(ctx, command)
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if err := repo.RecordTaskTransferAttempt(ctx, command, models.TaskTransferAuditFailed); err != nil {
		t.Fatalf("RecordTaskTransferAttempt after commit: %v", err)
	}
	replay, err := repo.TaskTransferPrecheck(ctx, command)
	if err != nil {
		t.Fatalf("TaskTransferPrecheck after commit: %v", err)
	}
	if replay == nil || replay.TaskGeneration != receipt.TaskGeneration {
		t.Fatalf("precheck receipt = %+v, want committed receipt %+v", replay, receipt)
	}
}

func TestTaskTransferPrecheckReturnsTerminalRejectionForBarrier(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	ctx := context.Background()
	command := taskTransferCommand(task)
	if err := repo.RecordTaskTransferAttempt(ctx, command, models.TaskTransferAuditFailed); err != nil {
		t.Fatalf("RecordTaskTransferAttempt: %v", err)
	}
	receipt, err := repo.TaskTransferPrecheck(ctx, command)
	if receipt != nil {
		t.Fatalf("precheck receipt = %+v, want nil for barrier", receipt)
	}
	reason, ok := repoerrors.TaskTransferConflictReasonOf(err)
	if !ok || reason != repoerrors.TaskTransferOperationRejected {
		t.Fatalf("precheck err = %v, want operation rejected conflict", err)
	}
}
