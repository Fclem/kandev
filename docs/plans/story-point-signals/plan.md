---
created: 2026-10-03
status: completed
requirements:
  - REQ-TASKS-USER-ACTIVITY-001
  - REQ-TASKS-USER-ACTIVITY-002
  - REQ-TASKS-USER-ACTIVITY-003
  - REQ-PLUGINS-EFFORT-SIGNALS-001
  - REQ-PLUGINS-EFFORT-SIGNALS-002
  - REQ-PLUGINS-EFFORT-SIGNALS-003
  - REQ-PLUGINS-EFFORT-SIGNALS-004
  - REQ-PLUGINS-EFFORT-SIGNALS-005
system_design:
  - ../../specs/tasks/system-design/session-user-activity.md
  - ../../specs/plugins/system-design/effort-signals-host.md
legacy_specs: []
---

# Implementation Plan: Effort signals for a story-point plugin

## Overview

A story-point estimator plugin needs human-effort facts that core does not
expose. This plan adds a measured session user-activity ledger and the
additive Host reads that deliver it, plus prompt origin, turn timing, file and
pull-request summaries, and change-request feedback. Order: the ledger first
because the activity read projects it; message origin early because it is
independent and unblocks the plugin's prompt counts; the three remaining Host
reads share `plugin.proto` and generated code, so they go in sequence; the
fixture and documentation close the Host package. The estimator plugin lives
in the external repository `Fclem/kandev-plugin-story-points`. The user's later
completion request includes that plugin and the remaining Host review fixes.

## Scope

### In scope

- Session user-activity ledger, WebSocket beat action, runtime flag, and
  browser reporter.
- Host reads: message `origin` and `AuthorTypes`, `Sessions().UserActivity` and
  `Sessions().Turns` behind a new `session_activity` read resource, file counts,
  PR summary fields, GitLab MRs in `Task.PullRequests`, and
  `ListChangeRequestFeedbackExact`.
- A fixture plugin, Playwright proof, and public plugin documentation.
- The standalone estimator plugin, formula, native UI, packaged artifact, and
  disposable-host verification in `Fclem/kandev-plugin-story-points`.

### Out of scope

- A core story-point field and a `/stats` slot.
- Backfilling activity from history.
- Enabling the flag in the `prod` profile; that is a separate release decision
  under `/runtime-feature-flags`.

## Technical approach

| Boundary | Change |
| --- | --- |
| Task repository and service | `task_session_user_activity` table, `RecordUserActivity`, `SumUserActivity` (with read-time `activity_measured`), `ListUserActivitySessions` (filtered, cursor-paged session selection), atomic `RecordCaptureTransition` over one `kandev_meta` JSON value, and the session metadata stamp written in the shared `createTaskSession` insert; no session-row writes on disable |
| Gateway | `ActionSessionUserActivityBeat` in `pkg/websocket/actions.go`, handler in `internal/gateway/websocket/` |
| Runtime flags | `features.sessionUserActivity` in `internal/runtimeflags/registry.go`, `profiles.yaml`, frontend flag contract |
| Web | `apps/web/hooks/domains/session/use-session-user-activity.ts` mounted by the focused session surface |
| Plugin Host | `plugin.proto`, `pkg/pluginsdk`, `internal/plugins/host_data*.go`, `host_exact_queries.go`, mappers, `UnimplementedHostData`, `internal/plugins/manifest` (`session_activity` resource and its minimum-version rule) |
| Fixture and docs | `apps/backend/cmd/plugin-fixture` and the shared e2e fixture package, `apps/web/e2e/tests/plugins/`, `docs/public/plugins-manifest.md`, `plugins-authoring.md`, `configuration.md`, `websocket-api.md`, `docs/plans/plugins/PLUGIN-API.md` |

Compatibility matrix for feedback and PR data:

| Provider | Association source | Feedback source | Behavior if unavailable |
| --- | --- | --- | --- |
| GitHub | `github_task_prs` via `ListTaskPRsByTaskIDs` | `github.Service.GetPRFeedbackItemsForWorkspace`, resolving the current workspace automation credential and reading native reviews/comments without feedback-cache or aggregate writes | provider result is `available`, `unavailable`, or `unsupported`; source failures affect only GitHub feedback |
| GitLab | `gitlab.Service.ListTaskMRsByTaskIDs` (no changed-file or review counts stored) | `gitlab.Service.GetMRFeedbackForWorkspace` approvals and discussions | provider result is `available`, `unavailable`, or `unsupported`; source failures affect only GitLab feedback; `PullRequestsPartial` is separate |

No rendered core UI changes; the browser reporter is invisible. The plugin's
reference UI sketch is in the
[design](../../specs/plugins/system-design/effort-signals-host.md#reference-plugin-design)
and is non-normative.

## Tests

| Criteria | Evidence |
| --- | --- |
| AC-TASKS-USER-ACTIVITY-001.3, 001.6, 002.1-002.7 | `internal/task/repository/sqlite/user_activity_merge_test.go`: `TestRecordUserActivityMergeMatrix`, `TestRecordUserActivityConcurrentBeatsDoNotOverlap`; `internal/task/repository/sqlite/user_activity_test.go`: `TestRecordUserActivityDisabledStoresNothing`, `TestRecordUserActivityRejectsArchivedAndCleanupBlockedTasks`; `internal/task/service/user_activity_test.go`: `TestRecordUserActivityMissingAndInaccessibleSessionsAreUniform`; `internal/task/handlers/user_activity_ws_test.go`: `TestSessionUserActivityBeatAcknowledgements`; `internal/gateway/websocket/dispatch_scope_test.go`: `TestUserActivityBeatDefersSessionAuthorizationToHandler`; `internal/task/repository/sqlite/user_activity_postgres_test.go`: `TestPostgresUserActivityLedger` (needs `KANDEV_TEST_POSTGRES_DSN`) |
| AC-TASKS-USER-ACTIVITY-001.1, 001.2, 001.4, 001.5, 003.1 | `apps/web/hooks/domains/session/use-session-user-activity.test.ts`: `reports only while visible, focused, and recently active`, `stops on blur, hidden, idle, and session switch`, `sends only session_id`, `stops on enabled false acknowledgement and while reload is required`; `internal/task/repository/sqlite/user_activity_test.go`: `TestRecordUserActivityDisabledStoresNothing` |
| AC-TASKS-USER-ACTIVITY-003.2-003.6 | `internal/task/repository/sqlite/user_activity_test.go`: `TestCreateTaskSessionStampsCaptureState`, `TestEveryCreateTaskSessionVariantStampsCaptureState`, `TestActivityBeatsAndCaptureTransitionsDoNotChangeSessionUpdatedAt`; `internal/task/repository/sqlite/user_activity_transition_test.go`: `TestCaptureTransitionIsIdempotentAndFirstRunOffCreatesNoGap`, `TestCaptureGapMeasuredStateUsesSessionLifetimeNotUpdatedAt`, `TestCaptureTransitionWriteFailureLeavesPriorStateAtomic`; `internal/backendapp/user_activity_startup_test.go`: `TestFlagOverrideStampsNewActivitySession`; `internal/runtimeflags/registry_test.go`: `TestSessionUserActivityFlagIsRestartRequiredAndOffByDefault`; `apps/web/lib/state/slices/features/features-contract.test.ts` |
| AC-PLUGINS-EFFORT-SIGNALS-001.* | `internal/plugins/host_data_message_origin_test.go`: `TestClassifyMessageOrigin`, `TestMessageListAuthorTypesFilter`; `internal/orchestrator/message_origin_test.go`: `TestQueuedPromptKeepsOrigin`, `TestUserMessageCreatorsHaveClassifiedOrigin` |
| AC-PLUGINS-EFFORT-SIGNALS-002.* | `internal/plugins/host_data_session_activity_test.go`: `TestSessionUserActivityRead`, `TestSessionTurnsRead`, `TestSessionActivityFilterBound`, `TestSessionActivityRequiresResource` |
| AC-PLUGINS-EFFORT-SIGNALS-003.* | `internal/plugins/host_data_mappers_test.go`: `TestTaskPullRequestSummaryFields`, `TestTaskPullRequestsIncludeGitLab`, `TestTaskPullRequestsPartialByProvider` (table-driven GitHub and GitLab source failures preserve the other provider's rows and set `PullRequestsPartial`); `pkg/pluginsdk/host_parity_test.go`: `TestTaskPullRequestsPartialPresence` (nil/false/true); `internal/analytics/repository/sqlite/session_code_stats_test.go`: `TestSessionCodeStatsFileCounts`; `session_code_stats_postgres_test.go`: `TestPostgresSessionCodeStatsFileCounts` (needs `KANDEV_TEST_POSTGRES_DSN`) |
| AC-PLUGINS-EFFORT-SIGNALS-004.* | `internal/plugins/host_exact_feedback_test.go`: `TestListChangeRequestFeedbackExact`, `TestFeedbackProviderFailureIsPerProvider` (successful provider items survive another provider's failure), `TestFeedbackBoundsAndTruncation`, `TestFeedbackOrderingUsesProviderTieBreakers` |
| AC-PLUGINS-EFFORT-SIGNALS-005.1 | `pkg/pluginsdk/host_parity_test.go`: `TestHostSDKProtoParity` (created by task 03, extended by 04 to 06); `internal/plugins/manifest`: `TestSessionActivityMinimumVersion` |
| AC-PLUGINS-EFFORT-SIGNALS-005.4 | Task 07's recorded content checklist verifies each added RPC/resource, capability, bound, and failure state against proto and public docs; `node scripts/validate-public-docs.mjs` separately checks public-doc structure and links. |

## E2E tests

| Flow | Criteria | File and project |
| --- | --- | --- |
| Focused and backgrounded page activity is visible through fresh Host reads | AC-TASKS-USER-ACTIVITY-001.1, 002.6, AC-PLUGINS-EFFORT-SIGNALS-002.1, 005.2 | `apps/web/e2e/tests/plugins/effort-signals-plugin.spec.ts`, desktop; explicit inert-surface `pointerdown`, about 20 s focused capture; fresh `Sessions().UserActivity` baseline, background through a second page, assert first page hidden, wait 30 s, then require another fresh Host read with unchanged `ActiveMs` (about 50 s real time total) |
| Fixture route, task panel, card badge | AC-PLUGINS-EFFORT-SIGNALS-005.3 | `apps/web/e2e/tests/plugins/mobile-effort-signals-plugin.spec.ts`, project `mobile-chrome` (Pixel 5); desktop path in the file above |

## Work orders

- [x] [Task 01: Session user-activity ledger](task-01-activity-ledger.md)
- [x] [Task 02: Browser activity reporter](task-02-activity-reporter.md)
- [x] [Task 03: Prompt origin and author filter](task-03-message-origin.md)
- [x] [Task 04: Session activity and turn reads](task-04-activity-turn-reads.md)
- [x] [Task 05: Code and pull-request summary](task-05-code-pr-summary.md)
- [x] [Task 06: Change-request feedback read](task-06-change-request-feedback.md)
- [x] [Task 07: Fixture plugin, E2E, and documentation](task-07-fixture-and-docs.md)

Dependency order: 01 -> 02; 01 -> 04; 03 -> 04 -> 05 -> 06 -> 07; 02 -> 07.
Waves: 1 (01, 03), 2 (02, 04), 3 (05), 4 (06), 5 (07). Every order is
sequential: 03-06 share `plugin.proto` and generated code, and 01 and 03 share
the task repository package.

## Verification results

Task 01 verification: focused Go regressions, runtime/config/profile tests, web
feature contract, web typecheck/lint, public-doc validation, and backend
format/lint passed. PostgreSQL integration was not run because
`KANDEV_TEST_POSTGRES_DSN` was unset.

Task 02 verification: Node 24 focused web tests passed (36 activity-reporter
and WebSocket tests; 7 panel-activity tests); web typecheck and lint passed.
The Chromium E2E smoke `tests/session/session-focus-signal.spec.ts` passed
(1 test), exercising desktop session mounting and the existing subscribe/focus
flow. It does not assert activity beats.

Task 03 verification: proto regeneration, origin/author-filter/parity tests, orchestrator origin tests, `make fmt lint`, managed-dispatch regression, and public-doc validation passed. Tasks 04–07 are complete; their individual Results record verification and limitations.

### Completion review

The Host work orders and standalone plugin implementation are complete.
Workspace-cache, move-event ordering, cohort memory, and authorization-error
corrections passed their regression checks. Both source branches have been
normally pushed and their remote commit identities verified.

- [x] Finish Host round 3 findings, including tablet activity and timestamp precision.
- [x] Reconcile existing tasks on plugin install/restart and formula upgrades.
- [x] Debounce event-driven recomputes and serialize per-task writes.
- [x] Render derived per-session detail in the task panel.
- [x] Distinguish incomplete source totals from the summary display-row cap.
- [x] Prove cold phone navigation to the canonical plugin route.
- [x] Verify the final five-platform package and isolated failure/recovery paths.
- [x] Complete the requested review loop and push the standalone source branch.

Host review rounds 4, 5, and 6 returned no findings against the completion fixes.
Host follow-up `97c83c7575c2852e6caaf76ce8faf05d2e605829` is committed and
normally pushed to `Fclem/kandev`, branch `feature/design-story-point-e-928`.
All applicable pre-commit and commit-message hooks passed without bypass.
The standalone plugin pins that remotely verified SDK commit.
Hook receipt: `host-effort-completion-receipt.json` in the parent task workspace.

Standalone source delivery:
[`Fclem/kandev-plugin-story-points`](https://github.com/Fclem/kandev-plugin-story-points/tree/feature/design-story-point-e-n4h),
branch `feature/design-story-point-e-n4h`, commit
`e6f54e430db4f80d8f351a50a4103fecf71fdf10`. The clean committed tree
`1b25f231131073f8418cc4c0e13cd38fe665b7a8` matches the reviewed and packaged tree.
Remote and upstream SHAs match. The standalone template has no configured Git
hooks; no hooks were bypassed, and the explicit local gates passed.
Receipt: `plugin-effort-completion-receipt.json` in the parent task workspace.
No PR, default-branch merge, tag, or release was created.

Parent Host verification: focused Go regressions in GitHub, task service,
handlers, and SQLite passed; mounted mobile and tablet tests passed (two each).
Changed-package Go lint passed against canonical `origin/main`'s merge base
`70c884c84b`; the default stale `upstream/main` base reported an unrelated
existing `goconst` finding. The override was command-scoped, with no persistent
configuration or linter suppression.

Final-package verification: the unchanged Go race suite passed in 12.154 seconds.
All 40 UI tests passed, along with format, vet, package-verifier,
release-version, host-package, and all-five-platform checks. The archive was
rebuilt after the final structured-error classifier correction and installed
successfully on the disposable backend, returning a fresh two-session estimate.
Archive SHA-256:
`d9465d28ffe09a7b2aecd0636e22e11aa6be8dcca57c86b5c6727eecbbe0bbb7`.

Review corrections cover mixed and unavailable human time, nullable
file/line statistics, first-prompt proxy boundaries, session grouping,
current and archived cohort membership, completion-chart date bounds, old or
row-capped task badges, cold saved-stale estimates, bounded cancellation
cleanup, pending index writes, and missing/repeated continuation cursors.
Interrupted writes cannot silently present divergent task/index records as
fresh or use pending records for calibration. Formula version 2 triggers
background repair of legacy records.
The plugin's 10,000-page / one-million-target guards are safety ceilings,
not a Host capacity promise. The pinned Host rejects an entire exact snapshot
above 5,000 rows or 8 MiB before paging. That returns a read error, preserving a
same-scope saved stale estimate where available; summary is unavailable rather
than falsely labelled complete or partial. A compatible Host serving more rows
can instead reach the plugin's separate materialization cap and truncation flag.
Cycle tests reproduced duplicate rows and endless traversal before their fix.
Known minutes from partially unavailable sessions retain a partial qualifier
in summary/day counts, task explanations, and TSV exports. Completion charts
use authoritative live completion timestamps, never unrelated edits or legacy
index dates. Unknown dates remain in totals and are disclosed separately.
The pinned Host does not populate `Task.CompletedAt`; those dates are unknown.
Committed files require both the committed-capture availability flag and a
non-null count, matching AC 003.1 and the Host mapper; otherwise the plugin
uses observed peak-pending files or reports unknown. Card fallbacks share a
four-request concurrency limit, coalesce by task, and drop queued work on
destroy without delaying explicit task-panel or recompute actions.
Reconciliation validates cohort membership once per pass, then shares immutable
sorted workspace snapshots across batch jobs. Direct actions and ordinary
events still read fresh membership. Tests prove one task-stream scan and one
index read per workspace, rather than repeating both for every task. Refresh
targets come from that validated snapshot, not a second mutable enumeration:
the regression first enqueued only a later task instead of both original tasks.
Repeated streamed task IDs cannot inflate cohort size; another regression
reproduced 23 instead of 11 peers before deduplication. Sorted rank matches
ordinary calibration, including ties, old-self exclusion, and the 9/10-peer boundary.
Review 32 identified that ordinary task pages use live offsets: concurrent
inserts or removals can duplicate or omit summary and direct-cohort rows.
The plugin now uses the existing immutable exact task API for scoped metric
enumeration, with explicit workspace approval of `host.v2.read:tasks`. Global
discovery uses the existing workspace catalog reader, avoiding repeated
full task-list rebuilding merely to find workspace IDs. Unapproved workspaces do not
block approved scopes. Batch reconciliation retains unique task/workspace IDs
before queue backpressure rather than holding snapshot cursors open.
Review 35's moved-event test reproduced both a missing estimate and an unchanged
20-minute estimate when a later batch overwrote the newer workspace. Both event
orderings now produce the current workspace's 30-minute evidence. A default
estimate action also recomputes a moved task for its verified current owner,
without a forced refresh; wrong-workspace requests still fail, and a failed
new-scope read never falls back to the previous workspace's record.

Cohort construction first drains membership IDs, then loads and discards one
workspace's full state index at a time. Rows modified during the membership
stream do not enter that batch's completed cohort. The retained global batch
contains task/workspace references and immutable sorted scores, not all
feature-heavy workspace indexes.

Summary authorization failures map to HTTP 403 and clear all cached 7/30/90-day
windows for the affected workspace, aborting matching requests and rejecting
late completions. A structured non-authorization status or machine code takes
precedence over message-text matching: a 502 `upstream` error mentioning
permission retains the same-scope saved summary rather than erasing it.

Final read-only reviewer addenda 48, 49, and 50 returned no findings on that
classifier delta, following their full reviews and intervening fix reviews.
The requested three-consecutive-clean stopping condition is satisfied;
initialization failures were not counted as clean reviews.

An install estimated a pre-existing task without an estimate action or task
event; completion events updated stored results. Fresh desktop and Pixel 5
contexts directly opened and reloaded `/story-points`, exercised Why, and
showed no document overflow or page errors. The phone task panel rendered both
session rows. Capture-off smoke displayed `Unavailable`, not measured zero,
both in the headline and daily chart. Phone and narrow fine-pointer controls
measured 44px; desktop fine-pointer controls remained 24px. A January 2 task
outside the summary window displayed `1 pts` on its real kanban card after
exactly one estimate request, without changing its database timestamp.

Final-package fault injection corrupted only the disposable capture-state
metadata. The API and a cold phone task panel retained the saved point value
with an explicit stale notice. The exact original metadata was restored in a
`finally` block; five concurrent refreshes then returned fresh scoped records.
A genuinely empty workspace displayed `Unavailable / no estimates`. The Host
rejected a conflicting task/workspace action envelope with HTTP 400. Archiving
the task preserved its estimate in the workspace summary. Browser page errors:
zero. Scope-move denial and partial-write recovery are additionally covered by
regressions, not claimed as live provider tests.

Evidence is retained under
`/home/clem/.kandev/tasks/investigate-potentia_cw2zw7bk/story-final-jls69X/`:
`browser-evidence.json`, `availability-evidence.json`,
`final-package-api-evidence.json`, `recovery-evidence.json`, and screenshots.
`bounded-package-evidence.json`, `final-wall-clock-evidence.json`, and
`final-install-evidence.json` retain later package/API proofs. The final wall
time action uses the explicit completion timestamp or now, not a task edit.
`partial-time-evidence.json` and final desktop/phone screenshots prove the
installed archive labels both the headline and table cell as partial, exposes
unknown completion dates, and copies the partial qualifier through the real
clipboard. Both viewports had no document overflow or page errors. The exact
session fixture was restored, both owned services stopped, and throwaway smoke
scripts removed. Earlier completion-chart screenshots predate the authoritative
date correction; the final UI omits those unknown dates instead.
`final-badge-evidence.json` proves the bounded fallback still renders an old
task's `1 pts` badge after one scoped action without changing its task timestamp.
`final-snapshot-evidence.json` proves the latest package automatically refreshed
both existing task records before any estimate action, then served a fresh
explicit refresh with two sessions and retained archived summary history.
`exact-task-evidence.json` records an earlier exact-task package:
installation returned 201; a summary without task approval then failed with 502.
After granting only `host.v2.read:tasks` and restarting, startup estimated all
125 fixture tasks without estimate actions. The summary contained exactly 125
unique fixture rows across multiple pages (126 total including the existing
task), with neither truncation flag set. A fresh task estimate retained both
session rows and the optional feedback-approval gap; granting feedback removed
that gap. All 125 fixture task rows were removed in `finally`, the owned backend
was stopped, and the throwaway smoke script was deleted.
`final-auth-evidence.json` records the later native 403 mapping, another complete
125-task startup pass, and the native 5,000-row snapshot boundary. A snapshot
over that boundary failed rather than returning invented partial totals; the
saved task estimate remained stale. Desktop and cold Pixel 5 summaries cached
30- and 90-day windows, then a real task-capability revocation cleared both
windows, disabled Copy, and produced no page errors or horizontal overflow.
Approvals and fixture rows were restored afterward.

`workspace-cache-evidence.json` proves that switching to a moved task's new
workspace hides the old card badge while the scoped estimate request is
pending. The real successful response restores that workspace's badge without
reusing the old record. The task's workspace, workflow, and step were restored,
and a fresh current-owner estimate succeeded.

`final-classifier-evidence.json` records the current archive's successful 201
installation and fresh two-session estimate. A browser transport fault injected
HTTP 502 with code `upstream` and permission wording; the real API client retained
the saved summary and enabled Copy. Real native HTTP 403 revocation then removed
both cached windows on desktop and cold Pixel 5. There were no page errors or
horizontal overflow. Restoring approval returned summary HTTP 200. The initial
Vite load-event deadline required a browser-only retry, not repeated API setup.
Screenshots were inspected, both owned services stopped, and the throwaway
script removed.

The smoke used a Vite frontend against an isolated backend, not a shipped
embedded-web artifact. PostgreSQL requires `KANDEV_TEST_POSTGRES_DSN`; real
provider credentials were not used, and emulated visibility is not native
browser-background proof. Kandev 0.98.0 remains an unreleased install floor.

## Risks

- **Double-counted files and lines across sessions.** Sessions that reuse a
  workspace could attribute the same commits to several sessions; the design
  documents per-session semantics and the plugin must not sum blindly. Task 05
  checks the reuse path and records the result.
- **Prompt origin misclassification.** `human` is the fallback, so an unmarked
  generated prompt would count as human, and PR/MR automation carries both
  `auto_start` and `workflow_message`. The design fixes first-match order and
  adds a `plugin` origin; task 03 enumerates every creator of user-authored
  messages, confirms the `plugin:` source prefix at each plugin send path, and
  fails a test if a creator's marker is not classified.
- **Postgres evidence is DSN-gated.** Repository Postgres tests skip without
  `KANDEV_TEST_POSTGRES_DSN`; the work orders run them with an explicit DSN and
  report skips as not-run.
- **Capture is restart-scoped.** The flag is restart-required. Each startup
  stamps and confirms capture transitions in one `kandev_meta` value. A
  persisted unconfirmed boot becomes a measurement gap; failure of the first
  marker write cannot leave durable evidence and remains a documented limit.
- **Live provider cost.** Feedback reads use current workspace automation
  credentials; rate limits surface as `unavailable`, not failures.
- **No history.** The ledger starts empty and ships disabled, so the plugin
  initially depends on its proxy.
- **Tasks README size.** `docs/specs/tasks/README.md` is near its 16 KiB limit;
  this package does not edit it.

## Open questions

- Should `FilesChangedCommitted` become a distinct-path count? Excluded here
  because it needs a new column; revisit if the plugin's calibration shows
  churn distortion.
