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
serialized size bounded at 64 KB per definition), invalid entries are dropped
individually, an unreadable blob defaults to `{}` (all groups pinned).

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
classify(columns, centerColumnId, columnId, columnIndex, columnPinned):
  center column (the column containing centerColumnId)
      -> kind "center", orientation "horizontal", edge "bottom"
  any other root column
      -> kind "side", orientation "vertical",
         edge "left" if columnIndex < center column index else "right"
  unresolved column id / index
      -> kind "custom", orientation "vertical", edge "right" (documented fallback)
```

- Plan/preview/vscode/compact presets put their extra groups in root columns
  **to the right of** the center column, so they classify as `side`/vertical/
  right — matching the user-facing requirement. The classifier must NOT use
  `isCenterCandidateGroupId` (it returns true for generated plan/preview ids
  and would misclassify them as center).
- The center column identity comes from the live store (`centerGroupId`,
  resolved by `findCenterGroupId`); column index and pinned/width metadata come
  from the `LayoutState` columns array.
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
   Storage-write failure is non-fatal: the in-memory state stays authoritative
   for the session and degrades to ephemeral (documented, no crash).

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
3. Insert a group at the saved `treePath` (creating branch nodes as needed) or
   as a deterministic leaf in that column, containing the floating panels in
   saved order; set `activePanel`.
4. Apply the delta through the existing serializer/`applyLayoutAndSet`
   machinery (the same path presets, maximize, and `toggleRightPanels` use).

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

- `floatingDetachRegistry: Map<envId, Map<panelId, transactionToken>>` in the
  store. A float transaction registers each panel id it intends to remove,
  tagged with its transaction token.
- `setupPortalCleanup`'s `onDidRemovePanel` checks the registry before running
  side effects: for a registered id with the **current** token, it skips
  `release`, terminal park/stop, vscode stop, and
  `handleMaximizeExitOnLastClose`, then **consumes** the registration (removes
  that panel id from the registry). Ordinary unregistered removals — including
  a user closing a floating tab mid-transaction — run the full cleanup path.
  The existing `isRestoringLayout` early return keeps its current precedence
  (all removals during restore skip cleanup).
- `panelPortalManager.reconcile` and `releaseByEnv` accept an explicit
  exclusion set (the current env's registered ids) instead of consulting a
  global set, so an env-scoped panel id that also exists in another env is
  still released when its own env switches away.
- A stale transaction (token mismatch) never clears a newer transaction's
  registrations. The registry is cleared for the env on transaction settle
  (success, failure, or unmount).
- If a registered panel id is closed by the user before the float removal runs
  (its registration was consumed by the close), the float transaction detects
  the panel is gone at removal time and drops it from the floating
  definitions instead of removing it again.

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
- True outside interaction still collapses: clicking the grid, the app
  sidebar, or the page chrome while a layer is open collapses the window as
  soon as the layer closes (the layer remains part of the region while open).

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
  definition title. Titles are resolved at render time, so a plugin
  re-registration or session replacement updates the bar on the next render;
  the persisted `FloatingPanelDef.title` is only the reload fallback.

## Stacking

- `order` is allocated from the per-env monotonic `nextOrder` counter; after a
  group is removed, its order is not reused.
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

`replaceStaleSessionPanels` returns an old→new panel identity mapping, applied
to floating entries (panelIds, definitions, portal params/title, and
`activePanelId`). When several stale `session:*` panels float, the **winner**
is deterministic: the group's active stale panel if it is stale, else the
first stale floating panel in the group's saved tab order. Only the winner is
mapped to the incoming id; every other stale floating panel is dropped, and
`activePanelId` points to the winner's new id (or the first surviving panel if
the winner itself was dropped). Duplicate mappings never occur because each
floating entry maps at most one old id.

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
  portal, or unrecoverable blob survives.
- **Reset layout / "clear UI state":** reset is a docking reset: every
  floating group is materialized into the reset target grid (side groups →
  the reset layout's right column, center groups → the center column,
  fallback center), floating state is cleared (memory + storage) only after
  the grid contains them, then persisted. The floating preference is cleared —
  groups do not re-float after reset. `cleanupTaskStorage` removes the
  floating key alongside the layout/maximize keys.
- **Storage write failure after float/dock:** the in-memory state is
  authoritative for the session; the blob may be stale until the next
  successful write; behavior degrades to ephemeral (documented, no crash).

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
  reset completes, **THEN** all groups are back in the grid (materialized into
  the reset layout), no floating storage remains for the environment, and the
  groups do not re-float.
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
