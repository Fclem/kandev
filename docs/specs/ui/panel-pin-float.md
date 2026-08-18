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

FloatingGroupState
  groupId          string        # original grid group id (informational)
  columnId         string        # root LayoutColumn id the group lived in
  columnIndex      number        # index of that column in the columns array
  columnKind       "center" | "side" | "custom"   # see Placement capture
  columnPinned     boolean       # column's pinned flag at float time
  columnWidth      number | null # root column width (px) at float time
  columnMinWidth   number | null # root column min width, if any
  columnMaxWidth   number | null # root column max width, if any
  treePath         number[] | null   # path into the column's tree (or groups index)
  edge             "left" | "right" | "top" | "bottom"   # collapsed-bar edge
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
   non-destructive no-op with a debug reason while busy (a programmatic add
   mid-transaction would mutate the live grid after the journal snapshot and
   make rollback stale). A single store/coordinator selector
   `isFloatingTransactionBusy(envId)` is consumed by **all three pin
   surfaces** — the grid group header, the floating window header, and the
   collapsed edge bar — which render the pin disabled during the busy window,
   so no pin click can re-enter the phase machine. Re-entrancy tests cover
   mutation, portals-adopted, rollback, and stale-cleanup phases, per surface
   and for programmatic add/reset during each.
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
     snapshots: { beforeFloating, beforeLayout, afterFloating, afterLayout } }
   ```
   Recovery is **digest-based, never schema-validity-based**: for each key,
   the current stored value is compared against the before/after digests, and
   the four partial-write orderings are handled explicitly — blob-written/
   layout-old applies the after pair; layout-written/blob-old applies the
   after pair; both-old applies before; both-after applies after (the
   `phase` marker records whether the mutation itself completed). The journal
   is cleared only after the chosen pair is persisted **and** verified
   against the after digests. Both write paths return/throw status (the
   current `setEnvLayout`/`persistEnvLayoutNow` swallow failures —
   status-returning APIs are part of the contract). **`recoverFloatingJournalOnce(api,
   envId)` is the single pre-restore gate**: it reads the persisted journal
   `{envId, transactionId, phase, digests}`, is idempotent via an in-memory
   recovery cache keyed by `(envId, transactionId, api instance)` — never one
   global generation, so env B's journal is never skipped by env A's recovery
   and a new API instance re-checks a still-present journal — and runs before
   every restore entry (initial mount, env-switch fast/slow, maximize
   restore, preset/custom apply, `toggleRightPanels`, reset/default build).
   The recovery matrix covers float, dock, and restore in **both** crash
   directions, with deterministic tests for all four partial-write orderings
   including a write throwing after storage mutation. The two-key divergence
   rules in step 6 are the journal-free fallback only (journal
   lost/unreadable), not the primary guarantee.
   **Size budgets:** a per-env cap (96 KB default: blob + journal
   snapshots) **and** a **global floating allocation budget** across all
   environments' blobs + journals (default 384 KB), enforced by scanning the
   owned storage prefix before any mutation; the step-1 preflight fails
   non-destructively with an explicit warning when either cap would be
   exceeded (the shared tab sessionStorage quota is never exhausted by
   floating state alone).
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
   by the exact group being restored; otherwise a bounded generated id is
   allocated and a saved→live group mapping is preserved for active-panel and
   tree insertion (dockview group ids are global in the live grid, so a stale
   id left by a conflicting panel can never be reused for the survivor
   group). Salvage and group-id collision tests cover the saved column and
   the live conflicting column. Recovery runs as a
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
   not duplicated): during an active transaction it writes the journaled
   pre-transaction layout and never `api.toJSON()`; otherwise it performs the
   normal live flush. On transaction begin the guard also cancels/holds an
   already-scheduled debounce timer and marks it dirty, so a timer scheduled
   before the transaction cannot fire mid-transaction. Persistence runs only
   at the settled phase (explicit `persistEnvLayoutNow`), and the token is
   reset on unmount. Ordinary unregistered closes are unaffected (their
   cleanup does not depend on the guard).

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

- The registry is keyed by **panel id** with a record `{ envId, token }`
  (panel ids are unique within the live grid; the env tag travels with the
  record so lookups never depend on the mutable current store env). A float
  transaction registers each panel id it intends to remove.
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
- `panelPortalManager.reconcile` and `releaseByEnv` accept an explicit
  exclusion set derived from the **target env's** registered ids, so a panel
  id that also exists in another env's blob is still released when its own
  env switches away.
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
3. Maximize restore (`restoreMaximizeFromStorage`) — **one selected
   sequence (defer-until-exit):** `recoverFloatingJournalOnce` runs first;
   floating session entries are reconciled/mapped (including
   `floatingSessionWinner` written before the auto-session hook in this
   branch); the two-column overlay is **never mutated** — a per-env
   **pending-floating-restore marker** is set instead, and materialization/
   re-float runs only after `exitMaximizedLayout` applies the pre-max layout
   and its rAF settles. While maximized, floating windows render above the
   overlay with their saved display state; the E2E asserts the visibility
   during maximize and the restore after exit. The alternative (pre-max clone
   materialization on the live API) is rejected because the overlay is the
   live grid while maximized.
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
  Dialog/Popover/DropdownMenu/ContextMenu/HoverCard surfaces inside chat,
  plan, terminal, files, changes, diff, plugins, and editors) must call
  `useFloatingOwnedLayer`. The inventory is tracked at
  `docs/plans/panel-pin-float/owned-layer-inventory.md` (created in task-03):
  each row names the exact component/file, the Radix primitive family, and
  the registration mechanism. The **host/plugin API is concrete** — the plugin
  SDK/types/host gain `host.ui.registerFloatingOwnedLayer(layerRoot: HTMLElement)
  : () => void` (idempotent; unregister on close, unmount, and plugin
  unregistration; plugin ownership validated against the registered task
  panel), implemented in `lib/plugins/host-api.ts` with the registry cleanup
  wired into `unregisterPlugin`. An unregistered layer is a collapse bug by
  contract; one real test per primitive family, including a plugin-panel
  layer.
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
  atomically with the replacement. The field is **memory-only (never
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
- **Placement normalization (defined moment):** `fromDockviewApi` derives
  unrecognized root column ids from the first panel id
  (`layout-manager/serializer.ts`), so a center chat group captured after chat
  was replaced by `session:<A>` can hold `columnId: "session:<A>"` — which no
  longer exists after a switch to `session:<B>`, and the materializer would
  insert a new root column instead of returning to center. Normalization runs
  in a **post-apply hook** — after the synchronous layout/session replacement
  AND the incoming-session insertion have completed (fast, slow,
  route-intent, maximize-restore, and reload paths alike) — and resolves the
  root column by **direct live group membership plus index**, never through
  `fromDockviewApi`'s panel-derived ids and never through `findCenterGroupId`'s
  fabricated fallback. When no real center column exists, normalization
  returns `isCenterKnown=false` and the entry keeps the custom fallback
  (vertical/right); it rewrites `columnId`/`columnIndex` to the live center
  only when a real center is known. Tested with an A→B session switch (fast,
  slow, delayed replacement), reload, maximize restore, and dock asserting
  center placement.

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
  exists** (`hasPinnedRightColumn`-style check on the live grid). With all
  right-column groups floated, the live grid has no right column while
  `rightPanelsVisible` may still be true; the current
  `restoreColumnToTarget(sv, sv.length - 1, target)` (`dockview-pinned-enforce.ts`)
  would resize the center column to the right target. `rightPanelsVisible` is
  derived from the live grid/floating state where appropriate.
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
  **One authoritative meaning for `rightPanelsVisible`:** the bit is exactly
  **live canonical pinned-right-column presence**, defined by the single
  predicate `hasLivePinnedRightColumn` — a root column that is (a) pinned
  (carries pinned/width metadata) and (b) identified by canonical right-group
  ids or pinned-column metadata, **never** merely "a column right of center"
  and **never** an unpinned preset side column. Plan/preview/vscode layouts
  have unpinned side columns: the predicate returns false for them (matching
  today's preset assignments), and applying a preset never hides a
  plan/preview/vscode side column as a right-panel toggle. The bit is derived
  from this predicate at all times (restore, float, dock, toggle,
  preset/default assignment), never persisted, never an independent intent;
  show with no pinned-right panels and hide without one are no-ops. The
  toggle, `enforcePinnedTargets`, and `dockview-layout-setup`'s detection all
  share this one predicate. Tested with one and both right groups floated
  (incl. an ordinary terminal id), hide→show while floating,
  float-last-right → hide → show → reload, plan/preview/vscode/compact
  presets (asserting the predicate and bit values), and container resize,
  asserting the serialized tree is legal (no empty branch/leaf).
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
  explicitly — **scoped to valid definitions**. `session:*` floating
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
  the floating key alongside the layout/maximize keys.

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
