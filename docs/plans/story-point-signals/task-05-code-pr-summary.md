---
id: "05-code-pr-summary"
title: "Code and pull-request summary"
status: completed
wave: 3
depends_on:
  - "04-activity-turn-reads"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-EFFORT-SIGNALS-003
  - REQ-PLUGINS-EFFORT-SIGNALS-005
acceptance_criteria:
  - AC-PLUGINS-EFFORT-SIGNALS-003.1
  - AC-PLUGINS-EFFORT-SIGNALS-003.2
  - AC-PLUGINS-EFFORT-SIGNALS-003.3
  - AC-PLUGINS-EFFORT-SIGNALS-003.4
  - AC-PLUGINS-EFFORT-SIGNALS-003.5
  - AC-PLUGINS-EFFORT-SIGNALS-003.6
  - AC-PLUGINS-EFFORT-SIGNALS-005.1
system_design:
  - ../../specs/plugins/system-design/effort-signals-host.md
---

# Task 05: Code and pull-request summary

## Summary

Add changed-file counts to `SessionCodeStats`; creation time, optional changed
files, optional approved-reviewer count, and `SizeObserved` to `TaskPullRequest`;
include linked GitLab merge requests in `Task.PullRequests`; and add the
proto3-optional `PullRequestsPartial` field (`*bool` in the SDK). New Hosts set
false for complete reads and true on any source failure; nil means the Host did
not report completeness.

## In scope

- Analytics model and SQL for `FilesChangedCommitted` (sum of
  `task_session_commits.files_changed`) and `FilesChangedPeakPending` (max
  distinct files across `task_session_git_snapshots`) as optional proto scalars.
  The peak subquery branches by driver in `stats.go`, so the Postgres branch gets
  its own test.
- Proto, SDK, and mapper fields. `ChangedFiles` (`github.TaskPR.ChangedFiles` is
  `*int`) and `ApprovedReviewerCount` (`github.TaskPR.ReviewCount`, a plain int
  of reviewers whose latest review is approved) are optional proto scalars set
  only when `LastSyncedAt` is set. Make `PullRequestsPartial` a proto3-optional
  bool (`*bool` in the SDK); every new Host read sets false for complete source
  coverage or true if any source fails, while the field stays nil when absent
  from an older Host. GitLab follows the field-by-field table in the design:
  `locked` -> `closed`, `ApprovalCount` -> `ApprovedReviewerCount`,
  `ChangedFiles` unset, `Additions`/`Deletions` 0 with `SizeObserved` false.
  No comment count is added.
- GitLab source in the plugin task reader backed by
  `gitlab.Service.ListTaskMRsByTaskIDs`, merged newest first. A failing source
  omits its rows, sets `Task.PullRequestsPartial`, and does not fail the task
  read.
- Check whether sessions that reuse a workspace record the same commit under
  several sessions; record the finding in Results and the doc note.

## Out of scope

- Distinct committed-path counts; comment counts on `TaskPullRequest`.

## Acceptance

- File counts follow `CommittedLinesAvailable`; pre-activation sessions read
  unavailable, not zero.
- A task with GitHub and GitLab links returns both, provider-labelled. If either
  source fails, the task read still succeeds, preserves rows from the successful
  provider, and sets `PullRequestsPartial` true; a table-driven regression test
  covers GitHub-failure and GitLab-failure cases.
- The SDK preserves absent versus explicit false/true for `PullRequestsPartial`;
  old-SDK plugin fixtures still decode.

## Verification

```bash
(cd apps/backend && make proto && go test ./internal/analytics/... ./internal/plugins/ ./pkg/pluginsdk/ -run 'TestSessionCodeStatsFileCounts|TestTaskPullRequestSummaryFields|TestTaskPullRequestsIncludeGitLab|TestTaskPullRequestsPartialByProvider|TestTaskPullRequestsPartialPresence|TestHostSDKProtoParity')
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="${KANDEV_TEST_POSTGRES_DSN:?set a disposable Postgres DSN}" go test ./internal/analytics/repository/sqlite/ -run 'TestPostgresSessionCodeStatsFileCounts')
(cd apps/backend && make fmt lint)
```


## Files likely touched

- `apps/backend/internal/analytics/models/models.go`, `repository/sqlite/stats.go`
- `apps/backend/internal/plugins/host_data_mappers.go`, `host_data.go`
- `apps/backend/pkg/pluginsdk/data_types.go`, proto and generated code

## Dependencies

Task 04 (shared proto and generated files).

## Risks

- Peak-pending semantics must be documented exactly; they cannot change later.
- Postgres tests skip without a DSN; record a skip as not-run.

## Parallelism

`sequential`

## Inputs

- Design section: Code and pull-request summary.
- `ListSessionCodeStats` in `internal/analytics/repository/sqlite/stats.go`;
  `github.TaskPR` and `gitlab.TaskMR` models.

## Results

Completed. `FilesChangedCommitted` sums `task_session_commits.files_changed` per commit and is nil when committed capture is unavailable (pre-activation), with measured zero explicit. `FilesChangedPeakPending` is the max distinct files across the session's git snapshots, branched by driver in `stats.go` with a Postgres branch. Both are optional scalars in the SDK. `TaskPullRequest` adds `CreatedAt`, synced-only `ChangedFiles`/`ApprovedReviewerCount`, and `SizeObserved`. GitLab merge requests merge into `Task.PullRequests` newest first, provider-labelled; each source fails independently, omitting only its own rows and setting `PullRequestsPartial` true, while complete reads set false. The SDK field is `*bool`, so absent, false and true stay distinct. GitLab maps the design table field by field.

Finding: `CreateSessionCommit` deduplicates only on `(session_id, commit_sha)` (`git_snapshots.go:649-667`). The orchestrator attributes commits to the supplied session, and each session's sweep resolves its own base range, so overlapping observations in reused or shared workspaces count the same SHA once per session. Session totals are therefore not repository-wide unique commit totals. The public doc states this; no code change was made.

Verification (parent rerun): `make proto`; `go test -count=1 ./internal/analytics/... ./internal/plugins/ ./pkg/pluginsdk/ -run 'TestSessionCodeStatsFileCounts|TestTaskPullRequestSummaryFields|TestTaskPullRequestsIncludeGitLab|TestTaskPullRequestsPartialByProvider|TestTaskPullRequestsPartialPresence|TestHostSDKProtoParity'` passed. Child reports: `make fmt lint` 0 issues; `go test ./internal/plugins/... ./pkg/pluginsdk/...` passed; public-doc validation 62/62 and 47 pages valid. PostgreSQL `TestPostgresSessionCodeStatsFileCounts` NOT RUN: `KANDEV_TEST_POSTGRES_DSN` unset.
