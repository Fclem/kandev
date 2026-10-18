---
id: "04-activity-turn-reads"
title: "Session activity and turn reads"
status: completed
wave: 2
depends_on:
  - "01-activity-ledger"
  - "03-message-origin"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-EFFORT-SIGNALS-002
  - REQ-PLUGINS-EFFORT-SIGNALS-005
acceptance_criteria:
  - AC-PLUGINS-EFFORT-SIGNALS-002.1
  - AC-PLUGINS-EFFORT-SIGNALS-002.2
  - AC-PLUGINS-EFFORT-SIGNALS-002.3
  - AC-PLUGINS-EFFORT-SIGNALS-002.4
  - AC-PLUGINS-EFFORT-SIGNALS-002.5
  - AC-PLUGINS-EFFORT-SIGNALS-002.6
  - AC-PLUGINS-EFFORT-SIGNALS-005.1
system_design:
  - ../../specs/plugins/system-design/effort-signals-host.md
---

# Task 04: Session activity and turn reads

## Summary

Add `Sessions().UserActivity` and `Sessions().Turns` to the Host `SessionReader`,
gated by a new `api_read: session_activity` resource. Activity projects the
task 01 rollup with read-time measured state; turns list `task_session_turns`
with a keyset cursor.

## In scope

- New read resource `session_activity`: a constant beside the others in
  `internal/plugins/host_data.go`, the `CanRead` gate, the entries in
  `docs/public/plugins-manifest.md`, and a second rule in
  `validateCapabilityMinimumVersions`
  (`internal/plugins/manifest/min_version_policy.go`) keyed on that resource,
  with a constant `MinimumSessionActivityVersion` set to the next unreleased
  version per the release process (task 06 re-verifies it). The manifest package
  has no vocabulary list and the install review renders `read:<resource>`
  generically. Existing `sessions`-only plugins must still validate.
- Move `Sessions()` from a whole-accessor gate to per-method gates: `List` and
  `CodeStats` need `sessions`; `UserActivity` and `Turns` need only
  `session_activity`, each with its own nil-data-source `Unimplemented` fallback
  (`internal/plugins/host_data.go`).
- New `SessionActivityFilter{SessionIDs, TaskIDs}` type with a shared validator
  returning `InvalidArgument` above 400 combined values; existing `SessionFilter`
  and its `List`/`CodeStats` calls are unchanged. The repository first selects
  the filtered session rows (`SessionIDs` or `TaskIDs`) in `(started_at,
  session_id)` order using the supplied page cursor, then computes activity for
  those session IDs. `SumUserActivity` reads capture-gap metadata once, loads
  session `started_at`, `completed_at`, and metadata plus interval aggregates in
  chunks of at most 400 IDs, and evaluates `activity_measured` in Go using
  scanned timestamps. Its session-row-driven left join returns measured
  zero-interval sessions. `UserActivity` and `Turns` use stable keyset
  pagination and return `PageInfo`; the task repository owns resolving `TaskIDs`
  and page ordering, so the Host does not materialize an unbounded ID list.
- Task repository adds filtered, cursor-paged session-row selection for
  `UserActivity` and the existing turn read by session and task filters.

## Out of scope

- Turn metadata, usage, or runtime configuration in the DTO.
- Activity intervals as a read.

## Acceptance

- Not-measured sessions return zeros with `ActivityMeasured=false`; measured
  zero is distinct; multi-actor sessions report summed person-time.
- Turn pages are `(started_at, id)` ordered with stable opaque cursors across
  inserts; filters over 400 values fail `InvalidArgument`.
- A plugin declaring only `sessions` gets `PermissionDenied` naming
  `api_read:session_activity` on the new methods; one declaring only
  `session_activity` can call them; a manifest declaring `session_activity` below
  the floor fails validation. Extend `TestHostSDKProtoParity`.

## Verification

```bash
(cd apps/backend && make proto && go test ./internal/plugins/ ./internal/plugins/manifest/ ./pkg/pluginsdk/ ./internal/task/repository/sqlite/ -run 'TestSessionUserActivityRead|TestSessionTurnsRead|TestSessionActivityFilterBound|TestSessionActivityRequiresResource|TestSessionActivityMinimumVersion|TestHostSDKProtoParity')
(cd apps/backend && make fmt lint)
```

## Files likely touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto` and generated code
- `apps/backend/pkg/pluginsdk/host.go`, `data_types.go`, gRPC server and client
- `apps/backend/internal/plugins/host_data.go`, `host_data_mappers.go`
- `apps/backend/internal/plugins/manifest/min_version_policy.go`
- Task repository session-page and turn reads

## Dependencies

Tasks 01 and 03.

## Risks

- Proto field numbering must append, never reuse.
- Keying the version floor on `sessions` would reject existing plugins; it must
  key on the new resource only.

## Parallelism

`sequential`

## Inputs

- Design sections: Session activity and turns, Compatibility and authorization.
  Pattern: existing `Sessions().CodeStats` and `RequiresMessagesCapabilityMinimum`.

## Results

Completed. Added independent `session_activity` read resource (minimum version 0.97.0, keyed only on that resource) and per-method `Sessions()` gates: `List`/`CodeStats` need `sessions`; `UserActivity`/`Turns` need `session_activity` only, with PermissionDenied naming `api_read:session_activity`. Shared `validateSessionActivityFilter` bounds the combined 400-value filter. Repository does filtered, stable keyset pagination for session selection and turns (`(started_at,session_id)` and `(started_at,id)`); capture state is read once, aggregated in <=400-ID chunks, and measured state is evaluated in Go. Host zeroes totals when unmeasured and preserves measured zero. Turn DTO carries no metadata. Proto fields/RPCs appended only; SDK, gRPC server/client, mappers and bufconn parity updated.

Verification (parent rerun): `make proto`, then `go test -count=1 ./internal/plugins/ ./internal/plugins/manifest/ ./pkg/pluginsdk/ ./internal/task/repository/sqlite/ -run 'TestSessionUserActivityRead|TestSessionTurnsRead|TestSessionActivityFilterBound|TestSessionActivityRequiresResource|TestSessionActivityMinimumVersion|TestHostSDKProtoParity'` passed. Child reports: `make fmt lint` 0 issues; `go test ./internal/plugins/... ./pkg/pluginsdk/...` passed; public-doc validation 62/62 and 47 pages valid; standalone smoke program (removed) passed. PostgreSQL tests NOT RUN: `KANDEV_TEST_POSTGRES_DSN` unset.
