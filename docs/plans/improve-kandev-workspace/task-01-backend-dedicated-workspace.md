---
id: "01-backend-dedicated-workspace"
title: "Backend: dedicated Improve Kandev workspace"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/improve-kandev/spec.md"
---

# Task 01: Backend — dedicated Improve Kandev workspace

Make the improve-kandev bootstrap endpoint find-or-create a dedicated
"Improve Kandev" workspace, scope repository + hidden workflows to it, and
return its ID.

## Acceptance

1. `POST /api/v1/system/improve-kandev/bootstrap` with an empty (or any)
   `workspace_id` finds or creates a workspace named `Improve Kandev`
   (kanban-bootstrapped), returns its ID as `workspace_id`, and registers the
   kandev repository and both hidden workflows (`improve-kandev`,
   `report-kandev-issue`) in that workspace.
2. A second bootstrap reuses the same workspace and the same workflow IDs.
3. `BootstrapRequest.WorkspaceID` is optional and ignored;
   `canonicalWorkspaceID` is deleted along with `TestCanonicalWorkspaceID`.

## Verification

```sh
cd apps/backend && go test ./internal/improvekandev/... ./internal/integration/... 
```

## Files likely touched

- `apps/backend/internal/improvekandev/handler.go`
- `apps/backend/internal/improvekandev/handler_test.go`
- `apps/backend/internal/integration/improve_kandev_test.go`

## Dependencies

None.

## Parallelism

Sequential (wave 1).

## Inputs

- Spec: "API surface" and "Scenarios" (workspace creation/reuse) in
  `docs/specs/improve-kandev/spec.md`.
- Plan: `docs/plans/improve-kandev-workspace/plan.md` Backend section.
- Patterns: `ensureWorkflow`'s find-then-create-then-re-read race handling in
  `handler.go`; `taskSvc.ListWorkspaces` / `taskSvc.CreateWorkspace`
  (`internal/task/service/service_resources.go`), `CreateWorkspaceRequest`
  with `BootstrapKanbanWorkflow: true` (same as
  `internal/task/handlers/workspace_handlers.go`).

## Output contract

Summary, files changed, exact test command + result, blockers, risks, and
task/plan status update in the same conversation.

## Risks

- First bootstrap on an existing install clones `kdlbs/kandev` into the new
  workspace's isolated managed path (`~/.kandev/repos/workspaces/<id>/...`) —
  one-time network cost, expected.
- Concurrent bootstraps may race workspace creation; the re-list-on-failure
  path (same pattern as `ensureWorkflow`) converges on one row.

## Results

Pending.
