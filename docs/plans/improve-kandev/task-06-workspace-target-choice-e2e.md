---
id: "06-workspace-target-choice-e2e"
title: "Verify Improve Kandev workspace choice"
status: pending
wave: 4
depends_on: ["03-frontend-dialog-and-mobile", "04-e2e-coverage"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-KANDEV-001.7
  - AC-WORKSPACES-IMPROVE-KANDEV-001.8
  - AC-WORKSPACES-IMPROVE-KANDEV-001.10
  - AC-WORKSPACES-IMPROVE-KANDEV-001.11
  - AC-WORKSPACES-IMPROVE-KANDEV-001.12
  - AC-WORKSPACES-IMPROVE-KANDEV-001.13
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
---

# Task 06: Verify Improve Kandev workspace choice

## Summary

Add deterministic coverage for workspace choices and first-create GitHub
connection inheritance. The checked browser path starts at the first-run
intro; the fallback path starts with the intro preference already dismissed
and proves the separate workspace-choice panel gates bootstrap. Backend
integration tests exercise actual creation, reuse, fallback, and workspace
configuration isolation without cloning a remote repository.

## In scope

- Add desktop E2E scenarios titled `checked workspace choice places the task in the bootstrap target` and `declining dedicated workspace creation keeps the task in the active workspace`.
- In the checked-path E2E, start with no dedicated workspace and the intro visible; assert the workspace checkbox is checked by default, leave it checked, and continue. Observe `create_workspace: true`; seed a repository plus the `improve-kandev` and `report-kandev-issue` workflow instances in an isolated non-active workspace; return their IDs with that workspace ID from the controlled bootstrap handler; verify the task belongs to the response workspace.
- In the unchecked-path E2E, start with no dedicated workspace and preseed the intro-dismissal preference. Assert the workspace-choice panel appears and bootstrap has not run; observe the checkbox checked by default, uncheck it, then continue. Assert `create_workspace: false` and request `workspace_id` equals the active workspace ID; return that workspace together with its repository and both workspace-local workflow IDs from the controlled bootstrap handler; verify the task belongs to the active workspace.
- Add backend integration coverage for first-create connection inheritance using persisted connection state: the resolved default workspace's GitHub connection and applicable PAT are copied; an absent source connection leaves the new workspace unconnected; unrelated integrations, automations, workflows, and repositories are not copied beyond bootstrap defaults; and a repeated bootstrap leaves an existing dedicated workspace's configuration unchanged.
- Cover source-resolution priority in `workspacescope` tests: a valid active
  workspace setting wins over an earlier-created workspace; missing or stale
  active settings fall back to the earliest-created workspace; with no workspace
  rows, resolution returns the literal `default`.
- In bootstrap integration tests, seed distinct GitHub connection identities
  and PATs on the candidate sources and assert the created dedicated workspace
  receives the identity and secret from the resolved source.
- Keep the selected-workspace GitHub probe test separate from connection-copy
  tests; its active-versus-dedicated result assertions remain required.

## Out of scope

- Changing production workspace-choice or bootstrap behavior.
- Reworking the completed desktop/mobile scenarios in Task 04.

## Acceptance

- With no workspace named `Improve Kandev` and no saved intro preference, the checked-path E2E observes the intro's checkbox checked by default, leaves it checked, asserts `create_workspace: true`, and verifies the task uses the seeded non-active workspace and matching repository/workflow IDs returned by the controlled bootstrap response.
- With no workspace named `Improve Kandev` and the intro-dismissal preference saved, the fallback E2E observes the separate workspace-choice panel and checked checkbox, proves bootstrap is deferred until choice confirmation, unchecks it, asserts `create_workspace: false` and request `workspace_id` equals the active workspace ID, and verifies the task uses the active workspace and matching repository/workflow IDs returned by the controlled bootstrap response.
- Backend tests verify resolver precedence and source-specific connection/PAT inheritance, no connection when the resolved source is unconfigured, no copying of unrelated workspace configuration, and no mutation or recopy on dedicated-workspace reuse.

## Verification

The managed E2E runner rebuilds backend, E2E web, and plugin artifacts before Playwright; do not add `--no-build`.

```bash
(cd apps/backend && go test ./internal/integration -run 'TestImproveKandevBootstrap(CreatesBothHiddenWorkflowsIdempotently|ReusesExistingImproveWorkspace|FallsBackToRequestedWorkspaceWhenCreationDeclined|UsesResolvedWorkspaceGitHubAccess|CopiesGitHubConnectionOnWorkspaceCreation|DoesNotCopyWhenSourceHasNoGitHubConnection|PreservesExistingWorkspaceConfiguration|CopiesOnlyGitHubConfiguration)' -count=1)
(cd apps/backend && go test ./internal/github -run TestCopyWorkspaceConnectionToWorkspace -count=1)
(cd apps/backend && go test ./internal/integrations/workspacescope -run TestDefaultResolver -count=1)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/improve-kandev.spec.ts -- --grep "checked workspace choice places the task in the bootstrap target|declining dedicated workspace creation keeps the task in the active workspace")
```


## Files likely touched

- `apps/backend/internal/integration/improve_kandev_test.go`
- `apps/web/e2e/tests/improve-kandev.spec.ts`
- `apps/backend/internal/integrations/workspacescope/workspacescope_test.go`

## Dependencies

Tasks 03 and 04.

## Risks

- The worker-scoped backend persists workspaces between tests; setup must remove prior E2E-owned rows named `Improve Kandev`, then teardown must delete the scenario's task and non-active test workspace without mutating the active seed workspace.

## Parallelism

`sequential`

## Inputs

- `docs/specs/workspaces/requirements/improve-kandev.md`, AC `.7` and `.8`.
- `docs/specs/workspaces/system-design/improve-kandev.md`, workspace creation semantics.
- Existing bootstrap integration tests and `improve-kandev.spec.ts` fixtures.

## Results

Pending implementation.
