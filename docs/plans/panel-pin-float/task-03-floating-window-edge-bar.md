---
id: "03-floating-window-edge-bar"
title: "Floating window, collapsed edge bar, and focus coordinator"
status: pending
wave: 3
depends_on: ["01-floating-store-state"]
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 03: Floating window, collapsed edge bar, and focus coordinator

## Acceptance

- A floating overlay (`components/task/dockview-floating-panel.tsx`, new) mounted inside the `DockviewDesktopLayout` root renders one entry per `floatingGroups` entry, absolutely positioned over the workbench with z-index above the dockview grid. Expanded windows show a header (tab strip with active tab highlighted + pin control) and the active panel's content adopted from `panelPortalManager` (same portal element the grid uses; content stays alive across float/dock).
- Collapse is owned by one module-level coordinator (`dockview-floating-coordinator.ts`, pattern: `hooks/use-panel-search.ts`): window-capture pointerdown outside any floating window, `focusout` to a target outside the owning window's region, or Escape while `document.activeElement` is inside the owning window's region. Nested dialogs/editors consume Escape before the coordinator. Only the focused/last-interacted expanded window collapses; other floating groups keep their display state.
- The collapsed bar is a semantic `tablist` with `tab` roles, `aria-selected` on the active tab, an accessible group label, roving Tab + Arrow navigation, Enter/Space activation, and focus return on expand/collapse. It is a **vertical** title bar (titles stacked) for side groups and a **horizontal** title bar (titles in a row) for horizontal groups, showing each tab's title (registry ids via `panelTitle()`, unknown ids via the captured live title from the portal/dockview api) in tab order with the active tab highlighted, plus the pin control.
- Clicking a title sets it active and expands the window; clicking the bar's pin control re-docks the group.
- Stacking: groups on the same edge stack by `order` with deterministic offsets and z-index; an expanded window renders above collapsed bars of the same edge.
- Test ids: `dockview-pin-btn`, `dockview-floating-window-<groupId>`, `dockview-floating-bar-<groupId>`, `dockview-floating-tab-<groupId>-<panelId>`.

## Verification

```bash
cd apps/web && pnpm vitest run components/task/dockview-floating-panel.test.tsx components/task/dockview-group-actions.test.tsx
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/task/dockview-floating-panel.tsx` (new; expanded window, collapsed bar, portal adoption, tablist semantics)
- `apps/web/components/task/dockview-floating-coordinator.ts` (new; focus/outside-pointer/Escape ownership, stacking)
- `apps/web/components/task/dockview-desktop-layout.tsx` (mount overlay)
- `apps/web/components/task/dockview-floating-panel.test.tsx` (new)
- Reuse: `panelPortalManager` (`lib/layout/panel-portal-manager.ts`), `usePanelSearch` pointer-outside pattern (`hooks/use-panel-search.ts`), `panelTitle()` (`lib/state/layout-manager/panel-title.ts`), `PinButton` from task-02

## Inputs

- Spec: What (floating window, collapse orientation, bar contents, title click), Integration contract (Focus ownership, Collapsed bar accessibility, Stacking, Titles), Scenarios (outside click, Escape, tab-out collapse, two groups same edge, nested dialog Escape).
- `walkthrough-floating-window.tsx` for the overlay precedent; `panel-portal-host.tsx` `usePortalSlot` for element adoption mechanics.

## Dependencies and Risks

- Depends on task-01 (store + non-destructive removal) and task-02 (`PinButton`).
- Risk: the adopted portal element must be appended exactly once; guard against double-append when both the grid wrapper and the floating window could mount it (float removes the grid wrapper first).
- Risk: Escape ownership — the coordinator must lose to nested dialogs (capture after descendants) and must only collapse the owning window; test with a Radix modal and an editor inside a floating window.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
