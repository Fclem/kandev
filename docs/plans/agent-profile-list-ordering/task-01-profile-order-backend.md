---
id: "01-profile-order-backend"
title: "Profile order backend"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-LIST-ORDERING-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.1
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.2
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.4
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.5
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.6
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.8
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.9
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.10
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.12
system_design:
  - ../../specs/agents/system-design/profile-list-ordering.md
---

# Task 01: Profile order backend

## Summary

Store a per-agent profile order, serve profiles in that order, and add the
permission-gated reorder endpoint with its WebSocket event.

## In scope

- `agent_profiles.sort_order` in the `CREATE TABLE`, the table-recreation path
  (`agent_profiles_new`, `srcHasSortOrder` copy), and an `Apply` migration;
  ordered `ListAgentProfiles`.
- Create `agent_profile_orders`; ensure the `agent-settings` required-store
  descriptor lists `agents`, `agent_profiles`, and `agent_profile_orders`, and
  the fixed conformance adapter verifies the schema.
- `Repository.ReorderAgentProfiles` (transactional, set-equality check, no
  `updated_at` write, revision upsert in `agent_profile_orders`) and
  `GetAgentProfileOrderSnapshots`, which returns each requested agent's ordered
  profile rows and revision from one read-only transaction. PostgreSQL uses
  `sql.LevelRepeatableRead`; SQLite relies on the WAL snapshot established by
  the transaction's first query. Update every `Repository` fake.
- Serialize Postgres reorders with a transaction-scoped advisory lock keyed by
  namespace and agent ID before the first membership read; retain SQLite's
  single-writer serialization.
- Apply the same per-agent lock in the transaction for global profile creates,
  duplicates, soft-deletes, and agent deletion with profile cascade. Full-row
  updates (`UpdateAgentProfile` and `UpdateAgentProfileWithDynamic`) also take
  a profile-ID lock, plus sorted locks for known global source/target agents;
  reread ownership under lock and rollback/retry if the actual global owner is
  not covered by the held lock set.
- `Controller.ReorderAgentProfiles`, `ErrProfileOrderStale`, Dynamic-agent
  rejection; the handler broadcasts the event.
- `PUT /api/v1/agents/:id/profiles/order` and `ActionAgentProfilesReordered`.

## Out of scope

- Any web code. Agent-card order. Workspace-scoped profiles.

## Acceptance

- A reorder persists, is returned by `GET /agents` in that order with an
  increasing `profile_order_revision`; both `GET /agents` and `GET /agents/:id`
  pair ordered profile rows and revision from one consistent database snapshot.
  Controller regression tests give the legacy `ListAgentProfiles` getter
  deliberately different rows and assert both GET paths use the combined
  snapshot rows and revision without calling the legacy getter. Later-created
  profiles list first, an untouched install keeps newest first, and reorder
  leaves `updated_at` and configuration unchanged. Fresh and replay migrations,
  legacy `CHECK(model)` table recreation, Postgres, and the tagged `v0.93.0`
  upgrade fixture work; required-store/conformance checks assert `agents`,
  `agent_profiles`, and `agent_profile_orders`, and the upgrade manifest verifies
  legacy `sort_order = 0`.
- A request that is not exactly the agent's global profiles returns `409
  profile_order_stale`, a request for the Dynamic agent returns `400
  profile_order_unsupported`, both change nothing; a caller without
  agent-configuration permission is refused; a changed order broadcasts
  `agent.profiles.reordered` from the handler and an unchanged order does not.
- Deterministic two-connection SQLite and Postgres tests exercise both lock
  winners for global create, `DuplicateAgentProfile`, soft-delete, and
  `DeleteAgent` cascade against reorder; assert final membership/order and
  HTTP/event outcomes, including `404` and no reorder event when agent deletion
  wins. Ownership-update tests race A→B against A→C and force the stale-owner
  path (pre-read A, concurrent A→C commit, retry with sorted locks {B,C}), then
  race workspace→global and global→workspace moves against reorder in both lock
  orders. Each scenario asserts final order/membership; mutation-first returns
  `409` or `404` as applicable and emits no reorder event; reorder-first returns
  success and broadcasts only the exact locked set.
- Snapshot race tests use a package-private after-profile-query barrier in the
  same private implementation called by the production repository method. The
  SQLite case uses a file-backed WAL database with independent reader and writer
  connections (not `:memory:`): while the reader transaction is paused after
  its first query, the writer commits a reorder; the active result must be
  old-order/old-revision and the next snapshot new-order/new-revision. The
  Postgres case uses two connections and `sql.LevelRepeatableRead` for the same
  old/old then new/new assertions.

## Verification

```bash
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/agent/settings/store -run '^TestPostgres' -count=1)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
make -C apps/backend test
make -C apps/backend lint
```

The Postgres line needs `KANDEV_TEST_POSTGRES_DSN`; record when it is unavailable.

Every DSN-gated Postgres test in this package uses the `TestPostgres` name
prefix so `-run '^TestPostgres'` exercises schema and concurrency coverage.

## Files likely touched

- `apps/backend/internal/agent/settings/store/sqlite.go`
- `apps/backend/internal/agent/settings/store/sqlite_profile_order.go`
- `apps/backend/internal/agent/settings/store/store.go`
- `apps/backend/internal/agent/settings/store/sqlite_profile_order_test.go`
- `apps/backend/internal/agent/settings/store/sqlite_migration_test.go`
- `apps/backend/internal/agent/settings/store/postgres_schema_test.go`
- `apps/backend/internal/agent/settings/controller/profile_order.go`
- `apps/backend/internal/agent/settings/controller/profile_order_test.go`
- `apps/backend/internal/agent/settings/store/postgres_profile_order_concurrency_test.go`
- `apps/backend/internal/agent/settings/store/profile_order_snapshot_test.go`
- `apps/backend/internal/agent/settings/controller/agent_crud_snapshot_test.go`
- `apps/backend/internal/agent/settings/controller/reconciler_test.go` (`fakeStore`)
- `apps/backend/internal/agent/settings/store/sqlite_profile_order_concurrency_test.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/profile_handlers.go`
- `apps/backend/internal/agent/settings/dto/dto.go` (`profile_order_revision`)
- `apps/backend/internal/agent/settings/controller/agent_crud.go`
- `apps/backend/internal/agent/settings/handlers/profile_order_handlers_test.go`
- `apps/backend/internal/agent/settings/handlers/agent_settings_org_scope_test.go` (route table)
- `apps/backend/internal/agent/settings/handlers/interim_settings_interlock_test.go` (route table)
- `apps/backend/internal/agent/settings/handlers/profile_duplicate_handlers_test.go` (`fakeSettingsRepo`)
- `apps/backend/internal/agent/runtime/lifecycle/profile_resolver_test.go` (`MockRepository`)
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/persistence/requiredstores/catalog.go`
- `apps/backend/internal/persistence/requiredstores/catalog_test.go`
- `apps/backend/internal/persistence/storeconformance/adapters.go`
- `apps/backend/internal/persistence/storeconformance/upgrade_test.go`
- `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/manifest.json`
- SQLite and Postgres tagged-upgrade/conformance fixture expectations

## Dependencies

None.

## Risks

- Other `store.Repository` fakes fail to compile; find them with an LSP
  references lookup on the interface.
- The table-recreation copy must include `sort_order` or a legacy upgrade resets
  every order.

## Parallelism

`sequential`

## Inputs

- System design sections Data and contracts, Persistence, Security.
- `apps/backend/AGENTS.md` table-rebuild migration rule.
- `controller/agent_crud.go` (`filterGlobalProfiles`), `controller.go`
  (`broadcastProfileUpdated`) for the broadcast pattern, and
  `profile_duplicate_handlers_test.go` for handler test setup.

## Results

Implemented the schema, snapshot reads, reorder endpoint/event, ownership locks,
and concurrent membership/reorder coverage.

Scoped store, controller, handler, required-store, and store-conformance tests,
SQLite race tests, SQL guard, and backend lint passed. `make -C apps/backend
test` remains blocked by two failures in the unchanged
`internal/testutil/envscan_test.go` environment-scan tests; the expected
diagnostics were absent. `KANDEV_TEST_POSTGRES_DSN` was unset, so the
DSN-gated PostgreSQL migration and concurrency tests were not exercised.
