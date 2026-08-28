---
id: "03-ownership-aware-repoint"
title: "Ownership-aware owned-link repoint"
status: done
wave: 3
depends_on: ["02-owned-link-self-heal"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
parallelism: sequential
---

# Task 03: Ownership-aware owned-link repoint

Task 02's repoint receives no task identity and never reads the ownership marker before removing and
recreating a Kandev-owned link, so on a shared legacy task root the task that reconciles last can
repoint another task's live workspace entry to its own repository. Thread the current task identity
into the reconcile and materialize paths and only allow a repoint when the owned root's
`.kandev-workspace.json` marker names **this** task; otherwise fail closed and leave the other task's
entry intact.

This is the **single-signature-change** task. Design the new `EnsureOwnedDirectoryLink` contract once
here — an ownership descriptor input plus a result struct — and have Task 04 and Task 05 build on it.

Current workspace-reuse scope supersedes the original ready-reconcile caller:
the ownership-aware primitive remains valid, but only creating materialization
or explicit journaled source mutation may invoke it. Ready reuse is validation
only and returns unsafe/reset on target mismatch.

## Acceptance

- `ValidateOwnedDirectoryLink` is side-effect-free. `EnsureOwnedDirectoryLink`
  takes ownership plus workspace-mutation capability and returns
  `{Path,Created,PriorTarget}`.
- Repoint requires marker task/workspace/task-dir/layout identity plus exact
  directory fence and environment projection generation, then revalidates all
  capability fields.
- Only initial creating journals absent-marker initialization; ready/source
  paths treat absent or stale marker generation as unsafe.
- Creating/explicit source uses Ensure; ready lifecycle validates.

## Verification

```bash
cd apps/backend
go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/...
golangci-lint run ./internal/worktree/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/worktree/directory_link.go`: separate Validate/Ensure;
  Ensure requires mutation capability.
- `apps/backend/internal/worktree/directory_link_test.go`: validation no-op,
  capability/key/epoch rejection, marker ownership, authorized repoint.
- lifecycle reconcile/manager launch/execution: ready paths call Validate only.
- backend workspace source materializer: creating/source paths call Ensure with
  the exact capability and propagate rollback result.

## Dependencies

Task 02 (owned-link self-heal) lands first; this task makes that repoint ownership-aware.

## Parallelism

`sequential`. It defines the shared `EnsureOwnedDirectoryLink` signature that Tasks 04 and 05 consume.

## Inputs

- Spec: the failure-mode row and scenario for an ownership-marker-mismatch repoint that fails closed
  and leaves the other task's entry intact.
- Plan: PR #2253 review remediation — Finding 1 and the single-signature-change decision.
- `WriteOwnershipMarker` / `existingMarkerMatches` / `ReadOwnershipMarker`
  (`apps/backend/internal/system/storage/workspaces/marker.go:17-62,79-91,120-122`) and
  `OwnershipMarker` (`.../workspaces/types.go:23-28`). Marker is written only in
  `prepareTaskWorktreePath` (`apps/backend/internal/worktree/manager_lifecycle.go:324-329`) and for
  scratch workspaces (`manager_launch.go:418-421`); a plain local-executor reconcile root may lack
  one, which is why absent → allowed.

## Output contract

Summary, files changed, tests run (including the marker-mismatch fail-closed regression), blockers,
risks, and task/plan status updates in the same conversation. Reconcile **Files likely touched** with
the actual diff before marking done.

## Results and superseding contract

Historical Task 03 allowed absent markers and threaded the mutator through
lifecycle/materializer callers. Current workspace reuse supersedes both:
lifecycle/manager ready callers move to `ValidateOwnedDirectoryLink`; authorized
materializers use capability-gated Ensure; only initial creating may initialize
an absent marker. Prior marker-conflict/rollback tests remain, with new absent-
marker and stale/wrong-key capability coverage.
