---
id: "03-responsive-move-flow"
title: "Deliver unified responsive movement actions"
status: pending
wave: 3
depends_on:
  - "01-human-transfer-api"
  - "02-transfer-reconciliation"
plan: "plan.md"
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-004
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-005
acceptance_criteria:
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.3
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.4
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.3
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.4
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.5
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.6
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.3
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.4
system_design:
  - ../../specs/tasks/system-design/cross-workspace-task-transfer.md
---

# Task 03: Deliver unified responsive movement actions

## Summary

Expose a complete one-task workspace move flow on desktop and phone, alongside existing step and workflow actions. Connect the real transfer API and reconciliation, prove successful and rejected transfers through the UI, and publish the matching user documentation.

## In scope

- Add the typed `transferTask` API client, proposed `useTaskWorkspaceMove` domain hook, and `TaskWorkspaceMoveDialog`. Source task ID/generation and destination selectors are shared business state; components only render.
- Use existing workspace/workflow catalog APIs and request-generation patterns. No active-workspace switch for catalog browsing, automatic destination-step choice, or client-side lane-equivalence algorithm.
- Make a common `Move to` parent with step, different-workflow, and different-workspace choices across all single-task entry points, including card, sidebar, detail, preview, Threads, and palette. Keep existing step options, progress, workflow agent mapping/preview, hidden-step rules, and plural bulk behavior. Do not duplicate the old standalone workflow command or leave dead adapters/props after caller migration.
- Widen only move step/progress lists to a preferred 20rem with viewport bounds; wrap labels/details without displacing dots/badges/options controls. Keep portaled vertical scrolling and touch navigation.
- Add shared immutable-submission state with admission guard and exact generation strings. Retain body/key across explicit Check result and close/reopen after network uncertainty. Distinguish original-request rejection from resolution-attempt denial: a check's 403/404 or arbitrary 409 enters unavailable-to-verify, retains the original body/key, and never promises rollback or enables a new key. Only its exact receipt or authoritative `operation_rejected` failed-key result settles an uncertain operation. Canonical placement reads do not prove its historical outcome. Freeze choices while unresolved; clear on auth identity change.
- Use Task 02 reconciliation on success. Listing stays selected with Open task; active detail route/session stays open and adopts canonical workspace. Late completion after user navigation must not steal focus.
- Localize all new menu, status, error, recovery, and explanatory copy in English plus six shipped translations. Use `i18n:zh-hant` for Traditional Chinese catalogs.
- Add unit/component behavioral coverage and desktop/mobile Playwright scenarios from the plan; update every existing E2E interaction that uses the replaced menu route. Keep existing consumer-visible step/workflow scenarios rather than re-pin menu array order.
- Cover real committed/lost-response checks after destination write revocation (403) and reach loss (404), close/reopen retention, and same-receipt replay after access restoration. Keep initial-submit denial tests. Add the A -> B -> A delayed-removal schedule, including a completed task with unchanged `updated_at`, and transient reconciliation-read failure.
- Exercise lost original `source_changed` and `step_incompatible` 409 responses through the real Web flow: keep the uncertain key until Check result returns authoritative `operation_rejected`, then refresh/review and submit a fresh key. Preserve existing task/history/sessions and do not classify a generic replay 409 as terminal.
- Update public how-to copy in `docs/public/tasks-and-workflows.md`, affected menu description in `sessions-and-review.md`, and cross-link from `automation-and-mcp.md`. Search root README and screenshots catalog; change only descriptions made stale, not speculative screenshots.
- Reconcile delivery references with original transfer/change-workflow packages. Mark this package implemented and paired specs active/current only after all work orders pass; do not re-label historical validation as current evidence.

## Out of scope

- Loosening equivalent-step or same-owner restrictions, mapping relations/profiles, multiple-task workspace transfer, new agent surfaces, new runtime toggles, or new transfer eligibility preview.
- Replacing the existing same-workspace workflow-change form or its lifecycle/profile semantics.

## Acceptance

1. All named single-task surfaces expose the grouped movement actions and the workspace form can move a supported task through actual API/storage, preserving UUID/history/sessions; existing step/workflow/bulk capabilities remain intact and long move labels remain readable.
2. Desktop and phone handle explicit dependent selections, catalog loading/error/empty states, cancel, safe original rejection, lost domain-conflict response recovery, uncertain-result check, unavailable-to-verify after permission loss, duplicate click, subject change, and canonical post-success context without partial mutation or lost draft/session state. Only authoritative operation resolution releases an uncertain key.
3. Production-build desktop/mobile E2E and rendered inspection match UI-01..UI-04, localization and public docs match the shipped behavior, and all requested checks have recorded actual results before package completion.

## ASCII UI preview

Full [combined preview](plan.md#ascii-ui-preview) is authoritative; this excerpt retains its labels. Copy/spacing are illustrative; hierarchy, grouping, scroll owner, recovery states, and actions are structural requirements. Applicable criteria: AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.1 through 004.6 and 005.3.

### UI-01: Desktop task movement menu, expanded

```text
Move to > Workflow step > [readable step/progress list]
          Different workflow...  -> existing form
          Different workspace... -> UI-02
```

### UI-02: Desktop workspace transfer, ready

```text
+------------------------------------------------+
| Move task to another workspace              X  |
| From: Personal / Kanban / In progress          |
| Workspace [Product                          v] |
| Workflow  [Delivery                         v] |
| Step      [o In progress                    v] |
| History and sessions kept. Matching settings.  |
|                          [Cancel] [Move task]  |
+------------------------------------------------+
```

### UI-03: Phone task movement and inset transfer drawer

```text
+-----------------------------+
|       underlying task       |
| +-------------------------+ |
| | Move task             X | | fixed header
| | From: Personal          | |
| | Workspace [Product   v] | |
| | Workflow  [Delivery  v] | | one scrolling body
| | Step      [In prog.  v] | |
| | History/sessions kept.  | |
| +-------------------------+ |
| | [Cancel] [Move task]     | | fixed safe-area footer
| +-------------------------+ |
+-----------------------------+
```

Nearest exemplar: `ChangeWorkflowDialog` anatomy plus `MobilePickerSheet` inset bottom geometry. Use `isMobile` below 768px even for fine pointers. Desktop ordinary controls are 28px; phone/coarse-pointer targets at least 44px, primary nominal 48px. Temporary three-choice depth favors a bottom drawer rather than a full workbench or stacked selector drawers. Dynamic viewport cap, safe-area footer, one body scroller, no page horizontal overflow. Preserve draft on responsive transitions.

### UI-04: Loading, unsupported, uncertain, and completed states

```text
Loading/empty: dependent selectors and Move disabled
Load failure: explanation + Retry
Rejected:     different step settings; choose matching step
Stale:        task changed + Refresh task; review before submit
Uncertain:    result not confirmed + Check result; choices locked
Cannot check: access unavailable; same request kept, no new move
Success:      task moved + Open task (listing stays selected)
```

Focus/keyboard dismissal and translated live status/error announcements are required. The design notice must not claim the selected destination is eligible before server validation, or that a denied result check proves the original move failed. Placement reads and operation settlement remain separate; unknown outcomes retain their submitted key.

## Verification

Fresh worktree only: run `(cd apps && pnpm install --frozen-lockfile)` once if not already installed. Begin with behavioral RED coverage and run headless E2E against fresh production builds, never `--headed`, all-worker overrides, overlapping suites, or stale `--no-build` output. Commands below are rooted independently:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-task-workspace-move.test.ts components/task/task-workspace-move-dialog.test.tsx components/task/task-move-context-menu.test.tsx components/task/change-workflow-dialog.test.tsx components/task/task-palette-move.test.tsx lib/ws/handlers/task-transfer.test.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/task-workspace-move.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/task/threads-task-actions.spec.ts tests/task/command-palette-task-actions.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-task-workspace-move.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts tests/task/mobile-threads-task-actions.spec.ts tests/task/mobile-command-palette-task-actions.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
make fmt
make typecheck test lint
```

These are minimum exact suites. If the caller inventory changes additional menu tests/E2E specs, include those suites and record their exact commands too. Root broad formatting/check commands are explicitly requested by this task, not an automatic extra review gate; run them only after the feature exists and do not overwrite unrelated user changes. Inspect a real desktop and phone rendered flow (including incompatible destination and uncertain result) using the disposable managed E2E runtime, capture screenshots, and compare them with the preview structure. Record which layouts and scenarios were exercised.

## Files likely touched

- `apps/web/lib/api/domains/kanban-api.ts` and existing API export surface
- New `apps/web/hooks/domains/kanban/use-task-workspace-move.ts`, focused data/submit helpers if needed, and tests
- New `apps/web/components/task/task-workspace-move-dialog.tsx` and tests
- `apps/web/hooks/use-task-actions-menu.ts`, `use-task-menu-dialog-state.ts`, and task-menu dialog host
- `apps/web/components/kanban-card-menu-items.tsx`, `kanban-card-menu-builder.tsx`, `task/task-move-context-menu.tsx`, `task/task-switcher-context-menu-move-items.tsx`, and menu adapters discovered through references
- `apps/web/lib/kanban/task-actions-menu-entries.ts` and palette/sidebar/Threads consumers
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko}/` affected namespaces
- New `apps/web/e2e/tests/kanban/task-workspace-move.spec.ts`, `mobile-task-workspace-move.spec.ts`, shared helpers/page object
- Existing affected E2E files named in Verification; existing change-workflow page object and menu helpers
- `docs/public/tasks-and-workflows.md`, `sessions-and-review.md`, `automation-and-mcp.md`
- Owning requirement/design, this plan/work orders, and affected companion delivery links/presentation wording

## Dependencies

Tasks 01 and 02. Do not expose workspace transfer while source-removal notifications can destroy local session context.

## Risks

- Some existing single-task actions derive eligibility only from current workflow step count; that must not hide the new workspace choice.
- One movement group has multiple render adapters. Migrating only card actions would leave sidebar/palette/Threads inconsistent.
- Portaled menu pointer events can close a form mounted under the menu; keep dialog state above ephemeral menu content and preserve captured task identity.
- The source generation is nanosecond precision. Using a JavaScript Date causes false stale conflicts.
- Strict step equivalence and unmapped relations can reject common configurations. Explain them; never weaken guards to get E2E green.
- E2E backend state is worker-scoped. Use disposable workspaces and restore modified shared seed definitions in `finally`/cleanup rather than leaking them into adjacent specs.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/cross-workspace-task-transfer.md), especially 004 and 005.
- [System design](../../specs/tasks/system-design/cross-workspace-task-transfer.md): all Web sections.
- [Plan test matrix and previews](plan.md).
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, `/e2e`, `/docs-maintainer`.
- Existing `ChangeWorkflowDialog`, `use-change-workflow-data.ts`, `TaskMoveContextMenuItems`, `MobilePickerSheet`, cross-workflow E2E, and Task 01/02 contracts.

## Results

Pending. Record RED/GREEN, exact test discovery/results, rendered desktop/phone screenshots, localization/public-doc checks, and requested broad checks. Only after every work order is complete promote specs/plan and hand control to the user; this work order does not authorize commit/push/PR on its own.
