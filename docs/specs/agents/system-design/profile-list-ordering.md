---
status: draft
system: agents
requirements:
  - REQ-AGENTS-PROFILE-LIST-ORDERING-001
  - REQ-AGENTS-PROFILE-LIST-ORDERING-002
  - REQ-AGENTS-PROFILE-LIST-ORDERING-003
---

# Agent Profile List Ordering System Design

## Purpose and boundaries

The agent settings store owns one saved profile order per agent and a
monotonic revision of it. The settings controller validates and applies a
reorder, the HTTP handler exposes it behind the existing agent-configuration
permission and broadcasts a WebSocket event, and clients accept an order only
when its revision is newer than the one they already hold. The Settings > Agents
page owns drag interaction and the sort action. The sort action is client-side:
it computes an order and saves it through the same reorder contract, so the
backend has no sort logic.

Agent-card order stays with `sortAgentsByDisplayOrder` in the settings
controller and `orderAgentsForDisplay` in the web app. The Dynamic agent
(`agents.DynamicAgentID`, name `dynamic`) is excluded.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-PROFILE-LIST-ORDERING-001` | [Frontend](#frontend), [Save coordination](#save-coordination) |
| `REQ-AGENTS-PROFILE-LIST-ORDERING-002` | [Sort action](#sort-action) |
| `REQ-AGENTS-PROFILE-LIST-ORDERING-003` | [HTTP](#http), [Persistence](#persistence), [Order revisions](#order-revisions), [Store updates](#store-updates), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `store.Repository` gains `ReorderAgentProfiles(ctx, agentID, orderedIDs)`,
  returning the resulting revision and whether the order changed, plus
  `GetAgentProfileOrderSnapshots(ctx, agentIDs)` for snapshot reads. It returns
  each requested agent's ordered profile rows and revision together. The method
  lives in a new `store/sqlite_profile_order.go` (`sqlite.go` is far over the
  file-length limit). It reads requested profile rows and the revision map in
  one read-only transaction. PostgreSQL uses `sql.LevelRepeatableRead`; SQLite
  correctness relies on the read transaction retaining its WAL snapshot from
  the first query, not on `TxOptions` (the SQLite driver ignores its isolation
  setting). `GetAgent` and `ListAgents` use this result rather than separate
  profile and revision reads. Thus a concurrent reorder yields the pre-commit
  rows with the pre-commit revision or the committed rows with the committed
  revision, never a mixed pair. The reorder runs in one transaction, takes a
  PostgreSQL advisory lock via
  `pg_advisory_xact_lock(hashtextextended('agent-profile-order:' || agentID, 0))`
  before reading profile membership, then reads the agent's non-deleted global
  profile IDs (`workspace_id = ''`) in list order, requires set equality with
  `orderedIDs`, and, only when the sequence differs, writes
  `sort_order = index + 1` and increments the agent's revision. On SQLite, it
  reserves the single writer before reading membership with a no-op update of
  the owning `agents` row. It never writes profile `updated_at`.
- Global-membership writes acquire the same per-agent serialization lock in
  their transaction: `CreateAgentProfile`, `DuplicateAgentProfile`,
  `DeleteAgentProfile`, and `DeleteAgent` (which cascades profiles).
  `UpdateAgentProfile` and `UpdateAgentProfileWithDynamic` are full-row writes
  of `agent_id` and `workspace_id`; each acquires a stable per-profile advisory
  lock, plus per-agent locks for any known global source/target, even when the
  preliminary read suggests no membership change. Acquire all per-agent keys in
  sorted order before the per-profile key, then reread the source row under the
  transaction. If its current global owner is not covered by a held per-agent
  lock, roll back and retry with the newly observed owner; never write under a
  stale lock set. The per-profile lock also serializes competing promotions
  from a workspace-scoped row, which have no shared old-agent key. Global
  creates and duplicates know the target agent ID; deletion resolves the
  candidate owner, acquires the same lock order, and rechecks owner and scope
  before its soft-delete. A changed owner invalidates the preliminary lookup
  and retries from a fresh transaction. PostgreSQL uses
  `pg_advisory_xact_lock(hashtextextended('agent-profile-membership:' || profileID, 0))`
  for the per-profile lock. SQLite reserves its single writer before reading
  membership with a no-op update of the owning `agents` row inside the
  transaction; membership changes use the same write-reservation step.
- `ListAgentProfiles` orders by `sort_order ASC, created_at DESC, id ASC`.
  `filterGlobalProfiles` in `controller/agent_crud.go` still filters
  workspace-scoped rows; it preserves input order.
- `Controller.ReorderAgentProfiles` resolves the agent, rejects the Dynamic
  agent, calls the repository, and maps a set mismatch to `ErrProfileOrderStale`.
  `GetAgent` and `ListAgents` set `profile_order_revision` and profiles from the
  same `GetAgentProfileOrderSnapshots` result. Controller regression tests make
  `ListAgentProfiles` return deliberately different rows and verify both GET
  paths use the snapshot rows and revision without calling that legacy getter.
- `handlers.httpReorderAgentProfiles` binds the body, maps errors, and, when the
  order changed, broadcasts the event with `h.hub.Broadcast`, next to
  `broadcastProfileEvent`, with a `//ws:global` comment: only global profiles are
  ever reordered. The route uses the same `cfg` permission and `h.interlock`
  middleware as `POST /agents/:id/profiles`.
- The web client adds `reorderAgentProfilesAction(agentId, profileIds)` beside
  the other `*Action` exports in `app/actions/agents.ts`, returning
  `ok`, `stale`, or `error`, with a 15 s abort timeout that takes the `error`
  path.
- `lib/settings/agent-profile-order.ts` holds the pure helpers:
  `sortProfileIdsByName(profiles, locale)`, `reorderIds(ids, activeId, overId)`,
  `reorderFlatOptions(options, agentId, ids)`, and `insertFirstInAgentGroup(options, agentId, option)`.
- The settings slice owns the order state and the save queue, see
  [Store updates](#store-updates) and [Save coordination](#save-coordination).
  `lib/settings/profile-order-queue.ts` holds the queue functions, which take
  the store, so the WebSocket handler, the page, and every list use one queue.
- `hooks/domains/settings/use-profile-order.ts` is a thin accessor over the
  queue, mounted once on the Settings > Agents page and passed down.
- `AgentProfilesSubList` in `components/settings/agents/agent-profiles-section.tsx`
  wraps rows in a dnd-kit `DndContext` and `SortableContext`, following
  `components/task/sidebar-filter/automatic-color-rule-list.tsx`. The sortable
  row is a separate component so `ProfileRowCard` does not grow.
- `InstalledAgentsHeader` in `app/settings/agents/page.tsx` hosts the sort
  button. The page passes it only the rendered installed-agent cards, which
  exclude the Dynamic agent.
- `lib/ws/handlers/agents.ts` handles the new event.

## Data and contracts

### Persistence column and table

```sql
ALTER TABLE agent_profiles ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS agent_profile_orders (
    agent_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
```

`sort_order INTEGER NOT NULL DEFAULT 0` is added in four places in
`store/sqlite.go`, like `command_prefix` and the columns after it:

1. The `agent_profiles` `CREATE TABLE`.
2. The `CREATE TABLE agent_profiles_new` statement in
   `recreateAgentProfilesWithoutModelCheck`.
3. A `srcHasSortOrder := columnExists(...)` guard that appends the column to
   `srcCols` and `dstCols` of its copy statement.
4. An `r.migrate.Apply("agent_profiles.sort_order", ...)` after the
   `migrateDropModelCheckConstraint` block, which upgrades existing databases
   and is a tolerated duplicate on fresh ones.

`agent_profile_orders` is created with the other `CREATE TABLE IF NOT EXISTS`
statements. A missing row means revision `0`. The reorder transaction upserts
the row with `revision = revision + 1`; the primary key makes the increment
atomic per agent. The table is listed in the `agent-settings` required-store
descriptor and verified by its fixed conformance adapter. The explicit-tag
`v0.93.0` upgrade manifest asserts that existing profiles receive
`sort_order = 0` on SQLite and Postgres; the historical SQL fixture itself
remains unchanged.

All existing profile rows keep `sort_order = 0`, so the first read orders by
`created_at DESC` exactly as today. A reorder writes positions `1..n` to global
rows only. A profile created or duplicated later has `0` and sorts first, which
matches newest-first. Workspace-scoped rows keep `0`. Creating or deleting a
profile does not change the revision.

### HTTP

`PUT /api/v1/agents/:id/profiles/order`, body `{ "profile_ids": ["..."] }`.

| Status | Condition |
| --- | --- |
| `200` | Saved or already equal. Body `{ "agent_id": "...", "profile_ids": [...], "revision": 7 }` |
| `400` | Malformed body, empty list, an ID longer than 255 bytes, or the Dynamic agent (`code: "profile_order_unsupported"`) |
| `404` | Agent not found |
| `409` | `{ "code": "profile_order_stale" }`: list is not exactly the agent's current global profiles |
| `403` | Caller lacks agent-configuration permission |

A request equal to the stored order returns `200` with the current revision and
without a write or event. `GET /agents` and `GET /agents/:id` carry
`profile_order_revision` on each agent. The route is added to
`mutatingSettingsRoutes` in `handlers/agent_settings_org_scope_test.go` and to
the interlock table in `handlers/interim_settings_interlock_test.go`.

### WebSocket

`agent.profiles.reordered` with
`{ "agent_id": "...", "profile_ids": [...], "revision": 7 }`. The action
constant lives beside `ActionAgentProfileUpdated` in `pkg/websocket/actions.go`,
and the payload type is added to `BackendMessageMap` in
`apps/web/lib/types/backend.ts`. The IDs describe membership at the reorder
transaction's commit and are an order patch, not an authoritative profile set:
clients reorder IDs present in the current agent group, ignore IDs no longer
present, and preserve current profiles omitted from the event. Delivery order is
not relied on: broadcasts can interleave with create/delete events and reorder
events can arrive out of commit order; revision comparison applies to reorder
events.

## Control flow

### Order revisions

The slice holds, per agent, `ProfileOrderSync`:
`{ revision, order, inFlight, queued }`: the newest server order and revision
this client knows (`order` is `null` until one is known) and the pending
optimistic orders.

`acceptServerOrder(agentId, ids, revision)` stores the order and revision only
when `revision` is greater than the stored one, or when no order is stored yet,
and returns whether it did. Every source of a server order goes through it:
the `200` body, the event, and the `profile_order_revision` on each agent of a
fetched or hydrated snapshot. A snapshot with a revision not newer than the
stored one does not replace the known order.

The repository snapshot method reads ordered profile rows and their revision
map in one read-only transaction. PostgreSQL uses `sql.LevelRepeatableRead`.
SQLite's driver ignores the requested isolation enum; correctness comes from a
file-backed WAL read transaction retaining its snapshot from the first query
while the independent writer commits. A deterministic two-connection test
uses a package-private after-profile-query barrier in the same private
implementation called by the production method. It commits a reorder on the
writer while the read transaction is paused, then proves the in-flight result
is O/r and the next snapshot is O2/r+1 on SQLite and PostgreSQL. The test must
exercise the repository implementation rather than repeat its SQL reads.

`reconcileAgentOrders(agents, sync)` is the pure function every list writer
uses. For each agent it first calls the acceptance rule, then returns the
agent's profiles sorted by `inFlight`/`queued` (the overlay) if present, else by
the known `order`, else unchanged. Profiles the order does not name keep their
relative place at the front, matching the backend's sort for new profiles.
`reorderFlatOptions` applies the same order to the flat list.

### Store updates

- `setSettingsAgents` and `setAgentProfiles` reconcile order overlays; they do
  not establish membership freshness. `profile_order_revision` protects order
  only and is unchanged by create/delete.
- `agentProfiles.version` is the client-local profile snapshot epoch, distinct
  from the backend order revision. The profile created, updated, and deleted
  WebSocket handlers advance it. Every browser `GET /agents` result written
  directly to the live store captures the epoch before the request and applies
  both `settingsAgents` and `agentProfiles` atomically through
  `applyAgentListSnapshot(agents, epoch)`. The action rejects the result if the
  current epoch differs; otherwise it reconciles `ProfileOrderSync` ordering
  before committing both slices. A rejected result is discarded and the caller
  uses a fresh resource read rather than writing either list.
- `AgentListResourceScope` captures the epoch at request start, rejects and
  retries a response if profile events advanced it while the request was in
  flight, and keys its cached response by that epoch. Direct browser list
  requests, including `handleCreateCustomTUI` and the missing-profile fallback
  in `use-agent-profile-settings.ts`, use the same guarded action rather than
  writing raw response arrays. `loadSettingsInitialState` repeats its complete
  read until the epoch is stable; `hydrateSettings` rejects an older incoming
  epoch, preserves the live membership in both slices, and leaves
  `settingsData.agentsLoaded` false so the list is retried.
- The order revision acceptance rule cannot prevent a stale snapshot from
  deleting or resurrecting membership. `reconcileAgentOrders` only reconciles
  ordering; the epoch fence above handles membership freshness.
- `setAgentProfileOrder(agentId, ids)` reorders that agent's entries in
  `settingsAgents` and, in place, within the agent's group in the flat
  `agentProfiles` list (`reorderFlatOptions`). It does not rebuild the flat list
  from `settingsAgents`, so orphan options (profiles delivered for an agent
  missing from `settingsAgents`) and per-ID newest options kept by
  `mergeOptionsByNewest` are preserved.
- The `agent.profile.created` handler, `applyProfileDuplicated` in
  `hooks/domains/settings/use-profile-duplicate.ts`, and the writers in
  `app/settings/agents/[agentId]/agent-save-helpers.ts` (`saveNewAgent`,
  `saveExistingProfiles`, `reconcilePartialProfileSave`) place a created profile
  first within its own agent: in `settingsAgents` and, with
  `insertFirstInAgentGroup`, before the agent's first entry of the flat list (or
  at the end when the agent has none). The agent-save reconcile keeps the store's
  current order for existing profiles instead of the draft's order. The Office
  setup writer `app/office/setup/agent-profile-setup-controls.tsx`, which upserts
  only the flat list, uses `insertFirstInAgentGroup` on the flat list (and
  `settingsAgents` when that agent exists), so the wizard pickers still contain
  the profile it just created. `components/agent/cli-profile-editor.tsx` returns
  the profile to its caller and writes no store.

### Save coordination

All per-agent state lives in the slice, so the page, the sort button, every
`AgentProfilesSubList`, and the WebSocket handler share it.

1. `requestProfileOrder(agentId, ids)` sets the optimistic overlay and applies
   `setAgentProfileOrder`.
2. If nothing is in flight for the agent, it moves `ids` to `inFlight` and sends
   the PUT. Otherwise it replaces `queued`, so a drag and a sort of one agent
   never overlap and the last request wins.
3. Events and snapshots are never suppressed: they update the known server order
   through `acceptServerOrder`, and the overlay keeps masking it while an intent
   is pending. Own echoes carry the revision the `200` returns and are therefore
   no-ops.
4. On `200`, `acceptServerOrder(agentId, ids, revision)` runs, `inFlight` is
   cleared, and the next PUT starts from `queued` if set. When both are clear the
   overlay is gone and the known order shows. A foreign order with a higher
   revision that arrived meanwhile is already the known order, so it wins over
   the own `200`.
5. On network, `5xx`, or timeout failure, if a newer `queued` intent exists,
   promote it to `inFlight` and submit it without clearing the optimistic
   overlay. Show an error for the failed save, but do not roll back the newer
   intent. If there is no queued intent, clear the overlay and show the known
   server order with an error message.
6. On `409`, show an error for the stale save and capture the current
   `agentProfiles.version` before refetching `listAgents({ cache: "no-store" })`.
   Retain a newer queued intent and its overlay while refetching. Apply the
   response only through `applyAgentListSnapshot`; if a create/delete event
   advanced the epoch during the request, discard the response without changing
   either list and obtain a fresh result through `AgentListResourceScope`. Only
   after an accepted snapshot, reconcile against the latest membership in the
   store: place newly present IDs first in their refreshed server order, then
   retain queued IDs that still exist in their queued relative order. This
   keeps a profile created after the queued drag first and excludes deleted
   IDs. Submit the reconciled intent after the refetch, using the latest queued
   value if a further drag arrived meanwhile.
   If no newer intent exists, clear the overlay and show the
   accepted server order. A refetch failure clears the overlay and shows the
   known server order with an error. Other agents' pending overlays and queues
   remain independent.

Other clients, and this client while idle, apply the event through
`acceptServerOrder` and `setAgentProfileOrder`. `setAgentProfileOrder` treats
event IDs as an order patch over current membership, preserving profiles added
after the reorder commit and ignoring IDs already deleted. The Settings
navigation tree (`use-settings-menu-branches.ts`) reads `settingsAgents`, so it
follows without extra code.

### Sort action

`sortProfileIdsByName(profiles, locale)` returns the IDs sorted by
`profile.name` with `new Intl.Collator(locale, { sensitivity: "accent", numeric: true })`
built per call from the active i18n language. `accent` ignores case but keeps
accented letters distinct. `Array.prototype.sort` is stable, so equal names keep
their order. The button runs it for every rendered installed-agent card with two
or more profiles, skips agents whose ID list is unchanged (no request), and
submits the rest through `requestProfileOrder`, one agent each.

### Frontend

- Each `ProfileRowCard` gets a handle button (`IconGripVertical`) with
  `data-testid="agent-profile-drag-handle"`, `touch-none`, a 44 px touch-sized
  hitbox, an `aria-label` from `agents:dragProfile` with `{{name}}`, and the
  dnd-kit `aria-roledescription` overridden with `agents:profileSortable`
  (the repo pattern in `automatic-color-rule-card.tsx`). The handle is the only
  drag activator (`setActivatorNodeRef`) and sits in the `z-10` action layer
  above the row's overlay link, so row links and action buttons stay clickable.
  Handles render only when `canManage` is true and the agent has two or more
  profiles.
- Sensors: `PointerSensor` with `distance: 8` and `KeyboardSensor` with
  `sortableKeyboardCoordinates`. The handle uses `touch-none`, so a touch drag on
  the handle arrives as pointer events and starts after 8 px of movement; page
  scroll outside the handle is unaffected. No `TouchSensor` is registered.
- Drag is restricted to one agent by giving each `AgentProfilesSubList` its own
  `DndContext`; a drop over another agent's row has no `over` target and changes
  nothing.
- The sort button is an outline button with a sort icon and
  `data-testid="sort-profiles-by-name-button"` in `InstalledAgentsHeader`,
  rendered only for `canManage`, placed between the Terminal and Rescan buttons
  so Rescan stays immediately before the creation action
  (`AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.4`). For an administrator the toolbar
  test IDs are `["open-host-shell", "sort-profiles-by-name-button",
  "rescan-agents-button", "new-agent-button"]`; both layout specs assert it.
- Copy lives in the `agents` namespace in all seven locales: `dragProfile`,
  `profileSortable`, `sortProfilesByName`, `profileOrderSaveFailed`. Traditional
  Chinese comes from `pnpm run i18n:zh-hant` and the pseudo catalog from
  `pnpm run i18n:pseudo`.

## Failure and recovery

- Network or `5xx` failure, or timeout: step 5 of Save coordination. An aborted
  request may still have committed; its event or the next snapshot carries a
  higher revision and converges the page.
- `409`: step 6 of Save coordination. The refetch captures the client membership
  epoch and applies only through `applyAgentListSnapshot`; if the epoch changed,
  it discards the response and obtains a fresh resource result before reconciling
  or replaying queued IDs.
- Concurrent reorder and global-membership mutations serialize on the same
  PostgreSQL per-agent advisory lock. If a create/delete commits first, reorder
  reads the changed set and rejects stale IDs; if reorder holds the lock first,
  it commits against the exact set validated, then the mutation follows.
  `DeleteAgent` takes the agent lock before its cascading delete. Ownership
  updates acquire candidate agent locks in sorted order, then the profile-ID
  lock, reread the source row, and retry from a fresh transaction if the current
  global owner is not locked. This serializes competing moves such as A→B and
  A→C and promotions from a workspace-scoped row. SQLite reserves its single
  writer before membership reads, serializing the same paths. Multi-connection
  tests on SQLite and Postgres cover create/delete, competing ownership/scope
  moves, and reorder in both lock acquisition orders.

## Persistence

`agent_profiles.sort_order` and `agent_profile_orders.revision` are install-wide,
not user-scoped. The reorder is a single transaction per agent, so no reader
sees a half-written order or an order without its revision. Membership-changing
transactions use the same lock as reorder so their commit cannot straddle the
set-validation-and-write interval. Soft-deleted rows keep their `sort_order`
and are excluded by `deleted_at IS NULL`. No index is added: reads already
filter by `idx_agent_profiles_agent_id` and sort a small set. Postgres and
SQLite share statements except for the per-engine lock acquisition.

## Security

Only callers with the agent-configuration permission reach the route; the
interim settings interlock applies as for profile writes. The handler rejects
IDs that do not belong to the addressed agent, so a caller cannot reorder or
probe another agent's profiles. Workspace-scoped profiles and the Dynamic agent
are never written by this route.

## Observability

The controller logs a reorder at debug level with agent ID, profile count, and
revision, and logs rejected stale requests at info. No metrics are added.

## Consumers of profile order

Code that reads the first profile as a default follows the saved order:
`app/office/tasks/[id]/advanced-panels/chat-panel.tsx` (`agentProfiles[0]`) and
`app/office/setup/setup-route-data.ts` (`profiles[0]`). This is accepted and
documented; no default is pinned. Operational pickers keep their own ordering
rules (see `profile-recent-use.md`) and use the saved order as source order.
`office/routing/provider.go` reads `ListAgentProfiles` for the execution-profile
catalog but sorts the result by name and ID, so it is unaffected. A new profile
is placed first within its own agent's entries in the flat list, never ahead of
other agents, so cross-agent defaults do not shift on creation.

## Related decisions

- [ADR 0005](../../../decisions/0005-agent-model-unification.md) introduced the
  `workspace_id`-scoped profile rows this design excludes. No new ADR is needed.
