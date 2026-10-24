---
status: draft
system: tasks
requirements:
  - REQ-TASKS-USER-ACTIVITY-001
  - REQ-TASKS-USER-ACTIVITY-002
  - REQ-TASKS-USER-ACTIVITY-003
created: 2026-10-03
owners:
  - kandev
---

# Session User Activity System Design

## Purpose and boundaries

The task system records measured human engagement per session as merged active
intervals. Capture is a browser reporter plus one WebSocket action. Storage and
rollup live beside the other session ledgers in the task repository. The
[effort-signals Host design](../../plugins/system-design/effort-signals-host.md)
projects the rollup to plugins and does not own capture.

No existing signal can stand in. `task_session.activity_changed` in
`internal/events/types.go` reports agent runtime activity, and the analytics
`active_duration_ms` sums completed agent turns
(`internal/analytics/repository/sqlite/stats.go`, `GetTaskStats`).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-USER-ACTIVITY-001` | [Browser reporter](#browser-reporter), [Server clock](#interval-merge) |
| `REQ-TASKS-USER-ACTIVITY-002` | [Interval merge](#interval-merge), [Persistence](#persistence) |
| `REQ-TASKS-USER-ACTIVITY-003` | [Enablement and measured state](#enablement-and-measured-state) |

## Components and responsibilities

1. **Reporter hook** in `apps/web/hooks/domains/session/` owns focus,
   visibility, and interaction tracking and sends the WebSocket action.
2. **Gateway handler** in `internal/gateway/websocket/` authenticates the
   client, checks session access, and forwards to the task service.
3. **Task service method** `RecordUserActivity(ctx, actor, sessionID, now)`
   owns merge rules and the rate guard.
4. **Task repository** owns the interval table and rollup query.

## Browser reporter

The hook runs once per focused session surface. State: `document.visibilityState
=== "visible"`, `document.hasFocus()`, and `lastInteractionAt`, updated by
passive `pointerdown`, `keydown`, `wheel`, and `touchstart` listeners on the
task page. Only a timestamp is stored. While all three conditions hold and
`now - lastInteractionAt < 60s`, the hook sends `session.user_activity.beat`
with `{ session_id }` every 15 seconds and immediately on gaining engagement.
It stops on blur, hidden, idle, session switch, unmount, or when the runtime
flag reads off. The session is the one whose chat or panel surface owns focus,
which the existing dockview active-panel state exposes; a task page with
several sessions therefore reports one session at a time (AC 001.4). On phones
every session panel (chat, plan, changes, files, terminal, review, and plugin task
panels) is that surface, and reporting pauses while the task switcher is open. Coarse-pointer
tablet layouts report the session they display through the same hook.

Payload and logs contain no text, key identity, coordinates, or paths (AC 001.5).
Two tabs on one session each beat, and the server merges them (AC 001.3).

## Interval merge

New WebSocket action constant `ActionSessionUserActivityBeat =
"session.user_activity.beat"` in `pkg/websocket/actions.go`. The handler
resolves the actor from the client identity and rejects a session that is
missing or inaccessible with the same not-found response (AC 002.7). The
service ignores any client time and uses its own clock (AC 001.6).

For `(session_id, actor_id)`, inside one transaction that first takes the task
row through `taskCleanupBarrierLocked` (`internal/task/repository/sqlite/task_cleanup_barrier.go`):
PostgreSQL takes a `SELECT ... FOR UPDATE` on the task row and SQLite's single
writer connection serializes the transaction, so two tabs cannot both read the
same latest interval and both insert. The barrier rejects only while a cleanup
job is prepared, pending, running, retry_wait, or waiting_for_clean, and does not
check that the task exists on SQLite. The beat therefore also rejects a task
with `archived_at` set. `ErrTaskCleanupInProgress`, an archived task, and a
missing task all map to the same uniform rejection with counter outcome
`rejected` (AC 002.7). On PostgreSQL the barrier returns a wrapped
`sql.ErrNoRows` for a missing task, so that error maps to the same rejection.

- Find the actor's open (latest) interval. If `now - ended_at < 5s`, return
  without recording (AC 002.4). A coalesced report does not advance `ended_at`,
  so the 45 s test below always measures from the last recorded report.
- If `now - ended_at < 45s`, set `ended_at = now`. The credited span is the
  real gap, bounded by 45 s (AC 002.1, 002.3).
- Otherwise insert `[now, now]` as a new interval and leave the previous one
  closed at its last recorded report (AC 002.2). The first report of an interval
  credits nothing, so measured time under-counts by at most one beat period per
  interval.

Intervals of different actors are summed, giving aggregate person-time; the
Host read documents this (AC 002.6). Total active time is
`SUM(ended_at - started_at)`.

## Data and contracts

Table `task_session_user_activity` (SQLite and Postgres parity, created in the
base schema and migration files that own `task_session_turns`):

| Column | Type | Notes |
| --- | --- | --- |
| `id` | integer PK | monotonic |
| `session_id` | text | FK to `task_sessions`, `ON DELETE CASCADE` |
| `task_id` | text | denormalized for task-scoped reads, FK cascade |
| `actor_id` | text | authenticated user ID; with auth off, the synthetic `userstore.DefaultUserID` |
| `started_at`, `ended_at` | timestamp | server clock, UTC, truncated to whole milliseconds at the repository write boundary |

Index `(session_id, actor_id, ended_at DESC)`. Turning auth on later changes the
actor ID, so one person's earlier intervals and later ones are separate rows and
sum normally. Repository interface additions (task repository):
`RecordUserActivity`, `SumUserActivity(sessionIDs)` returning
`{session_id, active_ms, interval_count, first_at, last_at, activity_measured}`,
`ListUserActivitySessions(sessionIDs, taskIDs, page)` returning selected session
and task IDs in `(started_at, session_id)` order with `PageInfo`, and
`RecordCaptureTransition(effective bool, now)`. The list operation applies
filters before paging so task-ID reads do not materialize an unbounded ID list.

## Enablement and measured state

A runtime flag `features.sessionUserActivity` registered in
`internal/runtimeflags/registry.go` (typed config reader and applier, no
parallel map) with `RestartRequired: true`, `false` in every profile in
`profiles.yaml` including `dev` and `e2e`; the e2e spec enables it through its
environment per `/runtime-feature-flags`. The beat period (15 s) and idle window
(60 s) are constants of the hook and are not tunable; the server's 5 s and 45 s
thresholds use real time, so an end-to-end proof takes real seconds (see
Verification). The browser reads the flag from the boot
payload (`initialState.features`, `internal/backendapp/boot_state.go`), which is
not refetched, and a restart only raises `signalBackendReloadRequired`
(`lib/platform/backend-reload-coordinator.ts`). The hook therefore stops while a
reload is required, and every beat acknowledgement carries `enabled`, so a page
that was open across a disable stops at its next beat (AC 003.1). A tab opened
before an enable and not reloaded does not beat; its time is under-counted, which
the reload alert exists to prevent. The server drops beats when the flag is off.

The repository is built before runtime-flag overrides are applied
(`provideRepositories` precedes `applyStartupRuntimeFlags` in
`internal/backendapp/main.go`), so it cannot read the effective value at
construction. A setter `SetUserActivityCapture(bool)` is called after
`applyStartupRuntimeFlags` and before any session can be created, so the
SQLite override, the documented enable path, is honored.

Measured state needs no write to any session row. Two facts decide it:

1. The stamp. `createTaskSession`, the only production session INSERT that every
   `CreateTaskSession*` variant calls, sets session metadata
   `user_activity_capture: true` in the same row when capture is enabled.
   Earlier sessions, or sessions created while the flag was off, carry no stamp,
   with no backfill (AC 003.2, 003.3, 003.4).
2. Capture gaps. At startup, after the setter, `RecordCaptureTransition` reads
   and updates one `kandev_meta` JSON value `user_activity_capture` containing
   the current state, capture-off gaps, and an optional `unconfirmed_since`
   boot marker. It runs two atomic read-modify-write transactions. The first
   stamps `unconfirmed_since` with the boot time unless an earlier boot's marker
   is still present or no capture state exists yet. The second applies the
   transition and clears the marker.
   An on-to-off transition appends `{off: now}`; off-to-on closes the latest
   open gap with `{on: now}`. If startup is off and no open gap exists,
   off-to-on records no gap. The first startup records state without inventing
   a prior gap. A metadata read or write error fails startup closed: session
   capture remains disabled and session creation adds no stamp.

   A boot whose second transaction never commits ran with capture disabled.
   While its marker remains and capture was last confirmed on, reads treat it
   as an open gap from the marker. The next startup whose second transaction
   commits first appends `{off: marker}` when the state was on, then applies
   its own transition, so an off-to-on startup closes that gap at its own time.
   A boot whose first transaction also fails leaves no durable record, and the
   next startup cannot detect it.

A session is `activity_measured` when it is stamped and no capture gap overlaps
its life. A gap `[off, on)` (an open gap has `on = infinity`) overlaps when
`on > task_sessions.started_at` and
(`task_sessions.completed_at IS NULL` or `completed_at >= off`). `updated_at`
is not used: unrelated session metadata and sibling-session updates can change
it. Session state writers clear `completed_at` when a session resumes, so the
open-session check also detects resumed sessions. The test is computed at read
time; a session that ended before the disable keeps its measured state even if a
later unrelated update changes `updated_at` (AC 003.6). No session-row write,
`updated_at` grace window, restart-recovery change, or per-writer hook is needed.
Intervals are retained (AC 003.4).

## Failure and recovery

A dropped beat shortens an interval by at most the merge gap. A reconnect
resumes beating and merges if within 45 s. A rejected beat (flag off, no access)
is silent in the browser and does not retry. A repository error returns a typed
error and logs without session content.

## Persistence

Cascade delete handles session and task removal (AC 002.5). Rows are small
(one per contiguous engagement span) and are not compacted. No backfill
runs at startup (AC 003.2). The migration is additive and idempotent on both
databases.

## Security

Only authenticated clients with access to the session may beat. The server
never trusts client time, and the stored data holds no content. Plugins read a
projection only through the Host contract and its capability checks.

## Observability

Structured logs at debug for accepted/ignored beats with `session_id` only.
An expvar counter `session_user_activity_beats_total` labelled by `outcome`
(`recorded`, `coalesced`, `rejected`, `disabled`); no session, task, or user
identifier is a label.

## Verification architecture

Service tests drive an injectable clock through the merge matrix (5 s coalesce,
45 s boundary, beats at 0 s, 4 s, and 47 s, concurrent beats, two actors, an
archived task, cleanup in progress, a missing task on both dialects), capture
gap tracking (on to off, off to on, repeated startups), measured computation
with a resumed, an untouched, and a later-created session, and a flag enabled
only through a SQLite override. Repository tests live in `*_postgres_test.go`
files and run only when `KANDEV_TEST_POSTGRES_DSN` names a disposable database,
so Postgres parity is DSN-gated evidence. Hook tests use fake timers for
visibility, focus, idle, session switch, `enabled: false` acknowledgement, and
reload-required. The fixture plugin Playwright test (task 07) enables the flag
through its environment, focuses a real page for about 20 s (two beats credit 15
s), and reads the rollup through the Host; the 60 s idle stop is proven by the
hook's fake-timer test, not end to end.

## Related decisions

- [ADR: Session user-activity ledger](../../../decisions/2026-10-03-session-user-activity-ledger.md)
