---
id: "04-rollback-faithful-repoint"
title: "Rollback-faithful owned-link repoint"
status: done
wave: 4
depends_on: ["03-ownership-aware-repoint"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
parallelism: sequential
---

# Task 04: Rollback-faithful owned-link repoint

Since Task 02, `EnsureOwnedDirectoryLink` returns `created=true` for a **repointed** pre-existing link,
not only a brand-new one. `materializeDirectoryLinks` and `materializeWorktreeSources` append such
entries to the `created []string` slice, and `rollbackHostWorkspaceMaterialization` does
`os.Remove(created[index])` on each. So a later failed multi-source attachment **deletes** a link that
existed before this attachment instead of restoring its prior target — breaking the spec's atomicity
guarantee. Make the materialization undo contract distinguish a repoint from a create and carry the
prior target so rollback restores it.

## Acceptance

- Undo records carry path/prior target/key/effect generation plus full marker
  identity, directory fence, environment/session projection, and owner epochs.
- Every restore/delete reacquires workspace/resource leases and fresh
  compensation capability. Exact authorization restores/deletes; CAS or marker
  equality loss leaves successor state untouched and journal recoverable.
- Marker initialization uses its directory member with absent before/full
  intended tuple and the same compensation path.
- Ready reconcile is validation-only and has no rollback slice.

## Verification

```bash
cd apps/backend
go test ./internal/backendapp/... ./internal/worktree/...
golangci-lint run ./internal/backendapp/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/backendapp/workspace_source_materializer.go` (undo-record type;
  `hostWorkspaceMaterialization` field; `materializeDirectoryLinks`, `materializeWorktreeSources`,
  `rollbackHostWorkspaceMaterialization`)
- `apps/backend/internal/backendapp/workspace_source_materializer_test.go`:
  - new/updated: repoint a pre-existing link, force a later failure at one of the rollback trigger
    points (`:134`/`:139`/`:145`; deferred rollback `:124-131`), assert the **original** target is
    restored (not deleted) and a genuinely created-new entry is still deleted

## Dependencies

Task 03 (defines the `EnsureOwnedDirectoryLink` result struct with `PriorTarget`).

## Parallelism

`sequential`. Consumes the Task 03 signature.

## Inputs

- Spec: strengthened atomicity behavior (`docs/specs/tasks/system-design/attach-workspace-sources.md:40-43`) and the
  failure-mode row for restoring a repointed pre-existing link on a failed submission. Cancel/intact
  guarantees at `:51` and `:62`.
- Plan: PR #2253 review remediation — Finding 2 (confirmed, most serious).
- Trigger points: rescan/persist/adopt at `workspace_source_materializer.go:134,139,145`; deferred
  rollback at `:124-131`.

## Output contract

Summary, files changed, tests run (including the restore-not-delete regression), blockers, risks, and
task/plan status updates in the same conversation. Reconcile **Files likely touched** with the actual
diff before marking done.

## Results

- Historical undo records distinguished create from repoint and restored prior
  targets. Current workspace-reuse supersedes the authorization boundary:
  rollback/`RestoreOwnedDirectoryLink` must take a freshly minted compensation
  capability under both leases; stale epoch/key/revision performs no physical
  change. Existing restore safety tests remain, plus takeover/newer-mutation
  tests prove stale rollback cannot overwrite the successor.
