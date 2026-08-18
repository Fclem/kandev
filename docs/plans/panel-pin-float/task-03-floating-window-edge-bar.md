---
id: "03-floating-window-edge-bar"
title: "Floating window, collapsed edge bar, and owned-region coordinator"
status: pending
wave: 3
depends_on: ["01-floating-store-state"]
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 03: Floating window, collapsed edge bar, and owned-region coordinator

## Acceptance

- A floating overlay (`components/task/dockview-floating-panel.tsx`, new) mounted inside the `DockviewDesktopLayout` root renders one entry per `floatingGroups` entry, absolutely positioned over the workbench with z-index above the dockview grid. Expanded windows show a header (tab strip with active tab highlighted + pin control) and the active panel's content adopted from `panelPortalManager` (same portal element the grid uses; content stays alive across float/dock).
- **Owned-region coordinator** (`dockview-floating-coordinator.ts`, pattern: `hooks/use-panel-search.ts`): a floating window's region = its DOM subtree plus any Radix layer opened from within it (`useFloatingOwnedLayer` hook wired to the existing `onOpenChange` handlers; layers stay owned until closed). Collapse triggers: window-capture pointerdown whose target is in no owned region; `focusout` whose `relatedTarget` (checked after a microtask, for Radix portal focus moves) is in no owned region; or Escape per the Escape contract. Only the focused/last-interacted expanded window collapses; other floating groups keep their display state.
- **Escape contract:** the coordinator listens on the **bubble** phase and honors `event.defaultPrevented` — a Radix dismissable layer or editor handler that handles Escape wins; otherwise Escape collapses the focused expanded window. No capture-phase claim.
- The collapsed bar is a semantic `tablist` with `tab` roles, `aria-selected`, an accessible group label, **roving tabindex** (one tab stop on the active tab), Arrow Up/Down and Left/Right navigation (both axes accepted in either orientation), Home/End, Enter/Space activation, Escape collapse, and focus return on expand/collapse. It is a **vertical** title bar (titles stacked) for side groups and a **horizontal** title bar (titles in a row) for horizontal groups, showing each tab's title (registry ids via `panelTitle()`, unknown ids via the live portal/dockview title, falling back to the persisted definition title) in tab order with the active tab highlighted, plus the pin control (a separate button in DOM order after the tablist).
- Clicking a title sets it active and expands the window; clicking the bar's pin control re-docks the group.
- Stacking: `order` allocated from the monotonic per-env counter; render order sorted by `(order, groupId)`; offsets `order × 12px`; z-index `1000 + order`; expanded windows render above collapsed bars of the same edge.
- Test ids: `dockview-pin-btn`, `dockview-floating-window-<groupId>`, `dockview-floating-bar-<groupId>`, `dockview-floating-tab-<groupId>-<panelId>`.

## Verification

```bash
cd apps/web && pnpm vitest run components/task/dockview-floating-panel.test.tsx components/task/dockview-group-actions.test.tsx
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/task/dockview-floating-panel.tsx` (new; expanded window, collapsed bar, portal adoption, tablist semantics)
- `apps/web/components/task/dockview-floating-coordinator.ts` (new; owned regions, pointer/focus/Escape ownership, `useFloatingOwnedLayer`, stacking)
- `apps/web/components/task/dockview-desktop-layout.tsx` (mount overlay)
- `apps/web/components/task/dockview-floating-panel.test.tsx` (new)
- Reuse: `panelPortalManager` (`lib/layout/panel-portal-manager.ts`), `usePanelSearch` pointer-outside pattern (`hooks/use-panel-search.ts`), `panelTitle()` (`lib/state/layout-manager/panel-title.ts`), `PinButton` from task-02

## Inputs

- Spec: What (floating window, collapse orientation, bar contents, title click), Focus ownership, Escape contract, Collapsed bar accessibility, Stacking, Scenarios (outside click, Escape, tab-out collapse, portaled menu, two groups same edge, nested dialog Escape).
- `walkthrough-floating-window.tsx` for the overlay precedent; `panel-portal-host.tsx` `usePortalSlot` for element adoption mechanics; `popup-menu.tsx`/`plan-selection-popover.tsx` for the body-portaled layer precedent.

## Dependencies and Risks

- Depends on task-01 (store + detach registry) and task-02 (`PinButton`).
- Risk: the adopted portal element must be appended exactly once; guard against double-append when both the grid wrapper and the floating window could mount it (float removes the grid wrapper first).
- Risk: an unregistered Radix layer (component that opens a menu without `useFloatingOwnedLayer`) collapses the window under the open menu — the hook must be applied to every interactive layer inside floating panels, not just dialogs.
- Risk: Escape must be tested with real Radix AlertDialog/DropdownMenu and an editor key handler, not only synthetic `fireEvent.keyDown` ordering.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
