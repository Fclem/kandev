---
id: "01-show-improve-kandev-cards"
title: "Show Improve Kandev cards on Kanban"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-KANDEV-001.9
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
---

# Task 01: Show Improve Kandev cards on Kanban

## Summary

Show task cards from both hidden Improve Kandev workflows on the unfiltered desktop Kanban board. On phones, both Improve Kandev templates remain available through the existing focused-column navigator even when no tasks match; selecting an empty workflow shows its normal empty lane. Unrelated hidden workflows remain off the unfiltered desktop board. Preserve workflow-template identity through backend DTO, boot, live WebSocket, and browser state so selectors recognize newly-created hidden workflows without reload.

## In scope

- Preserve workflow template identity in the backend DTO, boot-state projection, workflow WebSocket event payload, and browser workflow-to-store mappings.
- Update unfiltered desktop lane selection for the `improve-kandev` and `report-kandev-issue` templates only.
- Keep both Improve Kandev templates in the existing mobile workflow navigator regardless of task count or active task filters; preserve current eligibility rules for other hidden workflows.
- Extend the E2E-only hidden-workflow factory to pass template identity into workflow creation before the service publishes `workflow.created` for persisted/query seeding; add focused DTO/boot/event projection tests, WebSocket handler tests, selector tests, and desktop and mobile browser coverage.
- Add a separate desktop live-update E2E using the real bootstrap endpoint, not a route mock: ensure no dedicated Improve Kandev workspace exists, then keep All Workflows open in an isolated active workspace with no Improve Kandev templates. Call bootstrap with `create_workspace: false` and that workspace ID using a local kandev repository and the E2E GitHub mock, observe `workflow.created` events carrying template IDs, create a task in one new workflow, and verify its card appears without reloading. Keep all setup and cleanup scoped to this fixture workspace.

## Out of scope

- Changing production workflow creation/bootstrap behavior, workspace guards, hidden status, or workflow-picker visibility.
- Adding or changing the mobile navigation surface or interaction pattern.


## Acceptance
- The unfiltered desktop board displays cards from either hidden Improve Kandev workflow when tasks exist, and excludes unrelated hidden workflows. Explicit selection continues to show only its selected lane without exposing hidden workflows in ordinary workflow-management or task-create views.
- Both Improve Kandev templates remain selectable in the existing phone navigator when their filtered task snapshots are empty; selecting one shows its lane with zero cards. Other hidden workflows retain their current eligibility behavior.
- Template identity survives backend DTO and boot-state serialization, workflow WebSocket event serialization and handler-to-store projection, and browser route-to-store mapping. The desktop E2E opens All Workflows while no dedicated workspace or Improve Kandev templates exist, invokes the real bootstrap endpoint targeting the active workspace, verifies both live `workflow.created` events carry their template IDs, creates a task in one new workflow, and verifies the card appears without reloading; it does not intercept bootstrap. The E2E-only factory seeds actual template instances for persisted/query scenarios. The mobile E2E selects each empty template workflow and verifies its normal empty-lane state with zero cards; a task-bearing case verifies each card in its selected lane.

## ASCII UI preview

See the full [UI-01 and UI-02 previews in the plan](plan.md#ascii-ui-preview).

### UI-01: Desktop unfiltered board

```text
+----------------------+----------------------+
| Improve              | Open issue           |
|----------------------|----------------------|
| [Improve task card]  | [Issue task card]    |
+----------------------+----------------------+
```

### UI-02: Phone workflow navigation

Empty selected workflow:

```text
+--------------------------+
| Kanban       [Workflows] |
|                          |
| [Existing empty state]   |
+--------------------------+
```

Selecting an empty template keeps the selected lane and normal empty state;
the E2E asserts zero task cards. The phone navigation surface is unchanged.

## Verification

Both E2E commands below use the managed `pnpm e2e:run` runner without `--no-build`. The runner rebuilds the backend binary, E2E web bundle, and E2E plugin UI/package before Playwright ([runner implementation](../../../apps/web/e2e/scripts/run-e2e.sh); [E2E runner documentation](../../../apps/web/e2e/README.md)). The manual build sequence under the README prerequisites applies when running specs outside the managed runner.

```bash
(cd apps/backend && go test ./internal/task/dto ./internal/backendapp ./internal/task/service -run 'TestFromWorkflowIncludesTemplateID|TestMapWorkflowItemStateIncludesTemplateID|TestE2ECreateHiddenWorkflowIncludesTemplateID|TestService_CreateWorkflowEventIncludesTemplateID' -count=1)
(cd apps/web && pnpm vitest run lib/kanban/workflow-swimlanes.test.ts lib/ws/handlers/workflows.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/kanban/workflow-swimlanes.ts lib/kanban/workflow-swimlanes.test.ts lib/types/backend.ts lib/ws/handlers/workflows.ts lib/ws/handlers/workflows.test.ts src/kanban-route.tsx src/spa-routes.tsx e2e/helpers/api-client.ts e2e/tests/kanban/improve-kandev-hidden-workflows.spec.ts e2e/tests/kanban/mobile-improve-kandev-hidden-workflows.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/kanban/improve-kandev-hidden-workflows.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/kanban/mobile-improve-kandev-hidden-workflows.spec.ts)
```

## Files likely touched

- `apps/backend/internal/task/dto/dto.go`
- Backend DTO tests for `FromWorkflow`
- `apps/backend/internal/backendapp/boot_state_routes.go` and focused boot-state tests
- `apps/backend/internal/task/service/service_events.go` and `service_resources_test.go` workflow event payload coverage
- `apps/backend/internal/backendapp/e2e_reset.go` and `e2e_reset_test.go`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/kanban/workflow-swimlanes.ts`
- `apps/web/lib/kanban/workflow-swimlanes.test.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ws/handlers/workflows.ts` and `workflows.test.ts`
- `apps/web/src/kanban-route.tsx`
- `apps/web/src/spa-routes.tsx`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/kanban/improve-kandev-hidden-workflows.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-improve-kandev-hidden-workflows.spec.ts`

## Dependencies

None.

## Risks

- The workflow event payload must retain template identity through backend serialization, frontend typing, and WebSocket handler state updates; boot-state preservation alone does not cover workflows created while the active Kanban page is open.
- The E2E-only factory must seed hidden workflows with persisted template IDs; ordinary workflow creation does not support seeding hidden system instances.
- The phone keeps its focused-column layout and existing workflow navigator; validate it on `mobile-chrome` without changing its interaction model.

## Parallelism

`sequential`

## Inputs

- `docs/specs/workspaces/requirements/improve-kandev.md`, AC `.9`.
- `docs/specs/workspaces/system-design/improve-kandev.md`, Kanban visibility.
- Existing selector, DTO, boot-state, E2E factory, API-client, and Playwright patterns in the named files.

## Results

Implemented AC-WORKSPACES-IMPROVE-KANDEV-001.9. Workflow template identity now survives the DTO, boot payload, live workflow events, and browser store projections. Unfiltered desktop lanes include task-bearing Improve Kandev templates only; mobile keeps both templates reachable with empty filtered snapshots and shows each task card when selected. E2E hidden-workflow fixtures seed template IDs; live bootstrap coverage uses the real endpoint in a target-local workspace.

Verification passed:
- Backend focused DTO, boot-state, E2E factory, and event tests.
- Frontend selector and WebSocket handler tests: 30 passed.
- Web typecheck and targeted ESLint.
- Managed Chromium E2E: 2 passed.
- Managed mobile-chrome E2E: 2 passed.

Review corrections (implementation review round 1): the first implementation
dropped `hidden` from the boot projection and the workflow event payload,
dropped `sort_order` from `FromWorkflow`, dropped `style` from three browser
projections, and never mapped template identity in `use-workflows.ts`, so a
sidebar refetch or reconnect lost it. All are restored, and the DTO, boot,
event, WebSocket handler, and `use-workflows` tests now pin `hidden`,
`sort_order`/`style`, and template identity together. The live E2E removes any
existing dedicated workspace first and asserts both live events stay hidden, so
the card is visible because of template identity. A duplicate selector test was
removed and the hidden-step navigator test uses an unrelated hidden workflow.
Re-verified: focused backend tests, backend lint for the touched packages,
selector/handler/hook unit tests, web typecheck, targeted ESLint, managed
Chromium E2E (2 passed in the Kanban spec), and managed mobile-chrome E2E
(2 passed in the Kanban spec).

Review corrections (implementation review round 2): the task-detail hydration
(`lib/ssr/session-page-state.ts`) and the mobile session-sheet workspace switch
still wrote workflow items without template identity, so returning to All
Workflows after visiting a task dropped the Improve lanes. Every workflow store
write that reads `listWorkflows` (Kanban route, SPA route bootstrap,
`use-workflows.ts`, task-detail hydration, session sheet) now uses the shared
`toWorkflowStoreItem` in `lib/kanban/workflow-store-item.ts`, pinned by its unit
test.

Review corrections (implementation review round 3): the SPA route bootstrap
(`src/spa-routes.tsx`, used by `/tasks`, integration pages, stats, and runs)
listed only visible workflows and replaced `workflows.items` with them, so
returning to the board dropped the Improve lanes until a reload. It now reads
hidden workflows too, keeps them in the store, and passes only visible
workflows to the route's watch dialogs; a route test pins this. Showing the
hidden lanes also made them drag-sortable, and the dedicated workspace rejects
reorder requests, so lane sorting is now limited to boards that show only user
workflows (`canSortWorkflowLanes`, with unit tests).
