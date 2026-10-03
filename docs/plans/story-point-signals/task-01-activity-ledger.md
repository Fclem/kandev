---
id: "01-activity-ledger"
title: "Session user-activity ledger"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-USER-ACTIVITY-001
  - REQ-TASKS-USER-ACTIVITY-002
  - REQ-TASKS-USER-ACTIVITY-003
acceptance_criteria:
  - AC-TASKS-USER-ACTIVITY-001.3
  - AC-TASKS-USER-ACTIVITY-001.6
  - AC-TASKS-USER-ACTIVITY-002.1
  - AC-TASKS-USER-ACTIVITY-002.2
  - AC-TASKS-USER-ACTIVITY-002.3
  - AC-TASKS-USER-ACTIVITY-002.4
  - AC-TASKS-USER-ACTIVITY-002.5
  - AC-TASKS-USER-ACTIVITY-002.6
  - AC-TASKS-USER-ACTIVITY-002.7
  - AC-TASKS-USER-ACTIVITY-003.1
  - AC-TASKS-USER-ACTIVITY-003.2
  - AC-TASKS-USER-ACTIVITY-003.3
  - AC-TASKS-USER-ACTIVITY-003.4
  - AC-TASKS-USER-ACTIVITY-003.5
  - AC-TASKS-USER-ACTIVITY-003.6
system_design:
  - ../../specs/tasks/system-design/session-user-activity.md
---

# Task 01: Session user-activity ledger

## Summary

Add the `task_session_user_activity` table, the service merge rules, the
`session.user_activity.beat` WebSocket action, the capture stamp, and the
restart-required `features.sessionUserActivity` runtime flag. After this order a
client can beat and the server stores merged intervals; no browser reporter
exists yet.

## In scope

- Table, indexes, and migration on SQLite and Postgres (one shared schema in
  `base_schema.go` with driver-aware SQL).
- `RecordUserActivity` and `SumUserActivity` on the task repository and service;
  the beat transaction takes `taskCleanupBarrierLocked`
  (`task_cleanup_barrier.go`) first, so PostgreSQL serializes concurrent beats
  with a task-row lock. The beat also rejects an archived task, and maps
  `ErrTaskCleanupInProgress`, archived, and missing tasks to the one uniform
  rejection with outcome `rejected`. Each acknowledgement carries `enabled`.
- Gateway handler performs one authorization-and-existence check and returns
  the identical not-found status, message, and outcome for missing, inaccessible,
  archived, cleanup-in-progress, and PostgreSQL missing-task cases. Do not reuse
  `maySubscribeSession`'s forbidden response or auth-off success path. Tests
  compare complete acknowledgement/error payloads for all cases, including a
  missing session with auth disabled.
- Stamp `user_activity_capture: true` inside the shared `createTaskSession`
  insert when capture is enabled; confirm every `CreateTaskSession*` variant goes
  through it and add a test that names each variant.
- `RecordCaptureTransition(effective, now)` at startup after the setter. Store
  state and capture-off gaps in one `kandev_meta` JSON value, updated through one
  read-modify-write transaction and upsert. Off-to-on with no open gap is a
  no-op; first-run-off records state but invents no gap. If reading or writing
  fails, startup fails closed and session stamps/beats remain disabled.
- Compute `activity_measured` at read time in `SumUserActivity`: stamped and no
  gap `[off, on)` with `on > started_at` and (`completed_at` null or
  `completed_at >= off`). Never use `updated_at`; test that a later sibling
  `SetSessionPrimary` update does not change the measured state of an ended
  session, and that resuming via each state-update/CAS path makes a session
  overlapping a gap not measured.
- `SetUserActivityCapture(bool)` on the repository, called after
  `applyStartupRuntimeFlags` and before any session can be created, because
  `provideRepositories` runs first in `internal/backendapp/main.go`.
- Runtime flag via `/runtime-feature-flags`: `RestartRequired: true`, `false` in
  every `profiles.yaml` profile, typed `FeaturesConfig` field, and the frontend
  flag type and contract test. The beat period and idle window stay hook
  constants; no e2e tuning knobs.
- Public docs: the flag row in `docs/public/configuration.md`
  (`features.sessionUserActivity`, `KANDEV_FEATURES_SESSION_USER_ACTIVITY`,
  restart-required, off) and the action in `docs/public/websocket-api.md`
  (`session.user_activity.beat`, payload `{session_id}`, acknowledgement
  `{enabled}`). Follow the docs-maintainer requirements for config and WebSocket
  contract changes; `coverage.json` is an inventory, not a requirement to edit
  documentation when these source files change.
- `session_user_activity_beats_total` counter.

## Out of scope

- Browser reporter (task 02) and Host reads (task 04).
- Enabling the flag in `prod`.

## Acceptance

- Merge matrix passes with an injectable clock: beats at 0 s, 4 s, 47 s open a
  new interval at 47 s; under 5 s coalesces without recording; under 45 s
  extends; concurrent beats do not overlap (SQLite and, when a DSN is set,
  Postgres).
- A beat for an unknown or inaccessible session gets one identical rejection;
  flag off stores nothing. `TestRecordUserActivityDisabledStoresNothing`
  verifies the server does not store an arriving report when disabled.
- A capture gap makes sessions alive during or after it not measured: a resumed
  session and one still open at disable are not measured; an ended session
  remains measured after sibling promotion or unrelated updates; one created
  after re-enable stays measured. Repeated startup transitions add no duplicate
  gap; first-run-off then on creates no gap; transition persistence is atomic
  under injected write failures; `updated_at` is never changed.
- A flag enabled only through a SQLite override stamps new sessions.
- A missing task on PostgreSQL (`sql.ErrNoRows`) gets the same rejection as on
  SQLite.

## Verification

(cd apps/backend && go test ./internal/task/service/ ./internal/task/repository/sqlite/ ./internal/gateway/websocket/ ./internal/backendapp/ -run 'UserActivity|CreateTaskSessionStamps|CaptureGap|CaptureTransition|SessionMeasured|FlagOverrideStamps|BeatRejects')
(cd apps/backend && go test ./internal/runtimeflags/ ./internal/common/config/ ./internal/profiles/)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="${KANDEV_TEST_POSTGRES_DSN:?set a disposable Postgres DSN}" go test ./internal/task/repository/sqlite/ -run 'Postgres.*UserActivity|UserActivity.*Postgres')
(cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint)
node scripts/validate-public-docs.mjs
(cd apps/backend && make fmt lint)
git diff --check -- docs
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_schema.go`, `base_migrations.go`, `session.go` (`createTaskSession`), new `user_activity.go`
- `apps/backend/internal/task/service/` (new `user_activity.go`)
- `apps/backend/pkg/websocket/actions.go`, `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/runtimeflags/registry.go`, `profiles.yaml`, `internal/common/config/config.go` (`FeaturesConfig`), `internal/profiles/` tests that pin profile values
- `apps/web/lib/state/slices/features/types.ts` and the boot payload mapping
- `docs/public/configuration.md`, `docs/public/websocket-api.md`

## Dependencies

None.

## Risks

- Beat rate is one per 15 s per focused session; the handler must stay off the
  hot WebSocket path's locks.
- Postgres tests skip without a DSN; record a skip as not-run, not as passing.

## Parallelism

`sequential`

## Inputs

- Design sections: Interval merge, Data and contracts, Enablement and measured state.
- Pattern: `commitCaptureActivatedAtMetaKey` migration in `base_migrations.go`;
  existing `session.subscribe` handler in `internal/gateway/websocket/client.go`;
  `SetSessionMetadataKeyIfAbsent` in `session.go`.

## Results

Pending.
