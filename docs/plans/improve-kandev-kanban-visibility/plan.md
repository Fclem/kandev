---
created: 2026-09-26
status: done
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
legacy_specs: []
---

# Implementation Plan: Improve Kandev Kanban Visibility

## Overview

Show task cards assigned to the hidden Improve Kandev implementation and issue-reporting workflows on the unfiltered desktop Kanban board. The interactive route already fetches hidden workflows, but the selector discards them and workflow-template identity is not preserved through the backend DTO, boot/browser state, or live workflow-event path.

One vertical work order covers the metadata path, frontend behavior, regression tests, and browser proof.

## Scope

### In scope

- Include task-bearing `improve-kandev` and `report-kandev-issue` workflow instances in unfiltered Kanban lanes in the selected target workspace, whether dedicated or the active-workspace fallback.
- Keep unrelated hidden/system workflows out of the unfiltered Kanban board.
- Keep explicit workflow selection behavior unchanged.
- Preserve `workflow_template_id` from the backend workflow DTO through boot state, workflow WebSocket event payloads, and every browser workflow-to-store projection.

### Out of scope

- Making Improve Kandev workflows visible in ordinary task-creation choices or editable workflow settings.
- Changing the mobile navigation surface or interaction pattern; preserve the existing navigator.
- Changing workflow persistence, hidden flags, bootstrap, task creation, or workspace immutability. Extend only the E2E-only hidden-workflow fixture to seed template identities for browser tests.
- Migrating existing tasks between workspaces.

## Technical approach

- Add optional `WorkflowTemplateID` to `apps/backend/internal/task/dto/dto.go::WorkflowDTO` and map it in `FromWorkflow`. Preserve it in `apps/backend/internal/backendapp/boot_state_routes.go::mapWorkflowItemState`.
- Serialize the template ID in workflow events in `apps/backend/internal/task/service/service_events.go`; preserve it in `WorkflowPayload` and the `workflow.created`/`workflow.updated` handlers so live workflows enter the store with identity before a reload.
- Add `workflowTemplateId` to the internal frontend workflow-state item and preserve it in both `apps/web/src/kanban-route.tsx` and `apps/web/src/spa-routes.tsx` workflow-to-store mappings.
- Update `apps/web/lib/kanban/workflow-swimlanes.ts` so unfiltered desktop lanes include only the two Improve Kandev workflow templates when they have a loaded task snapshot. On phones, include both templates in the existing workflow navigator regardless of task count or active task filters; keep the current task/hidden-step eligibility rules for other hidden workflows.
- Extend the E2E-only hidden-workflow factory (`apps/backend/internal/backendapp/e2e_reset.go`) and `apps/web/e2e/helpers/api-client.ts` to seed a hidden workflow with a specified template identity. Do not add template seeding to production workflow creation.
- Add focused tests for DTO, boot-state and WebSocket event projections, E2E fixture, WebSocket handler state updates, desktop lane selection, and mobile navigator eligibility with empty/filtered task snapshots. Browser scenarios must traverse the real backend DTO and route mappings rather than supplying template IDs directly to selector fixtures.

## ASCII UI preview

### UI-01: Desktop unfiltered board

Before, in the dedicated workspace with only hidden Improve Kandev workflows:

```text
+------------------------+
| Kanban                 |
| No workflows available|
+------------------------+
```

After:

```text
+----------------------+----------------------+
| Improve              | Open issue           |
|----------------------|----------------------|
| [Improve task card]  | [Issue task card]    |
+----------------------+----------------------+
```

### UI-02: Phone workflow navigation

Before, an Improve Kandev workflow with no matching tasks can be absent from the phone navigator.

After, both Improve Kandev templates remain selectable even when no tasks
match. Selecting one preserves the normal empty-lane state; with tasks, the
same navigator selection shows that workflow's cards.

Empty selected workflow:

```text
+--------------------------+
| Kanban       [Workflows] |
|                          |
| [Existing empty state]   |
+--------------------------+
```

Task-bearing selected workflow:

```text
+--------------------------+
| Kanban       [Workflows] |
| Improve task card        |
|                          |
+--------------------------+
```

Structural requirement: the unfiltered desktop board presents cards from both task-bearing Improve Kandev workflows; unrelated hidden workflows remain omitted. On phones, the existing focused-column workflow navigator always offers both Improve Kandev templates, even when no tasks match, and selecting one opens its lane. Other hidden workflows retain their existing task/hidden-step eligibility. Column layout and card anatomy remain unchanged.

## Tests

- `AC-WORKSPACES-IMPROVE-KANDEV-001.9`: `apps/web/lib/kanban/workflow-swimlanes.test.ts` verifies desktop lane selection includes task-bearing Improve Kandev templates, excludes unrelated hidden workflows, and preserves explicit filtering. Mobile navigator coverage verifies both templates remain available when their filtered task snapshots are empty and selecting an empty template shows the normal empty lane.
- Backend DTO, boot-state, event-payload, and E2E-factory tests verify template identity in persisted/query projections and seed data. WebSocket handler tests cover the store transition; the live browser scenario below verifies delivery from the actual bootstrap route through the rendered board.

## E2E tests

- Add `apps/web/e2e/tests/kanban/improve-kandev-hidden-workflows.spec.ts` (Chromium) to seed both real hidden template instances for persisted/query coverage and verify their cards appear without a workflow selection in desktop All Workflows. In a separate live case, ensure no dedicated Improve Kandev workspace exists, open All Workflows in an isolated active workspace with neither template workflow, use a local kandev repository and the E2E GitHub provider fixture, and call the real bootstrap endpoint with `create_workspace: false` and that active `workspace_id`; do not intercept the bootstrap route. Observe both `workflow.created` events with their template IDs, create a task in one workflow, and verify its card appears without reloading. Clean up only resources owned by this target-local fixture.
- Add `apps/web/e2e/tests/kanban/mobile-improve-kandev-hidden-workflows.spec.ts` (mobile-chrome) with an empty-workflow case that selects each template and asserts its normal empty-lane state with zero cards, plus a task-bearing case that selects each workflow and verifies its card.

## Work orders

- [x] [Task 01: Show Improve Kandev cards on Kanban](task-01-show-improve-kandev-cards.md) (`done`)

## Verification results

Task 01 complete. Focused backend regression tests passed; frontend selector and WebSocket tests passed (30 tests), web typecheck and targeted ESLint passed, Chromium E2E passed (2 tests), and mobile-chrome E2E passed (2 tests).

## Risks

- Workflow metadata must distinguish the two Improve Kandev templates from other hidden workflows; matching only `hidden` would broaden board visibility beyond the requirement.
- The phone uses one focused column with the existing workflow navigator; validate the same shared lane selection on the `mobile-chrome` project without changing its interaction model.
