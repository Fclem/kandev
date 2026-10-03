---
id: "05-verification-and-commit"
title: "Required verification and commit"
status: pending
wave: 6
depends_on:
  - "02-backend-issue-workflow"
  - "03-frontend-dialog-and-mobile"
  - "04-e2e-coverage"
  - "06-workspace-target-choice-e2e"
  - "07-diagnostic-bundle-attachment"
  - "01-show-improve-kandev-cards"
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
  - AC-WORKSPACES-IMPROVE-KANDEV-001.9
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
  - ../../specs/platform/system-design/diagnostic-logging-01.md
  - ../../specs/platform/system-design/diagnostic-logging-02.md
---

# Task 05: Required verification and commit

## Acceptance

- The user-requested repository formatting, typecheck, test, and lint commands
  pass after all feature files and planning statuses are final.
- The companion Kanban visibility Task 01 passes AC `.9` verification before
  this final verification and commit gate.
- The requirement remains `active` as the current product contract, every
  implementation task and plan checkbox is updated accurately, and
  `git diff --check` is clean.
- All scoped changes are committed with a Conventional Commit and normal active
  pre-commit/commit-msg hooks.

## Verification

```bash
make fmt
make typecheck test lint
git diff --check
```

Then follow `.agents/skills/commit/SKILL.md` with explicit `git add` paths and a
message such as:

```text
feat: add issue-only Improve Kandev workflow
```

## Files likely touched

- Files owned by Tasks 01–04, follow-up Tasks 06–07, and companion Kanban Task 01.
- `docs/specs/workspaces/requirements/improve-kandev.md`
- `docs/plans/improve-kandev/plan.md`
- `docs/plans/improve-kandev/task-*.md`
- `docs/plans/improve-kandev-kanban-visibility/plan.md`
- `docs/plans/improve-kandev-kanban-visibility/task-01-show-improve-kandev-cards.md`

Tasks 02–04, Task 06, Task 07, and companion Kanban visibility Task 01. Tasks
06–07 and companion Task 01 must finish first so this final gate can verify and
commit the complete scoped change set.

## Parallelism

Sequential final gate.

## Inputs

- Original Improve phase requirement to run `make fmt` before
  `make typecheck test lint`.
- `.agents/skills/commit/SKILL.md`.
- Completed task verification receipts.

## Risks

- The environment was nearly out of disk space during the planning session.
  Check `df -h .` before dependency installation or a broad build, and clean
  only recoverable generated/cache artifacts if needed.
- Do not bypass hooks. If formatting hooks modify files, review, restage, and
  create a new commit attempt without amending.

## Output contract

Mark every task and plan checkbox accurately, provide the commit hook receipt,
record exact verification results, and stop so the user can move the Kandev
task to the next workflow step.

## Recorded verification

- `make fmt` — passed for the Task 02–04 scope.
- Initial `make typecheck test lint` was stopped by the RTK-injected indexed
  `GIT_CONFIG_*` environment in the unrelated
  `TestCollectAgentEnvPreservesParentIndexedGitConfig` test. With those
  environment variables unset, the exact `make typecheck test lint` command
  passed for the Task 02–04 scope: all backend tests, 922 web test files
  (7,037 passing and 4 skipped), 30 CLI test files, script tests, backend/web/
  harness lint, and typechecks.
- `git diff --check` — passed for the Task 02–04 scope.
- The full verification must be rerun after Tasks 06–07 and companion Kanban
  visibility Task 01; the Conventional Commit and commit-hook receipt remain
  pending.
