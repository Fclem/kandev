---
spec: docs/specs/automation-runs-delete-all-by-status/spec.md
created: 2026-08-12
status: draft
---

# Implementation Plan: Status-scoped delete-all for automation runs

## Overview

Scope the Recent Runs delete-all to the active status view and move the
control into the table header's rightmost column. The frontend hook gains an
optional run-id list on `deleteAllRuns`; a filtered view deletes exactly the
visible runs through the existing per-run `automation.run.delete` path, while
the "All" view keeps the single `automation.runs.delete_all` call. No backend
changes: `Archived`/`Cancelled` are read-time-derived statuses that only exist
in the loaded payload, so the visible set is the only exact scope.

## Backend

None. The per-run delete path already deletes each run's associated task, and
per-run deletes are id-targeted so the orphaned-task race guarded by
`DeleteAllRuns`' per-automation run lock (a broad DELETE catching a
concurrently-created run) cannot occur.

## Frontend

### Hook — `apps/web/hooks/domains/settings/use-automation-runs.ts`

Extend `deleteAllRuns(runIds?: string[])`:

- No argument → unchanged: optimistic `clearRuns(automationId)` + one
  `deleteAllAutomationRuns(automationId, workspaceId)` call, success
  re-clear, failure → toast + recovery refresh, refresh failure → pre-clear
  snapshot restore (existing tests keep passing).
- Empty array → no-op (defensive; the UI never renders the button for an
  empty view).
- Non-empty ids → snapshot the pre-delete list, `removeRun` each id
  optimistically, then `Promise.all(ids.map((id) => deleteAutomationRun(id,
  workspaceId)))`. On success re-`removeRun` each id (in-flight-refresh
  guard, same pattern as `deleteRun`). On any failure: one error toast
  (`automations:failedToDeleteRuns`), one recovery `listAutomationRuns`
  refresh; if that also fails, restore the pre-delete snapshot +
  `automations:couldNotRefreshRuns` toast. Aggregated: one toast, one
  refresh per failed batch, not per id.

Deps update: replace `clearRuns` with `removeRun` in the callback closure.

### Component — `apps/web/components/automations/runs-section.tsx`

- Remove the `DeleteAllButton` from the section header row (next to the
  refresh button).
- Render it inside the last `<TableHead className="w-8">` (the column holding
  the per-row delete buttons), gated on `visibleRuns.length > 0` and
  `expanded` (the table only renders when expanded):
  `onConfirm={() => deleteAllRuns(statusFilter === "all" ? undefined : visibleRuns.map((r) => r.id))}`.
- `DeleteAllButton` gains the current filter so the dialog picks its copy:
  - "all" → existing `deleteAllRunsTitle` / `deleteAllRunsDescription`
    (e2e asserts `permanently remove all run records`).
  - filtered → new `deleteAllRunsScopedTitle` / `deleteAllRunsScopedDescription`
    with `{{status}}` = the localized status label (`t` of the matching
    `STATUS_FILTERS` entry's `labelKey`, e.g. "Skipped", "Archived").

### Locales — `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/automations.json`

Add, in all four catalogs (plain punctuation, no em dash):

- `deleteAllRunsScopedTitle`: `Delete all {{status}} runs?`
- `deleteAllRunsScopedDescription`: `This will permanently remove the {{status}} runs shown in this view and their associated tasks. This cannot be undone.`

## Tests

- **What:** scoped delete-all removes exactly the given run ids and calls the
  per-run API per id.
  **File:** `apps/web/hooks/domains/settings/use-automation-runs.test.ts`
  **How:** mock `deleteAutomationRun`; assert per-id calls with
  `WORKSPACE_ID`, optimistic removal of exactly those ids, success re-removal
  guard against a stale refresh, single aggregated failure toast + recovery
  refresh, and double-failure snapshot restore. Add a no-op assertion for
  `deleteAllRuns([])`.
- **What:** delete-all lives in the table header and is view-scoped.
  **File:** `apps/web/components/automations/runs-section.test.tsx`
  **How:** assert the button renders inside the table header (not beside the
  Recent Runs heading); filtered view confirm calls `deleteAllRuns` with the
  visible ids; "All" view calls `deleteAllRuns()` with no ids; empty filtered
  view renders no button; filtered dialog shows the scoped title/description.
- **What:** the "All" view still uses the single bulk delete.
  **File:** `apps/web/hooks/domains/settings/use-automation-runs.test.ts`
  **How:** existing no-arg tests already assert `deleteAllAutomationRuns` is
  called with `(AUTOMATION_ID, WORKSPACE_ID)`; they must keep passing
  unchanged.

## E2E Tests

- **Scenario:** Skipped-filtered delete-all removes only the skipped rows.
  **File:** `apps/web/e2e/tests/automations-settings.spec.ts`
  **What to verify:** seed 2 skipped + 1 succeeded run; expand Recent Runs;
  filter Skipped (2 rows); assert `delete-all-runs` sits inside the table
  header; click it and confirm; dialog shows the scoped copy
  (`permanently remove the Skipped runs shown in this view`); table shows 1
  row; switch to All and assert the succeeded row remains.
- **Scenario:** All-view delete-all keeps the existing copy and placement.
  **File:** `apps/web/e2e/tests/automations-settings.spec.ts`
  **What to verify:** the existing "delete individual and all runs from
  Recent Runs" test keeps passing unchanged (it already asserts header
  visibility and the unqualified dialog copy).

## Verification Results

Pending. On completion, synchronize with each task's `## Results` and record
exact commands and outcomes.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [ ] [task-01-hook-status-scoped-delete](task-01-hook-status-scoped-delete.md)
- [ ] [task-02-runs-table-header-delete-all](task-02-runs-table-header-delete-all.md)
- [ ] [task-03-e2e-status-scoped-delete](task-03-e2e-status-scoped-delete.md)

The hook and the component are a vertical slice (the component consumes the
hook's new signature in the same change cycle); E2E follows both.

## Open Questions

None.
