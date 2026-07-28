---
spec: docs/specs/tasks/attach-workspace-sources.md
created: 2026-07-28
status: done
issue: https://github.com/kdlbs/kandev/issues/2005
---

# Implementation Plan: Windows Owned Directory Link Reconciliation

## Overview

Local launch and resume currently reconcile the primary repository beneath a workspace root that
may be that same repository, creating a self-referential link. Existing Windows junctions are then
rejected because `filepath.EvalSymlinks` does not follow mount-point reparse points. The fix first
makes owned-link matching and cleanup use filesystem identity, then makes repository reconciliation
skip and self-heal only a source that is the workspace root.

## Confirmed root cause

- `launchResolveWorkspacePath` uses the primary repository as the Local workspace when no promoted
  task root exists.
- `reconcileWorkspaceRepositories` attempts to materialize every repository spec, including that
  primary repository, beneath the workspace.
- `EnsureOwnedDirectoryLink` compares `filepath.EvalSymlinks` strings. On Windows, a directory
  junction's evaluated path remains the junction path, so an unchanged junction mismatches its
  target.

## Backend

### Owned directory-link identity

Update `apps/backend/internal/worktree/directory_link.go` so an existing platform directory link is
matched with `os.Stat` plus `os.SameFile`. Add a narrowly scoped removal helper that validates the
owned entry, confirms it is a platform directory link whose target is the root by filesystem
identity, and removes it with `os.Remove` only.

### Local repository reconciliation

Update
`apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go` to compare each valid
repository target with the workspace root by filesystem identity. When they match, remove an
existing exact self-reference through the worktree helper and skip materialization. Preserve every
repository link when the workspace is a distinct Kandev-owned task root.

## Tests

- **What:** matching owned links are idempotent, mismatched links fail closed, and only an exact
  self-reference is removed.
  **File:** `apps/backend/internal/worktree/directory_link_test.go`.
  **How:** filesystem unit tests using real temporary directories and platform directory links.
- **What:** a repository identical to the workspace root is skipped and a pre-existing self-link is
  healed, while a distinct primary source remains linked under a promoted task root.
  **File:**
  `apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile_test.go`.
  **How:** lifecycle package tests using real temporary directories.

No Playwright test is planned because this changes backend launch/resume filesystem behavior and
does not change a browser flow or UI contract.

## Implementation Waves And Parallel Candidates

Sequential execution in the primary conversation:

- [x] [Task 01: filesystem identity and safe self-link removal](task-01-directory-link-identity.md)
- [x] [Task 02: root-aware repository reconciliation](task-02-root-aware-reconciliation.md)

## Risks and out of scope

- Removal must fail closed for real directories, files, non-directory links, and links to any
  directory other than the workspace root.
- Multi-repository Local tasks already promoted to a Kandev-owned task root must retain every
  source link, including the first repository.
- The issue's unconfirmed attached-folder root-equality path and unmaterialized multi-repository
  Local layout are out of scope.

## Verification results

- `make fmt` — passed.
- `make typecheck` — passed.
- `make test` — passed, including backend, web, CLI, scripts, and desktop smoke coverage.
- `make lint` — passed.
- Windows `amd64` test binaries for `internal/worktree` and
  `internal/agent/runtime/lifecycle` — cross-compiled successfully.
