---
spec: docs/specs/platform/workspace-git-status.md
created: 2026-07-30
status: implemented
---

# Implementation Plan: Git Stale-Base Refresh (inflated commits panel)

## Overview

The Changes panel's COMMITS count is computed by agentctl as `git log <base>..HEAD`,
where `<base>` comes from the session's stored `base_branch` via
`computeMergeBase(HEAD, <target_branch>)`. When the stored target branch is a stacked-PR
**parent** whose upstream ref has been merged/deleted, `computeMergeBase` falls back to a
**stale local ref** and returns an old branch point, sweeping in commits that already
landed on the integration branch. The fix makes base resolution detect that the resolved
merge-base is a strict ancestor of the integration merge-base (`origin/main` → `origin/master`)
and, when so, anchor to the integration merge-base instead. This is a read-time correction in
agentctl; no persistence, route, or payload changes.

**Confirmed root cause.** For the reported session, stored `base_branch` =
`feature/CLIP-8304-rust-mock-server-adopt` (a merged/deleted stacked parent). Its `origin/`
ref is gone, so `computeMergeBase` fell back to the bare local ref → `ed630b8446` → 31 commits.
The true `merge-base(HEAD, origin/master)` is `19646efc83` → 1 commit (matches the PR and
`git rev-list --first-parent --count`).

---

## Backend

### Area 1 — Stale-base detection in the commits/diff base resolver

`apps/backend/internal/agentctl/server/process/git_log.go`

- Add `GitOperator.IsAncestor(ctx, ancestor, descendant string) (bool, error)` wrapping
  `git merge-base --is-ancestor <ancestor> <descendant>` (exit 0 = true, exit 1 = false,
  other = error). Reuse the existing `runGitCommand` plumbing; classify exit status like the
  other `GitOperator` helpers.

`apps/backend/internal/agentctl/server/api/git.go`

- Add `integrationMergeBaseCandidates = []string{"main", "master"}` (bare names;
  `computeMergeBase` already tries `origin/<name>` first). Mirror the existing
  `branchDiffCandidates` priority in `process/workspace_git_status.go` so the commits panel and
  the task-card stats land on the same anchor.
- Add `Server.integrationMergeBase(ctx, gitOp) (string, error)`: walks the candidates and
  returns the first successful `computeMergeBase(HEAD, <candidate>)`.
- In `runGitLogForRepo`, after resolving `baseCommit` from the target-branch merge-base:
  compute the integration merge-base; if it is non-empty and the resolved `baseCommit`
  `IsAncestor` of it (strict — not equal), replace `baseCommit` with the integration merge-base.
  Skip the correction when the target branch already IS an integration candidate (avoid a
  redundant self-compare), when no integration merge-base resolves, or when `IsAncestor` errors
  (fall back to current behavior). Keep the existing sanitiser barriers inline.
- Keep the function within the golangci limits (≤80 lines / ≤50 statements / complexity ≤15);
  extract the correction into `Server.correctStaleBase(ctx, gitOp, baseCommit, targetBranch)`
  helper if `runGitLogForRepo` grows past the limit.

### Area 2 — Cumulative diff parity (verify only, extend if needed)

`apps/backend/internal/agentctl/server/api/git.go` (cumulative-diff handler) and
`apps/backend/internal/agentctl/server/process/workspace_git_status.go`
(`resolveBaseBranch`/`computeBaseCommit`).

- The spec requires the cumulative diff to use the same corrected base as the commits panel.
  Confirm whether the live cumulative-diff path shares `runGitLogForRepo`'s base resolution; if
  it computes its own base, apply the same strict-ancestor correction there. Do NOT change the
  orchestrator snapshot path (`lifecycleAdapter.GetCumulativeDiff`), which intentionally anchors
  to a caller-provided SHA (see comment in `backendapp/adapters.go`).

---

## Frontend

> No user-facing changes. The panel already renders whatever count agentctl returns; the fix
> only changes the computed base. No component, API client, or store changes.

---

## Tests

- **Stale stacked-parent base is corrected to integration merge-base** (maps to spec scenario 1).
  File: `apps/backend/internal/agentctl/server/api/git_log_merge_base_test.go`.
  How: extend with `setupAPITestRepo`; build integration `main`/`origin/main`, a parent branch
  branched off an old `main`, a feature branch off the parent, then advance+merge the parent
  into `origin/main` and delete the parent's `origin/` ref. Assert the resolved base equals
  `merge-base(HEAD, origin/main)` and that `git log <base>..HEAD --first-parent` returns only the
  feature commits. This is the regression test — it MUST fail before the code change.
- **Stale local ref, upstream integration ref present** (spec scenario 2). Same file; assert the
  `origin/main` merge-base wins over the stale local parent ref.
- **Base already current is unchanged** (spec scenario 3). Same file; a base equal to / descendant
  of the integration merge-base is returned unchanged (no over-correction).
- **No origin, unrelated history falls back** (spec scenario 4). Same file; assert no error and the
  existing stored-base / branch-tip fallback path is taken.
- **`GitOperator.IsAncestor` unit test.** File:
  `apps/backend/internal/agentctl/server/process/git_log_test.go`. Table-driven: ancestor,
  descendant, equal, and unrelated cases via `setupTestRepo`.

Targeted commands:
```
cd apps/backend && go test -run 'TestComputeMergeBase|TestRunGitLogForRepo|TestGetLog|IsAncestor' ./internal/agentctl/server/api/... ./internal/agentctl/server/process/...
```

---

## E2E Tests

> Skipped: zero user-visible UI changes. The behavior is verified at the agentctl git layer,
> which E2E cannot deterministically drive (requires a merged/deleted stacked-parent upstream).

---

## Implementation Waves And Parallel Candidates

Small fix (2 tasks). Sequential by default in the primary conversation.

```
Wave 1:
- [x] [task-01-git-operator-is-ancestor](task-01-git-operator-is-ancestor.md)

Wave 2:
- [x] [task-02-stale-base-correction](task-02-stale-base-correction.md)
```

Task 02 depends on task 01 (`IsAncestor` helper). Not parallel-safe: both touch the same
agentctl git area and task 02 imports the task 01 helper.

---

## Open Questions
- RESOLVED (Area 2): the live cumulative-diff handler `runGitCumulativeDiffForRepo` computes its
  own base via the shared `computeMergeBase` (a separate call site from `runGitLogForRepo`, not a
  shared resolver). Task 02 applied the same `correctStaleBase` call in both places, so the
  commits panel and the cumulative diff re-anchor identically. The orchestrator snapshot path
  (`lifecycleAdapter.GetCumulativeDiff`) was left untouched — it anchors to a caller-provided SHA
  by design.
