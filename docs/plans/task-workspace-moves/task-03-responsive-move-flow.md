---
id: "03-responsive-move-flow"
title: "Deliver unified responsive movement actions"
status: done
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
  - ../../specs/tasks/system-design/cross-workspace-task-transfer-web-recovery.md
---

# Task 03: Deliver unified responsive movement actions

## Summary

Expose a complete one-task workspace move flow on desktop and phone, alongside existing step and workflow actions. Connect the real transfer API and reconciliation, prove successful and rejected transfers through the UI, and publish the matching user documentation.

## In scope

- Add the typed `transferTask` API client, proposed `useTaskWorkspaceMove` domain hook, and `TaskWorkspaceMoveDialog`. Source task ID/generation and destination selectors are shared business state; components only render.
- Use existing workspace/workflow catalog APIs and request-generation patterns. No active-workspace switch for catalog browsing, automatic destination-step choice, or client-side lane-equivalence algorithm.
- Make a common `Move to` parent with step, different-workflow, and different-workspace choices across all single-task entry points, including card, sidebar, detail, preview, Threads, and palette. Keep existing step options, progress, workflow agent mapping/preview, hidden-step rules, and plural bulk behavior. Do not duplicate the old standalone workflow command or leave dead adapters/props after caller migration.
- Widen only move step/progress lists to a preferred 20rem with viewport bounds; wrap labels/details without displacing dots/badges/options controls. Keep portaled vertical scrolling and touch navigation.
- Add shared immutable-submission state with admission guard and exact generation strings. Retain body/key across explicit Check result and close/reopen after network uncertainty. Distinguish original-request rejection from resolution-attempt denial: a check's 403/404 or arbitrary 409 enters unavailable-to-verify, retains the original body/key, and never promises rollback or enables a new key. A check's 409 `relations_busy` stays uncertain (not unavailable-to-verify), because it is transient and establishes nothing about the original attempt. Only its exact receipt or authoritative `operation_rejected` failed-key result settles an uncertain operation. Canonical placement reads do not prove its historical outcome. Freeze choices while unresolved; clear on auth identity change.
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
- [System design](../../specs/tasks/system-design/cross-workspace-task-transfer.md) and [web submission recovery companion](../../specs/tasks/system-design/cross-workspace-task-transfer-web-recovery.md): all Web sections.
- [Plan test matrix and previews](plan.md).
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, `/e2e`, `/docs-maintainer`.
- Existing `ChangeWorkflowDialog`, `use-change-workflow-data.ts`, `TaskMoveContextMenuItems`, `MobilePickerSheet`, cross-workflow E2E, and Task 01/02 contracts.

## Results

Completed 2026-11-11.

Implementation:

- `transferTask` API client and HTTP types; `lib/kanban/task-workspace-move-operation.ts` classifies outcomes (original rejection vs. `uncertain` / `check-unavailable` for a check's network, 5xx, 403/404 or non-terminal 409; only `operation_rejected` or a receipt settles) and admits one request per task synchronously. The registry is scoped by authenticated identity. Per the 2026-11-07 design change (`1e45e36263`), `task-workspace-move-storage.ts` persists unresolved requests to `sessionStorage` (identity fields, policy, FNV-1a request digest, caller only). Records are partitioned per caller and task (one `sessionStorage` entry each). A reload, a switch to another caller and back, or a logout and login restores the caller's record as `uncertain` with **Check result**, and keeps the fresh-key move blocked. There is no automatic POST. Other callers' records stay stored but are hidden and never sent. Only a committed receipt or an authoritative terminal rejection removes a record. A record that fails validation shows "cannot be checked from this browser". When tab storage cannot be read, or refuses a submission's write, a separate cause-neutral notice (`workspaceMoveStorageUnavailable`) disables new moves while checks of already stored records still re-send them; a refused write drops the submitting task's outcome and every resolved rejection (`rejected` or `operation-rejected`), so those forms show the notice, while committed outcomes stay until the form closes.
- `useTaskWorkspaceMove` handles the dependent workspace, workflow and step choices, with ordered steps and no automatic step choice. It never calls `setActiveWorkspace`, binds the exact `updated_at` string, uses a fresh key per reviewed submission, and requires a source refresh after `source_changed` or `operation_rejected`. `TaskWorkspaceMoveDialog` renders a compact dialog on desktop and an inset dynamic-viewport drawer below 768px, with one scroll body and a fixed safe-area footer. Form state lives above that responsive switch, so resizing keeps the draft. `TaskWorkspaceMoveHost` in the app shell owns the form, so transient menus capture the subject and cannot retarget it.
- One single-task `Move to` group (Workflow step, Different workflow..., Different workspace...) on card context and three-dot menus, task actions (detail/preview), the sidebar context menu (`task-move-context-menu-items.tsx`), Threads desktop menu and phone drawer, and the command palette. The workspace choice stays available for one-step workflows. Bulk selection keeps its plural step and workflow actions and has no workspace choice. Step and workflow lists use a preferred 20rem, viewport-bounded width with wrapping labels.
- 56 new `task` keys in en, pt-pt, zh-cn, ja and ko. zh-tw and zh-hk were generated by `pnpm run i18n:zh-hant`, and pseudo by `i18n:pseudo`. The old `task:changeWorkflow` key was removed. Unrelated generator churn in `zh-*/automations.json` was reverted.
- Public docs: `tasks-and-workflows.md` (new Move to / workspace sections), `sessions-and-review.md` (Threads menu), `automation-and-mcp.md` (write scope plus cross-link). Root README and `docs/screenshots.md` had no stale references. Active spec wording for change-workflow, task-menu grouping, task actions, Threads and mobile navigation now uses the grouped labels. Historical plans are unchanged.

RED/GREEN:

- `use-task-workspace-move.test.ts`: 9 behavioral cases. A mutation that made any 409 terminal and dropped the busy admission failed 2. A mutation that disabled persistence failed both reload-restore cases. After the round-2 caller-partition change there are 11 cases; a mutation that forgets the prior caller's records on identity change fails the identity switch-back case and the logout case. Restoring the code passes 11/11.
- `task-workspace-move-dialog.test.tsx` (7 presentation states) and updated menu, palette, drawer and sidebar tests: the full web vitest run passed 6,800 tests in 709 files before the reload change; the affected files pass after it.
- Phone render RED: the first managed phone run showed long selected workspace and workflow names overflowing their triggers (`ui-03-phone-rejected.png`). Adding `grid-cols-[minmax(0,1fr)]` to the form grid fixed it. A label/trigger containment assertion now runs in `expectPhoneGeometry`; the probe measured the label clipped at 342px inside a 377px trigger.

Verification (headless managed production builds, one worker per shard; Node 24 via mise):

- `(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-task-workspace-move.test.ts components/task/task-workspace-move-dialog.test.tsx components/task/task-move-context-menu.test.tsx components/task/change-workflow-dialog.test.tsx components/task/task-palette-move.test.tsx lib/ws/handlers/task-transfer.test.ts)`: passed.
- `pnpm run i18n:check`: all checks passed; the six locales are complete. `pnpm run i18n:ratchet`: 22 modified files clean; new files are not yet tracked, so the ratchet did not judge them. Oxlint has no warnings and `tsrs --noEmit` passes.
- `pnpm e2e:run --project chromium`: `task-workspace-move.spec.ts` (5), `cross-workflow-task-move`, `threads-task-actions`, `command-palette-task-actions`, `change-workflow`, `task-menu-grouping`, `workflow-steps`. All 36 pass after migrating the 2 change-workflow callers.
- `pnpm e2e:run --project mobile-chrome`: `mobile-task-workspace-move.spec.ts` (2), `mobile-sidebar-task-actions`, `mobile-threads-task-actions`, `mobile-command-palette-task-actions`, `mobile-change-workflow`, `mobile-workflow-session-focus`, `mobile-workflow-step-progress`, `mobile-workflow-step-move-overrides`, `mobile-auto-hide-empty-columns`, `mobile-wip-overflow-queue`: 39 passed.
- The desktop running-task scenario covers a live mock turn whose `after move` output arrives after the transfer, with the same session IDs, and reload persistence. A turn that updates the task while the form is open yields `source_changed`, then Refresh and resubmit.
- `node --test scripts/validate-public-docs.test.mjs` (63 pass); `node scripts/validate-public-docs.mjs` (55 pages); `python3 scripts/list-docs.py validate`; `python3 scripts/lint-spec-files.py --all`: passed.
- Rendered evidence (disposable managed runtime; screenshots kept outside the repo in `~/.cache/tmp/ws-move-shots/`): UI-01 desktop menu, UI-02 desktop ready, UI-04 desktop rejected, uncertain, cannot-check and success, UI-03 phone menu, ready and rejected, and UI-04 phone success. Each matches the preview structure.

Broad gates: see the plan's verification results.
