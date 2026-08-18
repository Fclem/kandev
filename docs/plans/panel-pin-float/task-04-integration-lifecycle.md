---
id: "04-integration-lifecycle"
title: "Floating lifecycle integration"
status: pending
wave: 4
depends_on: ["03-floating-window-edge-bar"]
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 04: Floating lifecycle integration

## Acceptance

- `restoreFloatingAfterLayout` (in `lib/state/dockview-floating.ts`) runs at **every** grid-restore completion point before `isRestoringLayout` clears: initial ready, env-switch fast path, env-switch slow path, maximize restore, built-in preset apply, custom layout apply, and reset/default build. It materializes each floating group (dock materialization from task-01) then applies the float transition; floated panel ids that no longer exist are dropped, empty floating groups are removed. No docked flash is visible after restore.
- Session replacement: `replaceStaleSessionPanels` returns an old→new panel identity mapping that is applied to floating entries (panelIds, `FloatingPanelDef`s, portal params/title, `activePanelId`) — a floated chat tab tracks the incoming session; stale floating siblings are dropped deterministically; delayed replacement and active-tab replacement are covered by tests.
- Add-panel routing: `focusOrAddPanel` becomes `focusOrAddFloatingOrGridPanel`; every single-instance/add action family (plan, changes, files, terminal, preview, review, plugin, session) routes through it. Adding a panel that already floats expands/focuses the floating window with that tab active; no duplicate grid panel is created for a floated panel.
- Maximize operates on grid groups only: floating windows render above a maximized grid, and `floatGroup` on the currently maximized group exits maximize first with placement derived from `preMaximizeLayout` (task-01 behavior verified end-to-end here).
- Closing all tabs of a floating group removes its floating entry (no orphan edge bar); the group's pin state resets to pinned.
- Reset layout / "clear UI state" clears floating state in memory and storage and returns groups to the grid; `cleanupTaskStorage` removes `kandev.dockview.env-floating.<envId>` keys on task deletion.

## Verification

```bash
cd apps/web && pnpm vitest run lib/state/dockview-env-switch.test.ts lib/state/dockview-layout-builders.test.ts lib/state/dockview-panel-actions.test.ts lib/state/dockview-floating-store.test.ts lib/state/dockview-store.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/state/dockview-floating.ts` (`restoreFloatingAfterLayout`; invoked from every restore path)
- `apps/web/lib/state/dockview-env-switch.ts` (replacement mapping for floating entries; fast + slow path restore invocation)
- `apps/web/components/task/dockview-layout-setup.ts` (restore completion points; `handleMaximizeExitOnLastClose` interplay)
- `apps/web/lib/state/dockview-layout-builders.ts` (`focusOrAddFloatingOrGridPanel`)
- `apps/web/lib/state/dockview-panel-actions.ts`, `dockview-terminal-panel-actions.ts` (route all add actions through the resolver)
- `apps/web/lib/state/dockview-store.ts` (reset clears, maximize exit-on-float)
- `apps/web/lib/local-storage.ts` (`cleanupTaskStorage`)
- Existing tests updated for the floating branches

## Inputs

- Spec: Integration contract (Add-panel routing, Layout persistence), Failure modes (layout rebuild, session deletion, maximize, reset), Scenarios (task switch with floated chat/env-scoped panel, add-panel duplicate prevention, maximize→float, reset).
- Existing flows: `restoreMaximizeFromStorage` (`dockview-store.ts:723-756`), `replaceStaleSessionPanels` (`dockview-env-switch.ts:132-180`), `applyLayoutFixups`, `persistEnvLayoutNow`.

## Dependencies and Risks

- Depends on task-03.
- Risk: restore ordering — one missed completion point (e.g. preset apply) re-exposes the docked flash; enumerate every path in the task and test each.
- Risk: `reconcile`/`releaseByEnv` must exclude floated ids of the incoming/current env or a fast env switch destroys floated portal content.
- Risk: duplicate-prevention refactor must not change grid behavior for non-floated panels; existing add-panel tests are the regression net.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
