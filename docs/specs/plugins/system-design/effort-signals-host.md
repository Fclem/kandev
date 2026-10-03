---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-EFFORT-SIGNALS-001
  - REQ-PLUGINS-EFFORT-SIGNALS-002
  - REQ-PLUGINS-EFFORT-SIGNALS-003
  - REQ-PLUGINS-EFFORT-SIGNALS-004
  - REQ-PLUGINS-EFFORT-SIGNALS-005
created: 2026-10-03
owners:
  - kandev
---

# Effort Signals Host System Design

## Purpose and boundaries

This design adds the Host data API surface a story-point estimator needs and
describes that estimator as a reference consumer. Core owns the reads,
authorization, and bounds. The estimator is a managed Go plugin with a native
UI bundle in its own repository (`kdlbs/kandev-plugin-story-points`, created
under the plugin authoring guide); this design does not ship it.

The [session user-activity design](../../tasks/system-design/session-user-activity.md)
owns capture and storage. [ADR 0043](../../../decisions/0043-plugin-host-data-api.md)
sets the additive-only DTO rule; [ADR 0047](../../../decisions/0047-plugin-host-conversation-reads.md)
sets the sanitized-message precedent.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-EFFORT-SIGNALS-001` | [Prompt origin](#prompt-origin) |
| `REQ-PLUGINS-EFFORT-SIGNALS-002` | [Session activity and turns](#session-activity-and-turns) |
| `REQ-PLUGINS-EFFORT-SIGNALS-003` | [Code and pull-request summary](#code-and-pull-request-summary) |
| `REQ-PLUGINS-EFFORT-SIGNALS-004` | [Change-request feedback](#change-request-feedback) |
| `REQ-PLUGINS-EFFORT-SIGNALS-005` | [Compatibility](#compatibility-and-authorization), [Reference plugin](#reference-plugin-design) |

## Gap inventory

| Signal | Today | Change |
| --- | --- | --- |
| Human prompt vs generated | `Message.AuthorType` is `user`/`agent` only (`pkg/pluginsdk/data_types.go`); workflow, automation, peer-task, and plugin prompts are all stored as `user` | add `Origin` |
| User active time | none | session activity rollup |
| Turn timing | not exposed to plugins | `Sessions().Turns` |
| Wall time | `Task.CreatedAt/StartedAt/CompletedAt`, session `StartedAt/EndedAt` | none; plugin derives |
| Lines changed | `Sessions().CodeStats` | none |
| Files changed | absent from `SessionCodeStats` | add counts |
| PR count and size | `Task.PullRequests` is GitHub-only (additions, deletions) | add GitLab MRs, created time, optional files and reviews |
| PR messages | absent; live-fetched by GitHub and GitLab services only | new exact read |
| Task cost/tokens | `ListTaskUsageExact` | none |
| Estimate storage | plugin `GetState/SetState` | none |

## Prompt origin

`messageModelToDTO` (`internal/plugins/host_data_mappers.go`) calls
`models.ClassifyMessageOrigin(m)`, a pure function over the stored metadata and
its `MessageOrigin` constants, which live in `internal/task/models` so the
orchestrator tests can import them (the orchestrator and plugins packages do not
import each other). First
match wins. Workflow step prompts and PR/MR automation both carry `auto_start`
and `workflow_message` (`workflowMessageMetadata`,
`event_handlers_workflow.go`, and `WithAutoStart(true).WithWorkflowStep(...)`),
so the explicit automation `origin` values are tested before `workflow_message`,
and a bare `auto_start` is tested last:

| Order | Marker in stored metadata | Origin | Creator |
| --- | --- | --- | --- |
| 1 | `sender_task_id` | `peer_task` | `UserMessageMeta.WithSenderTask` (`message_task_kandev`) |
| 2 | `source` with prefix `plugin:` | `plugin` | `pluginsTaskMessengerAdapter` (`internal/backendapp/adapters_plugin_messenger.go`) |
| 3 | `origin` of `github_pr_automation`, `ci_automation`, or the GitLab `mrAutomationOrigin` | `automation` | PR, CI, and MR automation |
| 4 | `workflow_message` | `workflow` | `workflowMessageMetadata` (workflow step prompts) |
| 5 | `auto_start` | `automation` | other triggers using `WithAutoStart` (`event_handlers_automation.go`); `recordInitialMessage` (`task_operations.go`) writes it on the initial message of an auto-started task, whose text may be a person's task description, so it counts as `automation` and undercounts human prompts conservatively |
| 6 | none | `human` | composer and API sends |

An agent-authored row has origin `agent`, so an empty origin always means the
Host predates the field. The function must cover every
creator of user-authored messages; task 03 enumerates them and a test fails when
a creator's marker is not in the table. The DTO carries only the enum; no
metadata key leaves the Host (AC 001.5). Queue delivery does not reuse the
original row: drain creates a new row through `CreateUserMessageIdempotent`
with the queued entry's metadata minus queue-only keys
(`metadataWithoutQueueOnlyKeys`, `internal/orchestrator/event_handlers_agent.go`).
Origin therefore survives only because those markers are copied, and AC 001.6
is tested at that boundary, including the workflow auto-start path that
pre-records its row.

`PluginMessageFilter` (`internal/task/models/models.go`) gains `AuthorTypes`,
applied as an `author_type IN (...)` predicate in `ListMessagesForPlugin`; its
values count toward `maxMessageFilterValues` (400). Origin is not a SQL filter:
it would need JSON-metadata predicates with SQLite/Postgres parity for no
benefit, since the plugin filters returned rows.

## Session activity and turns

Both methods extend `SessionReader` (`pkg/pluginsdk/host.go`). They are gated by
a new `api_read` resource `session_activity`, not by `sessions`: engagement
telemetry is personal data, and an installed plugin that holds `sessions` must
not gain it silently. Today the `Sessions()` accessor is gated as a whole
(`internal/plugins/host_data.go`: denied reader when `sessions` is not declared,
Unimplemented when the data sources are nil), so it moves to per-method gates
like `Tasks()` and `Messages()`: `List` and `CodeStats` need `sessions`;
`UserActivity` and `Turns` need only `session_activity`, and a plugin declaring
only `sessions` receives `PermissionDenied` naming `api_read:session_activity`.
Each new method has its own nil-data-source `Unimplemented` fallback. The
resource is a constant beside the others in `host_data.go` and is listed in
`docs/public/plugins-manifest.md`; the manifest package only checks membership
(`CanRead`), and the install review renders `read:<resource>` generically, so
neither changes.

- `UserActivity(ctx, SessionActivityFilter, Page) ([]SessionUserActivity, *PageInfo, error)`
  returns `{SessionID, TaskID, ActiveMs, IntervalCount, FirstActivityAt,
  LastActivityAt, ActivityMeasured}` from the task repository's paged session
  activity query. `ActiveMs` sums per-actor active time, so a session worked by
  several people reports aggregate person-time. `ActivityMeasured` is the
  repository's read-time measured state (stamped, with no capture gap over the
  session's life). A not-measured session returns zeros with the flag false
  (AC 002.2).
- `Turns(ctx, SessionActivityFilter, Page) ([]SessionTurn, *PageInfo, error)` returns
  `{ID, SessionID, TaskID, StartedAt, CompletedAt}` ordered `(started_at, id)`
  with an opaque keyset cursor, from `task_session_turns`. Turn `metadata` is not
  mapped (AC 002.6).

`SessionActivityFilter{SessionIDs, TaskIDs}` is a new type, because the existing
`SessionFilter` (`pkg/pluginsdk/data_types.go`) has no session-ID field and the
existing `List` and `CodeStats` calls are unchanged. A shared validator returns
`InvalidArgument` when the two lists together exceed 400 values, the same
number as `maxMessageFilterValues` but a separate constant. The task repository
applies the filters, selects a page of sessions ordered by `(started_at,
session_id)`, and returns an opaque keyset cursor; neither the Host nor plugin
materializes an unbounded ID list. Activity queries read the single persisted
capture-state/gaps value once, query session identity, `started_at`,
`completed_at`, and metadata, and left-join interval aggregates from
`task_session_user_activity` so sessions with no intervals remain visible.
Session rows are fetched in chunks of at most 400; measured state is evaluated
in Go against scanned timestamps, avoiding dialect-specific JSON and text
timestamp comparisons. The analytics service is not used: it aggregates
workspace statistics, while these are paged session reads.

## Code and pull-request summary

`SessionCodeStats` (`pkg/pluginsdk/data_types.go`, mirrored from
`internal/analytics/models.SessionCodeStats`) adds `FilesChangedCommitted` and
`FilesChangedPeakPending` as new fields after the existing ones.
`FilesChangedCommitted` is the sum of per-commit `files_changed` from
`task_session_commits`, so a file edited in several commits counts several times;
this matches the existing churn semantics of the committed line counts.
`FilesChangedPeakPending` is the maximum distinct file count across
`task_session_git_snapshots` for the session. The documented meaning is a
contract (AC 003.2); a distinct committed-path count would need a new column
and is excluded. `CommittedLinesAvailable` governs the committed file count
too (AC 003.1).

`TaskPullRequest` adds `CreatedAt`, `ChangedFiles`, `ApprovedReviewerCount`, and
`SizeObserved`, mapped at `internal/plugins/host_data_mappers.go`.
`github.TaskPR.ChangedFiles` is `*int`. `github.TaskPR.ReviewCount` is a plain
int that counts distinct reviewers whose latest review is `APPROVED`, written on
every poll, so the field is named for what it counts. A request is "observed"
when its `LastSyncedAt` is set; before that `ChangedFiles` and
`ApprovedReviewerCount` are unset on the wire (AC 003.4). Comment counts are not
added: `github.TaskPR.CommentCount` is no longer refreshed by polling and would
read as stale zeros; comments come from the feedback read.

`gitlab.TaskMR` (`internal/gitlab/models.go`) maps field by field:

| `TaskPullRequest` | From `TaskMR` |
| --- | --- |
| `Number`, `URL`, `Title` | `MRIID`, `MRURL`, `MRTitle` |
| `State` | `State`, with `locked` mapped to `closed` |
| `HeadBranch`, `BaseBranch`, `IsDraft`, `AuthorLogin` | `HeadBranch`, `BaseBranch`, `Draft`, `AuthorUsername` |
| `ReviewState`, `ChecksState`, `MergeableState` | `ApprovalState`, `PipelineState`, `MergeStatus` |
| `ChecksTotal`, `ChecksPassing`, `UnresolvedReviewThreads` | `PipelineJobsTotal`, `PipelineJobsPass`, `UnresolvedDiscussions` |
| `ApprovedReviewerCount` | `ApprovalCount`, when `LastSyncedAt` is set |
| `CreatedAt`, `MergedAt`, `ClosedAt` | `CreatedAt`, `MergedAt`, `ClosedAt` |
| `ChangedFiles` | unset (not stored) |
| `Additions`, `Deletions` | 0 with `SizeObserved` false (not stored) |

`SizeObserved` is true only for a GitHub row whose `LastSyncedAt` is set (an
unsynced row has zero additions and deletions that were never observed), and
false for GitLab, so a plugin does not read either as an empty change.

`taskPRsToDTOs` today maps only `github.TaskPR` and hard-codes
`Provider: "github"`, although the SDK doc already allows `gitlab`. The plugin
task reader gains a GitLab source with the shape of `h.taskPRs`, backed by
`gitlab.Service.ListTaskMRsByTaskIDs`, and merges its rows (newest first, by
creation time) into `PullRequests` with provider `gitlab` (AC 003.6). A missing
or failing source does not fail the task read: its rows are omitted and the
proto3-optional `PullRequestsPartial` is set true; on a complete read it is set
false. The SDK exposes `*bool`: nil means an older Host omitted the field and
completeness is unknown, false means all sources were read, and true means at
least one source failed.

## Change-request feedback

New exact read `ListChangeRequestFeedbackExact`, registered beside
`exactReadChangeEvidence` in `exactReadMethods`
(`internal/plugins/host_exact_queries.go`) and in `exactReadTaskDataMethods`
(`internal/plugins/host_exact.go`), using the existing capability string
`host.v2.read:change_requests`, `exactReadAdmission`, and `exactPage`. The query
is `{RequestID, WorkspaceID,
TaskIDs, Page}`. Snapshots are capped at 5000 rows and 8 MiB
(`internal/plugins/exact_read_snapshots.go`), measured on `json.Marshal` output,
and an over-cap snapshot fails the whole read. JSON escaping inflates a body of
`<`, `>`, or `&` sixfold, so 1000 items of 4096 bytes could reach about 24 MiB.
The read bounds itself to 20 tasks (`ResourceExhausted` above that limit), the
50 newest items across providers per task, and a 7 MiB encoded page budget,
accumulated as items are added. Encoding uses the same `json.Marshal` form as
`exact_read_snapshots.go`; the overall snapshot still has 5000-row and 8 MiB
caps. JSON escaping can expand a body of `<`, `>`, or `&` sixfold. Items beyond
either limit are dropped oldest first and counted in the corresponding
provider result's `items_omitted` (AC 004.5).

`exactPage` snapshots one homogeneous list of `ChangeRequestFeedbackTask` values,
ordered by task ID. Each row contains provider results and its selected items,
so a page never separates a task's status from its items:

```text
ChangeRequestFeedbackTask {
  task_id,
  providers: []ChangeRequestFeedbackProviderResult {
    provider, provider_state (available | unavailable | unsupported),
    provider_error, items_omitted
  },
  items: []ChangeRequestFeedbackItem {
    provider, repository_id, repository_owner, repository_name, number,
    item_id, kind (review | comment | review_comment | discussion_note),
    author_login, author_kind (human | bot | unknown), review_state,
    resolved (optional bool), created_at, updated_at, body, body_truncated
  }
}
```

Each queried association source has one provider result. A successful source and
feedback read reports `available`, including when it returns no linked requests
or no feedback. A source or provider-reader error reports `unavailable` for only
that provider. `unsupported` means linked requests exist but the feedback adapter
does not. One provider's failure does not suppress another provider's results.

Resolution per task: query GitHub and GitLab association sources independently,
then call each provider reader for the linked requests it returns. A failed
association source produces an `unavailable` provider result with no items;
successful data from the other provider is still returned. GitHub uses
`ListPRReviews` and `ListPRComments` (`internal/github/client.go`); GitLab uses
`Service.GetMRFeedback` (`internal/gitlab/service.go`) over its approvals and
discussions. Current reader shapes limit the fields: `github.PRReview` has no
bot flag and no `UpdatedAt`, and no GitHub reader supplies resolution. So review
items use `author_kind` `bot` only when the login ends in `[bot]` and `unknown`
otherwise, `updated_at` equals `created_at`, and `resolved` is absent; comment
items take `AuthorIsBot` and `UpdatedAt` from `PRComment`. A reader extension
that supplies more is allowed but not required.

Bodies are sanitized and cut at 4096 bytes on a rune boundary (AC 004.2). Items
use total ascending order `(created_at, provider, repository_id, number, kind,
item_id)`. Select the 50 newest in reverse order, then return the selected items
ascending; size-budget removal drops the oldest first. The provider-qualified
tuple makes provider-scoped item IDs deterministic across repositories and item
kinds. The task-page cursor is opaque. Log fields are task count and outcome
only (AC 004.6). `exactPage` loads a snapshot keyed by a generated version, with
the request ID only in the receipt; it does not join or deduplicate concurrent
calls. Each first page therefore performs a full live provider fan-out under the
user's credential, so the plugin reads incrementally and per task.

## Compatibility and authorization

All proto messages and RPCs are additive in `plugin.proto`; generated code,
`pkg/pluginsdk` mappings, `grpcHostServer`, `UnimplementedHostData` defaults,
and manifest docs move together (AC 005.1). A plugin that declares nothing new
is unaffected. The minimum-version floor
(`manifest/min_version_policy.go`) checks declared manifest capabilities only
and cannot see which RPCs a plugin calls, so it keys on the new
`session_activity` resource: `validateCapabilityMinimumVersions` gains a second
rule for it. Its floor is a constant `MinimumSessionActivityVersion` set to the
next unreleased version per the release process; task 04 introduces it and
task 06, the last proto order, verifies it still names the release shipping every
addition of this capability, so a plugin declaring `session_activity` can rely on
tasks 03 to 06. A new RPC on an older Host answers `Unimplemented` through the
`UnimplementedHostData` defaults. A new field on an existing RPC is silent on an
older Host, so each is made detectable: `origin` is never empty on a new Host
(agent rows are `agent`), file counts and `ChangedFiles`/`ApprovedReviewerCount`
are optional proto scalars, and `SizeObserved` is always set by a new Host.
`PullRequestsPartial` is a proto3-optional bool: a new Host sets false for a
complete result and true for partial data, while an older Host leaves it nil in
the SDK. `AuthorTypes` ignored by an older Host is caught by the empty `origin`
check. Keying the floor on `sessions` would reject every existing plugin.
Browser `host.conversation` is unchanged.

## Reference plugin design

Non-normative; implemented in the external repository.

**Shape.** Managed Go plugin plus native UI bundle. Manifest:
`api_read: [tasks, sessions, session_activity, messages, change_requests, usage]`, `state: true`,
`events: [task.state_changed, task.updated, github.task_pr.updated]`, an `actions`
entry `estimate` (scope task) and `summary` (scope workspace), `ui.bundle`.

**Data flow.** On a task event, a debounced worker reads, for that task:
sessions, `UserActivity`, `Turns`, human `Messages` (`AuthorTypes: [user]`, then
origin `human`), `CodeStats`, `Task.PullRequests`, and feedback; computes
features; stores `{features, estimate, formula_version, computed_at}` in
`SetState` keyed by task. The UI never reads raw data; it calls actions.

**Features.** Human prompt count (`origin == human`); human active minutes
(measured when `ActivityMeasured`, else a proxy: for each human prompt after the
first, the gap from the end of the previous agent turn to that prompt, each
capped at 10 minutes, labelled `proxy`); agent turn minutes; task wall time
(`CreatedAt` to `CompletedAt`, else now); files and lines changed; PR count;
review rounds; human comment count; and bot comment count. Add a
data-completeness note only when `PullRequestsPartial == true`; nil means the
older Host's completeness is unknown, not that the task is complete.
**Estimate.** A deterministic, explainable score: a weighted sum of
log-scaled features, mapped to the Fibonacci scale by percentile rank against
the workspace's completed tasks, so scores calibrate to local history. Weights
favor human active minutes and prompt count. Every estimate stores its
feature breakdown so the UI can show why. Below a minimum number of completed
tasks the plugin shows a fixed-threshold fallback labelled `uncalibrated`.

**UI**, built only from existing contributions and `host.ui` primitives
(`Card`, `ChartContainer`, `Badge`, `Empty`):

```text
UI-01: Story points route (desktop; phone stacks cards, one scroll owner)
+----------------------------------------------------------------+
| Story points              [7d] [30d] [90d]        [Copy]       |
+----------------+----------------+----------------+-------------+
| Estimated SP   | Human time*    | Prompts        | PRs         |
| 128            | 41h 20m        | 312 (human)    | 37          |
+----------------+----------------+----------------+-------------+
| SP by completion day (bar)    | Human time vs agent time (bar) |
+-------------------------------+--------------------------------+
| Task              SP  Human time*  Prompts  Files/Lines  PRs  Why |
| Fix login flow     5   1h 12m       9      7 / +310 -88    1  >  |
| * labelled "measured" or "proxy" per row and in the cards         |
+----------------------------------------------------------------+
```

A task panel (`registerTaskPanel`) shows the per-session breakdown and "why";
a `task-card-indicators` badge shows the SP. The route uses `registerRoute` and
`registerNavItem`. No new core slot is added (AC 005.3).

## Failure and recovery

A read that errors leaves the previous stored estimate and marks it stale; the
UI shows the stale state with retry. Missing activity, PR feedback, or
unsupported provider state lowers a feature's confidence label rather than
zeroing it. Plugin state survives upgrades; a `formula_version` bump triggers a
background recompute.

## Security

Every read is capability-gated. Feedback also needs the workspace approval.
Activity needs the explicit `session_activity` resource and its capability
review; it has no separate exact approval. Feedback bodies and author logins are third-party content: the plugin
stores features and a bounded excerpt only, not full bodies. Plugins run
same-origin and are not a hostile sandbox; the existing approval flow is the
boundary.

## Observability

Per-RPC request logs without bodies, task titles, or logins. Existing plugin
host counters apply. The plugin owns its own estimate-run metrics.

## Verification architecture

Go tests cover origin classification across every marker, filter bounds,
rollup measured/unmeasured, turn paging, code and PR mappers, exact-read
admission, per-provider failure, truncation, and SDK/proto parity. The
`cmd/plugin-fixture` package gains one fixture exercising all new reads and the
three contributions; Playwright (`apps/web/e2e/tests/plugins/`) renders it on
desktop and Pixel 5.

## Related decisions

- [ADR: Session user-activity ledger](../../../decisions/2026-10-03-session-user-activity-ledger.md)
- [ADR 0043: capability-gated Host data API](../../../decisions/0043-plugin-host-data-api.md)
- [ADR 0047: Plugins read conversation content via a capability-gated Host RPC](../../../decisions/0047-plugin-host-conversation-reads.md)
