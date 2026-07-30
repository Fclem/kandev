---
status: shipped
created: 2026-07-19
owner: kandev
---

# Workspace Git Status

## Why

Users opening or focusing Changes and Review need a current workspace snapshot without a large generated or untracked tree monopolizing agentctl. Repeated requests for the same repository must not amplify expensive Git and filesystem work, and the initial session-hydration path must remain within its two-second live-status budget by falling back when necessary.

## What

- Cached reads return the latest workspace-tracker snapshot. When no cached snapshot exists, the tracker performs a live observation.
- Fresh reads observe the live worktree and do not themselves replace the polling cache.
- Overlapping live observations for the same repository share one underlying observation. Different repositories in a multi-repository task may still be observed in parallel.
- Every non-cancelled caller receives the same completed snapshot or error from a shared observation. A caller whose own context is cancelled returns promptly without cancelling or otherwise poisoning the result for other callers.
- Tracker shutdown or the bounded shared-observation deadline cancels the underlying work. Cancelled work does not publish or cache a partial snapshot.
- After Git output is parsed, changed-file and synthetic untracked-diff enrichment performs work proportional to the number of changed entries plus the bounded content processed.
- Existing diff limits remain in force: 10 MiB maximum source file size, 256 KiB maximum emitted diff per file, and a 2 MiB enrichment threshold per status snapshot. Because the threshold is checked before enriching each file, the final accepted file may preserve the existing overshoot of up to the 256 KiB per-file cap. Existing skip reasons remain unchanged.
- Large changed sets retain every path and its status metadata. Once the total diff budget is exhausted, files that are not enriched retain `budget_exceeded` as their diff skip reason.
- Multi-repository responses retain repository identity and partial-success behavior.
- Verification tooling preserves shared managed Go and lint caches for reuse while keeping invocation scratch and command output outside repository worktrees. The root-level `.verify-cache` and `.tmp` paths are ignored as safeguards against legacy or misconfigured verification runs.

### Base-commit staleness and refresh

The commits panel (`git log <base>..HEAD`) and cumulative diff anchor to a base commit derived from the session's stored `base_commit_sha` and `base_branch`. That anchor becomes stale when the branch history moves relative to the true integration branch after the base was recorded — most commonly a stacked-PR parent that merges and disappears, a rebase onto the integration branch, or a base branch that was deleted upstream. When the anchor is stale, the panel enumerates commits that are not part of the branch's own contribution, inflating the count.

- A stored base commit SHALL be treated as **stale** when it is a strict ancestor of the true merge-base between `HEAD` and the resolved integration branch — that is, when `merge-base(HEAD, origin/<base_branch>)` (falling back through the integration-branch priority list `origin/main → origin/master`) advances past the stored `base_commit_sha`. A stored base that equals or is a descendant of that merge-base is NOT stale.
- When the stored base is stale, commit enumeration and cumulative diff SHALL use the freshly computed merge-base against the resolved integration branch instead of the stored SHA. The panel therefore reflects only the commits the branch actually introduces over the integration branch.
- Base resolution SHALL prefer the upstream integration ref (`origin/<name>`) over a bare local ref of the same name. A local ref that no longer tracks any live upstream (for example a merged/deleted stacked-parent branch that lingers only as a local ref) SHALL NOT anchor the commit range when an upstream integration ref is available.
- Staleness detection is a read-time correction: it changes which base the commits/diff are computed against. It does not by itself rewrite the persisted `base_commit_sha`; the persisted value continues to follow the existing capture and the "Compare against" base-branch reset paths.
- When neither an upstream integration ref nor a usable merge-base can be resolved (unrelated histories, offline mirror with no `origin/*`), behavior falls back to the existing stored-base / branch-tip anchor and the panel is unchanged from today.

## API surface

No route or payload shape changes.

- `GET /api/v1/git/status?repo=<subpath>&fresh=<bool>` returns the existing `GitStatusResult` shape.
- `GET /api/v1/git/status/multi?fresh=<bool>` returns the existing `MultiRepoGitStatusResult` shape containing `PerRepoGitStatus` entries.
- The `fresh` query parameter continues to select a live observation rather than a cached tracker snapshot.

## Failure modes

| Scenario | Observable behavior |
|---|---|
| Primary branch or porcelain observation fails | The live observation fails and the prior cached snapshot remains available. |
| Secondary diff enrichment fails | The established same-HEAD carry-forward behavior is preserved. |
| One caller cancels while a shared observation is running | That caller returns its context cancellation promptly; other callers remain eligible to receive the shared result. |
| The tracker stops or the shared deadline expires | Underlying work is cancelled and no partial result is published or cached. |
| One repository fails during a multi-repository request | Successful repository entries remain available and the failure is reported on its repository entry. |
| Stored base commit is a strict ancestor of `merge-base(HEAD, origin/<base_branch>)` | Commit enumeration and cumulative diff use the freshly computed merge-base, not the stale stored SHA; the count reflects only the branch's own commits. |
| Resolved base branch exists only as a stale local ref (upstream merged/deleted) but an integration `origin/*` ref is present | The `origin/*` integration ref anchors the range; the stale local ref does not. |
| No `origin/*` integration ref and no usable merge-base (unrelated histories) | Falls back to the existing stored-base / branch-tip anchor; behavior is unchanged from today. |

## Scenarios

- **GIVEN** a stale cached snapshot after a commit, **WHEN** a caller requests `fresh=true`, **THEN** the response reflects the live clean tree and a later cached read still returns the prior cached snapshot.
- **GIVEN** six simultaneous fresh requests for one repository, **WHEN** their observations overlap, **THEN** exactly one underlying status observation runs and all non-cancelled callers receive the same capture timestamp and result.
- **GIVEN** simultaneous fresh requests for two repositories, **WHEN** multi-repository status runs, **THEN** one observation per repository may run in parallel and each response remains identified with its repository.
- **GIVEN** one waiter cancels during a shared observation, **WHEN** other waiters remain, **THEN** the cancelled waiter returns promptly and the remaining waiters receive the completed result.
- **GIVEN** tracker shutdown or the shared-observation deadline while enrichment is running, **WHEN** cancellation reaches the observation, **THEN** filesystem iteration stops and no partial snapshot is cached.
- **GIVEN** approximately 15,000 untracked text files, **WHEN** fresh status is computed, **THEN** every path is present, emitted diff content obeys the existing limits, files not enriched after total-budget exhaustion have `budget_exceeded`, and post-porcelain enrichment remains linear in the number of entries.
- **GIVEN** one invalid repository in a multi-repository request, **WHEN** other repositories succeed, **THEN** the response retains the successful entries and reports the failure only on the invalid repository.
- **GIVEN** verification needs writable scratch space, **WHEN** it selects a location, **THEN** the location is outside every Git worktree and existing shared caches remain reusable; if a legacy run creates root-level `.verify-cache` or `.tmp`, Git status ignores it.
- **GIVEN** a session whose stored `base_commit_sha` is a strict ancestor of `merge-base(HEAD, origin/<base_branch>)` (for example a stacked-PR parent branch that has since merged into the integration branch), **WHEN** the commits panel is requested, **THEN** the enumerated commit count matches `git rev-list --first-parent --count $(git merge-base HEAD origin/<base_branch>)..HEAD` and excludes the commits that already landed on the integration branch.
- **GIVEN** a stored base branch that no longer has an upstream ref (the remote branch was deleted) but a local ref of the same name still points at an old branch point, **WHEN** the commits panel resolves its base, **THEN** it anchors to the merge-base against the `origin/main`/`origin/master` integration ref rather than the stale local ref.
- **GIVEN** a session whose stored `base_commit_sha` equals or is a descendant of the current merge-base against the integration branch, **WHEN** the commits panel is requested, **THEN** the stored base is used unchanged and the count is identical to today's behavior.
- **GIVEN** a worktree with no `origin/*` integration ref and a HEAD sharing no history with any candidate branch, **WHEN** the commits panel resolves its base, **THEN** it falls back to the existing stored-base / branch-tip anchor and does not error.

## Out of scope

- Changing Git-status API routes, response shapes, or frontend rendering.
- Raising or removing existing diff-content limits.
- Changing multi-repository fan-out behavior.
- Making fresh reads owners of the polling cache.
- Replacing Git subprocesses with a native Git implementation.
- Rewriting the persisted `base_commit_sha` as part of staleness detection. The read-time correction changes only which base the commits/diff compute against; persistence continues to follow the existing capture and "Compare against" reset paths.
- Auto-retargeting the session's `base_branch` when a stacked parent merges. Detecting a stale base and picking a live integration ref is in scope; changing the stored base branch is not.

## Implementation plan

See [Workspace Git Status Scalability plan](../../plans/workspace-git-status-scalability/plan.md).
