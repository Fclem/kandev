---
id: "03-frontend-dialog-and-mobile"
title: "Dialog persistence, workflow selection, and mobile entry"
status: done
wave: 2
depends_on: ["02-backend-issue-workflow"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-KANDEV-001.1
  - AC-WORKSPACES-IMPROVE-KANDEV-001.2
  - AC-WORKSPACES-IMPROVE-KANDEV-001.3
  - AC-WORKSPACES-IMPROVE-KANDEV-001.4
  - AC-WORKSPACES-IMPROVE-KANDEV-001.5
  - AC-WORKSPACES-IMPROVE-KANDEV-001.6
  - AC-WORKSPACES-IMPROVE-KANDEV-001.7
  - AC-WORKSPACES-IMPROVE-KANDEV-001.8
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
---

# Task 03: Dialog persistence, workflow selection, and mobile entry

## Acceptance

- The desktop app-sidebar action enters the Improve Kandev flow: first-use intro
  or task creation without the intro. **Do not show this again** persists
  safely; GitHub-auth recovery takes precedence, and a missing dedicated
  workspace still shows its choice gate. The phone Utilities row is at least
  44px tall, closes the menu, and opens the shared dialog without stacked
  overlays.
- **Open issue** selects its workflow and one-step preview, shows report-only
  guidance, and remains available when implementation kinds are fork-blocked.
  Log capture defaults on for **Bug fix**, off for **Feature request** and
  **Open issue**, and remains adjustable for every kind.
- When no dedicated workspace exists, the intro shows a checked-by-default
  workspace-creation checkbox; if the intro is skipped, a choice panel appears
  before bootstrap. Opting out targets the active workspace. An existing
  dedicated workspace is reused without showing the choice, and the bootstrap
  response supplies the task's target workspace, repository, and workflow IDs.

## UI Preview

These excerpts use the same labels as the [plan preview](plan.md#ui-preview);
they define structure and order, not pixel spacing. UI-01–02 cover ACs `.3`,
`.4`, and `.8`; UI-03 and UI-05 cover ACs `.1`, `.2`, and `.5`–`.7`; UI-04
covers AC `.1`. Task 04 proves desktop/mobile presentation; Task 06 proves
workspace selection and task placement.

### UI-01: Desktop intro, first open
Entry point: the app-sidebar footer **Improve Kandev** action.

```text
+-----------------------------------------------+
| Improve Kandev                                 |
| Contribution-flow explanation                  |
| [x] Create a dedicated Improve Kandev workspace|
| [x] Do not show this again                     |
| [Cancel]                            [Contribute]|
+-----------------------------------------------+
```

If GitHub authorization is missing, show its actionable recovery instead of
the intro, regardless of the saved intro preference.

### UI-02: Desktop workspace choice, intro skipped

```text
+-----------------------------------------------+
| Improve Kandev                                 |
| Dedicated workspace does not exist yet        |
| [x] Create a dedicated Improve Kandev workspace|
| [Cancel]                             [Continue]|
+-----------------------------------------------+
```
On phones this choice appears in the viewport-contained dialog after the menu
closes, with a touch-sized checkbox.

### UI-03: Desktop task creation

```text
+------------------------------------------------------+
| [Bug fix] [Feature request] [Open issue]              |
| Repository: kdlbs/kandev (locked)                     |
| Branch: main (locked)                                 |
| Workflow: Improve Kandev (locked)                     |
| Title: [                                           ]  |
| Description: [starter template text                ]  |
| [x] Include recent logs                               |
| Workflow preview                                      |
| [Cancel]                                  [Create task]|
+------------------------------------------------------+
```

**Open issue** replaces the implementation preview and contributor guidance
with the report-only notice and one-step workflow.

### UI-04: Phone entry
Entry point: **Open menu**, then **Improve Kandev** in Utilities.

```text
+-------------------------------+
| Mobile menu                   |
| Utilities                     |
| [Improve Kandev]              | 44px-or-larger row
+-------------------------------+
```

### UI-05: Phone task creation

```text
+-------------------------------+
| Improve Kandev                | fixed header
| [Bug fix] [Feature] [Issue]   |
| Repository: kdlbs/kandev      |
| Branch: main                 |
| Workflow: Improve Kandev   |
| Title: [                    ]|
| Description: [starter text  ]|
| [x] Include recent logs      |
| [Create task]                |
+-------------------------------+
```

The mobile menu closes before the shared task-create dialog opens. The dialog
owns internal scrolling and safe-area padding; no overlays are stacked.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/improve-kandev-dialog-model.test.ts components/improve-kandev-dialog-helpers.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/improve-kandev-dialog.tsx components/improve-kandev-dialog-create.tsx components/improve-kandev-dialog-model.ts components/kanban/mobile-menu-sheet.tsx
```

## Files likely touched

- `apps/web/lib/api/domains/improve-kandev-api.ts`
- `apps/web/components/improve-kandev-dialog.tsx`
- `apps/web/components/improve-kandev-dialog-create.tsx`
- `apps/web/components/improve-kandev-dialog-model.ts`
- `apps/web/components/improve-kandev-dialog-model.test.ts`
- `apps/web/components/improve-kandev-dialog-helpers.test.ts`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`

## Dependencies

Task 02.

## Parallelism

Sequential. This task consumes the bootstrap contract and owns the UI used by
Task 04.

## Inputs

- Spec: **What**, **Persistence guarantees**, **Failure modes**, and mobile
  scenario.
- Plan: **Frontend**, **Workspace target choice**, **Mobile design contract**,
  and [UI Preview](plan.md#ui-preview).
- Mobile exemplar:
  `apps/web/components/kanban/mobile-menu-sheet.tsx`.

## Completed implementation notes

- The response type, dual step loading, intro checkbox/local-storage behavior,
  issue tab/notice, workflow switching, EMU scoping, and mobile utility entry
  are already present.
- Frontend typecheck and existing description-helper tests passed before the
  final mobile-menu edit.
- Pure model tests were added before extraction, and the known ESLint
  complexity, nested-ternary, and line-length warnings were resolved without
  suppressions.

## Recorded implementation and verification

- Added `improve-kandev-dialog-model.ts` with safe local-storage persistence,
  intro-mode selection, workflow/start-step selection, and EMU fork-block
  decisions.
- Added five table-driven model tests before extraction; the initial run failed
  because the model module was absent, then passed after implementation.
- Refactored the dialog to consume the pure model and extracted the report-only
  bottom slot/contributor message decisions.
- Added the mobile utility entry and extracted its render surface so the
  touched component passes repository line/complexity/nested-ternary rules.
- `cd apps && pnpm --filter @kandev/web test -- --run components/improve-kandev-dialog-model.test.ts components/improve-kandev-dialog-helpers.test.ts` — passed (10 tests).
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm exec eslint components/improve-kandev-dialog.tsx components/improve-kandev-dialog-create.tsx components/improve-kandev-dialog-model.ts components/kanban/mobile-menu-sheet.tsx` — passed with no warnings.

Task 03 is complete. Continue with Task 04 for rebuilt desktop and mobile
browser coverage.

## Risks

- `TaskCreateDialog` resets form state when its locked `workflowId` prop
  changes. Confirm report-kind switching before data entry or preserve entered
  title/description deliberately; do not silently lose user text.
- Closing the mobile drawer before opening the dialog must preserve predictable
  focus/dismiss behavior.
- Local-storage access can throw in restricted browser contexts.

## Output contract

The shared dialog persists the intro preference safely, switches between the
implementation and report-only workflows, and is reachable through the native
mobile menu without stacked overlays.

Update this task and `plan.md`, list the helper behavior proven test-first,
record exact unit/typecheck/lint results, and note any remaining rendered
behavior for Task 04.
