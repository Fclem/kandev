package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestListTasksWithProviderLimitWaitsIncludesLaunchOnlyTasks(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	task := &models.Task{ID: "provider-limit-launch-only", Title: "Deferred launch"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	record, prior, err := repo.GetTaskDeferredLaunch(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record, err = models.PutProviderLimitLaunch(record, models.ProviderLimitLaunch{
		ID: "launch-only", Kind: models.CeilingLaunchStart, Origin: "automatic",
		WorkflowStepID: "step-one", MarkKey: "private-mark-key", Model: "vendor/model",
		NotBefore: now.Add(time.Hour), QueuedAt: now, Payload: map[string]interface{}{"prompt": "private"},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, task.ID, prior, record)
	if err != nil || !stored || lost {
		t.Fatalf("store launch: stored=%v lost=%v error=%v", stored, lost, err)
	}
	listed, err := repo.ListTasksWithProviderLimitWaits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range listed {
		if candidate.ID == task.ID {
			return
		}
	}
	t.Fatalf("launch-only task was omitted from recovery sweep: %+v", listed)
}

func TestTransferProviderLimitLaunchProbeRequiresPersistedExactOwner(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	task := &models.Task{ID: "provider-limit-owner-transfer", Title: "Probe transfer", WorkflowStepID: "step-one"}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-one", TaskID: task.ID, State: models.TaskSessionStateRunning}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-one", TaskSessionID: "session-one", TaskID: task.ID, StartedAt: now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	lease := models.ProviderLimitProbeLease{Key: "private-mark-key", Until: now.Add(time.Minute)}
	record, prior, err := repo.GetTaskDeferredLaunch(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	record, err = models.PutProviderLimitLaunch(record, models.ProviderLimitLaunch{
		ID: "launch-one", Kind: models.CeilingLaunchResume, Origin: "automatic", WorkflowStepID: "step-one",
		MarkKey: lease.Key, Model: "vendor/model", NotBefore: now, QueuedAt: now, SessionID: "session-one",
		ProbeLease: &lease, Payload: map[string]interface{}{"prompt": "private input"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored, lost, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, task.ID, prior, record); err != nil || !stored || lost {
		t.Fatalf("store pending launch: stored=%v lost=%v error=%v", stored, lost, err)
	}
	owner := models.ProviderLimitProbeOwner{LaunchID: "launch-one", SessionID: "session-one", WorkflowStepID: "step-one", Model: "vendor/model", ExecutionID: "execution-one", Generation: 9, Lease: lease}
	transferred, err := repo.TransferProviderLimitLaunchProbe(ctx, task.ID, "stale-launch", owner)
	if err != nil || transferred {
		t.Fatalf("stale launch identity transferred probe ownership: transferred=%v error=%v", transferred, err)
	}
	transferred, err = repo.TransferProviderLimitLaunchProbe(ctx, task.ID, "launch-one", owner)
	if err != nil || !transferred {
		t.Fatalf("transfer first-turn ownership: transferred=%v error=%v", transferred, err)
	}
	storedRecord, _, err := repo.GetTaskDeferredLaunch(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	owners, err := models.ReadProviderLimitProbeOwners(storedRecord)
	if err != nil || owners[models.ProviderLimitWaitIdentity("session-one", "turn-one")].ExecutionID != "execution-one" {
		t.Fatalf("durable owner did not capture the exact admitted turn: %+v error=%v", owners, err)
	}
	remaining, err := models.ReadProviderLimitLaunch(storedRecord)
	if err != nil || remaining != nil {
		t.Fatalf("launch slot remained occupied after its exact ownership was transferred: %+v error=%v", remaining, err)
	}
	listed, err := repo.ListTasksWithProviderLimitWaits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range listed {
		if candidate.ID == task.ID {
			return
		}
	}
	t.Fatal("probe-owner-only task was omitted from recovery sweep")
}
