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
  (the mobile task surface is a separate composition without dockview), so the
  pin control appears only where the workbench renders — consistent with the
  message queue pin.

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
  columnPinned     boolean       # column's pinned/width metadata at float time
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
nonnegative, and within practical maxima (order/nextOrder ≤ 10_000, size ≤
100_000, columnIndex ≤ 64), and duplicate panel ids or group ids within the
blob are rejected deterministically (first occurrence wins, later ones
dropped). Invalid entries are dropped individually; an unreadable blob
defaults to `{}` (all groups pinned).

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

1. **Capture + validate:** capture placement, panel definitions, and size;
   serialize the current `LayoutState` into the journal. If the target group's
   definitions cannot be captured or serialized, abort without mutating the
   grid.
2. **Mutate:** perform the grid mutation (float: non-destructive removals;
   dock/restore: materialize via the `LayoutState` materializer).
3. **Commit:** only after every removal/materialization succeeded, commit the
   store entry, persist the floating blob, then `persistEnvLayoutNow`.
4. **Failure recovery:** if any step throws mid-mutation, re-apply the
   journaled `LayoutState`, drop the partial entry, and keep the previous
   floating blob. Suppression cleanup runs in `finally` and is token-guarded
   (a stale transaction's cleanup never clears a newer transaction's state).
   **Storage commits fail closed:** if the floating blob cannot be written, the
   transaction rolls back (journal re-apply) and the group stays pinned, with
   a console warning — the worst case is "the pin did not stick", never a lost
   panel. The env layout is written only after a successful commit.
5. **Persistence suppression:** a store-owned transaction token (`floatingTransactionToken` with token-guarded begin/end actions; the same field is read by `setupLayoutPersistence`, so there is no module-local flag and no import cycle) gates **every** layout-persistence entry point through one `canPersistLayout()` guard: `persistNow`, the debounce callback, the `onDidLayoutChange` handler, and the `beforeunload` flush (`dockview-layout-setup.ts:346-403`). On transaction begin the guard also cancels/holds an already-scheduled debounce timer and marks it dirty, so a timer scheduled before the transaction cannot fire mid-transaction. Persistence runs only after commit or journal recovery (explicit `persistEnvLayoutNow`), and the token is reset on unmount. Ordinary unregistered closes are unaffected (their cleanup does not depend on the guard).

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
3. Maximize restore (`restoreMaximizeFromStorage`).
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
  window or its layer owner unmounts; it applies only at refcount zero, so two
  layers closing in either order never collapse while the second is open, and
  a successor window reusing the same group id never inherits a stale pending
  collapse.
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

- **Winner floats, absent from the grid:** the incoming session id is **not**
  added to the grid (`restoreMissingSessionPanel`/`addCurrentSessionSiblings`
  are suppressed for that id); only the floating entry is updated. Without
  this, the same panel id would exist in both the grid and the floating
  overlay — panel identity is single-valued.
- **Winner is in the grid:** the floating stale copy is dropped entirely.
- Only the winner maps to the incoming id; every other stale floating panel
  is dropped, and `activePanelId` points to the winner's new id (or the first
  surviving panel if the winner itself was dropped). Duplicate mappings never
  occur because each floating entry maps at most one old id.

The coordinator runs before `restoreMissingSessionPanel`,
`addCurrentSessionSiblings`, **and `useAutoSessionTab`'s ensure path**
(`dockview-desktop-layout.tsx` always mounts the hook; it calls
`ensureSessionPanel` when the active session panel is missing from the grid).
The coordinator's winner decision is visible to the hook's ensure/reconcile
guard before its effect runs, so a floating winner skips session-tab
insertion and activation for that id; grid winners are ensured as today. The
coordinator is tested through the real desktop hook, not only direct tests of
`replaceStaleSessionPanels`, for stale-only-floating, stale-in-grid-plus-
floating, and delayed replacement orderings.

## Failure modes

- **sessionStorage unavailable (private mode / quota):** floating state
  degrades to per-session ephemeral; the UI stays functional and groups stay
  floating until the next layout rebuild; writes degrade silently. Defaults
  back to pinned on reload.
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
- **Partial transaction failure:** the journal recovery in the State machine
  re-applies the pre-transaction layout; no half-floated grid, orphaned
  portal, or unrecoverable blob survives. The transaction-scoped persistence
  suppression (State machine, step 5) guarantees the partial layout is never
  persisted before commit or recovery.
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
  replacing the floating copy. The show path is floating-aware: floating
  groups stay absent (their ids are excluded from the re-added column) or the
  toggle is treated as an explicit dock for them; `rightPanelsVisible` is
  tracked independently of whether a floating right group exists. Tested with
  one and both right groups floated, hide→show while floating, and container
  resize.
- **Storage-write failure after float/dock:** the commit fails closed — the
  transaction rolls back (journal re-apply) and the group stays pinned with a
  console warning. A reload or env switch can therefore never observe a
  missing floated group that the user believes exists; the worst case is the
  pin simply not sticking.
- **Reset layout / "clear UI state":** reset is an **id-aware docking merge**
  with explicit collision precedence: the reset layout owns group/column
  **placement**, the floating definition owns the panel **payload** (component,
  params, tabComponent) and saved tab order, and the active panel is merged
  explicitly. Existing reset panels (chat, files, changes, terminal) are
  reused by id — never duplicated — and the floating definition's payload
  wins, so a floated ordinary terminal keeps its real terminal id rather than
  being retargeted to the default `terminal-default` params. Floating tabs
  merge preserving saved tab order and the active panel; only missing
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
