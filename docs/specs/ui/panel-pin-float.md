---
status: draft
created: 2026-08-18
owner: kandev
revision: 47
prior-round: 46
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
floatingGroups: Record<groupLogicalId, FloatingGroupState>   # in useDockviewStore
                  # KEYED BY THE LOGICAL ID (native groupId is a current hint
                  # only); actions/selectors use groupLogicalId; no native-key
                  # rekeying is ever needed
                  # Public actions accept groupLogicalId; grid-header clicks
                  # resolve via resolveLogicalGroupId(nativeGroupId, registry)
                  # and reject unmapped native ids
```

```
EnvFloatingState                     # persisted as JSON, versioned
  version      1
  nextOrder    number            # monotonic stack-order counter for this env
  groups       Record<groupLogicalId, FloatingGroupState>   # logical key (native groupId is a validated hint/value only)
  rootColumns  Record<columnLogicalId, { pinned: boolean; width: number;
                                  minWidth: number | null; maxWidth: number | null;
                                  role: "center" | "pinned-right" |
                                        "side-other" | "custom" }>
               # denormalized geometry/role CACHE, INSIDE the blob (NOT authority —
               # the validated v4 LayoutState / normalized-live registry is
               # the sole writable role authority):
               # role is captured from an explicit LayoutColumn.role assigned
               # by presets/custom-layout normalization (with a documented
               # old-v3 migration: runs ONCE as `migrateEnvLayoutV3(raw,
               # envId)` — a versioned, one-time-marked transform on the raw
               # v3 native JSON BEFORE fromJSON, with a **CLOSED role table**:
               # center candidate = the column holding chat or
               # session:* (chat-in-left/middle/right each assigned by the
               # table); **'stripped' means the chat/session PANEL DEFINITION
               # is removed while the root column is RETAINED**; chat-stripped
               # → the remaining column at the center position index (first
               # column when only two columns exist, else the middle index);
               # an originally-absent chat follows the same positional rule;
               # MULTIPLE chat/session candidates FAIL CLOSED (ambiguous);
               # zero-column layouts fail migration; **EMPTY-STRIP
               # representation: a column left with zero panels after
               # stripping RETAINS its logical column metadata in the
               # normalized sidecar but the empty native column is OMITTED
               # until a panel is materialized (the sanitizer already
               # removes empty leaves/branches and nulls an empty root);
               # stripped ids are removed from panels and every tree/flat
               # view list, activeView is repaired, empty branches are
               # pruned, and the logical center role is preserved
               # separately; only-chat, chat-in-right, both-empty, and
               # fromJSON-validity fixtures**; pinned-right = the pinned column containing
               # files/changes (tie → leftmost); side-other = known preset
               # side columns (plan/preview/vscode); custom = any unknown
               # column; no center/right candidate ⇒ roles assigned by the
               # closed table with no pinned-right role when absent; vectors
               # cover empty/no-chat, plan/preview/vscode, multiple right
               # candidates, and custom layouts; **raw v3 lacks
               # min/max constraint fields — missing old constraints are
               # NORMALIZED (not recovered) from the preset/default role and
               # constraint metadata, and the sidecar is persisted only after
               # a validated post-apply live capture**; migrated roles are
               # persisted before any hasLivePinnedRightColumn decision and
               # later restores consume only stored roles (never re-inferred)
               # — the registry/live role map REMAINS the authority; the
               # sidecar can only weaken (never strengthen) a pinned-right
               # qualification rebuilt in memory after every layout
               # apply and reload, invalidated on preset/reset/env switch,
               # covered by the same journal transaction, budget, and cleanup
               # as the groups — there is no third storage key; native
               # columnId is NEVER a sidecar key (a guard rejects/migrates
               # native-keyed sidecars). Persistence is
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
                  # sidecar as DIAGNOSTIC/CACHE input to hasLivePinnedRightColumn
                  # alongside live membership and pinned state — the
                  # validated registry/live-layout role map is the
                  # AUTHORITY (a stale/corrupted sidecar can never qualify
                  # a column; precedence: registry role > live pinned state
                  # > sidecar cache)
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
  tabComponent string | null       # wire schema: NULL not undefined (JSON drops undefined — persisted panels must round-trip; TWO DISTINCT FUNCTIONS: `normalizePersistedFloatingPanelDef` (load/persistence — maps absent → null BEFORE validation; the closed guard accepts ONLY string|null for this field and REJECTS undefined) vs `captureFloatingPanelDef` (live capture/registry/materialization — copies live fields WITHOUT recursively normalizing `params`; NO normalizer may walk `DockviewPanel.params`); the live `LayoutPanel` type becomes `tabComponent: string | null` (types.ts:1-7 no longer optional-undefined); absent/null/string/undefined/extra-key round-trip tests + load→capture→persist + live-param-preservation tests)
  params       Record<string, unknown>   # must be JSON-safe (see guard)
```

Persisted under sessionStorage key `kandev.dockview.env-floating.<envId>`
where `<envId>` is ALWAYS `encodeURIComponent(envId)` via the single
canonical key constructor (all examples in this spec use `<envId>` as
shorthand for the encoded form; the raw-interpolated v3/legacy keys are
migrated once — see Migration — with collision handling: an encoded key
colliding with a legacy raw key resolves by exact-bytes check, never by
overwrite).
**The layout/maximize key coexistence table is explicit (the stale
"alongside v3" phrasing is removed): v3 READ keys `env-layout-v3` and
`env-maximize-v3`; v4 WRITE keys `env-layout-v4` and `env-maximize-v4`;
v3 is deleted only after a validated v4 apply; the v4 env slot is the
SOLE authoritative layout surface.** Reads go through a versioned type
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

**Reset runs through the SAME journaled transaction as docking:** the
  merged grid + floating-blob clear are one coordinator pair (journal →
  blob → layout → verified); pair-persistence failure rolls back BOTH (the
  floating blob is NOT cleared on a failed reset commit) and returns the
  typed rejected result; a reset quota-full/partial-write test asserts
  grid, floating blob, journal, and next-reload consistency.** **Reset
  native rollback is EXPLICIT: an immutable PRE-RESET LayoutState/native
  snapshot is captured before the merged apply; the coordinator stays
  busy through apply → pair persistence → verification → (on failure) a
  ROLLBACK PHASE that re-applies + verifies the pre-reset grid under the
  same lease; PERSISTENCE SUPPRESSION (the `canPersistLayout` guard)
  covers the ENTIRE reset window — merged apply, pair persistence,
  verification, AND the rollback phase — so a rollback re-apply can never
  fire onDidLayoutChange/debounce/beforeunload hooks with intermediate
  state; the rollback has its OWN fromJSON budget (1 rollback apply per
  attempt, max 2 rollback attempts — separate from the forward-apply
  budget, asserted by call count); **TOTAL RESET BUDGET is stated
  explicitly: `1 forward + max 2 rollback = max 3 reset fromJSON calls`
  with separate forward/rollback counters; a rollback attempt that
  partially mutates or throws consumes its attempt and goes to the next
  (bounded), then fail-closed repair on exhaustion**; if the rollback itself partially
  fails or throws, the
  journal/repair record is RETAINED (fail-closed, materialization and
  portal adoption forbidden) and a fresh validated rebuild is required;
  memory `floatingGroups`/blob are cleared only after BOTH grid and pair
  commit; tests: reset partial-apply, blob-write failure, layout-write
  failure, rollback-throw, rollback fromJSON call-count, persistence-hook
  suppression during rollback, reload, portal-liveness.**
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
`logicalId`), preset merges, dock, and reset. **`logicalId` is MANDATORY in
normalized `LayoutGroup`/`LayoutColumn`:** a synchronous
`normalizeLayoutIdentities(state)` precondition runs on EVERY reset/custom/
default path (legacy v3 included — reset must never run against an
un-migrated layout) **and on EVERY restore route** (`readEnvLayoutForRestore`/
the single apply adapter) with **TWO explicit modes**: v3/legacy input
ASSIGNS deterministic UUIDs (migration); **v4 input is VALIDATE-ONLY** —
every group/column must carry a valid unique UUID-format `logicalId`, an
allowed `role`, and tree/flat identity equality, then a FRESH native-ID
registry is installed atomically after the single `fromJSON` and before
session replacement/reconciliation (missing/duplicate identity returns a
typed fail-closed result that prevents capture/materialization — v4 never
mints new ids). The operation is O(groups + columns), runs once per restored
state, and the normalized registry is invalidated/rebuilt on every
`fromJSON`, add/remove, dock, preset, reset, and resize (events enumerated);
call-order fixtures prove no capture/materializer runs before the check. **The live Dockview model
cannot carry custom fields, so a store-owned **normalized-live-layout
registry** keys logical metadata by the NATIVE group/column ids and is
merged into every capture: `fromDockviewApi`/capture accept that registry
and fail closed when a native object has no mapping; the registry is
**MEMORY-ONLY (never serialized)** — it is built from the validated v4
envelope after every `fromJSON` using only validated native-key hints plus
canonical semantic signatures, and atomically swapped in before session
replacement/capture (a v4 validation failure leaves native state untouched
and blocks materialization); it is rebuilt on add/remove, dock, preset,
reset, and resize — native-ID regeneration can never orphan a logical
identity. **ONE role authority: the validated v4 `LayoutState` envelope /
  normalized-live registry — **the registry EXISTS IN TWO NAMED FORMS:
  `plannedRegistry` (the PRE-CALL plan of native ids, built before the
  single fromJSON) and `liveRegistry` (the POST-CALL binding installed
  after fromJSON regenerates native ids); post-call native ids are
  MATCHED to planned logical ids by a VERSIONED SEMANTIC SIGNATURE
  (logicalId is absent from native Dockview JSON, so the match is:
  role + normalized root-column index + canonical tree path + sorted
  non-session panel-id/component multiset + bounded geometry projection
  [min, width, max], exact UTF-8 encoding, explicit tie rules); the
  matcher builds a BIPARTITE CANDIDATE MAP and REQUIRES a unique perfect
  assignment for every non-empty logical object — missing, duplicate, or
  partial assignment FAILS CLOSED (native state preserved, NO
  capture/materialization); **EXACT TIE RULE: candidates are ordered
  by the FULL signature tuple (byte-lexicographic); if two logical
  objects remain INDISTINGUISHABLE (identical signature AND identical
  tie values — e.g. two identical panel multisets in one column whose
  paths regenerated identically), the ENTIRE binding/materialization
  attempt is REJECTED (fail closed — never arbitrary ordering);
  fixture: two-identical-candidates-remain-identical-after-fromJSON
  with the expected fail-closed result;** fixtures:
  two-identical-candidates,
  inserted-column, cardinality mismatch; capture/materialization is
  PROHIBITED
  until the liveRegistry install succeeds — the spec's "installed after
  the single fromJSON" (identity section) and "built before" (restore
  contract) are thus BOTH true of their respective forms and the
  contradiction is resolved** — the floating-blob `rootColumns` sidecar is a
  CACHE only (its role entries are DENORMALIZED copies, never written
  back as authority, and `FloatingGroupState.columnRole` is marked a
  derived cache field — the v4 LayoutState/registry role map is the ONLY
  writable role representation); **the normalized registry RETAINS empty
  logical columns with their roles (an emptied, not-yet-materialized
  column stays in the registry so the materializer's logical-first lookup
  finds both identity and role — the live dockview grid omits it, the
  registry does not; conflict precedence is explicit: registry role wins
  over any cache copy);** the empty-blob bootstrap input is the validated v4 envelope
  (never membership inference); **v3-first-mount bootstrap is an explicit
  sequence: read v3 slot → run the migration transform → normalized v4 +
  native-ID registry installed → single fromJSON → THEN bootstrap
  sidecar/visibility; v4 is written and v3 deleted only after the
  validated apply (a v3-first-mount call-order fixture covers it)**; `hasLivePinnedRightColumn` accepts that
  validated role map; a no-floating first-mount test proves plan/preview/
  vscode = `side-other` and custom pinned terminal = `pinned-right`.
  **`LayoutColumn.role` is REQUIRED** in normalized columns and in
every built-in/custom-v4 constructor (presets gain explicit role fields);
membership inference is permitted ONLY inside the documented v3 migration,
and missing/duplicate v4 roles fail closed before `rightPanelsVisible` is
derived. **Layout persistence is
versioned:** the env layout key is bumped to **v4** and stores a **versioned
envelope** — `{ version: 4, dockview: <native serialized JSON>,
  layout: <normalized LayoutState carrying logicalId/role> }` — because
native dockview JSON drops custom fields while `fromDockviewApi` regenerates
traversal-derived ids; **every restore route uses one envelope-aware adapter
`readEnvLayoutForRestore()`** that unwraps to `{native, normalized}` with
**separate route contracts** — a wrong ordering creates duplicate session ids,
loses the incoming session, or mutates the maximize overlay:
- **Regular v4 layout (initial mount, env fast/slow, custom, preset,
  fallthrough):** **the single-`fromJSON` contract holds by CONSTRUCTION via
  ONE pure native-JSON planning transform:** the planner computes the final
  native JSON (session/ephemeral replacement, incoming insertion, fixups,
  sizing, role/placement reconciliation — EVERY post-apply transform folded
  in as pure state transforms) BEFORE the one `api.fromJSON` call, together
  with the fresh normalized native-ID registry; post-call work is
  **observational/rebinding only** (never a second `fromJSON`, never
  `applyLayoutAndSet`/materializer application). **Ground truth — explicit comparison matrix and enforcement order:**
  (a) EXACT bytes for the persisted envelope, panel identity, role, and
  placement fields; (b) canonical structural equality for the native tree
  and generated IDs; (c) GEOMETRY compared via a **bounded projection predicate**: inputs =
  `(plannedContainerW/H, assertedLiveW/H, rootWidths, minWidths,
  maxWidths, pinnedFlags)`; widths are projected/distributed with a
  documented algorithm and rounding rule, and accepted iff each root width
  is within `max(2px, 1% of the asserted container width)` of the projected
  value (chosen bound; min/max constraints respected); **pinned/side
  column widths are PLANNED (side columns planned in root-column array
  order); the CENTER width is DERIVED LAST as the remaining width after
  deterministic integer rounding (all remainder assigned to center),
  clamped/redistributed under min/max conflicts, and subject to the SAME
  projection/tolerance rule; **ZERO center-role columns is VALID v4**
  (centerColumnId=null, columns classify custom vertical/right;
  rightPanelsVisible/enforcement and reset/session insertion for the
  no-center state are documented); MULTIPLE center-role columns FAIL
  CLOSED (ambiguous); 350px, narrow-container,
  multi-side-column, and min/max-conflict vectors cover it**; resize during
  `[fromJSON, assertion]` is frozen/deferred, or the assertion recaptures
  and reprojects against the assertion container (one declared mode); a
  resize-between-plan-and-assertion fixture asserts the exact
  accept/replan behavior (widths are never byte-compared — the measured
  container differs per restore); the
  equivalence assertion runs BEFORE portal adoption/commit and a mismatch
  rolls back and replans (regenerated IDs and session insertion are covered
  by mismatch tests); the persisted after is the PLAN's raw after.
  **Enforcement ordering (mechanism):** **ONE exported enforcement
  implementation and ONE `hasLivePinnedRightColumn` predicate — the
  component-local duplicate is removed or routed through the shared
  adapter, with a source-boundary test rejecting direct panel-ID/column-
  count detection in either path and a runtime test exercising both
  layout-change and resize callbacks.** The coordinator exposes phases
  `planned → applied → asserting → rollback → committing`; BOTH proactive
  `enforcePinnedTargets` calls and the reactive `onDidLayoutChange` handler
  REJECT unless passed the active coordinator token/phase — **the token is
  a store-owned `{transactionId, generation, phase}` lease read by both
  paths (proactive store calls read it directly; the reactive callback
  reads it from the store at fire time), one atomic validate-and-mutate
  gate, and absent/stale tokens return the typed `{status:"skipped",
  reason:"stale-identity"}` result (non-user-visible cleanup outcome); **USER RESIZE is gated with a CHOSEN mid-drag policy: sash starts are
   disabled/ignored while any transaction/lease is active; if busy flips
   MID-DRAG, the active drag is ABORTED/INVALIDATED and its mouseup
   persistence is suppressed — **GEOMETRY ROLLBACK: the EXACT captured root widths and prior targets
   are restored VERBATIM under the same generation on abort via a
   restoration path that does NOT recompute/clamp (**a task-01
   implementation-time Dockview API SPIKE against the pinned
   `dockview-core ^4.13.1` is REQUIRED: document the exact
   `sv.setConstraints` + `resizeView` same-frame sequence and prove the
   captured widths survive narrow/min-max conflict cases — the "or
   explicit constraint override" alternative is NOT acceptable without a
   concrete API contract); **if the spike proves the override cannot be
   implemented (dockview recomputes constraints internally), the DESIGN
   FALLBACK is triggered: mid-drag rollback relaxes to within-constraint
   restoration (captured widths clamped to the captured min/max — the
   captured constraint set is still reapplied, so the deviation is
   bounded and documented) with a spec revision and a deviation note in
   task-01; the spike result determines the contract, and task-01 is NOT
   accepted until one of the two paths is recorded** (partial-width retention and re-clamped
   restoration are rejected); narrow-container and min/max-conflict cases
   assert the exact captured values and target storage; live width, target width, sidecar, and next-reload
   assertions cover busy-mid-drag and busy-before-mouseup; mousedown, mouseup, layout events, and
   persistence bind to the same generation token; pre-busy-start,
   busy-mid-drag, and busy-before-mouseup tests** — enforcement is
  INCLUDED in the pure plan's deterministic targets, and on an assertion
  mismatch suppression is KEPT through rollback, the plan is rebuilt from
  validated live state, asserted again, and cleared in a token-guarded
  `finally` (never leaving the grid uncorrected or a stale callback
  mutating it); a test changing the measured container between planned
  sizing and `onDidLayoutChange` asserts live, persisted, and next-reload
  identity under the declared comparison rules, plus throw/mismatch/
  resize-callback/stale-callback-after-newer-transaction cases. Then:
  unwrap + sanitize → `api.fromJSON` ONCE →
  session/ephemeral replacement and incoming-session insertion (folded into
  the planned JSON; runtime replacement is a no-op guard) → a
  **defined
  normalized-state reconciliation** with a **staged algorithm**:
  1. **Native→logical binding:** before the diff, map each native
     group/column to a normalized candidate — first by a persisted native key
     (when the v4 registry carries one), then by a **length-delimited
     semantic signature** computed over **CANONICALIZED inputs**: both sides
     are normalized AFTER session/ephemeral replacement, `session:*` ids are
     EXCLUDED from both multisets, and the index is the position in the same
     normalized root-column sequence (never the pre-diff native index, which
     shifts when a column is inserted/removed); ambiguous or duplicate
     candidates are rejected. Fixtures cover a column inserted before the
     candidate and an A→B session replacement.
  2. **Diff:** match priority = logical group identity → panel ID → the
     closed fallback (component/panel semantic, then position; otherwise
     fail-closed); exactly ONE live instance per panel ID; **native owns
     component/params/title/tabComponent for live/env-scoped panels** while
     **normalized owns identity/placement/role/active-tab**; persisted
     `session:*` panels EXCLUDED.
  3. **Ambiguity policy:** on any ambiguity the native state is preserved and
     only the unmappable normalized metadata is dropped (never a silent
     move of a terminal/editor/browser payload); duplicates are rejected.
  Duplicate-prevention fixtures cover same panel ID in two normalized groups
  and a session replacement that changes the first panel. → fixups/bootstrap
  → floating restore.
- **Maximize-only:** unwrap the native two-column overlay → `fromJSON` once;
  the normalized pre-max `LayoutState` stays ONLY in the store
  (`preMaximizeLayout`); it is **never applied while maximized** (no
  unmaximize, no overlay mutation); post-exit restore applies it. **Reload-
  while-maximized renders the saved floating content above the overlay via
  an explicit detached-portal protocol:** a new `acquireDetached(panelId,
  component, params, envId)` portal-manager API creates `PortalEntry`s with
  `api: null`, owned by an **owner token + generation**; **lease records
  make `release(ownerToken, generation)` ignore stale owners** (an old grid
  slot cleanup can never release a newly adopted portal), and the
  expected-detach registry token stays valid through all delayed removal
  callbacks; the **detached content contract** defines virtual active state
  with **implementable active-state authority**: the portal manager/store
  holds a **per-panel lease record `{ owner: "grid" | "floating",
  generation, apiInstance }`**, and EVERY Dockview `onDidActiveChange`
  callback routes through one atomic critical section
  `acceptPanelActive(panelId, source, generation, apiInstance)` that
  validates the source generation against the lease and mutates
  `activePanelId` only when the source is authoritative (a stale grid
  callback after float is rejected; callbacks for OTHER panels/groups are
  scoped to their own lease and never overwrite the floating group's active
  tab). **Authority transfer is one coordinator-owned synchronous operation:
  `transferPanelLease(panelId, from, to, generation, apiInstance)` runs
  atomically BEFORE the corresponding mutation** (float: grid owner revoked
  immediately before remove/`fromJSON`; dock: floating owner revoked before
  `fromJSON`, grid adoption after the post-rAF; maximize exit: same ordering)
  — external active callbacks are rejected during the transfer window, and
  floating callbacks validate owner+generation via an **explicit detached
  capability / null-api rule** (a floating tab click has no Dockview api —
  it authorizes with the lease's floating generation, never `apiInstance`).
  Same-panel dual mounting (grid slot + floating window) is allowed
  ONLY during the named handoff window (post-rAF adoption at dock) with
  exactly one authoritative owner at every instant; **the boundary is a
  NAMED STATE: `handoff = {operationUUID, generation, expectedPanelIds,
  phase}` — phase `adopting` STARTS at the atomic lease transfer; dual
  DOM is legal ONLY while `phase == "adopting"`; the window ENDS when
  the post-rAF callback has acquired every expected grid lease AND
  floating leases are released (phase `settled`), or TERMINALLY via
  verified rollback (phase `aborted`); stale grid cleanups after
  `settled`/`aborted` are rejected; assertions run immediately before
  transfer, during dual mount, after each portal, after rAF, and after
  settle**. **OWNER + BACKGROUND SAFETY: the COORDINATOR owns the rAF
  callback AND an ABSOLUTE generation-bound handoff DEADLINE (timer
  independent of rAF — a background/hidden tab that throttles or
  suppresses rAF can never leave dual DOM/leases/busy open forever);
  the rAF callback only REPORTS adoption; the coordinator ATOMICALLY
  verifies all expected leases, releases floating leases, and settles;
  deadline expiry CANCELS/ABORTS with verified rollback/rebuild,
  clears the rAF + timer, and returns the typed terminal result;
  hidden-tab/no-rAF tests**; **BACKGROUND-TAB ROLLBACK POLICY: the
  deadline ATOMICALLY claims the handoff, revokes floating authority,
  and performs a SYNCHRONOUS API rollback from the immutable snapshot
  WITHOUT rAF/layout measurement (no geometry reads — the snapshot
  supplies everything); visibility-only DOM adoption repair is QUEUED;
  if the synchronous API rollback itself cannot run hidden (adapter
  reports blocked), the coordinator PERSISTS QUARANTINE and sets a
  `visibilitychange` continuation with a HARD MAX deadline; hidden-tab
  tests assert native call count, lease state, busy clearing, and
  post-visibility repair**; `api: null` is never
  treated as inactive (`use-panel-active.ts` is extended/replaced and added
  to task-03's files). **Global api lease:** there is ONE live Dockview api
  and ONE global active transaction — env switch, task teardown, reset, and
  every restore acquire/reject that lease. **Per-env gates consult the
  GLOBAL lease: `isBusy(envB)`/`isFloatingTransactionBusy(envB)` return
  `lease-held` (not merely busy) while env A holds the lease, with the
  `{status:"rejected", reason:"lease-held"}` result and ALL-env
  suppression scope (matrix row 2); a per-env transaction can acquire the
  sole global lease only from the IDLE state (lease acquisition is part
  of `begin(envId)` and is rejected `lease-held` otherwise); no per-env
  mutation wrapper may run without holding the global lease — same-env
  and other-env calls are tested at every public boundary (A-busy /
  B-requested per operation).** (a switch to env B while env A is
  in mutation/portal-adoption is **deferred, not silently dropped**: the
  switch returns a discriminated `switched | deferred | rejected` result,
  retains the desired env, and retries once at settle with generation checks;
  **a second busy/failed retry is a ONE-SHOT terminal `rejected` transition**
  — the desired env is cleared (or surfaced explicitly) with a localized
  error/retry UI whose user action re-attempts or cancels (no implicit retry
  loop; target-deleted/unmount cancel the deferred intent);
  A-busy → B-requested → A-settle → retry-success/retry-busy/retry-failure,
  cancellation, unmount, and target-deleted assertions cover the state
  machine;
  every async
  phase rechecks `{envId, generation, api, currentLayoutEnvId}` before any
  mutation or write; deterministic A-busy→B-switch and
  A-settle-after-B-switch tests cover the race. **Lease-transfer abort
  path (boundary defined):** every mutation wrapper returns a typed failure;
  **transfer-back is legal ONLY when the wrapped mutation provably did not
  start (no api call/observable mutation)**; ANY throw/cancellation after
  invocation INVALIDATES the lease and the associated panel record in the
  same critical section, rejects active callbacks during rollback, and
  rebuilds ownership from the validated live state; pre-call,
  partial-mutation, cancellation, and unmount tests cover each branch. Tests cover
  float-before-active-event,
  delayed-old-grid-event-after-float, dock-before-event, maximize reload,
  different-panel callbacks, floating-click-during-handoff, and active-tab
  changes in both surfaces; the floating window adopts those detached
  portals in a documented stacking context (explicit z-index above Dockview's
  maximize overlay); on maximize exit the floating owner is marked released
  BEFORE `fromJSON`, the grid slot acquires each portal **exactly once after
  the exit rAF**, and one-owner-per-panel-id is asserted throughout (a grid
  slot and the floating window can never fight over the same DOM node —
  adoption is lease-based). Release-before-`fromJSON` and delayed-stale-
  cleanup tests cover the ownership boundary. The grid-side pending
  materialization completes at maximize exit. A route dispatcher selects maximize-only when a valid
  maximize blob exists, regular otherwise (malformed/failed maximize falls
  back to regular).
  **Maximize = TWO coordinator transactions (chosen):** transaction 1
  (overlay apply) performs exactly one native `fromJSON`, settles — while
  retaining a GENERATION-BOUND pending marker and the floating portal
  lease — and clears busy (unrelated layout mutations are NOT blocked for
  the whole maximized interval); **the pending marker/lease EPOCH IS
  PERSISTED in a versioned maximize v4 envelope** (`{version: 4,
  preMaximizeLayout, maximizedDockviewJson, pendingEpoch}`; v3 readers +
  v4 writers, marker-only/malformed migration behavior defined); a
  same-tab reload while maximized reconstructs a FRESH portal lease from
  live validated state at mount time — a persisted epoch is a marker, NEVER
  an owner token (marker-only and malformed-maximize cases are tested); **PORTAL FAILURE BOUNDARY SPLIT: preflight portal/lease failure
  (provably no native mutation) is retryable `rejected` (matrix 28);
  POST-partial-adoption failure (native mutation invoked) is TERMINAL —
  lease invalidated, quarantine + durable repair, fresh validated rebuild
  (matrix 29); the nested/repair failure path (spec fail-closed
  quarantine) is the same terminal class.** **The boundary is EVIDENCED
  and IMPLEMENTABLE: ALL task-workbench native mutations route through
  ONE coordinator-owned `invokeNativeMutation` adapter that (a) captures
  the PRE-ADOPTION native snapshot, (b) sets a `nativeMutationStarted`
  marker IMMEDIATELY BEFORE every `fromJSON`/native mutation call (the
  DOM `appendChild(entry.element)` in usePortalSlot is adoption, NOT the
  native call), (c) resets the marker after the call returns/throws;
  bypassing the adapter is a STATIC ERROR (all callsites rewritten; a
  source-boundary test rejects direct fromJSON calls); **ENCAPSULATION + MIGRATION CONTRACT:
  the adapter module is the ONLY exported mutation surface — raw
  dockview mutation functions (`fromJSON`, `layout`, `addPanel`,
  `removePanel`, `resizeView`, constraints, `applyLayout`) are NOT
  exported from their home modules (applier.ts, dockview-layout-
  restore.ts, dockview-env-switch.ts, dockview-store.ts), and the static
  gate checks EVERY mutation method against an approved-adapter
  allowlist, not just fromJSON. **EXISTING STORE ACTIONS REMAIN THE
  PUBLIC COMPATIBILITY SURFACE: every current `useDockviewStore` action
  (addPlanPanel, toggleRightPanels, applyCustomLayout, …) is KEPT and
  internally routes through the typed adapter methods (no consumer
  rewrite); **ONLY THE ADAPTER OWNS THE RAW `DockviewApi` MUTATION
  REFERENCE — an exported `DockviewObserverApi` FACADE (read +
  subscription members ONLY, no mutators — the raw type does not
  distinguish them, so the facade is a WRAPPED TYPE, not a cast) is what
  the store and all other modules receive; raw `DockviewApi` is PRIVATE
  to the adapter and consumers request mutations via adapter COMMANDS;
  the AST allowlist is a SECONDARY gate; a COMPILE FIXTURE fails if a
  store action can reach fromJSON/layout/addPanel through its stored
  handle; every task-workbench raw mutator (fromJSON, layout,
  addPanel, removePanel, resize, constraints, applyLayout) is
  enumerated; a test proves a store wrapper cannot bypass the adapter**;
  a caller manifest lists every store action + its
  adapter method.** matrix 28 is
  classified
  only when the marker is false AND the snapshot is unchanged; 29
  otherwise; tests: failure before adoption, after DOM adoption but
  before native invocation, during native invocation, after partial
  native mutation.** **MOUNT PROTOCOL (happens-before): the reload path runs an explicit
  HYDRATE TRANSACTION, `beginHydrate(envId)`, for EXACTLY ONE active env:
  it atomically claims the GLOBAL api lease from
  IDLE (begin-hydrate → validate envelope → install the validated
  logical registry + fresh floating lease → acquire detached portals →
  render/adopt → settle-hydrate) BEFORE registry installation or portal
  acquisition — the in-memory coordinator is IDLE after a reload while
  the persisted marker + detached lease already exist, so without the
  hydrate claim a competing restore/mutator could pass an IDLE check or
  another env could start first; OTHER envs with persisted markers are
  discovered deterministically (marker index) and hydrated SERIALLY on
  env switch; **hydrate uses an explicit handoff protocol with NO
  externally observable IDLE gap: coordinator states `hydrating(A) →
  hydrate-handoff(B) → hydrating(B)` under ONE generation/queue — the
  global lease is transferred DIRECTLY from A to B (reserve-B happens
  BEFORE release-A; all other begins are rejected while the queue is
  non-empty; a third env/mutator can never win the released lease);
  QUEUE CONTRACT: items are typed (`marker-hydrate` vs `switch-intent`),
  FIFO within type with SWITCH-INTENT PRIORITY over marker-hydrates (a
  user's switch intent is never starved by a marker backlog), duplicate
  switch intents COALESCE (latest target wins); **SELF-FIRST: a
  `switch-intent(B)` CLAIMS/CONSUMES B's marker as its own target hydrate
  BEFORE any other marker is processed (B is hydrated exactly once — the
  switch's own hydrate and any queued marker-hydrate(B) are the SAME
  operation, never duplicated; non-target markers are retained);**
  **`claimMarker(B)` + `beginHydrate(B)` are ONE ATOMIC
  coordinator/store transition (the claim removes at most ONE queued
  marker-hydrate(B); markerless B runs the plain switch hydrate — the
  claim is a no-op; duplicate switch intents coalesce to one claim).
  **MARKER QUEUE IDENTITY IS UNIQUE BY `(envId, markerGeneration)` with
  atomic upsert/dedupe — duplicate discovery or a retry marker can NEVER
  leave a second B marker; `claimMarker` consumes the COMPLETE
  equivalent set or proves only one exists; tests: duplicate-marker,
  switch-during-hydrate — one hydrate, one consume, one requeue on
  transient failure.** **THE COORDINATOR IS THE SOLE MARKER WRITER:
  marker records are ONE logical operation `{envId, markerGeneration,
  attempt}` — `markerGeneration` is allocated by the coordinator at
  discovery; a TRANSIENT-FAILURE retry REUSES the same key (attempt+1)
  so discovery+retry is ONE logical hydrate, never two items; atomic
  transitions: claim → success-consume OR claim → transient-requeue
  (same key) OR terminal-consume (durable); tests: duplicate-discovery,
  retry-after-claim, crash-before-requeue, terminal-consume.**
  **RELOAD CONTINUITY: on mount the fresh coordinator HYDRATES the
  persisted `(envId, markerGeneration, attempt)` as the SOLE record —
  generation is PRESERVED, a retry ATOMICALLY increments attempt (never
  restarts at 1 — restart would defeat retry caps and split one logical
  hydrate into many), a NEW markerGeneration is NEVER allocated for an
  existing record (would queue a second hydrate of the same env); a
  missing/invalid persisted attempt ENTERS the fail-closed repair path,
  never defaults to attempt 1.**
  HYDRATE FAILURE TABLE: transient pre-hydrate/adoption failure
  (validation, lease acquisition, portal acquisition) REQUeUES exactly
  one marker-hydrate(B) (claim undone — B is never stuck unhydrated);
  TERMINAL failure (quarantine path) CONSUMES the marker and records
  durable repair; a retry consumes the requeued marker exactly once;** when
  the queue drains,
  a generation-bound `hydrate-settled` signal wakes EXACTLY ONE deferred
  switch (the head item), which then begins its own hydrate; switch-
  during-hydrating(A) enqueues the intent ahead of marker-B;
  A/B/C + B-marker-queued + switch-during-A tests assert exactly ONE
  B hydrate;
  markers of non-active envs are retained, never consumed; while
  the pending marker remains, the
  hydrate lease is NOT released (transaction-1 lease semantics: released
  only at exit-restore or explicit invalidation); stale-env markers are
  deleted with generation invalidation; hydrate failure for one env is
  isolated (quarantine that env only); A/B/C scheduling tests; reload + competing
  env/mutator ordering tests + A/B-markers-with-switch-during-hydrate; the markerless reload test
  asserts BOTH content visibility above the overlay AND registry
  readiness (no unbound native identity / api:null-inactive first
  render).** **the state is renamed `pendingGridMaterialization` and its gating is
  UNambiguous: a valid maximize (epoch present OR `null`) suppresses GRID
  INSERTION ONLY — detached portal acquisition/rendering is IMMEDIATE and
  NEVER marker-gated (floating windows render above the overlay even
  markerless); the marker is consumed only after exit rAF + validated
  pre-max restore + successful grid-side materialization; a markerless
  reload assertion checks DOM visibility above the overlay AND absence
  from the overlay/grid**; **TWO EXPLICIT GATES WITH PRECEDENCE: `busy` (coordinator lease
  ownership — mutators return rejected/busy) vs `recovery-pending`
  (pending grid insertion — mutators return suppressed/recovery-pending;
  `isBusy` is NOT the only UI gate; each public mutator has an exact
  status+reason per gate, and controls render disabled for either)**
  **competing mutations (preset/reset/add-panel/layout) between the two
  transactions are REJECTED while `pendingGridMaterialization` exists
  (the safer option — rebase is rejected because it can silently restore
  stale pre-max over newer state); **every public mutator returns a
  DISCRIMINATED result (`{status:"rejected", reason, retry}` vs
  `{status:"applied", …}`) and affected controls are disabled with a
  localized reason (toast/banner) — per-operation-family tests; **the
  RESULT CHANNEL is explicit: `applied` (incl. row 36 normal success) is
  returned to AWAITED/PROGRAMMATIC callers and is SILENT for
  user-initiated normal operations (a successful pin click NEVER shows a
  success toast — localized UI is reserved for rejected/terminal/
  recovered-with-drops outcomes); no-success-toast + returned-result
  acceptance tests**; **ONE
  public result algebra covers EVERY layout-mutating boundary: pin
  click, float/dock, storage quota-full, journal/quarantine,
  busy/lease-held, stale identity, and recovery failures each return the
  same discriminated type with the EXHAUSTIVE reason enum (generated
  from the committed matrix's `OperationReason` set —
  result-matrix.md):
  `busy | lease-held | quota-full | settle-timeout | invalid-definition |
  stale-session | stale-identity | recovery-pending | journal-unavailable |
  quarantine | repair-active | recovered-with-drops | apply-failed |
  portal-failed | plugin-contract-failure | stale-capability |
  intent-cancelled | automatic`
  (intent-cancelled = env-switch user-cancel/unmount/target-deleted, ONE
  row — never also stale-identity; stale-capability = revoked-cached-
  capability rejection, matrix row 41, locale
  `task:floatingError.staleCapability`) and
  locale keys `task:floatingError.*` (one per reason) + `task:floatingRetry`
  + `task:floatingCancel`; **an exhaustive OPERATION × REASON × ACTION
  matrix is COMMITTED as `docs/plans/panel-pin-float/result-matrix.md` —
  one closed `LayoutMutationResult` union (applied | pruned |
  recovered-with-drops | skipped | rejected | suppressed | terminal) with
  ALL rows (count GENERATED from the file — the generator parses rows,
  never trusts headings; never hand-copied), every
  one carrying exactly one (status, reason, action) mapped to suppression
  scope, user action,
  terminality, and clearing condition (implemented as a switch with an
  exhaustiveness test over the CLOSED operation-state union): each row defines suppression scope (none / current
  env / all envs), user action (retry / cancel / repair-clear / export /
  none), terminal-or-transient, and what clears it — `busy`/`lease-held`
  = transient, auto-retry or disabled control; `quota-full` = transient,
  retry after freeing space; `journal-unavailable` = suppression until a
  verified read succeeds (retryable, never blind); `quarantine` /
  `repair-active` = suppression until repair-clear, user action is
  clear/export NOT retry (locale key `task:floatingError.repairActive`);
  `recovery-pending` = active DEFERRED state
  (controls disabled, not terminal); `stale-identity` /
  `invalid-definition` = pruned/recovered-with-drops result;
  `pruned` = expected stale-session outcome (`task:floatingError.pruned`,
  silent by contract); `recovered-with-drops` = salvage outcome with the
  dropped [{id, reason}] list surfaced in repair UI
  (`task:floatingError.recoveredWithDrops`);
  `settle-timeout` = terminal rejected + retry/cancel; NO UI path may
  attempt materialization while quarantine, repair-active, or
  journal-unavailable suppression is active**; the
  "non-destructive no-op with debug reason" and "console warning" paths
  are REPLACED by typed results surfaced as localized failure/retry/cancel
  state, and every terminal result is tested**;
  — it never silently overwrites newer state with stale pre-max; env
  switch never releases a still-owned portal lease — **an env switch is
  DEFERRED until the floating transaction/portal ownership settles, then
  lease release/transfer happens BEFORE `releaseByEnv` runs (tests:
  A-maximized/B-requested, env-scoped floating panel); **a coordinator-owned
  SETTLE DEADLINE applies: one timer handle + `{generation, intentId,
  deadline}` with an ABSOLUTE 5 s deadline from the switch request (no
  progress-reset — steady progress cannot defer expiry); settle and
  expiry are serialized atomically: settlement CLAIMS the intentId
  (compare-and-clear) before completing, so an expiry that fires at the
  same moment sees the intent already claimed and becomes a generation
  no-op; expiry cancels with the typed rejected result + localized
  retry/cancel UI; late callbacks (settle after expiry, expiry after
  settle) are generation no-ops; boundary ordering is tested with an
  injected timer, not sleeps; hung-lease, timeout, unmount, and
  target-deletion paths are covered**; transaction 2 (exit) starts after the
  exit rAF, consumes the pending marker, performs the regular pre-max
  restore with exactly one `fromJSON` under coordinator-internal busy
  ownership, runs identity/session validation and the planned-equivalence
  assertion BEFORE portal adoption, then settles and releases the lease; **ROUTE SELECTION HAPPENS BEFORE ANY NATIVE APPLY: when a VALID
  maximize envelope exists, the maximize-only route is chosen FIRST
  (regular env-layout fixups NEVER apply maximize — the live
  `applyFixupsWithMaximize` chain in dockview-layout-restore.ts:161-
  169/252-253, which calls `api.fromJSON` a second time, is REWRITTEN to
  a single selected route; env-layout + maximize-blob coexistence gets a
  call-order test).** **MAXIMIZE ENVELOPE FRESHNESS: the v4 maximize
  envelope PERSISTS an immutable base-layout digest + the env
  layout generation; **DIGEST BYTE CONTRACT: the input is the EXACT
  PRE-OVERLAY RAW v4 ENV-LAYOUT sessionStorage string, captured at route
  start before any overlay mutation, hashed as UTF-8 bytes; on route
  selection it is compared against the EXACT CURRENTLY STORED string +
  generation; NO structural-equivalence tolerance (a benign
  reserialization that changed bytes is a digest mismatch = stale —
  freshness is byte-exact by design); stable-key/number rules apply only
  to migration-generated bytes; golden-vector + reserialization test.**
  **CANONICAL WRITER INVARIANT: EVERY v4 env-layout write (settled
  debounce, unload, preset, reset, env-switch, migration) uses ONE
  canonical serializer with VERSIONED canonicalization rules (stable
  key order, number formatting, omission/null rules) — benign
  reserialization is impossible by construction, so byte-exact freshness
  never discards a usable maximize; a golden semantic-equality +
  identical-bytes reserialization test covers all writer paths.** route matrix: (a) schema-valid AND base digest
  matches the CURRENT env layout ⇒ maximize-only; (b) malformed envelope
  / no native call needed ⇒ discard + fall through to regular;
  (c) REGULAR LAYOUT CHANGED since maximize (digest mismatch) ⇒ REGULAR
  WINS or explicit fail-closed repair — a stale pre-max overlay is
  NEVER reapplied over newer layout state; (d) both surfaces describe a
  maximized layout ⇒ ONE canonical source (the env layout) with an
  equality assertion against the envelope's stored layout; tests:
  changed-layout-after-maximize, redundant-maximized-layout,
  stale-envelope, before any native apply.** **"one fromJSON" is defined per
  ATTEMPT with a bounded retry (max 2 attempts, second attempt re-plans
  from the frozen pre-restore state before calling fromJSON again);
  resize-between-plan-and-assertion is tested and the actual call count
  is asserted; **failure-boundary contract: retry is eligible ONLY for
  the declared transient boundary (equivalence-assertion mismatch after a
  resize); **MAXIMIZE FALLBACK SPLIT: schema-invalid or PRE-CALL failure with
  `nativeMutationStarted=false` MAY fall through to the regular route;
  ANY invocation/partial mutation (marker true) enters the
  TERMINAL/quarantine/rebuild path FIRST — only after VERIFIED native
  restoration may the regular route be selected (never a blind second
  apply; maximize-specific throw-before-call, throw-after-mutation, and
  recovery tests with exact fromJSON counts).** **FALLBACK CASCADE
  TABLE keyed by (max route attempted, marker, snapshot equality,
  regular-route mutation state): (a) maximize pre-call failure → regular
  pre-call failure ⇒ regular's typed failure returned (no maximize error
  leaks); (b) maximize pre-call failure → regular INVOCATION/partial
  mutation ⇒ REGULAR quarantine/rebuild path (one terminal owner — the
  maximize pre-call error is NEVER returned after regular mutation);
  (c) maximize post-mutation failure ⇒ maximize terminal path (regular
  never selected); (d) the maximize envelope is discarded only AFTER a
  verified regular success (retained across a regular failure);
  exact-call-count + storage-envelope tests.** **POST-FALLBACK CLEANUP
  IS ONE IDEMPOTENT COORDINATOR TRANSACTION: verify regular pair →
  write a DURABLE cleanup phase/record → clear the maximize envelope
  + persisted epoch → compare-and-clear the in-memory
  pendingGridMaterialization marker under the same generation → verify
  absence → publish settled; a crash at ANY boundary is recovered by
  re-running the same idempotent cleanup (recovery distinguishes
  "regular pair committed, envelope still present" from "regular
  failed" via the cleanup record); storage + marker assertions per
  boundary.** persistent structural mismatch, a fromJSON throw after partial
  native mutation, stale registry, or portal/lease failure FAILS CLOSED —
  lease invalidated, rebuild from the IMMUTABLE PRE-CALL native/layout
  snapshot + registry captured before the first fromJSON (never a
  post-partial live capture, which is by definition untrustworthy), never a blind
  second fromJSON; exact call counts asserted per class**; **the REBUILD
  ITSELF is a nested coordinator transaction: it runs INSIDE the already
  held outer lease via a PRIVATE entry protocol `beginNested(outerToken)`
  → `advanceNested` → `settleNested` (same generation, unique nested id,
  legal parent phases: nested entry is valid ONLY from outer
  `restoring`/rollback phases — the one-fromJSON equivalence RETRY is a
  DISTINCT NON-NESTED transition: the second attempt re-plans from the
  frozen pre-restore state and re-enters `planned → applied → asserting`
  inside the SAME outer transaction (no nested id, no beginNested);
  `advance`/`settle` are nested-specific
  operations — the OUTER `settle` called while a nested transaction is
  active returns the typed `{status:"rejected", reason:"busy"}` result
  (matrix row 27); the caller waits and retries settle after
  `settleNested`; token-guarded failure/abort cleanup per nested phase
  with defined reload behavior for each), with its own nested phase enum
  `nested-prepare → nested-apply → nested-verify → nested-settle`
  (nested-abort/repair on any failure), immutable native/layout/registry
  pre-call snapshots, a bounded fromJSON budget (1 per nested attempt,
  2 attempts), and suppression OWNERSHIP inherited from the outer lease
  (all persistence hooks stay suppressed through the nested apply,
  verify, and any nested rollback); the persisted pair is journaled
  BEFORE the rebuild apply, and on rebuild
  failure — including a rollback that partially mutates or throws —
  enter a DURABLE FAIL-CLOSED repair state (quarantine + repair record;
  materialization and portal adoption forbidden; a fresh validated
  rebuild required), never claiming recovery; a partial NATIVE mutation
  is durably represented by the retained journal + repair record (a
  reload re-enters recovery against that record, never the live grid);
  tests: partial first apply
  + partial rollback, rollback throw, reload after rollback failure,
  exact portal ownership, nested-phase crash at each boundary**;
  token/generation ownership, busy-clearing, and portal release/adoption
  ordering are specified per transaction; tests cover overlay-settle, exit,
  reload, env switch, and a competing mutation between the two phases.
**Fast env-switch is a distinct zero-`fromJSON` route:** it mutates
  panels/active views in place (the live fast path) and has its OWN
  planning/equivalence/identity/enforcement contract with separate
  call-order tests — it is NOT part of the regular single-`fromJSON` route.
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
**Migration UUID derivation is crash-idempotent and deterministic with a
versioned wire protocol:** canonical input is UTF-8 encoded as
`v1` ‖ `kind("group"|"column")` ‖ `role` ‖ sorted unique panel ids — each
field length-delimited (4-byte big-endian length prefix per field), sorted by
UTF-8 bytes — hashed with a **synchronous, cross-runtime SHA-256
implementation** (pure-JS — `crypto.subtle` is async and unavailable in some
insecure contexts; Node and browser paths must produce identical bytes, with
jsdom/Node/insecure-HTTP tests). **The one-time migration hash is the ONLY
synchronous hash** — ordinary journal hashing for float/dock commits uses
**a deterministic operation plan computed synchronously at `begin`, BEFORE
any mutation**: the plan derives the final after `LayoutState` (float
removal / dock materialization are deterministic transforms), serializes the
immutable exact raw before/after strings and the journal schema ONCE, and
the mutation must EXECUTE that plan or abort (a user close or callback that
changes the planned result aborts the transaction and re-plans, never
mutates a stale plan); `begin` assigns the generation and marks busy before
async hashing starts; all mutations are rejected/held while hashing is
pending; digests are cached keyed by `(envId, transactionId, pair bytes)`.
**Hash-pending unload uses an explicit `aborted` journal form:** if the
after digests are still pending, the synchronous unload path writes the
cached before pair plus a structurally valid `aborted` journal (never a
half-formed marker that fails the journal guard's digest recomputation);
**unload states are SPLIT: `afterDigestReady` (hashes cached) vs
`afterPairVerified` (blob/layout writes read back and verified) — a
cached digest alone never authorizes an AFTER write; only
`afterPairVerified` does. Cached-digest-but-unverified unloads write the
BEFORE pair + `aborted` (tests: hashing finished but storage
verification has not).** **Explicit unload phase table: digest-ready
alone ALWAYS writes BEFORE + `aborted`; ONLY `afterPairVerified` writes
AFTER + normal completion.** Mutation-and-unload-while-hashing-pending,
unload-before-each-digest-completes, and close-during-mutation tests cover
the ordering. The hash implementation is a **named direct
declared dependency** (e.g. `@noble/hashes/sha256`) or an audited vendored
module — never an undeclared transitive import — locked in `apps/web/
package.json`, with a static dependency check and a benchmark/threshold
acceptance for worst-case 96 KB blobs; a mutation-and-unload-while-hashing-
pending test covers the ordering. The canonical
  input maps to a UUID by taking the SHA-256 first 16 bytes, setting byte
  6's high nibble to `0x5` (version 5) and the RFC 4122 variant bits in
  byte 8, with documented byte order and string formatting and golden
  vectors; duplicate panel ids, empty sets, and duplicate source keys are
  rejected BEFORE hashing; a `canonicalKey → source path` map is maintained
  during migration to reject post-truncation UUID collisions before writing
  v4; v3 derivation runs ONCE (v4 logical ids are thereafter authoritative
  and membership changes NEVER re-derive them). The blob's `identities` map is a
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
   the typed `{status:"rejected", reason:"busy"}` result while busy (a programmatic add
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
   { version, envId, transactionId, phase: "mutating" | "aborted",
     before: { floatingDigest, layoutDigest },
     after:  { floatingDigest, layoutDigest },
     raw: { beforeFloating, beforeLayout, afterFloating, afterLayout } }
   ```
   **Attempt state is DURABLE in the journal: `attemptState:
  "not-started" | "started" | "returned"` is WRITTEN BEFORE native
  invocation (verified write, same crash semantics as the pair) and
  updated to `returned` only after the native call completes and the
  pair read-back verifies; the both-equal NO-OP row (matrix 15) requires
  `attemptState == "not-started"`; a both-equal journal with
  `started`/`returned` runs the native snapshot rebuild path (matrix
  16/17); UNKNOWN/missing attempt state after reload FAILS CLOSED
  (rebuild/quarantine — never clears as no-op); tests:
  crash-after-marker-before-call, during-call, equal-after-call.**
  **`started` MEANS "the native invocation MAY have begun": recovery
  REQUIRES the rebuild even for the crash-window-after-marker-write-
  before-call (a wasted native apply is ACCEPTED and is the documented
  conservative contract — never a no-op, never a second blind mutation);
  the rebuild runs with persistence suppression and portal exclusion
  (no layout/removal events or portal churn visible to the user), and
  tests assert bounded calls + no user-visible detach.**
  **`aborted` semantics (exact):** a structurally valid `aborted` journal
   is the SAFE-BEFORE-PAIR form written by the hash-pending unload path —
   its `before` digests are the cached pre-mutation pair, its `after`
   digests are the SAME cached pair (nothing was applied), and recovery
   treats it as a both-before row (never
   quarantine); **recovery write contract (phase-specific): if the
   current stored bytes match the `before` digests, recovery performs ZERO
   writes — verify equality and clear the journal; only a genuine
   before-mismatch (current bytes differ from the before digests) writes
   the before pair via verified writes before clearing; an unexpected
   digest remains fail-closed (quarantine + repair record)**; **journal
   clear is itself a verified operation: `clearVerifiedJournal` reads back
   absence and caches recovery success ONLY after absence is verified;
   clear failure retains the journal + suppression state and returns a
   typed retryable result (retried on the next recovery); PARTIAL
   before-pair writes during aborted unload are safe by construction —
   the before digests are the pre-transaction state, so any subset of
   before writes still matches the before digests and recovery completes
   the before pair idempotently; clear-throw and partial-write tests are
   specified**; the journal guard, type guard, and recovery matrix all
   accept `aborted` explicitly, and round-trip + unload-before-digest
   recovery tests cover it.
   **Digest protocol (exact):** each target storage value is serialized
   **once** and converted to bytes via the **canonical UTF-8 `TextEncoder`
   conversion** (sessionStorage stores DOM strings, not bytes — the
   string-to-byte encoding is part of the contract; all size budgets count
   UTF-8 bytes, and Unicode/non-ASCII golden vectors cover parity across
   jsdom/Node); the digest is a **tagged union** — `{kind: "absent"}` or
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
   **Write/verify ordering:** journal → blob → layout → **read-back verify
   of both keys → journal clear**; **the `committed` phase is REMOVED: the
   VERIFIED PAIR is the sole terminal evidence — digest equality settles
   both-after rows, so a `mutating` + both-after row is a TERMINAL
   equality row (verify equality, clear the journal, never re-mutate, and
   never write a second marker); there is no advisory marker to retry or
   gate downstream state**; a throw after any storage
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
   | both-after or both-equal (no-op) | settled ONLY when the no-op PRECONDITION holds (planned normalized native layout/identity/geometry AND pair bytes all equal AND no native mutation was invoked) | after digests (equality needs no write) |
   | **unexpected (a key's digest is neither its before nor its after)** | **fail closed** | **ONE policy for EVERY untrusted journal (invalid OR mismatched OR unexpected): quarantine the raw journal under the deterministic key AND persist a durable, VERSIONED repair record** (`kandev.dockview.env-repair.<envId>` with `{version, transactionId, envId, createdAt, journalRawDigest, quarantineKey, targetRawDigests}` — **`journalRawDigest` is DEFINED as SHA-256(TextEncoder(exact raw journal string read before quarantine)) with a golden vector; on load it is recomputed, the quarantined value's digest must match, and `quarantineKey === deterministicQuarantineKey(envId, journalRawDigest)` is required; mismatch ⇒ malformed ⇒ fail closed (nothing is deleted)**; the quarantine digest/key and target raw digests are IN the record so `clearRepair` identifies the exact quarantine copy; a record missing these fields is malformed and fails closed) — an **app-shell repair registry** scans only the owned `env-repair.*` prefix and renders an env/task-labeled banner on home, settings, AND task mounts (**CURRENT-TAB scope documented: sessionStorage is per-tab; cross-tab records are not visible until the tab mounts the env**); **the quarantine digest/key and transaction identity are part of the guarded repair state, and EVERY restore gate returns `quarantined` — suppressing materialization/salvage — while a repair record exists, even after reload/new API instance (the in-memory cache never bridges reloads; the durable record does)**; `loadRepair(envId)` runs on every mount and `clearRepair(envId, transactionId)` is token-guarded with **verified deletion ordering (repair record DELETED LAST, after quarantine → floating blob → layout/journal are all verified; or the clear is journaled and the record restored on any later failure)**, memory updated only after the entire deletion transaction is durably verified (a failed clear keeps the banner); **the clear uses a DURABLE CLEAR JOURNAL — key `kandev.dockview.env-repair-clear.<envId>` with `{version, envId, transactionId, generation, targetDigests, phase}` schema and an explicit phase machine `pending → verifying → done` (failed/unknown retained as `pending`; each phase has an idempotent reload action; recovery order = **write `pending` BEFORE any deletion → delete/verify quarantine → transition `verifying` → verify quarantine absence + record consistency → delete/verify the original journal + repair record → write `done` and delete the clear journal LAST** — **the clear journal is its own durable record (`kandev.dockview.env-repair-clear.<envId>` = `{version, envId, transactionId, phase: "pending"|"verifying"|"done", targets: [repairKey, quarantineKey, journalKey], createdAt}`); the delete+verify of EACH target is itself journaled (the phase advances only after the previous target is verified absent); crash-before/after each delete is recovered by re-running the phase machine; a repeated mount re-enters the machine idempotently; a PRE-CLEAR ADMISSION CHECK reserves the exact clear-record bytes before any deletion — if the reservation fails (quota-full-before-clear), NO deletion starts, suppression stays up, and the user gets the typed `{status:"rejected", reason:"quota-full"}` + retry UI**; **`done` is a TERMINAL, idempotently re-deletable record that RETAINS suppression: a mount that sees a `done` clear journal (with repair/quarantine already absent) re-deletes it and keeps suppression until clear-journal absence is verified — suppression lifts only when a mount observes BOTH no clear journal AND no repair record**; any present repair record OR incomplete clear journal keeps restore/materialization suppressed until verified completion; crash-at-each-delete tests), recovery-before-restore entry, owned-key-index/budget membership, and `cleanupTaskStorage` deletion after generation invalidation (task-01 owns it) — with idempotent retry and generation checks (a partial clear never loses the suppression record while quarantine remains, and never leaves indefinite suppression without a retry state); clear-failure, reload/navigation-mid-clear, and retry tests; STALE-RECORD POLICY: untrusted repair state is NEVER auto-cleared; the banner shows record age with retry/verified-clear actions and quarantine is retained until verified (a TTL, if any, never silently re-enables restore — it switches to a documented read-only/recovery state); beforeunload/reopen, task-remount, stale-age, and cross-tab tests**; **SUPPRESSION SCOPE: while repair is active, existing live grid panels REMAIN USABLE and only floating restore/materialization/mutations are blocked; **PORTAL TRANSFER ON REPAIR ACTIVATION: adoption of new portals stops, floating ownership is released, and each recoverable portal is REATTACHED **by LOGICAL group/column identity** — **a MINIMAL `create-materializable-column` REPAIR-CONTROL transform is EXPLICITLY ALLOWED under the repair token when the floating group's original slot was removed/sanitized away (no portal adoption, no persistence, rollback guarantees; a call-order test proves it cannot invoke normal float/dock/materializer code)**; retained-detached-with-non-rendering-lease is the documented alternative for env-scoped panels; a repair-activation-after-adoption test covers one floating-only portal and one existing-grid portal** — repair controls are reachable via the app shell on task/home/settings mounts (banner visibility + clear/retry without entering the unsafe restore path is an E2E scenario)**; **an explicit allow/deny matrix covers native grid reads (allow), ordinary grid edits (allow), persistence (allow for the current validated grid — CLEAR RETAINS THE LAYOUT KEY and removes only repair/quarantine/floating state; the current validated native grid is synchronously serialized + persisted BEFORE any deletion, with a replacement pair journaled and verified before the old repair record is removed; reload-during-clear and clear-before-debounce tests), floating actions (deny), restore/recovery (deny), portal adoption (deny), env switch (allow, generation-checked), and repair controls (allow)**; `cleanupTaskStorage` removes repair + quarantine keys; while repair is active the native grid is UNTOUCHED and **ALL floating windows, edge bars, and floating/layout mutations are SUPPRESSED** (no read-only salvage rendering — the salvage-render alternative is rejected because adopting portals or presenting unverified state as authoritative violates fail-closed); the only UI is a localized, non-dismissable repair banner (owner: the dockview store/coordinator) with export and an AlertDialog-style explicit clear confirmation; repeated automatic recovery is suppressed while the banner is active; reload, task-switch, settings-mount, confirmation-failure, and cleanup tests |
   The `phase` marker (`mutating` only) never
   replaces digest evidence; both-after rows are settled by digest equality
   alone (there is no `committed` refinement — the verified pair is the
   terminal evidence). **If fromJSON/apply WAS invoked before the crash,
   equality alone is NOT sufficient: recovery must run the native
   snapshot rebuild/rebind path (above) BEFORE clearing the journal; if
   that rebuild fails, quarantine + repair (the journal is retained);
   both-before and after-invocation-equal tests cover the distinction.** The journal is cleared only after the selected
   target is verified. Both write paths return/throw status (the current
   `setEnvLayout`/`persistEnvLayoutNow` swallow failures — status-returning
   APIs are part of the contract). **`recoverFloatingJournalOnce(api,
   envId)` is the single pre-restore gate**: it reads the persisted journal
   `{envId, transactionId, phase, digests}` and **validates it before
   trusting it** — `isEnvFloatingJournal(journal, envId)` checks version,
   env match, transaction id shape, phase enum, tagged-digest shape, raw
   snapshot bounds (per-env cap), and exact raw JSON-shape, then
   **recomputes SHA-256 from each raw snapshot and requires it to equal the
   journal's tagged digest** before any target is selected. **The journal
   decision is EXCLUSIVE:** a journal that fails validation or digest
   recomputation is **present-but-invalid** ⇒ quarantine (below) + durable
   repair record + FULL SUPPRESSION — journal-free divergence rules NEVER
   apply to a present invalid/mismatched/unexpected journal (they apply ONLY
   when no journal exists); a restore of an invalid journal cannot call the
   materializer or adopt portals (tests prove it). **Journal reads return
   a typed result `absent | present(raw) | unavailable(error)`: a
   read error (quota/private-mode throw — the current
   `getSessionStorage`/`getEnvLayout` helpers swallow read failures and
   return fallbacks, local-storage.ts:7-17/301-310) is NEVER treated as
   absent — `unavailable` FAILS CLOSED with full suppression and a typed
   retryable result, and journal-free divergence recovery runs ONLY on a
   verified `absent`**; Quarantine uses a
   **verified, idempotent protocol** — sessionStorage has
   no rename primitive, so quarantine derives **one deterministic key from
   `(envId, original raw digest)`** (`...-journal.<envId>.corrupt-<digest>`):
   copy the raw bytes to that key, read-back verify the copy, then remove the
   original and verify its absence. If the deterministic quarantine key
   already exists on a retry, it is read and verified and the flow proceeds
   directly to verified original removal — **a second copy is never allocated
   for the same original** (a crash between copy and remove cannot accumulate
   `.corrupt-<n>` copies on repeated restarts). A failed copy or removal
   keeps the original (never cached as recovered) and is retried on the next
   recovery; the repair-safety allowance is PROPORTIONAL and reserved BEFORE normal
  writes: `2 × exact raw journal bytes + repair record + clear journal +
  fixed overhead` per env (a quarantine copy of a near-cap journal must
  fit under quota pressure — sessionStorage has no rename primitive, so
  the copy needs its own bytes), OR a verified atomic replacement protocol
  that avoids the second full copy (the former is the chosen default);
  quota-full behavior retains the original
  journal and never caches recovery; max-size corrupt journal under
  quota-full tests; quarantine keys count toward the budget **— the COMPLETE owned-key set
   (floating, journal, quarantine, repair, repair-clear, layout-v4,
   maximize-v4) is byte-accounted in BOTH the per-env cap and the global
   384 KB index, and the recovery allowance is reserved ATOMICALLY across
   environments before any mutation (a float preflight can never pass and
   then leave no global capacity for mandatory recovery)** — and are removed by bounded
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
   divergence rules in step 6 are the journal-free fallback ONLY for an
   ABSENT journal (never for a present invalid/mismatched/unexpected one),
   not the primary guarantee.
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
   AFTER pair has NOT completed synchronous verification (digest-ready is
   NOT sufficient) ⇒ write the cached
   BEFORE pair + aborted/retained journal; ONLY a journal whose AFTER pair has
   completed synchronous verification ⇒ write AFTER + `mutating` journal,
   then read-back verify the pair and CLEAR the journal. There is NO
   `committed` value anywhere (the verified pair is the sole terminal
   evidence); a
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
   mid-pair is rejected with `{status:"rejected", reason:"busy"}`, never interleaved); the current
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
2. Look up the saved root column **by `columnLogicalId`/normalized
   registry FIRST**; the native `columnId` is only a validated hint — if
   the logical column exists, use it; if a NATIVE key exists without the
   logical id (regenerated key), resolve by logical identity, never insert
   a duplicate; only when neither resolves, insert at `columnIndex`
   (clamped) with its saved metadata, and FAIL CLOSED on ambiguous
   matches (two live columns claiming the same logical id).
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
   **Empty-stripped-column materialization is a pure
   `create-materializable-column` transform**: create the root column with
   its saved geometry, create ONE valid leaf/group (never an empty branch
   or a zero-children leaf — the serializer emits invalid empty shapes),
   insert the saved tabs + active panel into BOTH tree and flat forms, then
   apply once; empty-strip → reload → dock and last-side-column
   legal-tree tests cover it.

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
  composite key), holding one record `{ token }` per entry. **Event
  correlation WITHOUT token-carrying events:** `onDidRemovePanel` receives
  only the panel object — it NEVER carries a token — so the registry matches
  by **`(api instance, panelId, operationUUID)`**; operation UUIDs are unique
  per coordinator operation (never a per-env resetting counter). **Tombstone
  retention is ONE contract:** coordinator-owned tombstones with a **bounded
  TTL (30 s)**, using a **monotonic injected clock with an inclusive boundary
  (`now >= expiresAt` is expired)**, one timer per tombstone (or a documented
  min-heap), recorded with the operation UUID at arm time and dropped by a
  coordinator-owned timer on expiry (an idle app never accumulates them
  unbounded; timers are cleaned on unmount); a delayed `fromJSON` removal
  arriving within the TTL is
  correlated via the tombstone, after expiry it is treated as a real user
  close (documented — the TTL bounds the correlation window);
  registrations are armed immediately around each synchronous
  remove/`fromJSON`. A real user close is never registered and never consumes
  a detach entry. Idle/no-successor, event-before-successor,
  delayed-old-event-after-successor, duplicate-delivery, and **fake-timer
  expiry tests at TTL-1ms, TTL, and TTL+1ms plus timer cleanup on unmount**
  cover the contract deterministically (no wall-clock sleeps). **Portal env
  ownership on reacquire:** `acquire` on an existing entry updates
  api/params/component AND re-validates/updates `entry.envId` (a same panel
  id reacquired under a new env carries the new env; old-env blob entries
  are bookkeeping only — the live portal has exactly one env at a time).
  **Registrations are armed per expected removal, not for the transaction
  lifetime:** immediately before each synchronous `removePanel`/`fromJSON`
  that the transaction performs, the exact `(panelId, envId)` pairs being
  removed are registered; each registration is consumed once by the matching
  `onDidRemovePanel` **— the Dockview callback carries ONLY the panel object
  and NEVER a token; correlation is exclusively via the armed registry
  `(api instance, panelId, operationUUID)` records plus the bounded TTL
  tombstones (see Event correlation)** — and the set is drained when the
  operation completes. A
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
- **Focusout deferral while an owned layer is open:** `focusout` is
  routed through the SAME pending/refcount/generation lease as pointerdown
  (below): a focusout whose relatedTarget is in no owned region is DEFERRED
  while any owned layer of the window is open — Radix menus/dialogs move
  focus before dismissing, so the collapse must wait for the layer's
  `onOpenChange(false)` + rAF, never firing mid-layer; a real
  focusout-with-open-layer test is specified.
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
  `host.ui.registerFloatingOwnedLayer(capability, openHandlers): () => void` is
  added **together** to `apps/packages/plugin-sdk/src/index.ts` (the
  `PluginUIApi` type, as a callable outside the mapped component type), the
  host implementation `apps/web/lib/plugins/host-api.ts`, and the host
  contract docs (`docs/plans/plugins/PLUGIN-API.md` + `apps/web/lib/plugins/types.ts`).
  **Ownership proof = per-open handshake, not a root node:** Radix layers
  portal their content to `document.body`, so a WeakMap from capability to
  portal instance can never prove an arbitrary body node was opened by the
  panel. Instead the plugin passes a **per-open handshake object** — the
  host returns `{ onOpenChange(open: boolean), onDismiss() }` bound to the
  capability + portal generation; the plugin spreads `onOpenChange` onto
  the Radix root, so the ONLY way the layer counts as open is a real
  open-state transition on that panel's Radix instance. A body node
  without that binding is not an owned layer; unrelated nodes are
  rejected. Registration = open=true transition; unregistration =
  open=false / onDismiss / cleanup. **SINGLE-ACTIVE-LAYER ADMISSION:
  each capability/handshake pair admits at most ONE active open — the
  host keeps an active-open map keyed by (capability, handshake token);
  a second open=true for the same pair is REJECTED with a typed result
  (the plugin must close the first before opening another, or request a
  separately leased instance); duplicate open=false / onDismiss calls are
  idempotent (no refcount underflow, no unregistering a different layer);
  tests: one handshake spread on TWO roots (second open rejected),
  two handshakes on one capability (separate leases allowed), duplicate
  open=true, and close-ordering.** **REJECTION IS TRANSPORTED BACK VIA A VOID-COMPATIBLE ADAPTER:
  Radix consumes `(open: boolean) => void` and discards any return (the
  @kandev/ui wrappers do exactly this, dialog.tsx:37-45), so the host's
  returned handler is a `(open: boolean) => void` that on rejection
  SYNCHRONOUSLY invokes an explicit `requestClose(reason)` callback /
  controlled-state setter owned by the plugin. **HOST CLOSE IS
  AUTHORITATIVE AND OBSERVABLE: the host does NOT decrement its refcount
  or remove the active-open map entry on rejection — it retains the
  active lease until the plugin emits an ACTUAL `open=false`
  acknowledgement through the same adapter (no optimistic bookkeeping,
  because a non-controlled plugin can ignore `requestClose`); a
  noncompliant plugin (no ack within the lease timeout) keeps the
  window expanded, keeps the lease, and surfaces a TYPED PLUGIN CONTRACT
  FAILURE (locale key) instead of a phantom unregister; the window
  collapses only after a real ack or the failure path's explicit
  teardown.** **ACK TIMEOUT IS BOUNDED AND COORDINATOR-OWNED: 3 s
  deadline stored as `{capability, handshake, generation, deadline}`;
  ack vs timeout are serialized with compare-and-clear (ack-before-
  timeout wins; timeout-before-ack emits the typed
  `{status:"terminal", reason:"plugin-contract-failure"}` result, REVOKES
  the capability/handshake, unregisters the layer, and ends the retained
  lease — after revocation the window MAY collapse normally); **timeout
  revocation is followed by a coordinator-owned ONE-SHOT REISSUE: the
  host mints the replacement and DELIVERS it via a subscription callback
  the plugin receives at handshake time (`onCapabilityChange(newCap)`);
  additionally, before EVERY open the plugin MUST re-read via
  `requestCapability()` — a stale cached `PluginTaskPanelProps` capability
  is never trusted and using a revoked capability is a TYPED
  `{status:"rejected", reason:"stale-capability"}` failure (matrix row
  41) — SYNCHRONOUS, no side effects: NO second revocation, NO second ack
  timer (the revocation already happened at timeout); `requestCapability`
  itself has NO timer (it is a synchronous store read of the current
  generation) BUT it returns `{status:"pending-reissue"}` while a reissue
  is in flight (the revoked generation is NEVER handed out as fresh); the
  plugin MUST await `onCapabilityChange` before retrying (callback-
  before-request and request-before-callback are both defined and
  tested — a request during reissue returns pending-reissue, never the
  revoked value); **pending-reissue is BOUNDED: at most 2 consecutive
  pending-reissue observations per handshake, then a coordinator-owned
  TERMINAL outcome (reissue deadline exceeded / `onCapabilityChange`
  never delivered) that closes and cleans the handshake with the typed
  plugin-contract-failure result and capability revocation — no
  infinite pending loop; the reissue attempt has its own 3 s deadline**;
  **LATE RESURRECTION IS IMPOSSIBLE: each reissue carries a UNIQUE
  token/generation; the terminal timeout ATOMICALLY claims and cancels
  the token + timer, revokes the handshake, and records terminal state;
  `onCapabilityChange` COMPARE-AND-CLEARS the token and is a generation
  no-op after terminal — a fresh capability can NEVER be delivered after
  revocation; tests: callback-after-terminal, callback-vs-timeout at the
  exact boundary**; **DUPLICATE DELIVERY IS IDEMPOTENT: the delivered
  token is retained as a TOMBSTONE until handshake teardown — a second
  delivery of the SAME token is an idempotent no-op returning the same
  success (StrictMode/replayed subscriptions); a different/old token is
  a generation no-op; NEITHER starts a second timer or revocation;
  duplicate-callback-before/after-plugin-retry tests**;
  the plugin re-reads with the fresh generation and retries;
  a stale attempt is idempotent (repeated stale use returns the same
  typed rejection without further state change); teardown/unmount clears
  the handshake; retry ordering: stale → typed failure → fresh
  capability → successful next open; the next
  open transition on that panel receives the FRESH capability, so a
  timeout never poisons a still-mounted panel (the revoked capability
  dies; the panel is not permanently disabled; no remount required) —
  this is the single exception to capability stability, and it is
tested (timeout → reissue → successful next open, incl. a plugin that
retains old props)**; late ack
  after revocation is a generation no-op; tests: ack-before-timeout,
  exact-boundary, timeout-before-ack, unmount/unregister, late ack.** Ordering: open=true transition → admission check → owned
  or (rejected + requestClose + lease retained pending ack); tests:
  rejected second-root closes, host refcount consistent (incl. an
  INTENTIONALLY UNCONTROLLED plugin), Radix dismissal state, retry after
  close.**
  The host stores a WeakMap/token binding from capability to portal instance;
  a plugin rendering two task panels cannot register a layer from one panel
  against the other, and a hoarded plugin-scoped function cannot be reused
  across renders or after unmount. **Capability stability:** the capability is
  issued from a **portal-instance generation** and kept stable for the
  instance's lifetime via `useRef` (a benign re-render never issues a new
  token, so an open layer survives); it is revoked only on actual portal
  release or plugin unregistration, and **rotated on reacquire** (a released
  and reacquired portal gets a fresh token, so a hoarded old token is
  rejected); **the one exception is the ack-timeout one-shot reissue above**. **Mobile:** the same `PluginTaskPanel` renders on phone, but
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
  non-destructively — the group stays pinned and the typed
  `{status:"rejected", reason:"quota-full"}` result with localized
  `task:floatingError.quotaFull` (camelCase — the canonical generated spelling; no hyphenated aliases) is surfaced.
  No clamping or reuse (reuse would break monotonicity and stable z-order).
- Rendering order is stable: sort by `(order, groupLogicalId)` — native
  groupId regenerates across fromJSON/reload (a non-authoritative hint) and
  is used for diagnostics only; floating E2E/test ids embed `groupLogicalId`.
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
  atomically with replacement. **Global group ordering:** when several
  floating groups contain stale session panels, the winner group is selected
  first by a **deterministic total order** (`order`, then `groupLogicalId`),
  then the active-panel/first-tab rule applies within that group; two-group
  tests cover active stale panels in both with reversed insertion order.
  **Delayed replacement has a readiness barrier with a bounded timeout:** a
  **coordinator-owned** per-env barrier (an `AbortController`-based timer
  owned by the coordinator, duration 10 s, cancelled on success/failure/
  timeout/env-switch/unmount — never leaked) exposes
  `isSessionReplacementPending(envId)` which is included in EVERY mutation
  gate (float/dock/reset/preset/toggle/add-panel) and disables the pin
  controls; the selected fail-closed timeout policy is: **keep the existing
  floating winner and suppress only that ensure** (documented; the
  alternative is rejected). The hook DEFERS every mutation (the pending-
  session rebuild and reconciliation steps included) while replacement is
  pending and the winner is checked/consumed BEFORE any hook mutation, not
  only before ensure; if the hook ever ran first and inserted the incoming
  id, the coordinator atomically re-evaluates and MOVES the already-added
  winner back to the floating entry (no id ever exists in both surfaces OUTSIDE the named dock/float handoff window — during the bounded handoff generation a DOM dual-mount is legal with exactly ONE authoritative owner at every instant);
  tested with replacement-after-first-effect, StrictMode rerun, delayed WS
  ordering, and replacement-never-completing timeout. The field is **memory-only (never
  persisted)** and consumed by an atomic **compare-and-clear**
  `consumeFloatingSessionWinner(sessionId, envId, generation)` called from
  `shouldSkipPanelEnsure` (`dockview-session-tabs.ts`) before the hook's
  ensure effect runs — consumption is one-shot, so repeated/StrictMode effects
  cannot double-skip. **ALL-FLOATING-SESSION-IDS RULE: `shouldSkipPanelEnsure`
  skips EVERY session id currently owned by a floating group (an atomic
  `floatingSessionIds` ownership query), not only `floatingSessionWinner` —
  **OWNERSHIP QUERY CONTRACT: the query returns `{floatingIds,
  reservedIds, gridOwnedIds, envId,
  generation, transactionId, phase}` from ONE coordinator snapshot —
  ids are
  RAW SessionIds (normalizing `session:<id>` panel ids to raw ids,
  matching the hook's `effectiveSessionId` input); the domain is
  FLOATING ENTRIES ONLY
  (grid-visible session tabs are NOT in the query — a session may
  legitimately exist in multiple grid groups); `shouldSkipPanelEnsure`
  VALIDATES the snapshot immediately before ensure/insertion and, on
  mismatch (query vs current transaction), retries/deferr under the
  readiness barrier; DURING THE NAMED HANDOFF
  WINDOW (dock/float transfer) floating ownership WINS (the skip applies)
  and the grid insertion is suppressed by the second generation check;
  **PHASE TABLE for A→B winner replacement: `floatingIds` (floating
  entries only), `reservedIds` (being-replaced — both A and B reserved
  in the transaction mapping during the handoff, SEPARATE from the
  floating-only query), `gridOwnedIds`; A is SKIPPED while floating;
  B is NOT skipped once its target is the grid (B is not in the
  floating-only query — the intended grid insertion proceeds); the
  reservation set never leaks into the floating-only snapshot; the
  second generation check prevents a stale insertion during the actual
  handoff; query-before-removal, during-handoff, post-commit, and
  B-ensure tests assert EXACT ID SETS;**
  the same normalized query serves materialization fail-closed** —
  a session that exists ONLY in a floating group, or a non-winner floating
  session, can never be re-added to the grid by the auto-session hook;
  materialization and ensure both FAIL CLOSED on an existing live id
  (one live instance per panelId); tests: current floating session,
  stale winner, non-winner stale entries, StrictMode reruns,
  hook-before/after replacement.** Stale winners (generation or env mismatch) are cleared
  on generation/env transition and on every terminal path (ensure failure,
  unmount, env switch); a newer generation is never cleared by an older
  cleanup.
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
  rolls back and the group stays pinned, with the typed
  `{status:"rejected", reason:"quota-full"}` + localized result surfaced.
  There is no
  separate "ephemeral floating" mode: a float that cannot be persisted does
  not happen, so a reload can never lose a floated group the user believes
  exists. (This is the single policy; see the State machine, steps 4-7.)
- **Layout rebuild / preset switch / env switch while floating:** the floating
  blob is re-applied after the grid restore completes (Restore call sites). A
  floated panel whose definition cannot be materialized is classified:
  **stale/deleted-session pruning is an EXPLICIT expected result
  (`{status:"pruned", reason:"stale-session"}` — silent by contract, no
  error surface); invalid-definition and salvage drops return
  `{status:"recovered-with-drops", dropped: [{id, reason}]}` with the
  dropped ids+reasons logged and surfaced in the repair UI (never a bare
  silent drop); every OTHER materialization failure (storage error, lease
  failure) is a TYPED
  warning/retry/repair result surfaced with localized UI, and the
  surviving state is persisted only AFTER the result is recorded**.
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
  has no right column — the bit is then FALSE by definition (a floated/
  absent pinned-right column yields rightPanelsVisible=false; a stale
  sidecar intent can never make it true — asserted) — and no stale bit
  drives enforcement; the current
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
  consumes **only** the VALIDATED REGISTRY/LIVE-LAYOUT role map
  (from an explicit `LayoutColumn.role` assigned by presets/
  custom-layout normalization, with a documented migration default for old
  layouts — **never inferred from width, canonical group ids, panel
  membership, OR the sidecar cache**): the predicate returns true iff a
  live root column's registry role is `"pinned-right"` AND the column is
  live with matching pinned state; the sidecar `rootColumns` role is
  DIAGNOSTIC/WEAKENING-ONLY — a stale sidecar can never positively
  qualify a column (registry says side-other/custom ⇒ FALSE regardless of
  the sidecar), and sidecar-only input is REJECTED at the type boundary;
  stale-sidecar/live-role conflict vectors are tested.
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
  static AST/source-boundary test rejects bypasses **scoped to
  task-workbench production sources only** (approved adapter modules are
  enumerated; office-dockview, settings layout editor, and other non-task
  `fromJSON`/`applyLayout` uses are explicit fixtures/exclusions; a new
  task restore bypass fails with the exact callsite and required adapter).
  The wrapper invokes
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
  typed `{status:"rejected", reason:"quota-full"}` + localized result. A
  reload or env switch can therefore never observe a
  missing floated group that the user believes exists; the worst case is the
  pin simply not sticking.
- **Journal/floating state size budgets:** floating state plus the operation
  journal share the tab's sessionStorage quota with all task environments and
  other Kandev storage. Two caps are preflighted before any mutation (State
  machine, step 1): a **per-env cap** (96 KB: blob + journal before/after
  snapshots) and a **global floating allocation budget** across all
  environments' blobs + journals (384 KB), enforced by scanning the owned
  storage prefix. Exceeding either fails the transaction non-destructively
  with a typed rejected result — the float does not happen, nothing is corrupted,
  and the user's existing pinned/docked state is untouched. Near-limit,
  multi-env-combined, and quota-full behavior is tested.
- **Reset layout / "clear UI state":** reset is an **id-aware docking merge**
  with explicit collision precedence: the reset layout owns group/column
  **placement**, the floating definition owns the panel **payload** (component,
  params, tabComponent) and saved tab order, and the active panel is merged
  explicitly — **scoped to valid definitions**, and **executed through the
  same single tree+flat mutation helper as the materializer**, specified as
  a **pure `mapLayoutGroups(state: LayoutState, transform): LayoutState`
  function** (no live api): traverses each group once by stable identity,
  reuses the transformed object in the corresponding tree leaf and flat
  array, removes empty branches/leaves, and asserts tree/flat/panel id
  equality; the materializer and reset both call it and then apply the
  resulting state via the existing applier (the existing merger maps only
  `col.groups` and leaves `col.tree` stale while the serializer prefers the
  tree; nested-tree reset merge tests are required, not only materializer
  tests). `session:*` floating
  definitions are validated against the active task/session set and env
  before merging: stale/deleted/absent-session definitions are dropped, and
  when no valid session remains the reset chat placeholder/default behavior is
  retained (payload-wins is never a blanket override for sessions). **An EXACT
  placeholder allow-list keyed by panel id → (component, allowed params,
  allowed title) governs EVERY persisted definition: `session:` panels
  match the OPAQUE-ID contract: SessionId is a branded opaque string
  (ids.ts:23-37) — validation = membership in the active task/session set
  plus bounded length (≤ 200 chars) and JSON-safe encoding; NO UUID grammar
  is imposed (live fixtures/persisted sessions use `session-1`, `sess-*`,
  `s1`; the backend generates UUIDs only when the caller leaves the id
  empty, session.go:630-633 — a UUID regex would DROP valid sessions);
  **VALIDATION IS SPLIT BY PHASE: at LOAD only STRUCTURAL validation
  (session:<id> shape, bounded length, JSON-safe) — membership is NOT
  checked at load because active sessions may still be resolving (the
  load-time sanitizer dropping unresolved-but-valid sessions would lose
  chat tabs); at RESOLVE (before materialization/reset, against a
  task/session snapshot) MEMBERSHIP is validated — unresolved-session
  definitions are RETAINED and re-validated on the next resolve tick, and
  only a resolve-time non-member is dropped (stale-session pruning);
  **TOCTOU PROTOCOL: the snapshot is acquired ONCE per coordinator
  transaction with an immutable version/generation; resolve-to-
  materialize is bound to that single transaction (busy-guarded, so a
  session deletion or snapshot replacement cannot interleave); membership
  is REVALIDATED immediately before payload creation/materialization
  against the SAME snapshot version; a changed snapshot (task/session
  deleted mid-transaction) aborts that entry with the stale-session
  pruning result, never materializes it; a session deleted after resolve
  but before materialization is caught by the revalidation***; the
  sanitizer's
  `id.startsWith("session:")` + component-Set check at
  dockview-layout-restore.ts:79-97 is REPLACED by the closed table:
  canonical component, title, tabComponent, and a DEEP STRUCTURAL params
  schema (exact keys + value types + size bounds, no extra keys);
  malformed ids, known ids with wrong component/params, and unknown ids
  FAIL CLOSED (definition rejected, blob treated as suspect, never
  payload-wins) before reset/materialization; the CLOSED TABLE is committed here (ids and components from the live
  registry `dockview-desktop-layout.tsx` components map /
  `dockview-panel-content.tsx` PANEL_RENDERERS): chat → ChatContent
  (params `{sessionId}` for session:*), diff-viewer → DiffViewerContent,
  file-editor → FileEditorPanel, commit-detail → CommitDetailPanel,
  changes → ChangesContent, files → FilesContent, terminal →
  TerminalPanel, browser → BrowserPanel, vscode → VscodePanel, plan →
  PlanContent, todos → TodosContent, pr-detail / review-detail →
  ReviewDetailPanelComponent, mr-detail → MRDetailPanelComponent,
  plugin-panel → PluginPanel (params = plugin manifest contract), plus
  the two alias ids diff-files→changes and all-files→files; params are
  schema-bounded per entry (session:* ⇒ `{sessionId: string}` — the
  OPAQUE session id, bounded length; never a UUID-only rule),
  with tests
  covering corrupt and user-mutated blobs.** Valid
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
  the floating key **and the journal key, governed by ONE authoritative
  cleanup matrix (committed machine-readable owned-key matrix + validator
  with EXACT constructed key matches — base keys = prefix + `encodeURIComponent(envId)` — the ONE canonical
  encoding used by every writer/reader/validator/cleanup helper (the
  current raw-interpolation writers are migrated), with reserved-character
  and adversarial-prefix-key tests; **E2E helpers MUST call a shared
  test-safe key-constructor module (or a single exported contract
  constant) instead of copying the prefix — the matrix validator and
  runtime writers share the same constructor**;
  quarantine keys = exact base journal key + `.corrupt-<lowercase-hex-
  digest>`; NEVER delete arbitrary `startsWith('kandev.dockview.env-')`
  keys; an adversarial user key with an owned prefix survives cleanup)
  covering ALL owned keys: env-floating, env-journal, env-repair,
  env-repair-clear, deterministic `.corrupt-<digest>` quarantine keys,
  env-layout-v4, and env-maximize-v4 — generation is
  invalidated before deleting EVERY key, and deletion tests cover an
  incomplete repair journal, a quarantine copy, and a clear in flight (a
  late clear can never resurrect a deleted task's state)** (`kandev.dockview.env-floating-journal.<envId>`)
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
  pinned, and a typed rejected result is surfaced — no panel is lost on the next
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
  re-recovered on every restore; journal-free divergence rules NEVER apply
  to a present invalid journal (they apply only when no journal exists); the
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
  typed `{status:"rejected", reason:"busy"}` result with a localized
  busy reason (all public layout-mutation boundaries are busy-
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
  typed rejected result is surfaced.
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
