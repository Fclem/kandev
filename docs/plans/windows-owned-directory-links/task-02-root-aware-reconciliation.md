---
id: "02-root-aware-reconciliation"
title: "Root-aware repository reconciliation"
status: done
wave: 2
depends_on: ["01-directory-link-identity"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 02: Root-aware repository reconciliation

## Acceptance

- A Local task whose workspace root is its primary repository does not create a nested repository
  link.
- A pre-existing exact self-referential platform directory link is removed without modifying the
  repository target.
- A host-materialized task root still receives links for every distinct repository, including the
  first repository spec.

## Verification

```bash
cd apps/backend && go test -run TestReconcileWorkspaceRepositories ./internal/agent/runtime/lifecycle
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go`
- `apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential because it consumes the worktree cleanup helper.

## Inputs

- `docs/specs/tasks/attach-workspace-sources.md`
- `docs/plans/windows-owned-directory-links/plan.md`
- `buildRemoteWorkspaceRepositories` primary-root skip as a behavioral analogue

## Output contract

Report the failing and passing targeted test commands, changed files, blockers and risks, then mark
this task and the plan done.

## Results

- RED: the root-equality and existing-self-link regressions both failed because reconciliation
  left a self-referential entry in place.
- GREEN: all three targeted repository-reconciliation tests passed.
- Full affected packages passed with 1,237 tests across `internal/worktree` and
  `internal/agent/runtime/lifecycle`.
