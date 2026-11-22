package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestServiceTransferTaskAuthorizesBothWorkspacesAndPublishesOneCanonicalUpdate(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-1")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})

	receipt, err := svc.TransferTask(ctx, serviceTaskTransferCommand(task))
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if receipt.TaskID != task.ID || receipt.DestinationWorkspaceID != "transfer-ws-destination" {
		t.Fatalf("receipt = %+v", receipt)
	}
	if _, err := repo.DB().Exec(`UPDATE workflows SET workspace_id = ? WHERE id = ?`,
		"transfer-ws-source", "transfer-wf-destination"); err != nil {
		t.Fatalf("invalidate live destination workflow: %v", err)
	}
	replayed, err := svc.TransferTask(ctx, serviceTaskTransferCommand(task))
	if err != nil || replayed.OperationID != receipt.OperationID {
		t.Fatalf("TransferTask replay = %+v, %v", replayed, err)
	}
	replayActor, found, err := svc.ResolveTaskTransferReplayActor(ctx, serviceTaskTransferCommand(task))
	if err != nil || !found || replayActor.ID != "owner-1" {
		t.Fatalf("ResolveTaskTransferReplayActor = %+v, found=%v, err=%v", replayActor, found, err)
	}

	var moved, updated int
	for _, event := range eventBus.GetPublishedEvents() {
		switch event.Type {
		case events.TaskMoved:
			moved++
		case events.TaskUpdated:
			updated++
		}
	}
	if moved != 0 || updated != 1 {
		t.Fatalf("events moved=%d updated=%d", moved, updated)
	}
	published := eventBus.GetPublishedEvents()
	data, ok := published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("task.updated data = %T, want map[string]interface{}", published[0].Data)
	}
	for key, want := range map[string]interface{}{
		"workspace_id":          receipt.DestinationWorkspaceID,
		"workflow_id":           receipt.DestinationWorkflowID,
		"workflow_step_id":      receipt.DestinationStepID,
		"old_workflow_id":       receipt.SourceWorkflowID,
		"source_workspace_id":   receipt.SourceWorkspaceID,
		"source_workflow_id":    receipt.SourceWorkflowID,
		"source_step_id":        receipt.SourceStepID,
		"transfer_operation_id": receipt.OperationID,
		"preservation_digest":   receipt.PreservationDigest,
	} {
		if got := data[key]; got != want {
			t.Fatalf("task.updated %s = %#v, want %#v", key, got, want)
		}
	}
}

func TestServiceTransferTaskReconcilesVacatedSourceWIP(t *testing.T) {
	svc, _, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-1")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})
	if _, err := repo.DB().Exec(`UPDATE workflow_steps SET wip_limit = 1 WHERE id IN (?, ?)`,
		"transfer-step-source", "transfer-step-destination"); err != nil {
		t.Fatalf("set WIP limits: %v", err)
	}
	if _, err := repo.DB().Exec(`UPDATE tasks SET wip_admitted = 1, queued_for_step_id = '' WHERE id = ?`,
		task.ID); err != nil {
		t.Fatalf("admit transfer task: %v", err)
	}
	task, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask transfer task: %v", err)
	}
	queued := &models.Task{
		ID: "transfer-queued-task", WorkspaceID: "transfer-ws-source", WorkflowID: "transfer-wf-source",
		WorkflowStepID: "transfer-step-source", Title: "Queued", State: v1.TaskStateTODO,
		QueuedForStepID: "transfer-step-source", WIPAdmitted: false,
	}
	if err := repo.CreateTask(ctx, queued); err != nil {
		t.Fatalf("CreateTask queued: %v", err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"transfer-step-source": {
			ID: "transfer-step-source", WorkflowID: "transfer-wf-source", Name: "Work", WIPLimit: 1,
		},
	}})

	command := serviceTaskTransferCommand(task)
	command.ExpectedTaskUpdatedAt = task.UpdatedAt
	if _, err := svc.TransferTask(ctx, command); err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	stored, err := repo.GetTask(ctx, queued.ID)
	if err != nil {
		t.Fatalf("GetTask queued: %v", err)
	}
	if !stored.WIPAdmitted || stored.QueuedForStepID != "" {
		t.Fatalf("vacated source WIP was not reconciled: %+v", stored)
	}
}

type cancelAfterTaskTransferRepository struct {
	*sqliterepo.Repository
	cancel context.CancelFunc
}

func (r *cancelAfterTaskTransferRepository) TransferTask(
	ctx context.Context,
	command models.TaskTransferCommand,
) (*models.TaskTransferReceipt, error) {
	receipt, err := r.Repository.TransferTask(ctx, command)
	if err == nil {
		r.cancel()
	}
	return receipt, err
}

func TestServiceTransferTaskFinishesCommittedReconciliationAfterCancellation(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-1")
	if _, err := repo.DB().Exec(`UPDATE workflow_steps SET wip_limit = 1 WHERE id IN (?, ?)`,
		"transfer-step-source", "transfer-step-destination"); err != nil {
		t.Fatalf("set WIP limits: %v", err)
	}
	if _, err := repo.DB().Exec(`UPDATE tasks SET wip_admitted = 1, queued_for_step_id = '' WHERE id = ?`,
		task.ID); err != nil {
		t.Fatalf("admit transfer task: %v", err)
	}
	task, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask transfer task: %v", err)
	}
	queued := &models.Task{
		ID: "transfer-cancel-queued", WorkspaceID: "transfer-ws-source", WorkflowID: "transfer-wf-source",
		WorkflowStepID: "transfer-step-source", Title: "Queued", State: v1.TaskStateTODO,
		QueuedForStepID: "transfer-step-source", WIPAdmitted: false,
	}
	if err := repo.CreateTask(context.Background(), queued); err != nil {
		t.Fatalf("CreateTask queued: %v", err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"transfer-step-source": {
			ID: "transfer-step-source", WorkflowID: "transfer-wf-source", Name: "Work", WIPLimit: 1,
		},
	}})

	identityCtx := authn.WithIdentity(
		context.Background(),
		authn.Identity{UserID: "owner-1", Role: authn.RoleMember},
	)
	ctx, cancel := context.WithCancel(identityCtx)
	defer cancel()
	svc.tasks = &cancelAfterTaskTransferRepository{Repository: repo, cancel: cancel}

	command := serviceTaskTransferCommand(task)
	command.ExpectedTaskUpdatedAt = task.UpdatedAt
	if _, err := svc.TransferTask(ctx, command); err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	published := eventBus.GetPublishedEvents()
	if len(published) != 3 || published[0].Type != events.TaskUpdated ||
		published[1].Type != events.TaskUpdated || published[2].Type != events.TaskQueuePromoted {
		t.Fatalf("post-commit events = %#v, want transfer update then queued-task update and promotion", published)
	}
	for index, wantTaskID := range []string{task.ID, queued.ID, queued.ID} {
		data, ok := published[index].Data.(map[string]interface{})
		if !ok || data["task_id"] != wantTaskID {
			t.Fatalf("post-commit event %d data = %#v, want task_id %q", index, published[index].Data, wantTaskID)
		}
	}
	stored, err := repo.GetTask(context.Background(), queued.ID)
	if err != nil {
		t.Fatalf("GetTask queued: %v", err)
	}
	if !stored.WIPAdmitted || stored.QueuedForStepID != "" {
		t.Fatalf("cancelled request dropped source WIP reconciliation: %+v", stored)
	}
}

func TestServiceTransferTaskRejectsForeignDestinationWithoutMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-2")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})

	_, err := svc.TransferTask(ctx, serviceTaskTransferCommand(task))
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("TransferTask error = %v, want workspace not found", err)
	}
	stored, getErr := repo.GetTask(context.Background(), task.ID)
	if getErr != nil {
		t.Fatalf("GetTask: %v", getErr)
	}
	if stored.WorkspaceID != "transfer-ws-source" || stored.WorkflowID != "transfer-wf-source" ||
		stored.WorkflowStepID != "transfer-step-source" {
		t.Fatalf("denied transfer mutated task: %+v", stored)
	}
	var denied int
	if err := repo.DB().QueryRow(
		`SELECT COUNT(*) FROM task_transfer_audit WHERE task_id = ? AND result = 'denied'`, task.ID,
	).Scan(&denied); err != nil {
		t.Fatalf("read denied audit: %v", err)
	}
	if denied != 1 {
		t.Fatalf("denied audit rows = %d, want 1", denied)
	}
}

func TestServiceTransferTaskReplayRequiresCurrentDestinationAccess(t *testing.T) {
	svc, _, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-1")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})
	command := serviceTaskTransferCommand(task)
	if _, err := svc.TransferTask(ctx, command); err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if _, err := repo.DB().Exec(`UPDATE workspaces SET owner_id = ? WHERE id = ?`,
		"owner-2", "transfer-ws-destination"); err != nil {
		t.Fatalf("change destination owner: %v", err)
	}
	if _, err := svc.TransferTask(ctx, command); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("TransferTask replay error = %v, want task not found", err)
	}
	if _, found, err := svc.ResolveTaskTransferReplayActor(ctx, command); err == nil || found {
		t.Fatalf("ResolveTaskTransferReplayActor found=%v err=%v, want denied", found, err)
	}
}

func TestServiceTransferTaskDurableRejectionWritesOneAuditRowPerAttempt(t *testing.T) {
	svc, _, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-1")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})
	if _, err := repo.DB().Exec(`UPDATE workflow_steps SET prompt = ? WHERE id = ?`,
		"different instructions", "transfer-step-destination"); err != nil {
		t.Fatalf("make destination incompatible: %v", err)
	}
	command := serviceTaskTransferCommand(task)
	command.DurableRejection = true

	if _, err := svc.TransferTask(ctx, command); err == nil {
		t.Fatal("first durable attempt committed an incompatible transfer")
	}
	if got := serviceTransferAuditRows(t, repo, command.IdempotencyKey); got != 1 {
		t.Fatalf("audit rows after rejection = %d, want 1", got)
	}

	if _, err := svc.TransferTask(ctx, command); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		t.Fatalf("barrier retry error = %v, want task transfer conflict", err)
	}
	if got := serviceTransferAuditRows(t, repo, command.IdempotencyKey); got != 2 {
		t.Fatalf("audit rows after barrier retry = %d, want 2", got)
	}
}

func TestServiceTransferTaskDurableBoundaryAuditKeepsInterruptionsRetryable(t *testing.T) {
	t.Run("cancelled attempt leaves the key retryable", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		task := seedServiceTaskTransfer(t, repo, "owner-1")
		ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})
		command := serviceTaskTransferCommand(task)
		command.DurableRejection = true
		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		if _, err := svc.TransferTask(cancelled, command); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled TransferTask error = %v, want context canceled", err)
		}
		if got := serviceTransferAuditResults(t, repo, command.IdempotencyKey); got != "conflict" {
			t.Fatalf("audit results after cancellation = %q, want one conflict", got)
		}
		receipt, err := svc.TransferTask(ctx, command)
		if err != nil {
			t.Fatalf("same-key retry after cancellation: %v", err)
		}
		if receipt.DestinationWorkspaceID != "transfer-ws-destination" {
			t.Fatalf("retry receipt = %+v", receipt)
		}
	})
	t.Run("authorization denial stays a terminal barrier", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		task := seedServiceTaskTransfer(t, repo, "owner-2")
		ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})
		command := serviceTaskTransferCommand(task)
		command.DurableRejection = true

		if _, err := svc.TransferTask(ctx, command); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			t.Fatalf("TransferTask error = %v, want workspace not found", err)
		}
		if got := serviceTransferAuditResults(t, repo, command.IdempotencyKey); got != "denied" {
			t.Fatalf("audit results after denial = %q, want one denied", got)
		}
	})
}

// failingUnitReach fails exactly the failOn-th inherited-role lookup and grants
// no unit role otherwise, so workspace membership alone decides access.
type failingUnitReach struct{ calls, failOn int }

func (r *failingUnitReach) InheritedRole(context.Context, string, string) (string, error) {
	r.calls++
	if r.calls == r.failOn {
		return "", errors.New("unit reach unavailable")
	}
	return "", nil
}

func (r *failingUnitReach) InheritedRolesByUnit(context.Context, string, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *failingUnitReach) UnitReaders(context.Context, string) ([]string, error) { return nil, nil }

// Each case fails one access lookup in a different boundary check; the check is
// identified by the fail-closed not-found sentinel it returns.
func TestServiceTransferTaskDurableAccessLookupFailureStaysRetryable(t *testing.T) {
	for _, tt := range []struct {
		name             string
		breakMembership  bool
		failUnitLookupOn int
		want             error
	}{
		{name: "membership lookup in task check", breakMembership: true, want: repoerrors.ErrTaskNotFound},
		{name: "unit reach in task check", failUnitLookupOn: 1, want: repoerrors.ErrTaskNotFound},
		{name: "unit reach in destination workspace check", failUnitLookupOn: 2, want: repoerrors.ErrWorkspaceNotFound},
		{name: "unit reach in destination workflow check", failUnitLookupOn: 3, want: repoerrors.ErrWorkflowNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			task := seedServiceTaskTransfer(t, repo, "owner-1")
			for _, workspaceID := range []string{"transfer-ws-source", "transfer-ws-destination"} {
				if err := repo.UpsertWorkspaceMember(context.Background(), &models.WorkspaceMember{
					WorkspaceID: workspaceID, UserID: "member-1", Role: "collaborator", AddedBy: "owner-1",
				}); err != nil {
					t.Fatalf("UpsertWorkspaceMember: %v", err)
				}
			}
			if tt.failUnitLookupOn > 0 {
				if _, err := repo.DB().Exec(`UPDATE workspaces SET unit_id = 'transfer-unit'`); err != nil {
					t.Fatalf("place workspaces in a unit: %v", err)
				}
				svc.SetUnitReach(&failingUnitReach{failOn: tt.failUnitLookupOn})
			}
			setMembershipLookup := func(available bool) {
				from, to := "workspace_members_unavailable", "workspace_members"
				if !available {
					from, to = to, from
				}
				if _, err := repo.DB().Exec(`ALTER TABLE ` + from + ` RENAME TO ` + to); err != nil {
					t.Fatalf("toggle membership lookup: %v", err)
				}
			}
			ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-1", Role: authn.RoleMember})
			command := serviceTaskTransferCommand(task)
			command.Actor.ID = "member-1"
			command.DurableRejection = true

			if tt.breakMembership {
				setMembershipLookup(false)
			}
			if _, err := svc.TransferTask(ctx, command); !errors.Is(err, tt.want) {
				t.Fatalf("TransferTask error = %v, want fail-closed %v", err, tt.want)
			}
			if got := serviceTransferAuditResults(t, repo, command.IdempotencyKey); got != "conflict" {
				t.Fatalf("audit results after access lookup failure = %q, want one conflict", got)
			}
			if tt.breakMembership {
				setMembershipLookup(true)
			}
			receipt, err := svc.TransferTask(ctx, command)
			if err != nil {
				t.Fatalf("same-key retry after access lookup recovered: %v", err)
			}
			if receipt.DestinationWorkspaceID != "transfer-ws-destination" {
				t.Fatalf("retry receipt = %+v", receipt)
			}
		})
	}
}

func serviceTransferAuditResults(t *testing.T, repo *sqliterepo.Repository, key string) string {
	t.Helper()
	var results string
	if err := repo.DB().QueryRow(
		`SELECT COALESCE(GROUP_CONCAT(result), '') FROM task_transfer_audit WHERE idempotency_key = ?`, key,
	).Scan(&results); err != nil {
		t.Fatalf("read audit results: %v", err)
	}
	return results
}

func serviceTransferAuditRows(t *testing.T, repo *sqliterepo.Repository, key string) int {
	t.Helper()
	var count int
	if err := repo.DB().QueryRow(
		`SELECT COUNT(*) FROM task_transfer_audit WHERE idempotency_key = ?`, key,
	).Scan(&count); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	return count
}

func seedServiceTaskTransfer(t *testing.T, repo *sqliterepo.Repository, destinationOwner string) *models.Task {
	t.Helper()
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "transfer-ws-source", Name: "Source", OwnerID: "owner-1"},
		{ID: "transfer-ws-destination", Name: "Destination", OwnerID: destinationOwner},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
	}
	for _, workflow := range []*models.Workflow{
		{ID: "transfer-wf-source", WorkspaceID: "transfer-ws-source", Name: "Source"},
		{ID: "transfer-wf-destination", WorkspaceID: "transfer-ws-destination", Name: "Destination"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	for _, step := range []struct{ id, workflow string }{
		{"transfer-step-source", "transfer-wf-source"},
		{"transfer-step-destination", "transfer-wf-destination"},
	} {
		if _, err := repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, 'Work', 0)`,
			step.id, step.workflow); err != nil {
			t.Fatalf("insert workflow step: %v", err)
		}
	}
	task := &models.Task{ID: "transfer-task", WorkspaceID: "transfer-ws-source", WorkflowID: "transfer-wf-source",
		WorkflowStepID: "transfer-step-source", Title: "Synthetic", State: v1.TaskStateInProgress}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return stored
}

func serviceTaskTransferCommand(task *models.Task) models.TaskTransferCommand {
	return models.TaskTransferCommand{
		TaskID: task.ID, ExpectedSourceWorkspaceID: "transfer-ws-source",
		ExpectedSourceWorkflowID: "transfer-wf-source", ExpectedSourceStepID: "transfer-step-source",
		ExpectedTaskUpdatedAt: task.UpdatedAt, DestinationWorkspaceID: "transfer-ws-destination",
		DestinationWorkflowID: "transfer-wf-destination", DestinationStepID: "transfer-step-destination",
		IdempotencyKey: "service-transfer-key", PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
	}
}
