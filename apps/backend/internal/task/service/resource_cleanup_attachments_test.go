package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDurableDeleteCleanupRemovesTaskAttachments(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	taskResult, err := taskSvc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Attachment cleanup", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	taskID := taskResult.Task.ID

	storageRoot := t.TempDir()
	attachmentSvc, err := NewAttachmentService(repo, storageRoot, nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	taskSvc.attachmentSvc = attachmentSvc
	attachment, err := attachmentSvc.Stage(
		ctx, "owner", "ws-1", "cleanup.txt", "text/plain", "resource", "",
		strings.NewReader("attachment"),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := attachmentSvc.Claim(ctx, "owner", "ws-1", taskID, "", []string{attachment.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	attachmentPath := filepath.Join(storageRoot, "attachments", attachment.StorageKey)
	if _, err := os.Stat(attachmentPath); err != nil {
		t.Fatalf("staged attachment bytes: %v", err)
	}
	if err := repo.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	job := &models.TaskResourceCleanupJob{
		ID: "job-attachment-cleanup", OperationID: "delete:attachment-cleanup",
		TaskID: taskID, Trigger: models.TaskResourceCleanupTriggerCascadeDelete,
		State: models.TaskResourceCleanupStateRunning,
	}
	if err := taskSvc.executeTaskResourceCleanupJob(ctx, job, &taskResourceCleanupSnapshot{}); err != nil {
		t.Fatalf("executeTaskResourceCleanupJob: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, attachment.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("attachment row error = %v, want ErrAttachmentNotFound", err)
	}
	if _, err := os.Stat(attachmentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attachment bytes error = %v, want os.ErrNotExist", err)
	}
}

func TestRetryTaskResourceCleanupPersistsAfterDeadlineExpiry(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{
		ID: "job-expired-retry", OperationID: "delete:expired-retry",
		TaskID: "task-expired-retry", Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || !claimed {
		t.Fatalf("MarkTaskResourceCleanupJobRunning = %v, %v", claimed, err)
	}
	job, err = repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}

	expiredCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	cleanupErr := errors.New("cleanup deadline exceeded")
	if err := taskSvc.retryTaskResourceCleanupJob(expiredCtx, job, cleanupErr); !errors.Is(err, cleanupErr) {
		t.Fatalf("retryTaskResourceCleanupJob error = %v, want cleanup error", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("reload cleanup job: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("cleanup state = %q, want retry_wait", got.State)
	}
}
