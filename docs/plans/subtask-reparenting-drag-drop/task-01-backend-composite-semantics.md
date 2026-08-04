---
id: "01-backend-composite-semantics"
title: "Backend composite re-parent semantics"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/subtask-reparenting-drag-drop.md"
---

# Task 01: Backend composite re-parent semantics

## Acceptance

- `Service.UpdateTask` normalizes `metadata.workspace.mode` from `inherit_parent` to `shared_group` whenever the effective parent changes (set or cleared); other modes and root tasks are untouched. The returned task, the persisted row, and the `task.updated` payload all reflect the change.
- `DashboardService.UpdateTaskParentID` (Office dashboard PATCH) applies the same normalization on its non-empty parent path and includes `metadata` in the published `fields` when the mode changed.
- All existing reparent/detach service and handler tests pass unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard
```

## Files likely touched

- `apps/backend/internal/task/service/service_tasks.go` (parent block in `Service.UpdateTask`, new `normalizeWorkspaceModeAfterReparent` helper)
- `apps/backend/internal/task/service/service_reparent_test.go` (new cases)
- `apps/backend/internal/office/dashboard/service_tasks.go` (`UpdateTaskParentID`)
- `apps/backend/internal/office/dashboard/service_detachment_test.go` (parity test)
- `apps/backend/internal/office/repository/sqlite/tasks.go` (new `UpdateTaskWorkspaceMode`)
- `apps/backend/internal/office/dashboard/service.go` (repo interface member, if `UpdateTaskWorkspaceMode` is not already surfaced)

## Dependencies

None.

## Inputs

- Spec sections: What (composite semantics), Data model, API surface, Failure modes.
- Existing pattern: `Service.DetachTask` + `detachTaskQuery` (dialect-aware JSON mode normalization in `apps/backend/internal/task/repository/sqlite/task.go`), `resolveParentID` / `validateReparentDepth` in `service_tasks.go`, `publishTaskUpdated` in the office dashboard service.
- Do NOT change `resolveParentID` / `validateReparentDepth`; validation behavior is already shipped and tested.

## Output contract

Report the service/repo changes, exact commands and results, files changed, blockers, residual risks; update this task and `plan.md` when acceptance passes.
