---
id: "05-verification-and-commit"
title: "Required verification and commit"
status: in_progress
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

- The original Task 02–04 gate passed after unsetting injected indexed
  `GIT_CONFIG_*` variables: backend tests, 922 web test files (7,037 passing,
  4 skipped), 30 CLI test files, script tests, lint, and typechecks. This is
  historical verification, not the final package gate.
- Final `make fmt` passed after all implementation and lint-fix changes, using
  `/var/tmp/kandev-go-cache` because the default Go cache was full.
- Final `make typecheck test lint` used `GOCACHE=/var/tmp/kandev-go-cache`,
  `TMPDIR=/var/tmp`, `GOTMPDIR=/var/tmp`, `GOFLAGS=-p=2`, and `GOMAXPROCS=4`;
  `CI=true`, Node `v22.22.3`, npm `10.9.8`. It unset `GIT_CONFIG_COUNT`,
  `GIT_CONFIG_KEY_0`, `GIT_CONFIG_VALUE_0`, and every inherited `KANDEV_*`
  variable listed by the session environment: `KANDEV_AGENT_PROFILE_ID`,
  `KANDEV_AGENT_STANDALONE_PORT`, `KANDEV_BACKEND_PORT`, `KANDEV_BUNDLE_DIR`,
  `KANDEV_CONSOLE_LOG_LEVEL`, `KANDEV_DEBUG_AGENT_MESSAGES`,
  `KANDEV_DEBUG_PPROF_ENABLED`, `KANDEV_DESKTOP_HEALTH_TOKEN`,
  `KANDEV_EXECUTION_PROFILE_ID`, `KANDEV_FEATURES_AGENT_SURVIVAL`,
  `KANDEV_FEATURES_AUTH`, `KANDEV_FEATURES_CANVASES`,
  `KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF`,
  `KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING`,
  `KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING`,
  `KANDEV_FEATURES_LSP_BROWSER_CONTINUITY`,
  `KANDEV_FEATURES_MULTI_TENANCY`, `KANDEV_FEATURES_NEEDS_YOU_INBOX`,
  `KANDEV_FEATURES_OFFICE`, `KANDEV_GITLAB_HOST`, `KANDEV_HOME_DIR`,
  `KANDEV_INSTALL_KIND`, `KANDEV_INSTANCE_ID`,
  `KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG`, `KANDEV_LOG_LEVEL`,
  `KANDEV_RESTART_ADAPTER`, `KANDEV_RUNNING_AS_SERVICE`,
  `KANDEV_SERVER_HOST`, `KANDEV_SERVER_PORT`, `KANDEV_SERVICE_MANAGER`,
  `KANDEV_SERVICE_METADATA`, `KANDEV_SERVICE_MODE`, `KANDEV_SESSION_ID`,
  `KANDEV_SUPERVISOR_MANIFEST`, `KANDEV_SUPERVISOR_SOCKET`, `KANDEV_TASK_ID`,
  `KANDEV_TRUSTED_PROXIES`, and `KANDEV_VERSION`.
- Exact remaining backend failures:
  - `TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc`:
    `managed_npm_runtime_test.go:250: workspace registry = "https://registry.npmjs.org/", want configured registry`.
    Reproduced alone under Node `v22.22.3` / npm `10.9.8`.
  - `TestProcessLifecycle_StartListGetCapturesOutput`:
    `processes_test.go:188: get process = 404, want 200 — the process was retired before its output could be read (body {"error":"process not found"})`.
  - `TestHandleGetProcess_OmitsOutputByDefault`:
    `processes_test.go:206: get process = 404, want 200 — the process was retired before its output could be read (body {"error":"process not found"})`.
  - `TestProcessRunnerCapturesOutput`: `runner_test.go:102: process output not captured in time`.
  - `TestProcessRunnerStopLogsSignalAttempts`: `runner_test.go:145: signal-ignoring fixture did not become ready`.
- The implementation commit changes no paths in
  `internal/agent/agents`, `internal/agentctl/server/api`, or
  `internal/agentctl/server/process` (checked with `git diff --name-only`
  against `224cdc4dd`). The npm assertion is in the untouched agent package;
  the other four failures are in untouched API/process packages. Their failure
  messages do not identify a task-scoped correction. The process timing cause
  remains unverified; do not weaken or exclude these tests.
- Separate final `make lint` passed (backend, web, harness, specs, architecture)
  after the lint findings introduced by the workflow payload edits were fixed.
- Focused DTO, workflow-event, boot-state, and E2E-fixture regression tests
  passed after the final backend lint edits.
- Commit `6e64c3808b7cd09d87ecb5165295692a4f4182e5` (`feat: complete Improve
  Kandev workspace flows`) passed the active pre-commit and commit-msg hooks;
  the resulting worktree was clean.
- Task 05 remains in progress because the final package-wide test gate failed
  in the existing backend tests listed above. The commit does not claim that
  gate passed.
