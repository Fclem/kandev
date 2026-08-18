---
status: draft
created: 2026-08-18
owner: kandev
---

# Floating (unpinned) workbench panels

## Why

The dockview task workbench is a fixed grid: every panel block occupies grid
space, so a user who wants a panel (chat, plan, terminal, changes) to overlay
the rest of the workbench — the floating/peek pattern of VS Code, Slack, and
Discord — has no way to get it. They must either keep the panel docked and
lose the space, or close it and reopen it per need. Unpinned floating panels
with an edge title bar give the same "peek without committing layout" flow the
message queue pin gave to queue management.

## What

- Every non-sidebar dockview group header gains a pin toggle placed
  immediately to the **left** of the maximize control, using the same icons as
  the message queue pin: `IconPinned` when pinned, `IconPin` when unpinned.
  The button exposes state through `aria-pressed` and a localized accessible
  name/tooltip ("Pin panel" / "Unpin panel").
- **Panels are pinned by default.** The control renders in the pinned state
  and the group stays in the grid.
- **Unpinning floats the group.** The group's tabs leave the grid (the freed
  space is reflowed to the remaining blocks) and the group's content renders
  in a floating window over the workbench, anchored to the edge the group
  occupied and sized to the group's pre-unpin width (side block) or height
  (horizontal block).
- **The floating window collapses when out of focus** (click elsewhere,
  pointer interaction outside the window, keyboard focus leaving the window,
  or Escape while the window owns focus), mirroring the left sidebar's
  collapsed rail state. Collapse shows a title bar on the edge:
  - a **vertical** bar (titles stacked) when the group was a **side panel
    block** (left/right root column);
  - a **horizontal** bar (titles in a row) when the group was a **horizontal
    panel block** (center/full-width column, or a bottom split).
  The bar lists the group's tab titles in tab order with the active tab
  highlighted, and carries the same pin control so the group can be re-docked
  without expanding first.
- **Clicking a title in the collapsed bar** expands the floating window with
  that tab active. The window stays expanded until focus leaves again.
- **Clicking the pin again re-docks the group** to its remembered column and
  position, restoring tab order and the active tab. The control returns to the
  pinned state.
- Panel content keeps running while floating: terminal PTYs, editors, browser
  iframes, and plugin panels must not restart or disconnect when a group is
  floated, collapsed, expanded, or re-docked.
- Floating state persists per task environment (same sessionStorage lifetime
  as the environment layout), including the full panel definitions needed to
  **recreate** floated panels after a reload, so reloading or switching tasks
  restores which groups float, their tabs, and their expanded/collapsed
  display state.
- **Desktop-only.** The dockview workbench does not render on phone viewports
  (the mobile task surface is a separate composition without dockview:
  `mobile-task-layout` plus `session-mobile-bottom-nav`, which exposes
  localized Plan/Changes/Files/Terminal buttons), so the pin control appears
  only where the workbench renders — consistent with the message queue pin.
  Floating/collapse is intentionally absent from the mobile state model;
  panel-access parity on phone is covered by the retained-path scenario in
  task-05: `apps/web/e2e/tests/mobile-panel-access.spec.ts` (test title
  "mobile task panels remain reachable"), which opens a task on the mobile
  project, activates Plan/Changes/Files/Terminal through the bottom-nav
  buttons (stable test ids added where missing), and asserts reachability
  plus viewport containment / no horizontal overflow.

## Data model

One transient store map plus one per-environment sessionStorage slot. No
backend state. The unit of pinning is the **group**, not the individual panel:
every tab in the group floats and collapses together, because the maximize
control it sits beside is group-scoped.

```
floatingGroups: Record<groupId, FloatingGroupState>   # in useDockviewStore
```

```
EnvFloatingState                     # persisted as JSON, versioned
  version      1
  nextOrder    number            # monotonic stack-order counter for this env
  groups       Record<groupId, FloatingGroupState>
  rootColumns  Record<columnId, { pinned: boolean; width: number;
                                  minWidth: number | null; maxWidth: number | null;
                                  role: "center" | "pinned-right" |
                                        "side-other" | "custom" }>
               # authoritative root-column metadata sidecar, INSIDE the blob:
               # role is captured from an explicit LayoutColumn.role assigned
               # by presets/custom-layout normalization (with a documented
               # old-v3 migration: runs ONCE as `migrateEnvLayoutV3(raw,
               # envId)` — a versioned, one-time-marked transform on the raw
               # v3 native JSON BEFORE fromJSON, with the exact root-column
               # geometry source, precedence for multiple right candidates,
               # and a closed policy for no/ambiguous candidates (default
               # right = the pinned column containing files/changes; tie →
               # leftmost; none → no pinned-right role); **raw v3 lacks
               # min/max constraint fields — missing old constraints are
               # NORMALIZED (not recovered) from the preset/default role and
               # constraint metadata, and the sidecar is persisted only after
               # a validated post-apply live capture**; migrated roles are
               # persisted before any hasLivePinnedRightColumn decision and
               # later restores consume only stored roles (never re-inferred) rebuilt in memory after every layout
               # apply and reload, invalidated on preset/reset/env switch,
               # covered by the same journal transaction, budget, and cleanup
               # as the groups — there is no third storage key. Persistence is
               # COALESCED: sidecar updates in memory during layout applies
               # and flush with the same verified debounce/beforeunload
               # transaction as the live layout (geometry durability equals
               # layout durability); the blob/journal are written only at a
               # settled boundary and only when the serialized bytes actually
               # changed (no floating groups + unchanged sidecar ⇒ no write).
  identities   Record<"group" | "column", Record<logicalKey, uuid>>
               # durable logical identity map (M4): UUIDs for every logical
               # group and root column, written whenever the identity set
               # changes (NOT gated on groups being non-empty — the key
               # survives an empty floating state), preserved across dock/
               # refloat/preset/reset, so a sibling/first-panel change never
               # mints a new identity for the same logical group/column.
```
FloatingGroupState
  groupId          string        # original grid group id (informational)
  groupLogicalId   string        # the persisted logical group identity (THE
                  # placement identity; native groupId is a hint only)
  columnId         string        # root LayoutColumn NATIVE id (non-authoritative hint)
  columnLogicalId  string        # the persisted logicalId — THE placement
                  # identity key for capture, materializer, and sidecar lookups;
                  # the rootColumns sidecar is keyed by columnLogicalId, never
                  # native ids (one key space everywhere)
  columnIndex      number        # index of that column in the columns array
  columnKind       "center" | "side" | "custom"   # see Placement capture
  columnPinned     boolean       # column's pinned flag at float time
  columnWidth      number | null # root column width (px) at float time
  columnMinWidth   number | null # root column min width, if any
  columnMaxWidth   number | null # root column max width, if any
  treePath         null | { kind: "tree", path: number[] } | { kind: "flat", index: number }
                  # tagged placement: tree path vs flat groups index are
                  # distinct coordinate systems and never conflated
                  # Shape-change mapping (exact): flat -> tree destination =
                  # DFS leaf-order index clamp; tree -> flat destination =
                  # leaf order index clamp; out-of-range paths clamp to the
                  # last valid leaf/group. DFS traversal is total: left-to-
                  # right child order, branches before/after leaves per
                  # preorder (leaf-order = the order leaves are visited in a
                  # left-to-right preorder walk); a zero-leaf or invalid
                  # destination fails the materialization non-destructively.
                  # The expected leaf for flat->tree and tree->flat (incl.
                  # out-of-range) is asserted in round-trip tests.
  edge             "left" | "right" | "top" | "bottom"   # collapsed-bar edge
  columnRole       "center" | "pinned-right" | "side-other" | "custom"
                  # persisted column role (M5): distinguishes a custom pinned
                  # right terminal column from pinned plan/preview/vscode
                  # side columns; captured with the geometry, kept in the
                  # sidecar, and the single input to hasLivePinnedRightColumn
                  # alongside live membership and pinned state
  orientation      "vertical" | "horizontal"             # derived from columnKind + position
  size             number       # px: window width for vertical, height for horizontal
  panels           FloatingPanelDef[]   # full tab definitions, tab order
  activePanelId    string | null
  display          "expanded" | "collapsed"
  order            number       # stack order (z-index, offsets); allocated monotonic

FloatingPanelDef                   # complete definition so a floated panel can
  id           string             # be recreated after a reload or env switch
  component    string
  title        string             # canonical English title (reload fallback only)
  tabComponent string | undefined
  params       Record<string, unknown>   # must be JSON-safe (see guard)
```

Persisted under sessionStorage key `kandev.dockview.env-floating.<envId>`,
alongside `kandev.dockview.env-layout-v3.<envId>` and
`kandev.dockview.env-maximize-v3.<envId>`. Reads go through a versioned type
guard (mirroring `isEnvMaximizeState`): each panel definition is validated
independently (JSON-safe values: no `undefined`, functions, or cycles; params
serialized size bounded at 64 KB per definition; recursive validation is
depth-bounded at 64 so a deeply nested value cannot overflow the validator),
numeric fields (`order`, `nextOrder`, `size`, `columnIndex`) must be finite,
nonnegative, and within practical maxima (order/nextOrder ≤ 10 000, size ≤
100 000, columnIndex ≤ 64), `nextOrder` is **normalized on load to
max(raw nextOrder, max(accepted group orders) + 1)** — the persisted
high-water counter is preserved even when the highest-order group was removed
(no accepted groups does NOT reset the counter to 1); if the normalized
counter exceeds the cap, allocation fails non-destructively — and duplicate
panel ids or group ids within the blob are rejected deterministically (first
occurrence wins, later ones dropped). Invalid entries are dropped
individually; an unreadable blob defaults to `{}` (all groups pinned).

**Layout persistence invariant:** the env layout keeps reflecting the **live
grid** (floated groups absent), exactly as it does today. The floating blob
carries everything needed to materialize floated groups back into the grid and
re-float them. The env layout is never replaced by a pre-float snapshot and
the floating blob never contains a layout.

**Stable identity (M4, attachment boundary):** logical group and root-column
identity is a **first-class persisted field on the live layout schema** —
`LayoutGroup` gains `logicalId: string` and `LayoutColumn` gains
`logicalId: string` (+ `role`), populated by layout normalization (built-in
presets, custom-layout normalization, and a documented old-v3 migration that
assigns UUIDs once and persists them), carried through the serializer
(including the tree+flat synchronization — both representations hold the same
`logicalId`), preset merges, dock, and reset. **The live Dockview model
cannot carry custom fields, so a store-owned **normalized-live-layout
registry** keys logical metadata by the NATIVE group/column ids and is
merged into every capture: `fromDockviewApi`/capture accept that registry
and fail closed when a native object has no mapping; the registry is rebuilt
from the v4 envelope after every `fromJSON`, and updated on add/remove,
dock, preset, reset, and resize — native-ID regeneration can never orphan a
logical identity. **Layout persistence is
versioned:** the env layout key is bumped to **v4** and stores a **versioned
envelope** — `{ version: 4, dockview: <native serialized JSON>,
  layout: <normalized LayoutState carrying logicalId/role> }` — because
native dockview JSON drops custom fields while `fromDockviewApi` regenerates
traversal-derived ids; **every restore route uses one envelope-aware adapter
`readEnvLayoutForRestore()`** that unwraps to `{native, normalized}` with
**separate route contracts** — a wrong ordering creates duplicate session ids,
loses the incoming session, or mutates the maximize overlay:
- **Regular v4 layout (initial mount, env fast/slow, custom, preset,
  fallthrough):** unwrap + sanitize the native value → `api.fromJSON` ONCE →
  session/ephemeral replacement and incoming-session insertion → a **defined
  normalized-state reconciliation** with a **deterministic diff contract**:
  match priority is logical group identity → panel ID → a closed semantic
  fallback; exactly ONE live instance per panel ID (duplicate-prevention);
  the **native payload owns component/params/title/tabComponent for
  live/env-scoped panels** (terminal ids, editors, browser urls — native
  params are authoritative) while the **normalized state owns
  identity/placement/role/active-tab**; persisted `session:*` panels are
  EXCLUDED (the live session insertion owns those); native-only
  non-session panels are retained, normalized-only non-session panels are
  materialized fresh; group placement and the active tab are reconciled from
  the normalized state; never a second full `fromJSON`. Duplicate-prevention
  tests cover `terminal-default`, ordinary `shell-<uuid>`, files/changes,
  plugin, and env-scoped panels. → fixups/bootstrap → floating restore.
- **Maximize-only:** unwrap the native two-column overlay → `fromJSON` once;
  the normalized pre-max `LayoutState` stays ONLY in the store
  (`preMaximizeLayout`); it is **never applied while maximized** (no
  unmaximize, no overlay mutation); post-exit restore applies it. **Reload-
  while-maximized renders the saved floating content above the overlay by
  materializing the floating panels' PORTALS only** (defs registered into
  the portal manager so the floating window can adopt them — never touching
  the grid or the two-column overlay, never duplicate grid ids); the
  grid-side pending materialization completes at maximize exit. A route
  dispatcher selects maximize-only when a valid maximize blob exists,
  regular otherwise (malformed/failed maximize falls back to regular, with
  exactly one `fromJSON` per selected route).
Call-order tests cover initial, fast/slow switch, route-intent, custom,
maximize-only, and fallthrough paths, asserting no stale/duplicate session,
no overlay mutation, and portal-above-overlay visibility during maximize. `getEnvLayout`
reads v4, and a **v3→v4 migration
(`migrateEnvLayoutV3(raw, envId)`)** assigns UUIDs once and writes v4 only
after a validated apply, keeping a v3 reader fallback; **explicit
v3-read/v4-write key constants** define coexistence (v3 read until v4 is
written; v3 deleted only after the validated v4 apply; the envelope's
`version` field is the idempotence marker — no third marker key) with
retry-on-failed-apply semantics that never mint new UUIDs;
`apps/web/e2e/helpers/
dockview-persistence.ts` prefixes and all layout consumers are updated
together, with old-v3-restore, v4 round-trip, tree/flat-equality, and e2e-
helper-compatibility tests. **The maximize slot is versioned too:** the
`preMaximizeLayout` state stored with maximize uses the same v4 normalized
`LayoutState` schema, with explicit `MAXIMIZE_V3_READ_PREFIX`/
`MAXIMIZE_V4_WRITE_PREFIX` constants: v3 maximize blobs are read on upgrade,
only `preMaximizeLayout` is migrated to normalized v4 (the native
`maximizedDockviewJson` is retained untouched), the migrated native overlay
is applied validated, the pre-max state restores after maximize exit, and v3
is deleted only after that validated apply — with retry/idempotence and
malformed/partial-migration behavior defined; reload-while-maximized and
env-switch-while-maximized never lose `logicalId`/`role`/placement, and the
pre-max state is never applied to the live overlay. **The legacy
`dockview-layout-v3` localStorage
write is removed** — the env sessionStorage v4 slot is the sole authoritative
layout surface, and tests/helpers that observed the legacy key are migrated
(there is exactly one layout persistence surface: the v4 env slot, through
the sole pair writer).
**Migration UUID derivation is crash-idempotent and deterministic:**
canonical input is **domain-tagged** — `kind(group|column) + role + sorted
unique panel-id set`, length-delimited encoded, mapped via a documented
SHA-256→UUID truncation; duplicate panel ids, empty/ambiguous sets, and the
same canonical key across distinct groups/columns are rejected. v3
derivation runs ONCE (v4 logical ids are thereafter authoritative and
membership changes NEVER re-derive them — a group identity changes on member
addition only by persisting the new canonical id at the next capture, never
by re-deriving the old one); a crash before the v4 write cannot mint
different ids on the next load (deterministic derivation); crash-before-
v4-write and repeated-retry tests assert id stability. The blob's `identities` map is a
**secondary cache keyed by `logicalId`**, never the source of truth: capture
reads the group/column's own `logicalId` from the live layout (no
traversal-derived `group-n` ids are ever treated as identity); a group or
column without a `logicalId` fails closed with ambiguity rejection (never a
silent new identity). Sibling/first-panel changes therefore never affect a
saved identity, because the id lives on the object, not in its position.
Dock-to-different-column-to-refloat, sibling-change, and first-panel-change
tests prove the ids survive.

## Placement capture (at unpin time, from the live `LayoutState`)

Placement comes from the live `LayoutState` (root columns, their ids/indexes/
pinned metadata, the column tree, and the group's position), never from a
group API. Classification is a pure function over that data:

```
classify(columns, centerColumnId | null, isCenterKnown, columnId, columnIndex, columnPinned):
  isCenterKnown && the column containing centerColumnId
      -> kind "center", orientation "horizontal", edge "bottom"
  any other root column
      -> kind "side", orientation "vertical",
         edge "left" if columnIndex < center column index else "right"
  unresolved (no known center, or column id/index not found)
      -> kind "custom", orientation "vertical", edge "right" (documented fallback)
```

- Plan/preview/vscode/compact presets put their extra groups in root columns
  **to the right of** the center column, so they classify as `side`/vertical/
  right — matching the user-facing requirement. The classifier must NOT use
  `isCenterCandidateGroupId` (it returns true for generated plan/preview ids
  and would misclassify them as center).
- **Center identity is nullable and explicit.** `findCenterGroupId` fabricates
  `CENTER_GROUP` or the first "centerish" group when no real center exists
  (`layout-manager/applier.ts:45-55`), so the classifier must receive the
  resolution result as `(centerColumnId, isCenterKnown)` — resolved from the
  live grid after the synchronous part of the layout apply — and treat an
  unknown center as `custom` (vertical/right fallback) rather than promoting
  an arbitrary side column to center. Initial-mount, no-center, and
  plan-preset cases are tested against the actual live id.
- Column index and pinned/width metadata come from the `LayoutState` columns
  array.
- Nested tree splits inside a column: v1 classifies by the **root column**
  rule above. A group in a bottom split of the center column therefore gets
  horizontal/bottom only when the column is the center column; branch-level
  refinement for exotic nested trees is explicitly future work.
- Sizes are captured from the layout data (column width for side groups,
  group height for center/bottom groups) before any removal.

## State machine

| State | Transition | Trigger | Actor |
|---|---|---|---|
| `pinned` | → `floating-expanded` | click pin in group header | user |
| `floating-expanded` | → `floating-collapsed` | focus leaves window (outside pointer, focusout, Escape) | user / system |
| `floating-collapsed` | → `floating-expanded` | click a title in the edge bar | user |
| `floating-expanded` | → `pinned` | click pin in floating window header | user |
| `floating-collapsed` | → `pinned` | click pin in the edge bar | user |
| any | → (removed) | all tabs of the group closed | user / system |

Float/dock/restore are **transactions** with an operation journal:

1. **Capture + validate + preflight:** capture placement, panel definitions,
   and size; serialize the current `LayoutState` into the journal. If the
   target group's definitions cannot be captured or serialized, abort without
   mutating the grid. **Size budget preflight:** the total serialized size of
   the floating blob plus the journal must fit a documented budget (see
   Failure modes — journal size) or the transaction fails non-destructively
   before any mutation.
2. **Mutate (one transaction coordinator):** a single coordinator
   (`floating-transaction.ts`) owns the per-env operation/generation and the
   phase state; its explicit `begin → advance → settle` API is the only path
   that mutates `floatingGroups`, the detach registry, the journal, or the
   token. While a transaction is in any phase, float/dock/reset/toggle-right
   actions are **rejected (not queued)**; **every** public layout-mutation
   boundary is guarded the same way — the add-panel resolver
   (`focusOrAddFloatingOrGridPanel`), `buildDefaultLayout`, preset/custom
   apply, maximize/exit, and direct programmatic panel actions return a
   a non-destructive no-op with a debug reason while busy (a programmatic add
   mid-transaction would mutate the live grid after the journal snapshot and
   **Restore/recovery paths enter the same
   coordinator:** `recoverFloatingJournalOnce` and `restoreFloatingAfterLayout`
   acquire the same phase/token; a restore completion arriving during
   mutation/portals-adopted/rollback/stale-cleanup is **skipped with a
   retained pending-floating-restore marker**. **The marker has a guaranteed,
   ordered drain with no reentrancy paradox:** the drain is a
   **coordinator-owned internal operation** (`drainPendingRestore(token)`)
   that runs inside `settle` **after the portal-adoption phase completes but
   before the busy flag clears**; it carries an **internal token and a
   phase/recursion guard** so its restore invocation is not re-rejected by
   the busy gate it runs under (restore paths invoked *outside* the
   coordinator's internal operations are still busy-rejected), and recursive
   **The internal operations live in a factory closure
   and only a public facade is exported** — `floating-transaction.ts`
   exports exactly ONE public value, `floatingTransactionFacade` (a runtime
   object; its type is derived and never separately imported by callers),
   with documented methods `begin(envId)`, `advance(envId)`,
   `settle(envId)`, `isBusy(envId)`, `persistSettledPair(...)`;
   internal `drainPendingRestore` and `restoreForFloat` are absent from all
   exports, and cross-file callers (`restoreFloatingMaximize`,
   `restoreFloatingAfterLayout`, every restore entry) import ONLY the
   `floatingTransactionFacade` value — enforced by a source-boundary test
   that fails on any non-facade import of `floating-transaction.ts`
   (covering direct imports, re-exports/barrels, and dynamic imports; type-
   only imports of the derived type are permitted). A forged/plain token or
   stale/released capability can never invoke an internal operation —
   plugin-facing capabilities are protected by host-side WeakMap/portal-
   generation binding, not token secrecy, and source-internal imports are
   explicitly not part of the plugin API.**
   `settle` is **async and keeps busy through portal
   adoption and the drain**; **portal adoption and the drain run in a
   `try/finally` that always token-guards the transition to `settled` (or an
   explicit failed-settled state) from every phase, so a throw during
   adoption or the drain can never leave `busy` stuck — busy clears exactly
   once, asserted in thrown/rejected/cancelled adoption and drain tests**; a drain that throws or schedules another restore
   fails the marker clear (marker retained, debug-logged, re-armed at the
   next settle). A new transaction's `begin` consumes any retained marker
   first, and the drain rechecks `envId`, generation, api instance, and the
   marker before invoking restore against the fresh layout.
   **Float-while-maximized uses the same internal mechanism:** `floatGroup`
   on the maximized group calls the coordinator-owned `restoreForFloat`
   (internal token, recursion guard) that restores the pre-max layout
   through the same busy-protected path, so the maximize/exit busy guards
   never deadlock the float. Deterministic schedule tests cover: skipped
   restore → settle → begin → settle (restore runs exactly once), drain
   throw, drain scheduling another restore, and float while maximized. Every
   restore entry is tested against each busy phase,
   including the drain after each. A single store/coordinator selector
   `isFloatingTransactionBusy(envId)` is consumed by **all three pin
   surfaces** — the grid group header, the floating window header, and the
   collapsed edge bar — which render the pin disabled during the busy window,
   so no pin click can re-enter the phase machine. Re-entrancy tests cover
   mutation, portals-adopted, rollback, and stale-cleanup phases, per surface
   and for programmatic add/reset/restore during each.
3. **Commit:** only after every removal/materialization succeeded, commit the
   store entry, persist the floating blob, then `persistEnvLayoutNow`.
4. **Failure recovery:** if any step throws mid-mutation, run the recovery
   phase (see step 6) and drop the partial entry. Suppression cleanup runs in
   `finally` and is token-guarded (a stale transaction's cleanup never clears a
   newer transaction's state).
5. **Bidirectional crash-consistent commit (durable journal marker):** a
   per-env **operation journal**
   (`kandev.dockview.env-floating-journal.<envId>`) is written **before** any
   mutation and holds:
   ```
   { version, envId, transactionId, phase: "mutating" | "committed",
     before: { floatingDigest, layoutDigest },
     after:  { floatingDigest, layoutDigest },
     raw: { beforeFloating, beforeLayout, afterFloating, afterLayout } }
   ```
   **Digest protocol (exact):** each target storage value is serialized
   **once**; the digest is a **tagged union** — `{kind: "absent"}` or
   `{kind: "present", sha256}` — over the **exact bytes written to
   sessionStorage** for that key (the raw JSON string, byte-for-byte — never
   a re-stringified parse), so absence is never conflated with a stored value
   whose bytes equal a marker string; `raw` retains the exact strings so
   recovery never re-serializes. **Writes are verified, not just
   status-returning:** every write goes through `writeVerified(key, raw)` —
   set, read back the exact raw bytes, compare against what was written; any
   mismatch (silent truncation/alteration included) is a **failed write** that
   enters the journal rollback path (mismatch-after-mutation is a row in the
   recovery matrix). The coordinator remains busy across the (bounded) async
   hashing/verification. **Journal writes are verified the same way** (a
   failed journal write aborts the transaction before any mutation).
   **Write/verify ordering:** journal → blob → layout → phase transition →
   read-back verify of both keys → journal clear; a throw after any storage
   mutation is a recoverable state via the phase marker (a
   `phase-write-throw-after-mutation` recovery case is specified and tested).
   **Recovery is a phase-aware decision matrix, never inference:** for each
   key, the current stored value's digest is compared against the before/
   after digests, and the **selected target pair is the one that is verified
   before the journal is cleared**:
   | observed | target | verify + clear against |
   |---|---|---|
   | both-before (pre-mutation crash) | before pair | before digests |
   | blob-after / layout-before | after pair | after digests |
   | layout-after / blob-before | after pair | after digests |
   | both-after or both-equal (no-op) | settled | after digests (equality needs no write) |
   The `phase` marker (`mutating` vs `committed`) refines the both-after row
   (a `committed` marker with both-after means the mutation finished; a
   `mutating` marker with both-after is still settled by equality), never
   replaces digest evidence. The journal is cleared only after the selected
   target is verified. Both write paths return/throw status (the current
   `setEnvLayout`/`persistEnvLayoutNow` swallow failures — status-returning
   APIs are part of the contract). **`recoverFloatingJournalOnce(api,
   envId)` is the single pre-restore gate**: it reads the persisted journal
   `{envId, transactionId, phase, digests}` and **validates it before
   trusting it** — `isEnvFloatingJournal(journal, envId)` checks version,
   env match, transaction id shape, phase enum, tagged-digest shape, raw
   snapshot bounds (per-env cap), and exact raw JSON-shape, then
   **recomputes SHA-256 from each raw snapshot and requires it to equal the
   journal's tagged digest** before any target is selected. A journal that
   fails validation or digest recomputation is treated as **unreadable**
   (journal-free divergence rules, using the caller's explicit `envId`) and
   **quarantined through a verified, idempotent protocol** — sessionStorage has
   no rename primitive, so quarantine derives **one deterministic key from
   `(envId, original raw digest)`** (`...-journal.<envId>.corrupt-<digest>`):
   copy the raw bytes to that key, read-back verify the copy, then remove the
   original and verify its absence. If the deterministic quarantine key
   already exists on a retry, it is read and verified and the flow proceeds
   directly to verified original removal — **a second copy is never allocated
   for the same original** (a crash between copy and remove cannot accumulate
   `.corrupt-<n>` copies on repeated restarts). A failed copy or removal
   keeps the original (never cached as recovered) and is retried on the next
   recovery; quarantine keys count toward the budget, are removed by bounded
   per-env/task corrupt-key cleanup in `cleanupTaskStorage`, and the recovery
   cache records success only after durable quarantine or verified original
   absence. Crash-after-copy, repeated-restart, and cleanup cases are
   tested. Recovery is idempotent via an in-memory
   recovery cache keyed by `(envId, transactionId, api instance)` — never one
   global generation, so env B's journal is never skipped by env A's recovery
   and a new API instance re-checks a still-present journal — and runs before
   every restore entry (initial mount, env-switch fast/slow, maximize
   restore, preset/custom apply, `toggleRightPanels`, reset/default build).
   Deterministic tests cover all matrix rows (including the pre-mutation
   both-before crash, the no-op equality row, read-back mismatch
   after-mutation, a phase-write throw after mutation, and setItem throwing
   before/after mutation). The two-key
   divergence rules in step 6 are the journal-free fallback only (journal
   lost/unreadable/invalid), not the primary guarantee.
   **Size budgets:** a per-env cap (96 KB default: blob + journal
   snapshots) **and** a **global floating allocation budget** across all
   environments' blobs + journals (default 384 KB). The budget is enforced
   through a **validated owned-key index** maintained by the coordinator
   (built on load/recovery and updated on budget-changing writes): before
   every budget decision the index is validated by comparing each indexed
   owned key against its stored raw length/digest and the key set, and a
   mismatch (external same-tab mutation — sessionStorage emits no
   same-document event) triggers one bounded prefix-scan rebuild, so the
   scan never re-iterates unrelated sessionStorage keys on every toggle.
   Coordinator writes are the only sanctioned mutation path for owned keys
   (external mutation handling is documented, not silently tolerated); a
   later storage-write failure after a passing preflight (quota
   race with unrelated Kandev storage) still fails closed via the journal
   rollback — the rollback/journal policy, not the preflight, is the
   correctness backstop. Near-limit, multi-env-combined, and quota-full
   behavior is tested.
6. **Divergence-tolerant restore + portal-safe recovery phase:** when no
   journal exists (lost/corrupt), `restoreFloatingAfterLayout` is idempotent:
   a group present in the restored grid whose blob entry says float is floated
   by the normal materialize-then-float path (the existing group is reused).
   **Same-id/different-column resolution is per panel, never wholesale:** for
   each saved panel id found live in a **different** group/column than its
   saved placement, the live panel is the deterministic authority (no
   duplicate is ever materialized); the **non-conflicting** panels of the
   group are retained and the surviving group is re-docked per the documented
   collision policy (survivors dock into the saved column when it exists,
   else the live column of the conflicting panel), so no tab, payload, or
   display state disappears silently — the cleaned result is persisted and a
   debug log records the dropped conflicting entries. **Group-id allocation
   is collision-safe:** materialization runs `allocateUniqueGroupId(savedId)`
   — the saved id is reused only when it is absent from the live grid or owned
   by the exact group being restored; otherwise a generated id from a
   dedicated namespace (`group-floating-<n>`) is allocated, reserving against
   both `api.groups` and ids allocated earlier in the same operation, with a
   **finite attempt cap** (64; exhaustion fails the materialization
   non-destructively). A saved→live group mapping is preserved for
   active-panel and tree insertion **and re-derived from saved ids against
   live ids on every reload** (the mapping itself is never persisted; it is a
   runtime re-derivation). Salvage and group-id collision tests cover the
   saved column, the live conflicting column, same-operation double
   allocation, mapping re-derivation after reload, and cap exhaustion.
   Recovery runs as a
   **phase model**: `mutating → restoring → portals-adopted →
   persist-recovered → settled`; the restore gate stays up through portal
   adoption, and only the portals-adopted phase may persist the journaled
   layout (through the status-returning API); token cleanup is
   generation-guarded and happens at settled.
7. **Persistence suppression + single unload handler:** a store-owned
   transaction token (`floatingTransactionToken` with token-guarded begin/end
   actions; the same field is read by `setupLayoutPersistence`, so there is no
   module-local flag and no import cycle) gates **every** layout-persistence
   entry point through one `canPersistLayout()` guard: `persistNow`, the
   debounce callback, the `onDidLayoutChange` handler, and the `beforeunload`
   flush (`dockview-layout-setup.ts:346-405`). The unload handler is **one
   idempotent, transaction-aware handler** (the existing listener is replaced,
   not duplicated). **Unload is synchronous and prevalidated:** the exact raw
   before/after bytes and digests are **precomputed and cached before any
   unload-sensitive phase** (they are already available from the journal),
   and the unload handler writes a complete pair using a **synchronous digest
   path or the prevalidated cached BEFORE pair + aborted marker** — it never
   awaits an async hash or abandons a half write; the handler is tested
   **Unload writes a complete, journal-consistent
   pair with a deterministic phase table:** `mutating` or any phase whose
   AFTER pair has NOT completed synchronous verification ⇒ write the cached
   BEFORE pair + aborted/retained journal; ONLY a journal whose AFTER pair has
   completed synchronous verification ⇒ write AFTER + committed marker. The
   journal marker ordering matches this definition (the `committed` phase is
   set only after final read-back verification, never before), and a
   synchronous `writeVerified` failure retains a recoverable journal and
   leaves the original state; tests cover the phase-write-throw and
   final-readback windows, not only individual blob/layout writes. It never
   writes a half pair or
   calls `api.toJSON()`. Outside a
   transaction the handler performs the normal live flush. On transaction
   begin the guard also cancels/holds an already-scheduled debounce timer and
   marks it dirty, so a timer scheduled before the transaction cannot fire
   **Persistence runs only at the settled phase** (explicit
   `persistEnvLayoutNow`), and the token is reset on unmount. **One sole
   pair writer:** every layout/blob write — the ordinary debounce and
   unload flush, `persistEnvLayoutNow`, `saveOutgoingEnv`, preset/custom/
   reset apply, and float/dock — routes through one coordinator-owned,
   status-returning `persistSettledPair(envId, before, after, token)` with
   transaction/generation ownership, exact raw before/after snapshots, and a
   documented lock/queue/reject policy (a second writer while one is
   mid-pair is rejected with a debug reason, never interleaved); the current
   independent `setEnvLayout`/`persistEnvLayoutNow` writers are replaced, so
   no path can compute a different pair or swallow a failure. Race-ordering
   tests cover a scheduled debounce, float begin, each individual write, and
   unload in both directions.
   **Ordinary
   layout changes with zero floating groups:** the debounce callback IS the
   settled boundary and runs `persistSettledPair` for the complete
   blob/layout pair whenever either the sidecar bytes or the layout bytes
   changed — a zero-group sidecar change (role/geometry
   refresh) still writes the pair (the blob can exist with `groups: {}`), so
   a crash cannot leave role metadata behind the live layout; ordinary
   preset/resize crashes and zero-group persistence are tested. Ordinary
   unregistered closes are unaffected (their cleanup does not depend on the
   guard).

- **float(groupId):** if `groupId` is the maximized group, first restore the
  pre-max layout and derive the target's placement from `preMaximizeLayout`
  (never from the maximized overlay's live geometry). Then capture, register
  the group's panel ids in the detach registry (see Non-destructive removal
  contract), remove the panels, commit, clear the registry entry (consume-once
  per panel removal), persist.
- **dock(groupId):** materialize the group back at its remembered column/
  tree path (see Materializer), re-add the tabs in saved order, restore the
  active tab, remove the floating entry, persist, then `persistEnvLayoutNow`.
- **restore:** after the grid restore completes (see Restore call sites),
  materialize each floating group via the dock materialization path, then
  apply the float transition. Floating entries whose definitions cannot be
  materialized are dropped; empty floating groups are removed.

## Materializer

Re-dock and restore both recreate groups through one code path:

1. Clone the live layout (`fromDockviewApi`) into a delta `LayoutState`.
2. If the saved root column (`columnId`, `columnIndex`, pinned/width metadata)
   is absent, insert it at `columnIndex` (clamped) with its saved metadata.
3. Insert a group at the saved `treePath` **and** into the column's `groups`
   array in one atomic mutation helper. Both representations must stay
   consistent: `serializeColumn` prefers `column.tree` while `serializePanels`
   iterates only `column.groups` (`layout-manager/serializer.ts`), so a
   tree-only insert yields a serialized view with no panel definition and a
   groups-only insert is ignored by the tree. The helper allocates branch
   orientation (alternating, matching dockview's grid invariant), child sizes
   (equal split when unspecified), and a deterministic leaf for a missing
   branch, and sets `activePanel`.
4. Apply the delta through the existing serializer/`applyLayoutAndSet`
   machinery (the same path presets, maximize, and `toggleRightPanels` use).

Round-trip tests cover a column with an existing nested tree and a newly
inserted column with a tree (serialize → `fromJSON` → re-capture equality).

`fallbackGroupPosition` is reserved for the explicit existing-group fallback
only (a missing column/group after sanitization); it is never the column
creation mechanism. A materialization failure for one floating group drops
that entry and continues with the rest.

## Non-destructive removal contract

Normal panel removal (user closes a tab) keeps today's behavior: the panel's
portal is released (`panelPortalManager.release`), terminals park/stop, vscode
stops, and `handleMaximizeExitOnLastClose` may exit maximize.

Float/restore removals are **non-destructive**, scoped by a **detach registry**
rather than a global id set:

- The registry is keyed by **composite `(panelId, envId)`** (nested map or
  composite key), holding one record `{ token }` per entry. **Live-grid
  scope:** a panel id exists in at most ONE env's live grid at any time
  (only one env is live; the portal manager's single `Map<string, PortalEntry>`
  is per-live-panel), so same-ID live coexistence is impossible — the
  composite key exists for **blob-level bookkeeping** (a same id in the old
  env's blob vs the new env's live grid), never to model two live portals.
  The overlapping-id scenario is therefore: old-env blob entry released /
  superseded while the new env's live entry is preserved, verified with
  stale-token and old-entry-first tests.
  **Registrations are armed per expected removal, not for the transaction
  lifetime:** immediately before each synchronous `removePanel`/`fromJSON`
  that the transaction performs, the exact `(panelId, envId)` pairs being
  removed are registered; each registration is consumed once by the matching
  `onDidRemovePanel` (which receives the transaction token with the removal
  call) and the set is drained when the operation completes. A
  panel registered but never removed (operation aborted) is unregistered by
  the settle cleanup. This keeps a **real user close** of a still-live tab
  during an async phase or after a throw distinct from an expected detach —
  the user close is never registered at that moment and runs full cleanup.
  `releaseByEnv(envId)`/`reconcile` evaluate the **full entry tuple**
  (`panelId`, entry `envId`, token) — a same-id panel in the old env is
  released while the target env's registration is preserved; the callsite env
  direction is documented (`saveOutgoingEnv` passes the OUTGOING env), and
  simultaneous A/B registration + stale-token tests cover old-entry-first
  removal with target preservation.
- `setupPortalCleanup`'s `onDidRemovePanel` runs a fixed decision order:
  1. **Registered id with the current token** → consume the registration
     (remove it from the registry) and skip `release`, terminal park/stop,
     vscode stop, and `handleMaximizeExitOnLastClose` — regardless of
     `isRestoringLayout`. Expected detach removals (including removals fired
     by `fromJSON` during restore) are consumed, never double-released.
  2. **Unregistered id while `isRestoringLayout`** → return (today's behavior;
     `reconcile` handles those portals with the exclusion set).
  3. **Unregistered id otherwise** → full cleanup path (a user closing a
     floated tab mid-transaction is an ordinary close: cleanup runs, the
     portal releases, and the transaction drops that panel from its
     definitions when it notices the id is gone).
- `panelPortalManager.reconcile` and `releaseByEnv` take an explicit
  **env-qualified exclusion** — a predicate over `(panelId, entryEnvId,
  token)` or a record set keyed by `(panelId, envId)`, never a plain id
  set: the portal manager is globally keyed by panel id with per-entry
  `envId`, so a same panel id in the old env and the target env's floating
  state must be distinguished by comparing entry env + token inside
  `releaseByEnv`/`reconcile`. The callsite env direction is documented and
  overlapping-id-across-envs tests cover old-env release while the target
  env's registration is preserved.
- A stale transaction (token mismatch) never clears a newer transaction's
  registrations. The registry is cleared for the env on transaction settle
  (success, failure, or unmount).

## Restore call sites

`restoreFloatingAfterLayout` (materialize + re-float; idempotent) runs at
**every** grid-restore completion point, before `isRestoringLayout` clears:

1. Initial mount: `restoreEnvLayout` (`components/task/dockview-layout-restore.ts`)
   — after each of its three branches: saved env layout
   (`tryRestoreEnvLayout`), maximize-only (`tryRestoreMaximizeOnly`), and the
   default/route-intent build fallthrough.
2. Env switch fast path and slow path (`lib/state/dockview-env-switch.ts`),
   including `applyInitialRouteLayout` and the post-`fromJSON` fixups.
3. Maximize restore — **one selected sequence (defer-until-exit) via one
   shared coordinator** (`restoreFloatingMaximize` in
   `lib/state/dockview-floating.ts`, the single symbol both callers invoke):
   both maximize-restore callers —
   `tryRestoreMaximizeOnly` (initial mount, `dockview-layout-restore.ts`) and
   `restoreMaximizeFromStorage` (env switch, `dockview-store.ts`) — route
   through it. Sequence:
   `recoverFloatingJournalOnce` runs first; floating session entries are
   reconciled/mapped (including `floatingSessionWinner` written before the
   auto-session hook in this branch); the two-column overlay is **never
   mutated** — a per-env **pending-floating-restore marker** (store state,
   not storage) is set, and materialization/re-float runs only after
   `exitMaximizedLayout` applies the pre-max layout and its rAF settles. The
   marker's consumption is token/generation-guarded and cleared only after
   the post-exit layout rAF AND the floating restore settle; an exit followed
   immediately by reload keeps the marker consistent (the journal and the
   marker are re-evaluated on the next mount). While maximized, floating
   windows render above the overlay with their saved display state; the E2E
   asserts the visibility during maximize and the restore after exit.
4. Preset and custom layout apply (`applyLayout` / `applyLayoutAndSet`).
5. `toggleRightPanels` (`lib/state/dockview-store.ts`) — its own rAF clears
   the restore gate; floating restore must already be settled there (no-op if
   applied).
6. Reset / default build (`resetToEffectiveDefault`, `buildDefaultLayout`).

Each call site is tested with one focused test; a missing call site re-exposes
the reload blocker because the env layout intentionally excludes floated
panels.

## Focus ownership

One module-level coordinator (pattern: `hooks/use-panel-search.ts`) owns
expansion/collapse, with **owned regions** rather than raw subtree checks:

- A floating window's region = its DOM subtree **plus any overlay layer opened
  from within it** (Radix menus/dialogs/popovers portal to `document.body`).
  Components inside a floating window register their open layers with the
  coordinator (via a small `useFloatingOwnedLayer` hook wired to the Radix
  `onOpenChange`/dismiss events they already handle); a layer stays part of
  the region until it closes.
- Collapse triggers: window-capture `pointerdown` whose target is in no owned
  region; `focusout` whose `relatedTarget` (checked after a microtask, because
  Radix portals move focus) is in no owned region; or Escape per the Escape
  contract. Only the focused/last-interacted expanded window collapses; other
  floating groups keep their display state.
- **Pointerdown deferral while an owned layer is open:** window-capture
  `pointerdown` fires before Radix's own outside-interaction handler, so a
  click on the grid while a menu/dialog is open would otherwise collapse the
  window before the layer can close. While any owned layer of a window is
  open, an outside `pointerdown` marks that window's collapse as **pending**
  (it does not collapse yet); the pending collapse is applied when the window's
  owned-layer **refcount reaches zero**, and is cancelled if the pointerdown
  actually landed inside an owned region.
- **Owned-layer lifecycle:** `useFloatingOwnedLayer` returns an idempotent
  unregister that runs on **both** `onOpenChange(false)` and React cleanup
  (unmount, route navigation, ancestor teardown), so a layer that disappears
  without its dismiss callback still decrements the refcount. Pending collapse
  is stored with the event/generation that created it and cleared when the
  window or its layer owner unmounts. **Zero-refcount application is deferred
  past the microtask and held by a same-frame lease:** the pending collapse
  applies only at the next animation frame if the refcount is still zero AND
  no new owned layer registered since the pointerdown (a Radix
  close-then-open replacement — menu → dialog, nested dialog, navigation
  replacement — within the same synchronous turn or the same frame never
  collapses; a successor registered by a later effect before the frame
  boundary cancels the pending collapse, which re-arms only on a fresh
  outside pointerdown). Two layers closing in either order never collapse
  while the second is open, and a successor window reusing the same group id
  never inherits a stale pending collapse. The guaranteed-handoff contract is
  same-turn and same-frame replacement; a transition spanning longer task
  gaps (multi-frame animation, deferred navigation) may collapse and is
  documented as such, not silently claimed.
- **Layer inventory (mandatory, auditable deliverable):** every
  floating-capable panel's interactive layer owners (Radix
  Dialog/Popover/DropdownMenu/ContextMenu/HoverCard/Drawer surfaces inside
  chat, plan, terminal, files, changes, diff, plugins, and editors) must call
  `useFloatingOwnedLayer`. The inventory is tracked at
  `docs/plans/panel-pin-float/owned-layer-inventory.md` (exists; every row
  must reach exact file/line or an explicit layer-free proof before task-03
  is accepted — `to-wire`/`verify` are audit-baseline states, not completion):
  each row names the exact component/file + primitive range, the Radix
  primitive family, and the registration mechanism. The **host/plugin API is
  concrete and transport-complete** — `PluginTaskPanelProps` gains a
  render-bound opaque `floatingOwnedLayerCapability` (injected at
  `PluginTaskPanel` render, bound to the exact portal instance, revoked in
  `PluginTaskPanel` cleanup and `unregisterPlugin`), and
  `host.ui.registerFloatingOwnedLayer(capability, layerRoot): () => void` is
  added **together** to `apps/packages/plugin-sdk/src/index.ts` (the
  `PluginUIApi` type, as a callable outside the mapped component type), the
  host implementation `apps/web/lib/plugins/host-api.ts`, and the host
  contract docs (`docs/plans/plugins/PLUGIN-API.md` + `apps/web/lib/plugins/types.ts`).
  The host stores a WeakMap/token binding from capability to portal instance;
  a plugin rendering two task panels cannot register a layer from one panel
  against the other, and a hoarded plugin-scoped function cannot be reused
  across renders or after unmount. **Capability stability:** the capability is
  issued from a **portal-instance generation** and kept stable for the
  instance's lifetime via `useRef` (a benign re-render never issues a new
  token, so an open layer survives); it is revoked only on actual portal
  release or plugin unregistration, and **rotated on reacquire** (a released
  and reacquired portal gets a fresh token, so a hoarded old token is
  rejected). **Mobile:** the same `PluginTaskPanel` renders on phone, but
  floating ownership is desktop-only — on mobile the capability is absent
  and host registration is rejected (documented, tested). Unregister is
  idempotent on close,
  unmount, and plugin unregistration, with cleanup wired into
  `unregisterPlugin`. **The revocation bridge is concrete:** a host-owned
  revocation registry keyed by plugin/portal generation is invoked
  **synchronously from `unregisterPlugin` before `notify`**, revoking every
  live layer of that plugin even before React cleanup re-renders the panel;
  an unregister-while-layer-open test covers that window. An unregistered
  layer is a collapse bug by contract;
  one real test per primitive family, including a plugin-panel layer
  (hoarding, cross-panel use, unmount revocation, release-reacquire
  rotation, benign re-render stability, mobile rejection, unregister
  cleanup).
- True outside interaction still collapses: clicking the grid, the app
  sidebar, or the page chrome while no owned layer is open collapses the
  window on pointerdown; with layers open, the first click closes the layers
  and the collapse follows their dismissal, matching the "stays expanded until
  the layer closes" contract.

## Escape contract

- The coordinator listens on the **bubble** phase (never capture) at the
  window/document boundary.
- A descendant handler that handles Escape calls `preventDefault()` (Radix
  dismissable layers do); the coordinator honors `event.defaultPrevented` and
  does nothing.
- Otherwise, Escape collapses the focused expanded window (the one containing
  `document.activeElement`, or the last-interacted expanded window).
- Precedence is therefore: focused editor/input key handler → Radix layer →
  coordinator — enforced by `defaultPrevented`, not by listener ordering.

## Collapsed bar accessibility

- The bar is a semantic `tablist`; each title is a `tab` role with
  `aria-selected` reflecting the active tab, and an accessible group label
  (e.g. the panel group's name from its tabs).
- **Roving tabindex:** exactly one tab stop (the active tab has `tabIndex=0`,
  the rest `-1`); Arrow Up/Down for vertical bars and Arrow Left/Right for
  horizontal bars (both axes accepted in either orientation) move focus
  between tabs; Home/End jump to first/last; Enter/Space activate the focused
  tab (set active + expand); Escape collapses.
- The pin control is a separate button in DOM order after the tablist; focus
  return: expanding or collapsing restores focus to the previously focused
  control (the pin or the tab that triggered the transition).
- Titles: registry panel ids render via `panelTitle()`; unknown ids (session
  tabs, file diffs, plugin panels) render the **live** panel title (from the
  portal/dockview panel api) when available, falling back to the persisted
  definition title. Resolution is **reactive**: the floating overlay
  subscribes to the plugin registry and the portal manager (a title/version
  signal), so a plugin re-registration or session replacement re-renders the
  bar even while the panel is detached — render-time lookup alone is not
  enough because nothing re-renders on registration otherwise. The persisted
  `FloatingPanelDef.title` is only the reload fallback.

## Stacking

- `order` is allocated from the per-env monotonic `nextOrder` counter; after a
  group is removed, its order is not reused.
- **Exhaustion policy:** at the `nextOrder` cap (10 000), a new float fails
  non-destructively — the group stays pinned and a console warning is logged.
  No clamping or reuse (reuse would break monotonicity and stable z-order).
- Rendering order is stable: sort by `(order, groupId)`.
- Offsets and z-index are formulas of `order`: offset = `order × 12px` from
  the edge; z-index = `1000 + order`; an expanded window renders above
  collapsed bars of the same edge.

## Add-panel routing

A single identity resolver (`focusOrAddFloatingOrGridPanel`) is the only path
that decides expand-vs-create. Adding a panel that already floats
expands/focuses the floating window with that tab active; genuinely new tabs
insert into their intended grid group (or into the floating group only where
the action explicitly targets it); no duplicate grid panels are created for a
floated panel. Every single-instance/add action family (plan, changes, files,
terminal, preview, review, plugin, session, deferred actions) routes through
it, including direct `api.getPanel()` checks today.

## Session replacement

Session replacement is a **single coordinator over grid and floating entries**
— never two independent passes. `replaceStaleSessionPanels` returns an old→new
panel identity mapping, applied to floating entries (panelIds, definitions,
portal params/title, and `activePanelId`). The deterministic **winner** is the
group's active stale panel if it is stale, else the first stale floating panel
in the group's saved tab order.

- **Winner ownership is store-owned and explicit:** the coordinator writes a
  store field — `floatingSessionWinner: { sessionId, envId, generation } | null` —
  atomically with the replacement. **Delayed replacement has a readiness
  barrier:** a per-env replacement-generation state makes the always-mounted
  `useAutoSessionTab` hook DEFER its ensure while replacement is pending —
  once replacement completes, the winner decision is consumed before the
  hook effect proceeds; if the hook ever ran first and inserted the incoming
  id, the coordinator atomically re-evaluates and MOVES the already-added
  winner back to the floating entry (no id ever exists in both surfaces);
  tested with replacement-after-first-effect, StrictMode rerun, and delayed
  WS ordering. The field is **memory-only (never
  persisted)** and consumed by an atomic **compare-and-clear**
  `consumeFloatingSessionWinner(sessionId, envId, generation)` called from
  `shouldSkipPanelEnsure` (`dockview-session-tabs.ts`) before the hook's
  ensure effect runs — consumption is one-shot, so repeated/StrictMode effects
  cannot double-skip. Stale winners (generation or env mismatch) are cleared
  on generation/env transition and on every terminal path (ensure failure,
  unmount, env switch); a newer generation is never cleared by an older
  cleanup. The hook skips only the winner id for grid
  insertion/activation; unrelated current-session siblings are still ensured
  as today (their anchor is the existing
  `addCurrentSessionSiblings`/`ensureSiblingPanels` behavior, with an explicit
  anchor when the winner floats).
- **Winner floats, absent from the grid:** the incoming session id is **not**
  added to the grid (`restoreMissingSessionPanel`/`addCurrentSessionSiblings`/
  `useAutoSessionTab` ensure are all suppressed for that id); only the
  floating entry is updated. Without this, the same panel id would exist in
  both the grid and the floating overlay — panel identity is single-valued.
- **Winner is in the grid:** the floating stale copy is dropped entirely.
- Only the winner maps to the incoming id; every other stale floating panel
  is dropped, and `activePanelId` points to the winner's new id (or the first
  surviving panel if the winner itself was dropped). Duplicate mappings never
  occur because each floating entry maps at most one old id.
- **Placement normalization (defined moment, floating-winner edge):**
  `fromDockviewApi` derives unrecognized root column ids from the first panel
  id (`layout-manager/serializer.ts`), so a center chat group captured after
  chat was replaced by `session:<A>` can hold `columnId: "session:<A>"` —
  which no longer exists after a switch to `session:<B>`, and the materializer
  would insert a new root column instead of returning to center. Normalization
  runs in a **post-apply hook** — after the synchronous layout/session
  replacement AND the incoming-session insertion have completed (fast, slow,
  route-intent, maximize-restore, and reload paths alike) — and resolves the
  root column by **direct live group membership plus index**, never through
  `fromDockviewApi`'s panel-derived ids and never through `findCenterGroupId`'s
  fabricated fallback. **Floating-winner edge:** when the incoming session is
  the floating winner, it is intentionally absent from the grid and **no live
  center may exist** — the entry's center intent is **preserved** (the
  center-kind classification and saved center placement are kept, not
  downgraded to the custom fallback), and normalization is **deferred until
  materialization/dock**, when a real center is available (resolved from the
  live grid, or from the winner's saved/pre-max layout). When no real center
  exists at the normalization moment and the entry is not a center-kind
  winner, it keeps the custom fallback (vertical/right). Tested with an A→B
  session switch (fast, slow, delayed replacement), reload, maximize restore,
  winner-floats-then-docks (returns to center), and dock asserting center
  placement.

The coordinator runs before `restoreMissingSessionPanel`,
`addCurrentSessionSiblings`, **and `useAutoSessionTab`'s ensure path**
(`dockview-desktop-layout.tsx` always mounts the hook; it calls
`ensureSessionPanel` when the active session panel is missing from the grid).
The integration contract is the desktop-hook behavior, tested through the real
hook for stale-only-floating, stale-in-grid-plus-floating, and delayed
replacement orderings.

## Failure modes

- **sessionStorage unavailable (private mode / quota):** floating commits
  **fail closed** exactly like any other blob-write failure — the transaction
  rolls back and the group stays pinned, with a console warning. There is no
  separate "ephemeral floating" mode: a float that cannot be persisted does
  not happen, so a reload can never lose a floated group the user believes
  exists. (This is the single policy; see the State machine, steps 4-7.)
- **Layout rebuild / preset switch / env switch while floating:** the floating
  blob is re-applied after the grid restore completes (Restore call sites). A
  floated panel whose definition cannot be materialized (e.g. its session was
  deleted) is dropped from the floating state without error.
- **Env-scoped panels (browser, file-editor, vscode, commit-detail,
  diff-viewer, pr-detail) floated at env switch:** they are released by
  `releaseByEnv` on switch away exactly as if docked (the exclusion set only
  protects the current env's detach registrations); switching back
  re-materializes them fresh from the blob. Global panels (chat, terminal,
  changes, files, plan) keep their portal entries and state.
- **Session/tab deletion while floating:** removing a session tab that is
  floating removes it from the floating group's `panelIds` and `panels`; an
  empty floating group is removed entirely. Session replacement follows the
  Session replacement contract.
- **Maximize while floating:** maximize applies to grid groups only; floating
  windows render above the maximized grid. Floating the group that is
  currently maximized restores the pre-max layout first, then floats the
  group, as one transaction.
- **Partial transaction failure:** the recovery phase in the State machine
  (steps 4-7: journal marker, divergence-tolerant restore, portal-safe
  phases) guarantees the partial layout is never persisted before the
  portals-adopted phase, and a reload recovers deterministically from the
  operation journal.
- **Right-width enforcement with floated right groups:** `enforcePinnedTargets`
  must only restore the right column target when a **live pinned-right column
  exists** (`hasLivePinnedRightColumn(api)` — the concrete predicate defined
  in the toggle section). With all right-column groups floated, the live grid
  has no right column while the bit may still be true; the current
  `restoreColumnToTarget(sv, sv.length - 1, target)` (`dockview-pinned-enforce.ts`)
  would resize the center column to the right target. `rightPanelsVisible` is
  derived from the predicate at all times.
- **Root-column geometry (authoritative helper):** placement capture and
  materialization use one helper (`captureRootColumnGeometry(api, columnId)`
  / `applyRootColumnGeometry`) implemented against the live root splitview
  traversal (the same traversal `fromDockviewApi` uses), reading pinned,
  width, min-width, and max-width metadata at the **root column** level
  (including nested-root membership) and preserving them through
  apply/restore. The existing serializer inference (pinned only from
  `files`/`changes` presence) is extended to retain min/max metadata, and a
  **runtime capture round-trip test** (float → resize → dock → reload) proves
  the geometry survives — not only a pure fixture test.
- **`toggleRightPanels` show path with floated right groups:** the show path
  re-adds the right column wholesale from `defaultLayout()` (`dockview-store.ts`),
  which would re-insert `files`/`changes`/`terminal-default` while those ids
  float — duplicate identity across grid and floating surfaces, or dockview
  replacing the floating copy. The show path is floating-aware: **every**
  floating panel id (not only the default `files`/`changes`/`terminal-default`
  set — custom and ordinary right panels included) is excluded from the
  re-added column or the toggle is treated as an explicit dock for them.
  **Empty-state legality:** after excluding floating ids, groups with zero
  remaining panels are removed and the right column is dropped entirely when
  no pinned-right panels remain (an empty group/column serializes to an empty
  branch that dockview may reject and that corrupts later classification).
  **Filtering is tree-aware and identity-preserving:** the exclusion updates
  BOTH the column's flat `groups` AND its nested `tree` through one traversal
  keyed by stable group id (or object identity where available) that computes
  each filtered group **exactly once** and reuses that exact result in both
  the tree leaf and the flat array (the live capture shares the same
  `LayoutGroup` object between tree leaf and flat `groups`, so independent
  transforms would normalize a group twice or produce divergent copies);
  empty leaves/branches are removed consistently, and a post-filter assertion
  checks the tree leaf group-id set equals the flat group-id set. Tests cover
  shared-object-identity groups AND structurally-equal-but-not-identical
  groups, nested right columns, empty leaves, and ordinary/custom floating
  ids. **Total identity rule (M6):** the traversal is keyed by unique group
  id only — capture assigns unique logical ids to every group BEFORE
  filtering (per the stable-identity rule) or carries a WeakMap identity
  token from capture into both forms; undefined or duplicate group ids fail
  closed before filtering, and the post-filter assertion covers tree leaf
  ids, flat ids, and panel ids all matching.
  **One authoritative meaning for `rightPanelsVisible`:** the bit is exactly
  **live canonical pinned-right-column presence**, defined by the single
  concrete predicate `hasLivePinnedRightColumn(api, columnMetadata)` that
  consumes **only** the persisted `role` field from `EnvFloatingState.rootColumns`
  (captured from an explicit `LayoutColumn.role` assigned by presets/
  custom-layout normalization, with a documented migration default for old
  layouts — **never inferred from width, canonical group ids, or panel
  membership**): the predicate returns true iff a root column's persisted
  role is `"pinned-right"` AND the column is live with matching pinned state.
  Plan/preview/vscode side columns carry `side-other`/`custom` roles and
  always return false (matching today's preset assignments) and are never
  hidden as right-panel toggles; a custom pinned ordinary-terminal-only right
  column is recognized because its normalized role is `"pinned-right"`.
  Applying a preset never hides a plan/preview/vscode side column as a
  right-panel toggle. The bit is derived from this predicate at all times
  (restore, float, dock, toggle, preset/default assignment), never persisted
  as intent; show with no pinned-right panels and hide without one are
  **Role bootstrap ordering (no sidecar hole):** one coordinator-owned
  **`applyLayoutAndBootstrap`/`settleLayoutApply` wrapper** is the single
  choke point used by EVERY layout apply path — with an explicit **callsite
  table** naming initial saved restore, maximize-only restore, env-switch
  fast/slow, route-intent, custom envelope/legacy, preset, toggle,
  reset/default, maximize exit, and materializer paths: every native
  `fromJSON` and `applyLayout` call goes through one adapter (or an
  immediate `settleLayoutApply` follows each direct native restore), and a
  static AST/source-boundary test rejects bypasses. The wrapper invokes
  `ensureRolesBootstrapped(api, envId)` after each synchronous apply AND
  before enforcement/toggle/predicate reads; enforcement additionally calls
  bootstrap defensively before reading. Bootstrap normalizes live `LayoutColumn.role`
  values and rebuilds the in-memory `rootColumns` sidecar from the
  **authoritative live state** (first mount with an empty/absent blob
  included); the predicate itself fails closed (or bootstraps lazily from the
  live state) when called before bootstrap; **sidecar persistence is
  prohibited while `isRestoringLayout` or a transaction is busy**; then
  derives visibility/enforcement, then schedules complete-pair persistence;
  first-mount, empty-blob, preset/reset-invalidation,
  mid-restore, and immediate-resize/toggle-before-debounce tests cover the
  ordering.
  toggle, `enforcePinnedTargets`, and `dockview-layout-setup`'s
  detection all share this one predicate, and materialization prefers
  **validated live-layout geometry** over the persisted sidecar when both
  exist (a stale sidecar after an interrupted resize never wins over the
  live grid). Tested with one and both right groups floated
  (incl. an ordinary terminal id), hide→show while floating,
  float-last-right → hide → show → reload, plan/preview/vscode/compact
  presets (asserting the predicate and bit values), custom pinned terminal
  columns before and after float/reload with no floating groups, and
  container resize, asserting the serialized tree is legal (no empty
  branch/leaf).
- **Storage-write failure after float/dock:** the commit fails closed — the
  transaction rolls back (journal re-apply) and the group stays pinned with a
  console warning. A reload or env switch can therefore never observe a
  missing floated group that the user believes exists; the worst case is the
  pin simply not sticking.
- **Journal/floating state size budgets:** floating state plus the operation
  journal share the tab's sessionStorage quota with all task environments and
  other Kandev storage. Two caps are preflighted before any mutation (State
  machine, step 1): a **per-env cap** (96 KB: blob + journal before/after
  snapshots) and a **global floating allocation budget** across all
  environments' blobs + journals (384 KB), enforced by scanning the owned
  storage prefix. Exceeding either fails the transaction non-destructively
  with a console warning — the float does not happen, nothing is corrupted,
  and the user's existing pinned/docked state is untouched. Near-limit,
  multi-env-combined, and quota-full behavior is tested.
- **Reset layout / "clear UI state":** reset is an **id-aware docking merge**
  with explicit collision precedence: the reset layout owns group/column
  **placement**, the floating definition owns the panel **payload** (component,
  params, tabComponent) and saved tab order, and the active panel is merged
  explicitly — **scoped to valid definitions**, and **executed through the
  same single tree+flat mutation helper as the materializer** (the existing
  merger maps only `col.groups` and leaves `col.tree` stale while the
  serializer prefers the tree — reset/preset merges must transform each
  group once by stable identity, reuse the transformed object in both
  representations, remove empty leaves/branches, and assert tree/flat/panel
  id equality; nested-tree reset merge tests are required, not only
  materializer tests). `session:*` floating
  definitions are validated against the active task/session set and env
  before merging: stale/deleted/absent-session definitions are dropped, and
  when no valid session remains the reset chat placeholder/default behavior is
  retained (payload-wins is never a blanket override for sessions). **Valid
  session insertion is canonical:** every valid active session gets a
  session tab — via the merger's center-chat replacement when the reset
  target has a center column, and via a **documented fallback when it does
  not**: session panels are appended to the first column's first group and
  the active session is set there, so the auto-session hook observes the
  resulting identity without a second insertion. Existing
  reset panels (chat, files, changes, terminal) are reused by id — never
  duplicated — and the floating definition's payload wins for same-id valid
  panels, so a floated ordinary terminal keeps its real terminal id rather
  than being retargeted to the default `terminal-default` params. Floating
  tabs merge preserving saved tab order and the active panel; only missing
  definitions are added. Floating state (memory + storage) is cleared only
  after the merged grid is committed and persisted. The floating preference is
  cleared — groups do not re-float after reset. `cleanupTaskStorage` removes
  the floating key **and the journal key** (`kandev.dockview.env-floating-journal.<envId>`)
  alongside the layout/maximize keys. **Deletion is cancellation-safe:** task
  deletion/mount teardown first invalidates the env's transaction generation
  (the coordinator's settle drain and any in-flight phase re-check
  generation and refuse to write after invalidation), then cleans the keys —
  a later settle can never rewrite a deleted env's blob or journal after
  cleanup; task-deletion cleanup with an incomplete journal and with an
  in-flight transaction is tested.

## Persistence guarantees

- Floating groups (placement, tab definitions + order, active tab, orientation,
  edge, size, display, stack order) survive within-tab reloads and
  task-environment switches via the per-env sessionStorage slot, because the
  blob carries complete panel definitions and the layout invariant keeps the
  env layout consistent.
- Nothing survives a browser-tab close or a different device; this is the same
  durability tier as the environment layout and maximize state. The pin state
  is not backend-persisted.

## Scenarios

- **GIVEN** a task workbench with a right-column group (e.g. Plan), **WHEN**
  the user clicks the pin control to the left of maximize, **THEN** the group's
  tabs leave the grid, the remaining blocks reflow to fill the freed space, a
  floating window appears over the workbench on the right edge, and the control
  shows the unpinned state.
- **GIVEN** the right-column Plan group is floating, **WHEN** the user clicks
  anywhere outside the floating window, **THEN** the window collapses to a
  vertical title bar on the right edge listing the group's tab titles with the
  active tab highlighted.
- **GIVEN** a Plan group floating from the `plan` preset (a root column right
  of center with a generated group id), **WHEN** it is unpinned, **THEN** it
  classifies as side/vertical with a right-edge bar (never horizontal/bottom).
- **GIVEN** a bottom terminal group is floating, **WHEN** the user clicks
  outside it, **THEN** the group collapses to a horizontal title bar on the
  bottom edge.
- **GIVEN** a collapsed floating group with two tabs, **WHEN** the user clicks
  the second tab's title, **THEN** the floating window expands with that tab
  active.
- **GIVEN** a floating group is expanded, **WHEN** the user presses Escape
  while focus is inside the window and no descendant handled it, **THEN** the
  window collapses to its edge bar.
- **GIVEN** a floating group with a Radix DropdownMenu open inside it, **WHEN**
  the user presses Escape, **THEN** the menu closes and the floating window
  does not collapse; **WHEN** the user clicks outside the window while the
  menu is open, **THEN** the window stays expanded until the menu closes.
- **GIVEN** a floating group is expanded and focus is inside it, **WHEN** the
  user tabs out of the window into the workbench, **THEN** the window collapses
  (no pointer event involved).
- **GIVEN** a floating group, **WHEN** the user clicks its pin control (in the
  floating window header or the collapsed bar), **THEN** the group returns to
  the grid at its remembered column position with tab order and active tab
  restored, and the control shows the pinned state.
- **GIVEN** a group pinned in the grid, **WHEN** the user inspects the header,
  **THEN** the pin control is the first control to the left of maximize and
  reports `aria-pressed=true`.
- **GIVEN** a running terminal in a group, **WHEN** the group is unpinned,
  collapsed, expanded, and re-pinned, **THEN** the terminal process and its
  output remain live throughout (no reconnect, no restart).
- **GIVEN** the user floats the last group in the right column, **WHEN** the
  workbench is reloaded, **THEN** the right column and the floating group are
  both recreated from the blob (no center fallback, no lost tabs).
- **GIVEN** a floating group, **WHEN** the user switches tasks and back,
  **THEN** the group is still floating in the same display state with the same
  tabs; a floated chat tab tracks the incoming session (title, params, active
  id rewritten per the Session replacement contract), and a floated env-scoped
  panel (browser/file editor) is recreated on return.
- **GIVEN** a group is floating and the user clicks "Add Plan Panel" (or any
  add-panel action for a panel in that group), **WHEN** the action fires,
  **THEN** the floating window is expanded/focused with that tab active rather
  than a duplicate grid panel being created.
- **GIVEN** a group is floating, **WHEN** the user closes all its tabs,
  **THEN** the floating state is removed and no edge bar or window remains.
- **GIVEN** a group is floating and the user closes one of its tabs while a
  float/dock transaction is mid-flight, **WHEN** the transaction settles,
  **THEN** the closed tab is dropped from the floating definitions and the
  remaining tabs float; its portal was released by the close (no leak).
- **GIVEN** two groups float on the same edge, **WHEN** the user collapses and
  expands them, **THEN** they stack with deterministic offsets/z-order
  (`order × 12px`, z-index `1000 + order`), the expanded window renders above
  the collapsed bar, and each bar's titles remain clickable.
- **GIVEN** a floating group and a nested dialog (e.g. Radix AlertDialog) open
  inside it, **WHEN** the user presses Escape, **THEN** the dialog closes and
  the floating window does not collapse.
- **GIVEN** the user clicks Reset UI state while groups float, **WHEN** the
  reset completes, **THEN** all groups are back in the grid (merged id-aware
  into the reset layout), no floating storage remains for the environment, and
  the groups do not re-float.
- **GIVEN** both right-column groups are floated (no live right column) while
  `rightPanelsVisible` is true, **WHEN** the workbench resizes or a layout
  change triggers `enforcePinnedTargets`, **THEN** the center column keeps its
  size and no column is resized to the right-panel target.
- **GIVEN** a floating group contains the same panel ids as the reset default
  (e.g. chat or terminal), **WHEN** the user clicks Reset UI state, **THEN**
  the reset grid reuses those panels by id, floating tabs merge preserving
  saved order and active tab, no duplicate panel ids exist, floating storage
  is cleared, and the groups do not re-float.
- **GIVEN** a float transaction starts while a debounced layout auto-save is
  already scheduled, **WHEN** the timer fires or the tab unloads mid-
  transaction, **THEN** no partial layout is persisted; the env layout is
  written only after the transaction commits or the journal restores.
- **GIVEN** a floating `session:*` group is the winner of a session switch,
  **WHEN** the desktop `useAutoSessionTab` hook runs, **THEN** it skips
  grid insertion and activation for the incoming id (the id lives only in the
  floating group), and a grid winner is still ensured as today.
- **GIVEN** the user hides and re-shows the right panels while right groups
  float, **WHEN** the show path re-adds the right column, **THEN** floating
  panel ids are excluded (or the toggle docks them explicitly) and no panel id
  exists in both surfaces.
- **GIVEN** the floating blob write fails during a float commit, **WHEN** the
  transaction settles, **THEN** the transaction rolls back, the group stays
  pinned, and a console warning is logged — no panel is lost on the next
  reload.
- **GIVEN** a transaction whose journal write itself fails, **WHEN** the
  transaction starts, **THEN** it aborts before any mutation (journal writes
  are status-returning and read-back verified).
- **GIVEN** a crash immediately after the verified journal write (both target
  keys at before digests), **WHEN** recovery runs, **THEN** the before pair is
  verified and the journal cleared — the matrix verifies the selected target,
  never "always after".
- **GIVEN** a storage write silently truncates a value, **WHEN**
  `writeVerified` reads it back, **THEN** the mismatch is a failed write that
  enters journal rollback (mismatch-after-mutation is a matrix row).
- **GIVEN** a syntactically valid journal whose raw snapshot digest does not
  recompute, **WHEN** recovery runs, **THEN** the journal is treated as
  unreadable and quarantined (`.corrupt` suffix) — never trusted, never
  re-recovered on every restore; journal-free divergence rules apply with the
  caller's `envId`.
- **GIVEN** a plugin hoards a registration function across renders, **WHEN**
  it registers a layer root, **THEN** the host requires the render-bound
  capability and rejects the hoarded call (capability is revoked at unmount
  and plugin unregistration).
- **GIVEN** a restore was skipped and its settle scheduled the drain, **WHEN**
  a new transaction begins before the drain, **THEN** `begin` consumes the
  retained marker and the restore runs exactly once at the new settle against
  the fresh API/layout.
- **GIVEN** a saved flat placement is docked into a column with a tree,
  **WHEN** the materializer applies the shape-change mapping, **THEN** the
  flat index maps to the DFS leaf-order index (clamped) and the expected leaf
  is asserted.
- **GIVEN** the user switches presets while a floating group's column had
  pinned metadata, **WHEN** the sidecar rebuilds after the layout apply,
  **THEN** the stored root-column metadata is refreshed (stale pinned state
  never drives `rightPanelsVisible`).
- **GIVEN** a right column's tree leaves and flat groups share object
  identity, **WHEN** the tree+flat filtering helper excludes floating ids,
  **THEN** each group is filtered exactly once and reused in both
  representations, and the leaf/group id sets are asserted equal.
- **GIVEN** a restore was skipped because a transaction was busy, **WHEN**
  the transaction settles, **THEN** the settle drain rechecks
  envId/generation/api/marker and invokes restore against the fresh layout,
  clearing the marker only on success.
- **GIVEN** the user closes a still-live tab during an async transaction
  phase, **WHEN** the close fires, **THEN** it runs full cleanup (the detach
  registry is armed per expected removal, so the close was never registered
  as an expected detach).
- **GIVEN** a saved placement uses a flat groups index but the destination
  column has a tree, **WHEN** the materializer docks, **THEN** the tagged
  placement (`kind: "flat"`) applies the documented shape-change fallback —
  no group lands in the wrong leaf.
- **GIVEN** a task is deleted while its env has an in-flight transaction,
  **WHEN** the deletion invalidates the env generation and cleans the keys,
  **THEN** a late settle refuses to write (no post-cleanup resurrection).
- **GIVEN** a plugin renders two task panels, **WHEN** it registers an owned
  layer root from one panel, **THEN** the host validates the per-panel
  capability and rejects registration for the other panel.
- **GIVEN** the right column's nested tree contains floating ids, **WHEN**
  the show path excludes them, **THEN** the tree+flat filtering helper updates
  both representations — no stale tree reintroduces the ids.
- **GIVEN** a legitimate no-op transaction (before bytes equal after bytes),
  **WHEN** digest recovery evaluates it, **THEN** equality is treated as
  settled — no phase inference, no unnecessary write.
- **GIVEN** group-id allocation exhausts its attempt cap during salvage,
  **WHEN** materialization runs, **THEN** it fails non-destructively and the
  surviving group's defs remain persisted (never a partial merge).
- **GIVEN** a restore completion arrives while a float transaction is
  mid-phase, **WHEN** the coordinator gates it, **THEN** it is skipped with a
  retained pending-floating-restore marker, never executed from a stale
  snapshot.
- **GIVEN** the incoming session is the floating winner (no live grid
  center), **WHEN** placement normalization runs, **THEN** the entry's center
  intent is preserved and normalization is deferred until dock, which returns
  to center.
- **GIVEN** a skipped restore whose drain is invoked inside `settle`, **WHEN**
  the drain calls the restore path, **THEN** the coordinator-owned
  `drainPendingRestore` internal token lets it run despite the busy gate —
  no re-rejection, no recursion; a drain throw retains the marker for the
  next settle.
- **GIVEN** the user floats the maximized group, **WHEN** `restoreForFloat`
  runs, **THEN** the pre-max layout is restored through the busy-protected
  internal path — the maximize/exit guards never deadlock the float.
- **GIVEN** a corrupt journal's quarantine copy fails, **WHEN** recovery
  retries, **THEN** the original journal is retained and never cached as
  recovered (verified copy → verified absence → cache ordering).
- **GIVEN** an ordinary preset switch with no floating groups, **WHEN** the
  sidecar updates, **THEN** the blob/journal are not written (coalesced
  persistence — settled boundary + bytes changed only).
- **GIVEN** a plugin panel re-renders while a layer is open, **WHEN** the
  capability is re-read, **THEN** the portal-instance-generation token is
  stable (useRef) — the layer survives the benign re-render; a released and
  reacquired portal rotates the token, rejecting a hoarded old one.
- **GIVEN** a crash between quarantine copy and original removal, **WHEN**
  recovery retries, **THEN** the deterministic `(envId, raw digest)` key is
  found and verified and the flow proceeds to original removal — no second
  `.corrupt-<n>` copy accumulates across restarts, and bounded corrupt-key
  cleanup removes leftovers.
- **GIVEN** a group docks and later refloats after a sibling change, **WHEN**
  capture runs, **THEN** the durable identity map in the blob (surviving the
  empty floating state) supplies the same UUID — no new identity is minted.
- **GIVEN** a custom pinned terminal-only column, **WHEN**
  `hasLivePinnedRightColumn` runs, **THEN** only the persisted
  `role: "pinned-right"` (assigned by layout normalization, never inferred)
  qualifies it — plan/preview/vscode side columns with `side-other`/`custom`
  roles never do.
- **GIVEN** a throw during portal adoption in the drain, **WHEN** the
  `try/finally` settles, **THEN** busy clears exactly once (failed-settled)
  and the retained marker re-arms for the next settle — no stuck busy.
- **GIVEN** the same panel id floats in env A and env B, **WHEN** env A
  switches away, **THEN** the composite `(panelId, envId)` registry entries
  release the old entry and preserve the target env's registration.
- **GIVEN** the tab unloads after the blob write but before the layout write,
  **WHEN** the transaction-aware unload handler runs, **THEN** it writes a
  complete journal-consistent pair (default: both before values + aborted
  journal) with `writeVerified` — never a half pair.
- **GIVEN** the production bundle is inspected, **WHEN**
  `__floatingTestHooks__` is probed, **THEN** it is absent (the seam is
  compiled only into the E2E build via `VITE_FLOATING_TEST_HOOKS`).
- **GIVEN** a saved group id derives from traversal order, **WHEN** a sibling
  panel changes, **THEN** the persisted UUID identity is stable — the saved
  id never silently changes.
- **GIVEN** a pinned custom terminal-only right column, **WHEN**
  `hasLivePinnedRightColumn` runs, **THEN** the persisted `columnRole:
  "pinned-right"` recognizes it — plan/preview/vscode side columns with
  `side-other`/`custom` roles never qualify.
- **GIVEN** the same panel id floats in two envs, **WHEN** the old env
  switches away, **THEN** the env-qualified exclusion predicate
  `(panelId, entryEnvId, token)` releases the old entry while preserving the
  target env's registration.
- **GIVEN** the user exits maximize and reloads immediately, **WHEN** the
  pending-floating-restore marker is re-evaluated on the next mount, **THEN**
  the journal and marker remain consistent and the group restores correctly.
- **GIVEN** a task is deleted while its env has an incomplete journal,
  **WHEN** `cleanupTaskStorage` runs, **THEN** both the floating blob and the
  journal key are removed.
- **GIVEN** a plugin panel registers an owned layer root outside its task-
  panel portal, **WHEN** the host validates ownership, **THEN** the
  registration is rejected (host-issued capability, never trusted plugin ID
  closure).
- **GIVEN** a dock transaction crashes after the blob write (group removed
  from the blob, old floated layout still live), **WHEN** the workbench
  restarts, **THEN** the operation journal's before-state is re-applied and
  the group is recovered — the journal-free divergence rules are never the
  primary guarantee for a mid-transaction crash.
- **GIVEN** a crash leaves the floating blob and env layout divergent (blob
  says float, grid has the group, no journal), **WHEN** the workbench
  restores, **THEN** `restoreFloatingAfterLayout` reuses the existing grid
  group and floats it — no duplicate id, no lost panel.
- **GIVEN** a saved floating panel id exists in a different live group/column
  than the blob's placement (journal lost), **WHEN** restore runs, **THEN**
  the live panel is the deterministic authority for that panel (no duplicate
  materialized) and the group's non-conflicting panels are salvaged per the
  collision policy — no tab or payload disappears silently.
- **GIVEN** a floating group has one conflicting panel id (live in another
  group) and one non-conflicting tab, **WHEN** journal-free restore runs,
  **THEN** the non-conflicting tab and its order/display state are retained
  and the surviving group docks per the collision policy — nothing disappears
  silently, and the conflicting entry is logged.
- **GIVEN** the user floats the last right group, **WHEN** the toggle or a
  reload derives `rightPanelsVisible`, **THEN** the bit equals live
  right-column presence (false) in both cases, hide/show are no-ops, and
  enforcement never resizes the center to the right target.
- **GIVEN** a float transaction is mid-phase (e.g. portals-adopted), **WHEN**
  the user clicks another group's pin, **THEN** the action is rejected (pin
  disabled during the busy window) and no re-entrant transaction begins.
- **GIVEN** an incomplete operation journal exists for the env, **WHEN** any
  restore entry runs (including maximize-only and route-intent branches),
  **THEN** `recoverFloatingJournalOnce` runs before the first `fromJSON` and
  no ordinary restore/divergence logic interprets the journaled mutation.
- **GIVEN** a crash after the blob write (blob has after-digest, layout has
  before-digest, both keys individually valid), **WHEN** digest-based journal
  recovery runs, **THEN** the after pair is applied and verified — the four
  partial-write orderings each recover deterministically, including a write
  throwing after storage mutation.
- **GIVEN** a saved group id collides with a live group in the destination
  column during salvage/dock, **WHEN** materialization runs, **THEN**
  `allocateUniqueGroupId` reuses the id only when absent/owned, otherwise
  allocates a bounded generated id with a saved→live mapping — no overwrite,
  no merge.
- **GIVEN** the user switches to the plan preset (unpinned side column),
  **WHEN** `rightPanelsVisible` is derived, **THEN** the
  `hasLivePinnedRightColumn` predicate returns false (the plan side column is
  never treated as a pinned-right target and never hidden by the toggle).
- **GIVEN** a float transaction is mid-phase, **WHEN** a programmatic
  add-panel or reset action fires, **THEN** it returns a non-destructive
  no-op with a debug reason (all public layout-mutation boundaries are busy-
  guarded), and all three pin surfaces render disabled.
- **GIVEN** env A's journal is recovered, **WHEN** env B's journal recovery
  runs on the same api, **THEN** B's journal is recovered independently (the
  recovery cache is keyed by envId + transactionId + api instance; a new api
  instance re-checks a still-present journal).
- **GIVEN** two environments' floating state plus journals approach the
  global budget, **WHEN** another float is attempted, **THEN** the preflight
  fails non-destructively before mutation.
- **GIVEN** a task restores with a saved maximize state and floating entries,
  **WHEN** the maximize-restore branch runs, **THEN** the overlay is never
  mutated; a pending-floating-restore marker is set and materialization runs
  only after `exitMaximizedLayout`'s rAF settles, with the E2E asserting
  visibility during maximize.
- **GIVEN** a floating center group's column id derives from `session:<A>`
  and the task switches to `session:<B>`, **WHEN** the post-apply
  normalization hook runs after the incoming session insertion, **THEN** the
  entry's column is rewritten to the real live center column (or keeps the
  custom fallback when no real center exists) — never a fabricated or
  panel-derived id.
- **GIVEN** the floating blob plus journal would exceed the size budget,
  **WHEN** the user unpins a group, **THEN** the transaction fails
  non-destructively before any mutation, the group stays pinned, and a
  console warning is logged.
- **GIVEN** a center chat group floated under `session:<A>` is switched to
  `session:<B>`, **WHEN** placement normalization runs, **THEN** the entry's
  column identity is rewritten to the live center column and docking returns
  to center — no new root column is inserted.
- **GIVEN** a task reloads with a saved maximize state AND a stale floating
  chat session, **WHEN** the maximize-restore branch runs, **THEN** the
  journal is recovered first, floating session entries are reconciled (winner
  written before the auto-session hook), and floating state is restored
  against the saved pre-max layout without mutating the two-column overlay.
- **GIVEN** the tab unloads during a transaction, **WHEN** the single
  transaction-aware unload handler runs, **THEN** exactly one write occurs
  (the journaled pre-transaction layout), and no second listener overwrites
  it with the mutated grid.
- **GIVEN** a failed float leaves the recovery phase mid-way, **WHEN** the
  phase model completes, **THEN** portals are adopted before the journaled
  layout is persisted, and storage equals the pre-transaction layout.
- **GIVEN** the user resets with a valid active session but a reset target
  without a center column, **WHEN** the reset merges, **THEN** the session
  panel lands in the first column's first group with the active session set,
  and the auto-session hook does not insert a second one.
- **GIVEN** the user re-shows the right panels while all right groups float,
  **WHEN** the show path runs, **THEN** every floating id is excluded
  (including ordinary/custom right panels), empty groups and the empty right
  column are removed, and after a reload `rightPanelsVisible` is derived from
  the live grid — enforcement never resizes the center to the right target.
- **GIVEN** a Radix menu→dialog→menu replacement spans more than one frame,
  **WHEN** the same-frame lease expires, **THEN** the window may collapse
  (documented handoff boundary) — same-turn and same-frame replacements are
  the guaranteed cases.
- **GIVEN** a float transaction's journal rollback re-applies the pre-
  transaction layout, **WHEN** the rollback completes, **THEN** no portal was
  released and no terminal/vscode process was stopped (the rollback ran as a
  restore-gated, exclusion-set transaction).
- **GIVEN** the user resets while a stale `session:*` definition floats (its
  session no longer exists), **WHEN** the reset merges, **THEN** the stale
  definition is dropped and the reset chat placeholder is retained; a valid
  active-session collision keeps the floating payload.
- **GIVEN** both right groups float and the user re-shows the right panels,
  **WHEN** the show path excludes floating ids, **THEN** empty groups are
  removed, the right column is dropped when empty, and the serialized tree is
  legal (no empty branch/leaf).
- **GIVEN** a Radix layer closes and its successor opens in the same turn
  (refcount passes through zero transiently), **WHEN** the coordinator's
  microtask re-check runs, **THEN** the window does not collapse (generation
  and refcount are re-validated before applying pending collapse).
- **GIVEN** a persisted blob has `nextOrder` smaller than an accepted group's
  order, **WHEN** it loads, **THEN** `nextOrder` is normalized to
  max(accepted orders) + 1 and allocation stays monotonic.
- **GIVEN** two owned layers are open in one floating window, **WHEN** they
  close in either order (including via unmount or navigation without dismiss
  callbacks), **THEN** the window collapses only after both are gone, and a
  successor window reusing the group id inherits no stale pending collapse.
- **GIVEN** a stale `session:*` panel exists only in a floating group, **WHEN**
  the user switches tasks (incoming session active), **THEN** the incoming id
  is added only to the floating group (not also to the grid), and no panel id
  exists in both surfaces.
- **GIVEN** a float transaction whose second panel removal throws, **WHEN**
  the transaction settles, **THEN** the grid is restored to its pre-transaction
  layout, no floating entry is committed, and no portal was released.
- **GIVEN** a phone viewport, **WHEN** the user inspects the task surface,
  **THEN** no pin control renders (the dockview workbench itself does not
  render).

## Out of scope

- Dragging, resizing, or repositioning the floating window (v1 anchors to the
  remembered edge/size; geometry is captured at unpin time).
- Floating the app sidebar (outside dockview) or the message queue panel (its
  own pin already exists).
- Floating on phone viewports; mobile has its own non-dockview task
  composition.
- Backend persistence or cross-device sync of pin state.
- Multiple floating instances of the same panel.
- Grid layout edits made while floating are persisted as today (live grid);
  they do not affect floating groups and need no special handling.
- Branch-level orientation refinement for exotic nested tree splits inside a
  non-center column (root-column classification only).
