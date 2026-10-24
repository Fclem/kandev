---
created: 2026-10-03
status: in_progress
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
| GitHub | `github_task_prs` via `ListTaskPRsByTaskIDs` | `ListPRReviews`, `ListPRComments` (reviews: bot by `[bot]` login suffix else `unknown`, no `updated_at`, no resolution) | provider result is `available`, `unavailable`, or `unsupported`; source failures affect only GitHub feedback |
| GitLab | `gitlab.Service.ListTaskMRsByTaskIDs` (no changed-file or review counts stored) | `gitlab.Service.GetMRFeedback` approvals and discussions | provider result is `available`, `unavailable`, or `unsupported`; source failures affect only GitLab feedback; `PullRequestsPartial` is separate |

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

The original Host work orders are complete, but the combined API/plugin
deliverable is not yet accepted. Remaining work from adversarial review:

- [x] Finish Host round 3 findings, including tablet activity and timestamp precision.
- [x] Reconcile existing tasks on plugin install/restart and formula upgrades.
- [x] Debounce event-driven recomputes and serialize per-task writes.
- [x] Render derived per-session detail in the task panel.
- [x] Distinguish incomplete source totals from the summary display-row cap.
- [x] Prove cold phone navigation to the canonical plugin route.
- [ ] Complete the requested review loop and package/install verification.

Host review rounds 4, 5, and 6 returned no findings against the completion fixes.
The standalone plugin still needs its final review and delivery. PostgreSQL
coverage requires `KANDEV_TEST_POSTGRES_DSN`; emulated visibility is not native
browser-background proof.

Parent verification of completion fixes: focused Go regressions in GitHub,
task service, handlers, and SQLite passed; mounted mobile and tablet tests
passed (two tests each). Changed-package Go lint passed against canonical
`origin/main`'s merge base `70c884c84b`; the default stale `upstream/main` base
reported an unrelated existing `goconst` finding. The override was
command-scoped, with no persistent configuration or linter suppression.

Standalone plugin: Go race tests and 21 UI tests passed, as did format, vet,
package-verifier, release-version, and host-package checks. A package installed
on a disposable backend automatically estimated a task created before install,
without an estimate action or task event. A subsequent completion event updated
the stored estimate. Desktop and fresh Pixel 5 contexts directly opened and
reloaded `/story-points`, exercised Why, and showed no document overflow or page
errors. The phone task panel rendered both session rows. Evidence:
`/home/clem/.kandev/tasks/investigate-potentia_cw2zw7bk/story-final-jls69X/browser-evidence.json`
and screenshots in the same directory. This smoke used a Vite frontend against
the isolated backend, not a shipped embedded-web artifact. Both seeded sessions
were capture-enabled measured zero; unmeasured/proxy distinctions are covered
by regression tests, not claimed from these browser fixtures.

A later capture-off smoke confirmed the unavailable-time headline and separate
unavailable count. Phone controls measured 44px; desktop fine-pointer controls
remained 24px, and narrow fine-pointer controls measured 44px. Evidence:
`story-final-jls69X/availability-evidence.json` in the same task workspace.
Subsequent plugin review identified mixed measured/proxy aggregation, pending-file
fallback, cancellation freshness, and partial state-write consistency issues;
their remediation and final review remain pending.

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
  atomically records capture transitions and gaps in one `kandev_meta` value; a
  session overlapping any gap reads as not measured. No session row is written,
  and later unrelated updates do not change a session's measured state.
- **Live provider cost.** Feedback reads call GitHub and GitLab under the user's
  credentials; rate limits surface as `unavailable`, not failures.
- **No history.** The ledger starts empty and ships disabled, so the plugin
  initially depends on its proxy.
- **Tasks README size.** `docs/specs/tasks/README.md` is near its 16 KiB limit;
  this package does not edit it.

## Open questions

- Should `FilesChangedCommitted` become a distinct-path count? Excluded here
  because it needs a new column; revisit if the plugin's calibration shows
  churn distortion.
