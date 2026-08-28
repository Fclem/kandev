---
id: "05-safe-atomic-link-replacement"
title: "Safe, atomic owned-link replacement"
status: done
wave: 5
depends_on: ["04-rollback-faithful-repoint"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
parallelism: sequential
---

# Task 05: Safe, atomic owned-link replacement

The Task 02 repoint runs the destructive `removeInspectedDirectoryLink` (`directory_link.go:122`)
**before** `CreateOwnedDirectoryLink` validates root/name containment (`directory_link.go:125`, check
at `:15`). So a traversal or out-of-root name is only rejected after the remove, and a
create-fails-after-remove leaves the entry missing with no restore of the prior target. Validate
containment before any removal and make the replacement safe against create-fails-after-remove.

## Acceptance

- Containment is validated with `isOwnedDirectoryLinkPath` / `ownedDirectoryLinkPath`
  (`directory_link.go:45-67`) **before** `removeInspectedDirectoryLink` runs. A traversal name
  (e.g. `../x`) is rejected before any removal.
- **Unix**: the new link is created at a temp sibling name and `os.Rename`d over the target — a single
  atomic swap that also closes the check→remove→create window. The temp+rename helper lives in
  `directory_link_unix.go` beside the existing `os.Symlink` path.
- **Windows**: junctions cannot atomically overwrite. Before removal, persist a
  verified prior-link backup in the source journal. The exclusive coordinator
  path lease is non-stealable while remove/create runs; takeover cancels/joins the
  effect before recovery, so a stale syscall cannot remove successor state.
- Protected `restore_pending|unsafe` reset claims the exact member under the
  physical admission, transitions it reset-owned, and may restore/retire only
  after exact member/attachment CAS, no references, and owned root empty.
  Nonrecoverable prior absence needs explicit authorization; otherwise retain
  unsafe. Reset-after-restart, pause-before-remove/recreate, crash, stale owner,
  archive/delete blocking schedules pin it.
- `CreateOwnedDirectoryLink`'s canonicalize / `mkdirOwned` / `verifyCreatedOwnedDirectoryLink` path
  is unchanged for the genuine create-new case.

## Verification

```bash
cd apps/backend
go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/...
golangci-lint run ./internal/worktree/... --timeout=5m
```
(Required `backend-tests` Windows gate probes `New-Item -ItemType Junction` in a
junction-capable temp root and runs `go test ./internal/worktree/...`, including
interrupted replacement/recovery; missing junction support fails the gate.)

## Files likely touched

- `apps/backend/internal/worktree/directory_link.go` (reorder containment ahead of the destructive
  remove; call into the platform atomic-replace helper)
- `apps/backend/internal/worktree/directory_link_unix.go` (temp-sibling + `os.Rename` helper)
- `apps/backend/internal/worktree/directory_link_windows.go` (remove/create plus durable restore)
- `apps/backend/internal/worktree/directory_link_test.go`: traversal-before-remove, original-link
  restoration, swap race, crash-after-remove, repeated failure, vanished target,
  stale takeover, admission retention, and restart each hold `restore_pending`
  until verified restoration.

## Dependencies

Task 04 (rollback contract); the Windows restore path reuses the `PriorTarget` from the shared result
struct introduced in Task 03.

## Parallelism

`sequential`. Consumes the Task 03 signature and the Task 04 undo semantics.

## Inputs

- Spec: the safe-replacement failure-mode row and scenario — never deletes a non-owned/changed entry
  and never leaves the entry missing on a failed recreate.
- Plan: PR #2253 review remediation — Finding 3.
- Platform helpers `isPlatformDirectoryLink`, `createPlatformDirectoryLink`,
  `requirePlatformDirectoryLink` in `directory_link_unix.go` / `directory_link_windows.go`; the atomic
  temp+rename pattern reference at `apps/backend/internal/agent/usage/fileutil.go:11-33`.

## Output contract

Summary, files changed, tests run (traversal-before-remove, recreate-failure-restores, swap-race),
blockers, risks, and task/plan status updates in the same conversation. Reconcile **Files likely
touched** with the actual diff before marking done.

## Results
> Historical completion record. It predates the later durable Windows
> `restore_pending|unsafe` protocol above and is not evidence that those new
> acceptance criteria are implemented; the updated work order and Windows CI
> verification are authoritative for the next implementation.


- `EnsureOwnedDirectoryLink` now resolves the owned entry path through `ownedDirectoryLinkPath(root, name)` before any filesystem mutation, so traversal and out-of-root names fail before a repoint can touch an existing entry.
- The repoint path now canonicalizes the replacement target before acting, then delegates to platform-specific replacement helpers:
  - Unix: `replacePlatformDirectoryLink` stages a sibling temp link and renames it over the target path.
  - Windows: `replacePlatformDirectoryLink` keeps remove-then-create, but restores the prior target if create fails after removal.
- Added the shared `revalidateInspectedDirectoryLink` / `renameInspectedDirectoryLink` guards so the changed-entry race still fails closed before a rename-over replacement.
- Tests:
  - Added `TestEnsureOwnedDirectoryLinkRejectsTraversalBeforeRepoint`.
  - Added `TestEnsureOwnedDirectoryLinkKeepsOriginalLinkWhenReplacementTargetIsInvalid`.
  - Added `TestRenameInspectedDirectoryLinkRejectsChangedEntry`.
- Commands:
  - `cd apps/backend && gofmt -w internal/worktree/directory_link.go internal/worktree/directory_link_unix.go internal/worktree/directory_link_windows.go internal/worktree/directory_link_test.go`
  - `cd apps/backend && go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/...` → all `ok`.
  - `cd apps/backend && base=$(git merge-base HEAD origin/main) && golangci-lint run ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/... --new-from-rev="$base" --timeout=5m` → `0 issues`.
- External side-effect boundary: on Unix the repoint stages a temp sibling link and renames it over the owned entry; on Windows it may remove and recreate the owned entry but restores the prior target on failure. In both cases the mutation stays confined to Kandev-owned task-root entries.
