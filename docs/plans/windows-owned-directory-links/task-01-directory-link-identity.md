---
id: "01-directory-link-identity"
title: "Filesystem identity and safe self-link removal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 01: Filesystem identity and safe self-link removal

## Acceptance

- Re-ensuring an owned platform directory link succeeds when the link and configured target refer
  to the same filesystem directory.
- A mismatched or non-link entry remains untouched and fails closed where applicable.
- Self-link cleanup removes only a platform directory link whose target is the owned root and uses
  non-recursive removal.

## Verification

```bash
cd apps/backend && go test -run 'Test(Ensure|RemoveSelfReferential)' ./internal/worktree
```

## Files likely touched

- `apps/backend/internal/worktree/directory_link.go`
- `apps/backend/internal/worktree/directory_link_test.go`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes the cleanup helper created here.

## Inputs

- `docs/specs/tasks/attach-workspace-sources.md`
- Issue #2005's measured Windows junction behavior
- Existing platform-link detection in `directory_link_unix.go` and `directory_link_windows.go`

## Output contract

Report the failing and passing targeted test commands, changed files, blockers and risks, then mark
this task done and update `plan.md`.

## Results

- RED: the Linux bind-alias regression failed because the existing link was rejected as a target
  mismatch.
- GREEN: all seven `EnsureOwnedDirectoryLink` and `RemoveSelfReferentialDirectoryLink` targeted
  tests passed.
- Windows `amd64` package test binary cross-compilation passed.
