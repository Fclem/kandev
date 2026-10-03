---
status: draft
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
created: 2026-07-27
---

# Implementation Plan: Improve Kandev Dialog and Issue Reporting

## Overview

Extend the existing Improve Kandev bootstrap so it provisions both the
implementation workflow and a one-step issue-reporting workflow, then let the
shared task-create surface select the correct hidden workflow. Add a
browser-local intro dismissal, preserve GitHub-auth and fork safeguards, and
expose the same flow from the existing mobile home menu.
Tasks 01–04 and their original implementation, tests, and verification are
complete. Follow-up work orders add browser acceptance coverage for workspace
targeting and diagnostic-bundle attachment. Task 05 remains pending as the final
repository-wide verification and commit gate after those follow-ups.

Relevant workflow decision:
[Single-Session Model Switching](../../decisions/2026-07-26-single-session-model-switching.md).

## Planning Guardrail

The session learning that triggered this plan is already encoded in:

- `AGENTS.md`
- `.agents/skills/spec-driven-development/SKILL.md`
- `apps/backend/config/workflows/improve-kandev.yml`

These files now state that workflow-generated implementation envelopes do not
skip spec/plan/task creation. See
[Task 01](task-01-harness-planning-gate.md).

---

## Backend

### Hidden issue-reporting workflow

- Add `apps/backend/config/workflows/report-kandev-issue.yml` with template ID
  `report-kandev-issue`.
- Keep it hidden and give it one auto-starting **Open issue** step.
- The prompt must read the current repository issue forms, classify bug versus
  feature, gather every required field through `ask_user_question_kandev`,
  reject public security disclosure, check duplicates, preview the complete
  public draft, obtain publication approval, and run `gh issue create`.
- Extend `apps/backend/config/workflows/loader_test.go` to prove the template is
  present, hidden, single-step, auto-starting, and contains the required
  reporting safeguards.

### Improve Kandev bootstrap

- Extend `BootstrapResponse` in
  `apps/backend/internal/improvekandev/handler.go` with
  `IssueWorkflowID string 'json:"issue_workflow_id"'`.
- Generalize `ensureWorkflow` so bootstrap idempotently resolves or creates
  both `improve-kandev` and `report-kandev-issue`, healing the hidden flag for
  either stale workspace instance.
- Return both workflow IDs in the same bootstrap response; preserve repository
  reuse/cloning, bundle generation, GitHub login, write access, and fork-status
  behavior.
- ACs `.10`–`.13` require first-time creation of the dedicated workspace to
  inherit only the GitHub connection and applicable PAT from the default
  workspace resolved by `workspacescope.ResolveMigrationTarget`. If no
  connection exists, none is copied; other integrations, automations, or
  user-created resources beyond bootstrap defaults are not copied. Reuse leaves
  existing user configuration unchanged.
- Add a handler integration test in
  `apps/backend/internal/improvekandev/handler_test.go` using a real test
  SQLite task/workflow repository plus fake cloner and GitHub probe. POST the
  bootstrap route twice and assert stable, distinct workflow IDs, hidden
  workspace workflows, and the issue workflow's start step.

---

## Frontend

### API and dialog model

- Add `issue_workflow_id` to
  `ImproveKandevBootstrapResponse` in
  `apps/web/lib/api/domains/improve-kandev-api.ts`.
- Load steps for both workflow IDs during bootstrap.
- Extract the non-rendering decisions currently accumulating in
  `improve-kandev-dialog.tsx` and
  `improve-kandev-dialog-create.tsx` into a focused pure model/helper with
  Vitest coverage:
  - read/write `kandev.improveKandev.skipIntro` safely when local storage is
    unavailable;
  - choose intro versus create mode;
  - choose implementation versus issue workflow and start step;
  - block implementation kinds, but not **Open issue**, for `blocked_emu`.
- Keep GitHub-auth recovery authoritative: a saved intro dismissal must not
  bypass a missing-auth message.

### Intro and task kind selection

- Add a controlled **Do not show this again** checkbox to the existing intro.
  The labeled touch target is at least 44px high.
- Add **Open issue** beside **Bug fix** and **Feature request**.
- Default log capture on for Bug fix and off for Feature request or Open issue;
  keep the toggle user-adjustable for every report kind.
- When Open issue is selected, switch the locked workflow/default step to
  `issue_workflow_id`, use issue-specific placeholder copy, show an explicit
  report-only notice, and show only the one-step workflow preview.
- The notice states that the agent follows the repository issue template, asks
  for missing information and approval, creates an issue, and does not
  implement code or open a pull request.
- Retain the existing contributor banner for implementation kinds. For issue
  reporting, explain that a fork and push access are not required.

### Workspace target choice

- When no dedicated workspace exists, the intro shows a checked-by-default
  **Create a dedicated Improve Kandev workspace** checkbox. If the intro was
  skipped, a separate choice panel appears before bootstrap. Unchecking either
  checkbox selects the active workspace; checking it creates or reuses the
  dedicated workspace.
- When the dedicated workspace already exists, skip the choice and target it
  directly. The bootstrap response's workspace owns the task, repository, and
  hidden workflow IDs.

### Mobile design contract

- **Desktop outcome:** the app-sidebar footer action opens the intro or task
  creation. A saved intro preference skips the intro, while GitHub-auth recovery
  and the missing-workspace choice panel retain precedence.
- **Mobile entry point:** add a 44px-or-larger **Improve Kandev** row to the
  existing `MobileMenuSheet` Utilities section.
- **Nearest exemplar:** `apps/web/components/kanban/mobile-menu-sheet.tsx`;
  reuse its inset full-height drawer, fixed header, internal scrolling body,
  safe-area padding, and utility-row geometry.
- **Hierarchy and primary action:** tapping the utility row closes the menu,
  then opens the shared Improve Kandev dialog. The task-create dialog remains
  the single focal surface; no stacked drawer/dialog.
- **Scroll and state:** the mobile menu remains the drawer's single scroll
  owner, the intro dialog is viewport-contained with internal vertical
  scrolling, and the dialog state/business logic is shared across viewports.
- Refactor `MobileMenuSheet` and `CreateModeView` so touched functions remain
  within the repository's line, complexity, and no-nested-ternary limits.

## UI Preview

These sketches define structure, order, and navigation; spacing is illustrative.
The task-create state reuses the shared dialog. UI-01–02 cover ACs `.3`, `.4`,
and `.8`; UI-03 and UI-05 cover ACs `.1`, `.2`, and `.5`–`.7`; UI-04 covers
AC `.1`. Task 03 and Task 07 carry their relevant excerpts and rendered checks.

### UI-01: Desktop intro, first open
Entry point: the **Improve Kandev** action in the app-sidebar footer.

```text
+------------------------------------------------------+
| Improve Kandev                                        |
| Explain the contribution flow                         |
| [x] Create a dedicated Improve Kandev workspace      |
|      (only when no dedicated workspace exists)        |
| [x] Do not show this again                            |
| [Cancel]                                  [Contribute]|
+------------------------------------------------------+
```

If GitHub authorization is missing, the intro body is replaced by the
actionable authorization message; saved intro dismissal never bypasses it.

### UI-02: Desktop workspace choice, intro skipped

```text
+------------------------------------------------------+
| Improve Kandev                                        |
| The dedicated workspace does not exist yet           |
| [x] Create a dedicated Improve Kandev workspace      |
| [Cancel]                                    [Continue]|
+------------------------------------------------------+
```

The choice panel appears only when the dedicated workspace is absent and the
intro was skipped. An existing dedicated workspace is selected directly.
On phones the same choice appears in the viewport-contained dialog; the mobile
menu is already closed and the checkbox remains touch-sized.

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

Selecting **Open issue** shows its report-only notice and one-step workflow;
implementation kinds retain the contributor information and multi-step
workflow. The log toggle remains user-adjustable.

### UI-04: Phone entry
Entry point: tap **Open menu**, then the **Improve Kandev** Utilities row.

```text
+-------------------------------+
| Mobile menu                   | fixed header
| ...                           |
| Utilities                     | scrolling body
| [Improve Kandev]              | 44px-or-larger row
| ...                           |
+-------------------------------+
```

Tapping the row closes the menu before opening the shared dialog.

### UI-05: Phone task creation

```text
+-------------------------------+
| Improve Kandev                | fixed dialog header
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

The dialog is viewport-contained with internal vertical scrolling and safe-area
padding. No menu and dialog are stacked.

---

## Tests

- **What:** embedded issue workflow contract.
  **File:** `apps/backend/config/workflows/loader_test.go`.
  **How:** load embedded templates and assert ID, hidden flag, one step,
  auto-start event, and prompt safeguards.
- **What:** bootstrap creates/reuses both hidden workflow instances.
  **File:** `apps/backend/internal/improvekandev/handler_test.go`.
  **How:** Gin handler integration test with real SQLite repositories and fake
  external boundaries; call twice and inspect response plus persisted workflow
  state.
- **What:** local-storage preference, workflow selection, and fork-block
  decisions.
  **File:** a focused `*.test.ts` beside the extracted dialog model/helper.
  **How:** table-driven Vitest cases using an in-memory Storage-compatible fake.
- **What:** standard diagnostic-bundle attachment and localized log-toggle copy.
  **File:** the helper/component tests, desktop and mobile Improve Kandev E2E
  specs, and `apps/web/src/locales/*/common.json`.
  **How:** assert exact backend/frontend/runtime sources excluding ACP, ready
  and polled partial job leasing, authoritative lease path in the submitted task
  description, and task creation without an unleased path after failed/expired
  collection or lease failure. Verify the generic localized toggle label and
  all locale catalogs with the i18n checks.
- **What:** workspace choice and active-workspace fallback.
  **File:** `apps/backend/internal/integration/improve_kandev_test.go`.
  **How:** retain integration coverage for default dedicated-workspace creation and reuse; assert that declining creation keeps both hidden workflows and the kandev repository scoped to the requested active workspace.
- **What:** managed GitHub probe uses the selected workspace.
  **File:** `apps/backend/internal/integration/improve_kandev_test.go`.
  **How:** make active and dedicated workspace probe results distinct; assert an existing dedicated target wins over a different request workspace and the response's GitHub identity/fork status match that target, then assert opt-out fallback uses the requested active workspace's result.
- **What:** ACs `.10`–`.13`; first-create GitHub connection inheritance and
  workspace isolation.
  **File:** `apps/backend/internal/integration/improve_kandev_test.go`,
  `apps/backend/internal/integrations/workspacescope/workspacescope_test.go`,
  and `apps/backend/internal/github/service_connections_test.go`.
  **How:** verify default-source priority, actual connection/PAT copying, no
  copy when the source has no connection, no unrelated integration,
  automation, workflow, or repository copying, and no mutation of an existing
  dedicated workspace's configuration.

---

## E2E Tests

- **Scenario:** selecting the intro dismissal persists through reload and later
  enters task creation without the intro when the dedicated workspace exists.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** use a seeded dedicated workspace, assert the local-storage value,
  reload, direct create dialog, and absent intro copy.
- **Scenario:** selecting **Open issue** creates a task in the issue workflow.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** report-only notice, submitted task title, and issue workflow start
  step.
- **Scenario:** an EMU-shaped account cannot submit implementation work but can
  submit an issue report.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** disabled submit for Bug fix/Feature request and enabled submit
  after selecting Open issue.
- **Scenario:** on first open with no dedicated workspace, the user leaves the
  default-checked intro workspace option enabled and submits an improvement
  task.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** controlled bootstrap receives `create_workspace: true`, returns
  the seeded non-active workspace with its repository and both workflow IDs,
  and the task is created there. Backend integration verifies real creation.
- **Scenario:** with the intro preference already dismissed and no dedicated
  workspace, the workspace-choice panel appears before bootstrap.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** assert bootstrap has not run before choice confirmation, observe
  the default-checked checkbox, uncheck it, then verify
  `create_workspace: false`, `workspace_id` equals the active workspace ID,
  and the task is created there; backend integration verifies real fallback.
- **Scenario:** diagnostic collection fails/expires or leasing fails while the
  user submits an improvement task.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **Verify:** controlled bundle responses return `failed`/`expired` or reject
  the lease request; task creation still succeeds with the original description
  and no bundle path.
- **Scenario:** a phone user opens Improve Kandev from the home menu and reaches
  the issue-only option.
  **File:** `apps/web/e2e/tests/mobile-improve-kandev.spec.ts`.
  **Verify:** visible 44px utility/preference targets, menu dismissal, report-
  only notice, and no document horizontal overflow.
- **Scenario:** phone task creation includes recent logs.
  **File:** `apps/web/e2e/tests/mobile-improve-kandev.spec.ts`.
  **Verify:** the localized **Include recent logs** toggle is visible in the
  shared create form at the phone viewport.

## Required cross-plan acceptance

AC-WORKSPACES-IMPROVE-KANDEV-001.9 is owned by the separate
[Kanban visibility plan](../improve-kandev-kanban-visibility/plan.md) and its
pending [Task 01](../improve-kandev-kanban-visibility/task-01-show-improve-kandev-cards.md).
The companion plan assigns Task 01 to its independent Wave 1 with no
dependencies. It is not a prerequisite of Tasks 06–07, but it is a prerequisite
of Task 05's final gate and overall completion. This plan remains incomplete
until its backend, desktop, mobile, and live-WebSocket identity verification
passes.

---

Tasks 01–04 and their implementation, focused tests, desktop/mobile E2E
coverage, and original verification are complete. Follow-up Tasks 06–07 and
Kanban visibility Task 01 remain pending. Task 05 remains pending for final
repository-wide verification and commit after Tasks 06–07 and Kanban visibility
Task 01. Task 06 covers workspace targeting; Task 07 aligns and verifies
diagnostic attachment and localized toggle copy. Kanban visibility Task 01 owns
AC `.9`. This plan is not complete until all three follow-ups and Task 05's
final gate pass their assigned verification. The PR remains the authoritative
record for the original delivery's final commit and CI results.

---

## Implementation Waves And Parallel Candidates

The default is sequential execution in this primary conversation.

Planning:

- [x] [Task 01: Harness planning gate](task-01-harness-planning-gate.md)

Wave 1:

- [x] [Task 02: Backend issue workflow and bootstrap](task-02-backend-issue-workflow.md)
  — complete; loader, bootstrap wiring, and real SQLite integration coverage pass.

Wave 2:

- [x] [Task 03: Dialog persistence, workflow selection, and mobile entry](task-03-frontend-dialog-and-mobile.md)
  — complete; model tests, responsive entry, and lint/typecheck pass.

Wave 3:

- [x] [Task 04: Desktop and mobile E2E coverage](task-04-e2e-coverage.md)
  — complete; rebuilt desktop and mobile Playwright suites pass.

Wave 4 (follow-up):

- [ ] [Task 06: Workspace target choice E2E coverage](task-06-workspace-target-choice-e2e.md)

Wave 5 (follow-up):

- [ ] [Task 07: Diagnostic bundle attachment](task-07-diagnostic-bundle-attachment.md)

Wave 6:

- [ ] [Task 05: Required verification and commit](task-05-verification-and-commit.md)

No tasks are marked parallel-safe: Tasks 02–04 share the bootstrap contract and
Improve Kandev test fixtures, Task 06 verifies the workspace-choice UI and
backend fallback, Task 07 follows Task 06 because both extend the Improve
Kandev browser E2E file, and Task 05 is final after Tasks 06–07 and companion
Kanban visibility Task 01.
