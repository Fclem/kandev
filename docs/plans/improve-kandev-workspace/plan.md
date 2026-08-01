---
spec: docs/specs/improve-kandev/spec.md
created: 2026-08-01
status: draft
---

# Implementation Plan: Improve Kandev Workspace Isolation

## Overview

Improve Kandev tasks currently land in the user's active workspace, mixing
contribution work with their regular tasks. This change makes the
`improve-kandev` bootstrap endpoint find-or-create a dedicated, idempotently
reused workspace named `Improve Kandev`, scope the repository registration and
both hidden workflows to it, and return its `workspace_id` so the frontend
creates the task there. Backend contract first, then frontend wiring, then E2E
evidence of isolation.

---

## Backend

### `internal/improvekandev/handler.go`
- `BootstrapRequest.WorkspaceID` becomes optional (`json:"workspace_id,omitempty"`).
  Remove the `req.WorkspaceID == ""` 400 validation and the
  `canonicalWorkspaceID` call; delete `canonicalWorkspaceID` (and its unit
  test). The field is accepted and ignored for backward compatibility.
- Add constants `improveWorkspaceName = "Improve Kandev"` and
  `improveWorkspaceDesc`.
- Add `ensureImproveWorkspace(ctx) (*taskmodels.Workspace, error)`, mirroring
  the existing `ensureWorkflow` idempotency pattern:
  1. `taskSvc.ListWorkspaces(ctx)` → match by exact `Name == "Improve Kandev"`.
  2. Miss → `taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
     Name: improveWorkspaceName, Description: improveWorkspaceDesc,
     BootstrapKanbanWorkflow: true })`.
  3. Create failure → re-list and match by name (concurrent-bootstrap race);
     still missing → return the error.
- `httpBootstrap`:
  - Call `ensureImproveWorkspace` first; use its ID everywhere the old
    `workspaceID` was used (`resolveOrCloneRepo`, `ListWorkflows`,
    `ensureWorkflow` ×2).
  - Add `WorkspaceID string \`json:"workspace_id"\`` to `BootstrapResponse`,
    set to the dedicated workspace's ID.

### Tests
- `internal/improvekandev/handler_test.go`: remove `TestCanonicalWorkspaceID`
  (function deleted).
- `internal/integration/improve_kandev_test.go`:
  - Update `TestImproveKandevBootstrapCreatesBothHiddenWorkflowsIdempotently`:
    drop the pre-created workspace; bootstrap with an empty
    `WorkspaceID`; assert the response `WorkspaceID` is non-empty, a workspace
    named `Improve Kandev` exists, both hidden workflows live in it, and a
    second bootstrap returns the same workspace + workflow IDs.
  - Add `TestImproveKandevBootstrapReusesExistingImproveWorkspace`: pre-create
    a workspace named `Improve Kandev`, bootstrap with a *different* request
    `workspace_id`, assert the response's workspace is the pre-created one.

---

## Frontend

### `lib/api/domains/improve-kandev-api.ts`
- Add `workspace_id: string` to `ImproveKandevBootstrapResponse`. Keep the
  request `workspaceId` parameter (still sent; backend ignores it).

### `components/improve-kandev-dialog.tsx`
- `useBootstrapKandev`: after bootstrap, call `listRepositories` and
  `setRepositories` with `data.workspace_id` instead of the active
  `workspaceId` so the locked-repo chip resolves a label for the dedicated
  workspace's repo.

### `components/improve-kandev-dialog-create.tsx`
- `CreateModeView`: pass `workspaceId={ready ? ready.data.workspace_id :
  props.workspaceId}` to `TaskCreateDialog`. Submit stays blocked until
  bootstrap is ready, so no task can be created in the wrong workspace.
  The sidebar `workspaceId` prop remains the fallback while loading and is
  still used by `useGitHubAuthCheck` for the fix URL.

---

## Tests

- **What:** bootstrap creates the dedicated workspace on first use
  (spec scenario). **File:** `internal/integration/improve_kandev_test.go`.
  **How:** integration test through the real handler + task service.
- **What:** bootstrap reuses the existing workspace and workflow IDs
  (spec scenario). **File:** same. **How:** second bootstrap call, compare IDs.
- **What:** a different request `workspace_id` is ignored.
  **File:** same. **How:** pre-create the dedicated workspace, pass another ID.
- **What:** frontend uses the bootstrap's `workspace_id` for repo listing and
  task creation. **File:** `apps/web/components/*` + E2E. **How:** E2E test
  seeds a real dedicated workspace and asserts the task lands there.
- **What:** E2E mocks stay contract-complete (`workspace_id` present).
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`,
  `mobile-improve-kandev.spec.ts`. **How:** update mock bodies.

---

## E2E Tests

- **Scenario:** GIVEN the user's active workspace is not the dedicated
  workspace, WHEN the dialog submits a task, THEN the task appears in the
  dedicated workspace and not the active one.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **What to verify:** `apiClient.listTasks(dedicatedWorkspaceId)` contains the
  task; `apiClient.listTasks(seedData.workspaceId)` does not.
  Seed the dedicated workspace with `apiClient.createWorkspace("Improve
  Kandev")`, a workflow via `createWorkflow(workspace.id, ...)`, and a
  repository via `createRepository(workspace.id, <seed repo dir>)`; mock
  bootstrap to return those IDs.

---

## Verification Results

Pending. On completion, synchronize with each task's `## Results`:
exact commands and outcomes/counts, generated artifact paths, cleanup/teardown
evidence.

---

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):
- [ ] [task-01-backend-dedicated-workspace](task-01-backend-dedicated-workspace.md)

Wave 2 (depends on 01):
- [ ] [task-02-frontend-bootstrap-workspace](task-02-frontend-bootstrap-workspace.md)

Wave 3 (depends on 02):
- [ ] [task-03-e2e-workspace-isolation](task-03-e2e-workspace-isolation.md)

The default is sequential execution in the primary conversation. No subagents
unless the user explicitly asks after selecting the implementation model.

---

## Open Questions

None.
