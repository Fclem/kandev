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
    block** (left/right column);
  - a **horizontal** bar (titles in a row) when the group was a **horizontal
    panel block** (full-width/center column, or a bottom split).
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
  groups       Record<groupId, FloatingGroupState>

FloatingGroupState
  groupId          string        # original grid group id (informational)
  columnId         string        # root LayoutColumn id the group lived in
  columnKind       "center" | "side" | "custom"   # derived via isCenterCandidateGroupId
  treePath         number[] | null   # path into the column's tree (or groups index)
  edge             "left" | "right" | "top" | "bottom"   # collapsed-bar edge
  orientation      "vertical" | "horizontal"             # derived from columnKind + treePath
  size             number       # px: window width for vertical, height for horizontal
  panels           FloatingPanelDef[]   # full tab definitions, tab order
  activePanelId    string | null
  display          "expanded" | "collapsed"
  order            number       # stack order among floating groups (z-index, offsets)

FloatingPanelDef                   # complete definition so a floated panel can
  id           string             # be recreated after a reload or env switch
  component    string
  title        string             # canonical English title (persisted value)
  tabComponent string | undefined
  params       Record<string, unknown>
```

Persisted under sessionStorage key `kandev.dockview.env-floating.<envId>`,
alongside `kandev.dockview.env-layout-v3.<envId>` and
`kandev.dockview.env-maximize-v3.<envId>`. Reads go through a versioned type
guard (mirroring `isEnvMaximizeState`): invalid entries are dropped
individually, an unreadable blob defaults to `{}` (all groups pinned).

**Layout persistence invariant:** the env layout keeps reflecting the **live
grid** (floated groups absent), exactly as it does today. The floating blob
carries everything needed to materialize floated groups back into the grid and
re-float them. The env layout is never replaced by a pre-float snapshot and
the floating blob never contains a layout.

**Placement capture (at unpin time, from the live `LayoutState`, not from a
group API):** the group's root column id and index in `columns`, its path in
the column's `groups`/`tree`, the column kind (center via
`isCenterCandidateGroupId`, side otherwise), and the group's dimensions from
the layout data. Orientation rule:

| Column kind | Orientation | Edge |
|---|---|---|
| side column right of center | vertical | right |
| side column left of center | vertical | left |
| center or full-width column | horizontal | bottom |
| custom/nested (unresolved) | vertical | right (documented fallback) |

## State machine

| State | Transition | Trigger | Actor |
|---|---|---|---|
| `pinned` | → `floating-expanded` | click pin in group header | user |
| `floating-expanded` | → `floating-collapsed` | focus leaves window (outside pointer, focusout, Escape) | user / system |
| `floating-collapsed` | → `floating-expanded` | click a title in the edge bar | user |
| `floating-expanded` | → `pinned` | click pin in floating window header | user |
| `floating-collapsed` | → `pinned` | click pin in the edge bar | user |
| any | → (removed) | all tabs of the group closed | user / system |

Float/dock are single store transactions:

- **float(groupId):** if `groupId` is the maximized group, first restore the
  pre-max layout and derive the target's placement from `preMaximizeLayout`
  (never from the maximized overlay's live geometry). Capture placement +
  panel definitions + size, mark the group's panel ids as floated (non-
  destructive removal set), remove the panels from the grid, record
  `floatingGroups[groupId]`, clear the removal set, persist the floating blob,
  then `persistEnvLayoutNow`.
- **dock(groupId):** remove the floating entry, materialize the group back at
  its remembered column/path (reusing the column if it still has groups;
  otherwise create a group at that column via `fallbackGroupPosition`-style
  resolution), re-add the tabs in saved order, restore the active tab, persist
  the floating blob, then `persistEnvLayoutNow`.
- **restore (reload / env switch / preset / custom layout / maximize
  restore):** after the grid restore completes (before `isRestoringLayout`
  clears), `restoreFloatingAfterLayout` materializes each floating group via
  the same dock materialization path, then applies the float transition
  (non-destructive removal + portal adoption). Floating panel ids that no
  longer exist or whose definitions are invalid are dropped; empty floating
  groups are removed.

## Non-destructive removal contract

Normal panel removal (user closes a tab) keeps today's behavior: the panel's
portal is released (`panelPortalManager.release`), terminals park/stop, vscode
stops, and `handleMaximizeExitOnLastClose` may exit maximize.

Float/dock/restore removals are **non-destructive**: while the store's floated
panel-id set is non-empty, `setupPortalCleanup`'s `onDidRemovePanel` handler
skips `release`, terminal park/stop, vscode stop, and
`handleMaximizeExitOnLastClose` for those ids. `panelPortalManager.reconcile`
(env-switch fast path) and `releaseByEnv` must also skip floated panel ids of
the incoming/current env. The set is cleared when the transaction settles;
subsequent user closes behave normally. Terminal processes, editor state, and
iframe contents survive float because their portal entries and DOM elements are
only reparented, never released.

## Failure modes

- **sessionStorage unavailable (private mode / quota):** floating state
  degrades to per-session ephemeral; the UI stays functional and groups stay
  floating until the next layout rebuild; writes degrade silently. Defaults
  back to pinned on reload.
- **Layout rebuild / preset switch / env switch while floating:** the floating
  blob is re-applied after the grid restore completes. A floated panel whose
  definition cannot be materialized (e.g. its session was deleted) is dropped
  from the floating state without error.
- **Env-scoped panels (browser, file-editor, vscode, commit-detail,
  diff-viewer, pr-detail) floated at env switch:** they are released by
  `releaseByEnv` on switch away exactly as if docked; switching back
  re-materializes them fresh from the blob. Global panels (chat, terminal,
  changes, files, plan) keep their portal entries and state.
- **Session/tab deletion while floating:** removing a session tab that is
  floating removes it from the floating group's `panelIds` and `panels`; an
  empty floating group is removed entirely. Session replacement on env switch
  rewrites the floating entry's panel ids, definitions, portal params/title,
  and `activePanelId` (see integration contract).
- **Maximize while floating:** maximize applies to grid groups only; floating
  windows render above the maximized grid. Floating the group that is
  currently maximized restores the pre-max layout first, then floats the
  group, as one transaction.
- **Reset layout / "clear UI state":** floating state for the environment is
  cleared (in-memory and storage) and all groups return to the grid on the
  next build. `cleanupTaskStorage` removes the floating key alongside the
  layout/maximize keys.
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

## Integration contract

- **Focus ownership:** one floating coordinator (module-level, like
  `use-panel-search`'s last-interacted tracker) owns expansion/collapse. The
  expanded window collapses on: window-capture pointerdown outside any floating
  window; `focusout` where the newly focused element is outside the owning
  window's region; or Escape while `document.activeElement` is inside the
  owning window's region. Nested dialogs/editors consume Escape before the
  coordinator (capture runs after descendants). Only the focused/last-interacted
  expanded window collapses; other floating windows keep their display state.
- **Collapsed bar accessibility:** the bar is a semantic `tablist` with `tab`
  roles, `aria-selected` on the active tab, an accessible group label, roving
  Tab + Arrow navigation, Enter/Space activation, and focus return to the
  previously focused control on expand/collapse. The pin control in the bar is
  reachable by keyboard.
- **Stacking:** floating windows/bars stack by `order` with deterministic
  offsets and z-index; an expanded window renders above collapsed bars of the
  same edge; clicking a stacked bar activates its group.
- **Add-panel routing:** a single identity resolver
  (`focusOrAddFloatingOrGridPanel`) is the only path that decides
  expand-vs-create. Adding a panel that already floats expands/focuses the
  floating window with that tab active; genuinely new tabs insert into their
  intended grid group (or into the floating group only where the action
  explicitly targets it); no duplicate grid panels are created for a floated
  panel. Every single-instance/add action family (plan, changes, files,
  terminal, preview, review, plugin, session) routes through it.
- **Titles:** registry panel ids render via `panelTitle()`; unknown ids
  (session tabs, file diffs, plugin panels) render the captured live title
  (from the dockview panel api / portal entry at float time), kept current
  through session replacement and plugin registration.
- **Layout persistence:** `persistEnvLayoutNow` runs explicitly after every
  settled float/dock/restore transaction (the normal persistence callback
  gates on `isRestoringLayout`, so transactions cannot rely on it).

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
- **GIVEN** a bottom terminal group is floating, **WHEN** the user clicks
  outside it, **THEN** the group collapses to a horizontal title bar on the
  bottom edge.
- **GIVEN** a collapsed floating group with two tabs, **WHEN** the user clicks
  the second tab's title, **THEN** the floating window expands with that tab
  active.
- **GIVEN** a floating group is expanded, **WHEN** the user presses Escape
  while focus is inside the window, **THEN** the window collapses to its edge
  bar.
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
- **GIVEN** the workbench is reloaded within the same tab while a group is
  floating, **WHEN** the layout restores, **THEN** the group is floating again
  with the same tabs, active tab, and display state (recreated from the blob,
  not merely re-adopted).
- **GIVEN** a floating group, **WHEN** the user switches tasks and back,
  **THEN** the group is still floating in the same display state with the same
  tabs; a floated chat tab tracks the incoming session (title, params, active
  id rewritten), and a floated env-scoped panel (browser/file editor) is
  recreated on return.
- **GIVEN** the user floats the last group in the right column, **WHEN** the
  user clicks the pin again, **THEN** a new right-column group is materialized
  at the remembered position with the tabs restored (not a center-column
  fallback).
- **GIVEN** the user floats the group that is currently maximized, **WHEN**
  the pin is clicked, **THEN** the group exits maximize and floats over the
  restored grid at its pre-max placement and size.
- **GIVEN** a group is floating and the user clicks "Add Plan Panel" (or any
  add-panel action for a panel in that group), **WHEN** the action fires,
  **THEN** the floating window is expanded/focused with that tab active rather
  than a duplicate grid panel being created.
- **GIVEN** a group is floating, **WHEN** the user closes all its tabs,
  **THEN** the floating state is removed and no edge bar or window remains.
- **GIVEN** two groups float on the same edge, **WHEN** the user collapses and
  expands them, **THEN** they stack with deterministic offsets/z-order, the
  expanded window renders above the collapsed bar, and each bar's titles
  remain clickable.
- **GIVEN** a floating group and a nested dialog (e.g. Radix modal) open inside
  it, **WHEN** the user presses Escape, **THEN** the dialog closes and the
  floating window does not collapse.
- **GIVEN** the user clicks Reset UI state while groups float, **WHEN** the
  reset completes, **THEN** all groups are back in the grid and no floating
  storage remains for the environment.
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
