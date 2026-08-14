---
spec: ../../specs/tasks/subtask-reparenting-drag-drop.md
created: 2026-08-13
status: done
---

# Fix Plan: Task drag must not start from the context menu

## Root cause

The sidebar task tree (desktop sidebar and the mobile task switcher sheet)
wires dnd-kit's drag listeners onto a handle `div` that wraps the task row.
`TaskRow` renders `TaskItemWithContextMenu`, whose Radix `ContextMenuContent`
portal is a **fiber descendant** of that handle. React synthetic events do not
follow the DOM tree for portals; they bubble through the **React fiber tree**.
So a `mousedown`, `touchstart`, or `pointerdown` on any menu item (e.g. the
`Color` submenu trigger or one of its swatches) reaches the handle's sensor
listeners:

- `MouseSensor.activators` ignores only right-click (`button === 2`), so a
  left-press on a menu item activates the sensor; after the 8px
  `DRAG_ACTIVATION_DISTANCE` the row drags (dnd-kit source:
  `MouseSensor.activators` in `@dnd-kit/core`).
- `TouchSensor` activates on `touchstart` after a 250ms hold.
- A plain `click` on a menu item additionally fiber-bubbles to the row's
  `onClick` → `onSelectTask`, activating the task as a side effect.

The kanban card context menu is **not** affected: there the
`ContextMenuContent` is a fiber *sibling* of the card shell that holds the
listeners, so no leak exists (verified structurally in
`apps/web/components/kanban-card-context-menu.tsx`). The kanban *dropdown*
menu was already fixed the same way (`DropdownEntry` guards
`onClick`/`onPointerDown`; see `kanban-card-menu-items.tsx` and its test).

## Fix

Contain pointer-start and click events at the menu boundary in
`TaskItemWithContextMenu` (`apps/web/components/task/task-switcher-context-menu.tsx`):
add bubble-phase `stopPropagation` handlers to the `ContextMenuContent` for
`onMouseDown`, `onPointerDown`, `onTouchStart`, and `onClick`.

This is safe because the handlers sit on an *ancestor* of every item: item
handlers (Radix `onSelect`, submenu hover, item click) run first as the event
bubbles up, then the content guard stops propagation before the event reaches
the drag handle or the row. Radix's outside-click dismissal and keyboard
activation are unaffected. All submenu portals (`ContextMenuSubContent`,
including the Color submenu) are fiber descendants of the content, so the
single guard covers them.

## Tests

### Unit regression (`apps/web/components/task/task-switcher-context-menu.test.tsx`, new)

Render `TaskItemWithContextMenu` inside a wrapper `div` with spy
`onMouseDown`/`onPointerDown`/`onClick` handlers that stand in for the drag
handle and row click. Open the real menu and assert:

1. `mousedown`/`pointerdown` on the `Color` submenu trigger do not reach the
   wrapper (fails before the fix: the spies are called).
2. `mousedown`/`pointerdown` on a color swatch inside the open submenu do not
   reach the wrapper.
3. `click` on a menu item (e.g. Archive) still invokes its action
   (`onArchiveTask`) and does **not** reach the wrapper's `onClick`.

i18n is initialized globally (`vitest.setup.ts`), so the real English labels
render; wrap in `StateProvider`/`ToastProvider` per `task-switcher.test.tsx`.

### E2E (desktop, extend `apps/web/e2e/tests/task/subtask-reparent-drag-drop.spec.ts`)

Seed a parent with a subtask, right-click the subtask row, assert the context
menu opens (Color item visible), then press the mouse down on the Color item,
move ≥8px, assert **no** nest drop zone appears and the row order is unchanged,
then release.

Mobile: the guard is shared code used by the mobile sheet; the existing
`mobile-subtask-reparent-drag-drop.spec.ts` (touch drag via `mobile-chrome`
project) must stay green to prove touch drag still works. No new mobile spec:
the changed surface is one shared event guard, and the desktop spec proves the
containment mechanism.

## Implementation Wave

1. [Guard context-menu pointer events](task-01-guard-context-menu-pointer-events.md) — pending, sequential.

## Risks

- A capture-phase guard would break item selection (it would stop the event
  before it reaches the items); the guard must be bubble-phase on the content.
- Guarding only the top-level items would miss submenu content (Color); the
  content-level guard covers all fiber descendants.
- The kanban card context menu must stay untouched (no leak there); changing
  it would be speculative.
