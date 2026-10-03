---
created: 2026-10-03
status: in_progress
requirements:
  - REQ-AGENTS-PROFILE-LIST-ORDERING-001
  - REQ-AGENTS-PROFILE-LIST-ORDERING-002
  - REQ-AGENTS-PROFILE-LIST-ORDERING-003
system_design:
  - ../../specs/agents/system-design/profile-list-ordering.md
legacy_specs: []
---

# Implementation Plan: Agent Profile List Ordering

## Overview

Administrators can drag profile rows within an agent on Settings > Agents and
sort every agent's profiles by name. The order is saved on the server and shared
by every client. The backend ships first because the frontend needs the reorder
endpoint, the WebSocket event, and the stored order. The frontend then adds
drag, the sort action, store sync, locales, and E2E coverage.

## Scope

### In scope

- `agent_profiles.sort_order`, the ordered read, and the reorder endpoint and event.
- Drag handles and sortable rows in each installed agent's profile list.
- The Installed agents "Sort by name" action.
- Prepending newly created and duplicated profiles on every client.
- Cross-client sync, including the Settings navigation tree.
- Locale strings in all seven languages and public documentation.

### Out of scope

- Agent-card order, the Dynamic agent's profiles, workspace-scoped (Office)
  profiles, and the Office agents list.
- A persistent sort mode, per-user orders, descending sorts, other sort keys,
  and picker ordering.

## Technical approach

### Backend (Task 01)

- `store/sqlite.go`: `sort_order` in the `CREATE TABLE`, in
  `agent_profiles_new`, behind a `srcHasSortOrder` copy guard, and via
  `r.migrate.Apply("agent_profiles.sort_order", ...)` after the
  `migrateDropModelCheckConstraint` block; `ListAgentProfiles` orders by
  `sort_order ASC, created_at DESC, id ASC`.
- `store/sqlite_profile_order.go`: `ReorderAgentProfiles` (set-equality check,
  per-agent PostgreSQL advisory transaction lock before the first read,
  `sort_order` write, revision upsert in one transaction) and
  `GetAgentProfileOrderSnapshots`, which reads ordered profile rows plus the
  revision map in one read-only transaction. PostgreSQL uses
  `sql.LevelRepeatableRead`; SQLite relies on the WAL snapshot established by
  the transaction's first query. Create `agent_profile_orders` in `sqlite.go`.
- `store/store.go`: add the repository methods; update every `Repository` fake.
  `GetAgent` and `ListAgents` use the snapshot result rather than separate reads.
- `controller/profile_order.go`: `Controller.ReorderAgentProfiles`,
  `ErrProfileOrderStale`, Dynamic-agent rejection; returns saved IDs, revision,
  and changed flag. `controller/agent_crud.go` and `dto/dto.go` add
  `profile_order_revision`; both `GetAgent` and `ListAgents` build profile DTOs
  and revisions from `GetAgentProfileOrderSnapshots`. Tests in
  `controller/agent_crud_snapshot_test.go` give the legacy profile getter
  deliberately different rows and assert each GET path uses the combined
  snapshot rows and revision without calling that getter.
- `handlers/handlers.go` and `handlers/profile_handlers.go`:
  `PUT /agents/:id/profiles/order` with `cfg` and `h.interlock`; the handler
  broadcasts `ws.ActionAgentProfilesReordered` (payload with `revision`) with a
  `//ws:global` comment.
- `pkg/websocket/actions.go`: `ActionAgentProfilesReordered = "agent.profiles.reordered"`.
- Persistence ownership: add `agent_profile_orders` to
  `requiredstores/catalog.go` and its catalog test, plus the fixed
  `storeconformance` adapter coverage. Update the tagged `v0.93.0` upgrade
  manifest with post-upgrade assertions for `agent_profiles.sort_order = 0`
  for both SQLite and PostgreSQL; keep the historical SQL fixtures unchanged.
- Global profile membership changes use the same per-agent transactional lock
  as reorder: create, duplicate, soft-delete, agent deletion with profile
  cascade, and full-row profile updates. A profile deletion locks its candidate
  owner and stable profile ID even when the preliminary row is workspace-scoped,
  then rereads owner and scope under lock and retries if either changed.
- Add deterministic multi-connection SQLite and Postgres tests for create/delete
  and competing A→B versus A→C or workspace/global ownership moves against
  reorder, in both lock acquisition orders. Stale membership rejects with `409`
  and no event; reorder-first commits against the exact set validated. Also
  cover a workspace-profile deletion racing promotion from A to B and reorder
  of B, proving deletion retries under B's lock without disturbing saved order.

### Frontend (Task 02)

- `lib/settings/agent-profile-order.ts`: `sortProfileIdsByName`, `reorderIds`,
  `reorderFlatOptions`, `insertFirstInAgentGroup`.
- `app/actions/agents.ts`: `reorderAgentProfilesAction` with a `stale` result for
  `409` and a 15 s abort timeout.
- Settings slice: per-agent `ProfileOrderSync` (`revision`, `order`, `inFlight`,
  `queued`), `acceptServerOrder`, `reconcileAgentOrders` applied inside
  `setSettingsAgents`, `setAgentProfiles`, and `hydrateSettings`, plus
  `applyAgentListSnapshot(agents, expectedProfileVersion)` as the membership
  freshness fence for fetched lists. It atomically rejects mismatched client
  epochs and reconciles `ProfileOrderSync` before applying accepted results.
  Profile events advance the client-local epoch; route bootstrap retries until
  stable, and browser GET consumers use the guarded action. Order revision
  protects ordering only; profile membership changes do not increment it, and
  order changes do not bump `agentProfiles.version`.
- `lib/settings/profile-order-queue.ts` and `hooks/domains/settings/use-profile-order.ts`:
  per-agent save queue and its thin accessor.
- `lib/ws/handlers/agents.ts`, `lib/types/backend.ts`: handle
  `agent.profiles.reordered` and read `profile_order_revision` from agents; place
  created profiles first within their agent in `handleProfileCreated`,
  `use-profile-duplicate.ts`, `app/settings/agents/[agentId]/agent-save-helpers.ts`,
  and `app/office/setup/agent-profile-setup-controls.tsx`.
- `components/settings/agents/agent-profiles-section.tsx`: sortable rows, drag
  handle.
- `app/settings/agents/page.tsx`: sort button in `InstalledAgentsHeader`.
- `hooks/domains/settings/agent-list-resource.ts` and direct browser
  `listAgents` consumers (`app/settings/agents/page.tsx`,
  `app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.ts`):
  reject a full-list response captured before a profile membership event.
- `src/locales/*/agents.json`: `dragProfile`, `profileSortable`,
  `sortProfilesByName`, `profileOrderSaveFailed`.
- `docs/public/agents-and-profiles.md`: short section on reordering and sorting.

## ASCII UI preview

### UI-01: Settings > Agents, desktop, administrator

Entry: Settings > Agents. Structural requirements: the sort button sits between
Terminal and Rescan, so Rescan stays immediately before the creation action
(`AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.4`); a drag handle leads each profile
row; handles are absent when an agent has fewer than two profiles or the caller
cannot manage agents. Spacing and icons are illustrative.

```text
Installed agents          [Terminal] [Sort by name] [Rescan] [Add TUI agent]
+--------------------------------------------------------------------------+
| Claude                                                                   |
|--------------------------------------------------------------------------|
| [::] * Work profile           [sonnet] [No fallback]        [dup] [del]  |
| [::] * Personal               [opus]   [Next model]         [dup] [del]  |
| [::] * Review (disabled)      [haiku]  [No fallback]        [dup] [del]  |
+--------------------------------------------------------------------------+
| Codex                                                                    |
|   * Default                   [gpt-5]  [No fallback]        [dup] [del]  |
+--------------------------------------------------------------------------+
[::] = drag handle (mouse, touch, keyboard). Codex has one profile: no handle.
Not an administrator: no handles and no Sort by name button.
```

### UI-02: Dragging a row (states)

```text
| [::] * Work profile      ...                                             |
| [##] * Personal  (lifted, follows pointer, drop slot shown below)        |
| - - - - - - - - - - - - - - drop here - - - - - - - - - - - - - - - - - |
| [::] * Review (disabled) ...                                             |
```

Save failure: the rows return to the saved order and an error toast reads
"Could not save the profile order".

### UI-03: Phone

Same composition as UI-01 with these differences: the header actions wrap, the
profile action menu replaces the inline buttons, and the handle has a 44 px
touch target. The handle has `touch-none`, so a touch drag on the handle starts
after 8 px of movement, and the page still scrolls when the touch starts outside
the handle.

```text
Installed agents
[Terminal] [Sort by name]
[Rescan]   [Add TUI agent]
+------------------------------+
| Claude                       |
|------------------------------|
| [::] * Work profile      [:] |
|      [sonnet] [No fallback]  |
| [::] * Personal          [:] |
|      [opus] [Next model]     |
+------------------------------+
```

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-AGENTS-PROFILE-LIST-ORDERING-003.1`, `003.4`, `003.9` | `store/sqlite_profile_order_test.go`: order persists, default order is newest first, `updated_at` unchanged, revision increments only when the order changes |
| `003.5` (server) | same file: a profile created after a reorder lists first and the revision is unchanged |
| `003.6`, `003.10` | same file, `TestReorderAgentProfiles_RejectsMismatchedSet` and deleted/workspace-scoped rows; `controller/profile_order_test.go`, Dynamic agent rejected, DTO carries `profile_order_revision`; `handlers/profile_order_handlers_test.go`, `409` and `400` |
| `003.8` | `handlers/agent_settings_org_scope_test.go` and `handlers/interim_settings_interlock_test.go` (route added to both tables); `handlers/profile_order_handlers_test.go`, `TestReorderProfilesRequiresConfigPermission` |
| `003.2` (server) | `handlers/profile_order_handlers_test.go`, `TestReorderBroadcastsEvent` (payload carries revision), `TestStaleReorderDoesNotBroadcast`, and agent-deletion `404` with no reorder event; no event on unchanged order |
| Migration and schema ownership | `store/sqlite_migration_test.go`: fresh install, same-DB replay, legacy `CHECK(model)` recreation preserving `sort_order`, `agent_profile_orders` created; `store/postgres_schema_test.go`: fresh and replay (DSN-gated); `requiredstores/catalog_test.go` and `storeconformance` assert both order tables; tagged upgrade manifest checks migrated `sort_order` on the pinned SQLite/Postgres fixtures |
| `003.6` serialization | Two-connection SQLite and env-gated Postgres tests exercise both lock winners for global create/duplicate, profile soft-delete, `DeleteAgent` cascade, A→B versus A→C updates (including pre-read A, concurrent A→C commit, retry with sorted locks {B,C}), and workspace→global/global→workspace moves against reorder; assert final membership/order, `409` or `404` with no reorder event when mutation wins, and exact committed order/event when reorder wins. A deterministic race pauses deletion after reading a workspace-scoped profile, promotes it from A to B, commits a reorder of B, and proves deletion retries under B's lock without disturbing saved order |
| `003.12` | `store/profile_order_snapshot_test.go`: file-backed SQLite WAL and DSN-gated Postgres tests call the production snapshot implementation through a package-private after-profile-query barrier, commit a reorder using an independent writer while the read transaction is paused, and assert old-order/old-revision in flight plus new-order/new-revision on the next snapshot. `controller/agent_crud_snapshot_test.go` gives the legacy profile getter different rows and proves both `GET /agents` and `GET /agents/:id` source DTO profiles and revision from the combined snapshot without calling the legacy getter |
| `003.13` | `hooks/domains/settings/agent-list-resource.test.ts`: dispatch profile-created and profile-deleted events after a pre-event list request starts, resolve its stale response at the same order revision, assert it is not applied, the created profile stays first, the deleted profile stays absent, and a fresh response is applied. `app/settings/agents/page.agent-list-snapshot.test.tsx`: trigger the custom-TUI refresh, defer its `listAgents` response, dispatch create/delete events before resolving the old response, and assert both mirrored lists preserve event-known membership. `app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.test.tsx`: trigger the missing-profile fallback, defer its GET, dispatch create/delete events, resolve the pre-event response, and assert neither list loses or resurrects membership. `lib/settings/profile-order-queue.test.ts`: during a deferred `409` refetch, dispatch each event, resolve a pre-event response at the unchanged order revision, assert neither mirrored list is partially replaced, then accept a fresh resource response and replay queued intent with the created profile first and deleted profile absent. `lib/state/slices/settings/settings-slice.test.ts` verifies `applyAgentListSnapshot` atomically rejects a mismatched client epoch. `lib/state/hydration/hydrator.test.ts` verifies pre-event create and delete bootstrap snapshots cannot erase or resurrect event-known membership and leave agent loading incomplete for retry |
| `002.2`, `002.3` | `lib/settings/agent-profile-order.test.ts`: name order, case-insensitive, numeric runs, accents distinct, stability |
| `001.8`, `001.5`, `002.6`, `003.7`, `003.11` | `lib/settings/profile-order-queue.test.ts`: coalescing to latest drag; older-revision snapshots and out-of-order events cannot replace known order; own echo is ignored; higher-revision foreign order beats own `200`; an earlier save failure with a newer queued drag submits the queued order without rollback; on a deferred `409` refetch, create/delete events interleave at unchanged order revision, stale response changes neither list, fresh resource response reconciles and replays queued intent with the new profile first and deleted profile absent; terminal failure without newer intent restores server order; other agents' overlays survive |
| `003.11`, `003.2`, `003.5` (client) | `lib/state/hydration/hydrator.test.ts` preserves known order for older snapshots in both lists; `lib/ws/handlers/agents.test.ts` applies order patches without changing membership, preserving later creates and ignoring deleted IDs, and applies a remote `agent.profile.created` event to a second store after manual order exists, asserting the new profile is first in both `settingsAgents` and its group in `agentProfiles`; `lib/settings/agent-profile-order.test.ts` preserves orphan/newer options and prepends new profiles; duplicate, agent-save-helper, provider-helper, and Office setup tests cover profile creation flows |
| `002.1`, `002.5`, `002.7`, `001.3`, `001.4`, `001.7` | `app/settings/agents/page.sort.test.tsx`: button for administrators only, Dynamic agent excluded, `reorderAgentProfilesAction` not called for an unchanged agent, no handle with one profile or without permission, a drop over another agent's row changes nothing and sends no request |

## E2E tests

`apps/web/e2e/tests/settings/agent-profile-order.spec.ts` (project `chromium`)
and `mobile-agent-profile-order.spec.ts` (project `mobile-chrome`). Both create
dedicated profiles through the API and delete them in cleanup, and restore any
changed order through `PUT /agents/:id/profiles/order`, because the e2e backend
is worker-scoped and never resets global profiles; other specs pick
`profiles[0]` and must not see a leaked order.

- Drag a profile with the pointer and with the keyboard, reload, order persists
  (`AC-AGENTS-PROFILE-LIST-ORDERING-001.1`, `001.2`, `003.1`).
- Clicking a drag handle does not navigate; opening, duplicating, and deleting a
  profile still work while handles are rendered (`001.6`).
- Sort by name (`002.1` to `002.4`).
- A second page sees the reorder without reload, and the navigation tree
  matches (`003.2`, `003.3`).
- A newly created profile appears first without reload (`003.5`).
- A non-administrator sees no handles or sort button (`001.4`), in
  `e2e/tests/auth/agent-profile-order-member.spec.ts` (project `auth`), because
  the `chromium` project runs with authentication disabled and every identity
  is an administrator.
- Mobile: touch drag on the handle using CDP `Input.dispatchTouchEvent`, following
  `e2e/tests/task/mobile-subtask-reparent-drag-drop.spec.ts`, and a touch scroll
  that starts outside the handle and scrolls the page (`001.2`, `002.1`).

Existing `agent-profile-layout.spec.ts` and `mobile-agent-profile-layout.spec.ts`
are updated for the new toolbar button: the expected test IDs become
`open-host-shell`, `sort-profiles-by-name-button`, `rescan-agents-button`,
`new-agent-button`.

## Work orders

- [x] [Task 01: Profile order backend](task-01-profile-order-backend.md)
- [x] [Task 02: Profile reorder and sort UI](task-02-profile-order-ui.md)

## Verification results

Task 01 and Task 02 implementation is complete. Focused backend tests, SQLite
race coverage, SQL guard, backend lint, focused frontend tests, typecheck, web
lint, i18n gates, auth/mobile E2E, and desktop profile ordering/layout E2E
passed. Desktop ordering and layout E2E passed 4/4 after the keyboard test
waited for dnd-kit's drag activation and resolved target announcement.

Full frontend tests reported 2,480 passed and 15 unrelated infrastructure or
timeout failures (including unavailable localhost:3000 and Docker bridge
services). The full backend test command remains blocked by two unchanged
`internal/testutil/envscan_test.go` failures. PostgreSQL DSN-gated migration and
concurrency tests were not run because `KANDEV_TEST_POSTGRES_DSN` was unset.
Plan status remains `in_progress` pending those environment-dependent checks.

Round-five review found workspace-scoped profile deletion could miss the
membership lock when promotion committed between its reads. The deletion path
now locks the candidate owner and stable profile identity, rereads, and retries
when owner or scope changes.

The full store-package race suite and focused regression passed; changed-backend
golangci-lint reported zero issues. The PostgreSQL case was skipped because
`KANDEV_TEST_POSTGRES_DSN` is unset.

## Risks

- Adding repository methods breaks every fake of `store.Repository`; Task 01
  must update them all.
- The `sort_order = 0` default makes new profiles appear first after a manual
  order exists. This matches today's newest-first rule but may surprise a user
  who placed a profile first.
- Code that uses the first profile as a default (Office chat panel, Office setup)
  and pickers that flatten `agent.profiles` follow the saved order.
- Order changes do not bump `agentProfiles.version`, so the agent-list
  resource's cached `response` can carry an older order. Every writer of the
  agent lists, including the hydrator, must reconcile with the known per-agent
  order revision; a writer that bypasses it reintroduces the stale-order bug.
- Each reorder adds a row to `agent_profile_orders` and a field to the agent DTO;
  clients older than the field treat the revision as `0` and keep today's behavior.
- A dnd-kit drag handle inside a row with an overlay link needs the handle above
  the link (`z-10`) so it does not open the profile.
