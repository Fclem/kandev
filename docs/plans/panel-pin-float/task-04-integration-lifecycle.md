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

- **Restore call sites (exhaustive):** `restoreFloatingAfterLayout` (in `lib/state/dockview-floating.ts`, idempotent) runs before `isRestoringLayout` clears at every completion point, each with one focused test:
  1. initial mount `restoreEnvLayout` — after saved-env-layout, maximize-only, and default/route-intent branches (`components/task/dockview-layout-restore.ts`);
  2. env-switch fast path and slow path (`lib/state/dockview-env-switch.ts`), including `applyInitialRouteLayout`;
  3. maximize restore (`restoreMaximizeFromStorage`);
  4. preset and custom layout apply (`applyLayout`/`applyLayoutAndSet`);
  5. `toggleRightPanels` (`lib/state/dockview-store.ts`), where the restore is a no-op if already applied;
  6. reset/default build.
  No docked flash appears after any restore.
- **Session replacement:** `replaceStaleSessionPanels` returns an old→new mapping applied to floating entries (panelIds, `FloatingPanelDef`s, portal params/title, `activePanelId`). With several stale floating `session:*` panels, the **winner** is deterministic: the group's active stale panel, else the first stale floating panel in saved tab order; only the winner maps to the incoming id, other stale floating panels are dropped, and `activePanelId` points to the winner's new id (or the first surviving panel). Duplicate mappings never occur. Delayed replacement and active-tab rewrite are tested.
- **Add-panel routing:** `focusOrAddPanel` becomes `focusOrAddFloatingOrGridPanel`; every single-instance/add action family (plan, changes, files, terminal, preview, review, plugin, session, deferred actions — including direct `api.getPanel()` checks in `dockview-panel-actions.ts` and `dockview-terminal-panel-actions.ts`) routes through it. Adding a panel that already floats expands/focuses the floating window with that tab active; no duplicate grid panel is created for a floated panel; grid behavior for non-floated panels is unchanged (existing add-panel tests are the regression net).
- **Maximize:** floating windows render above a maximized grid; `floatGroup` on the currently maximized group exits maximize first with placement derived from `preMaximizeLayout`, sequenced so the trailing restore rAF does not reassert the overlay (tested deterministically).
- **Closing tabs:** closing all tabs of a floating group removes its floating entry (no orphan edge bar); the group's pin state resets to pinned. A user close of a floated tab mid-transaction drops it from the floating definitions and releases its portal (no leak).
- **Reset docks:** reset materializes every floating group into the reset target grid (side → right column, center → center column, fallback center), clears floating state (memory + storage) only after the grid contains them, persists; groups do not re-float after reset. `cleanupTaskStorage` removes `kandev.dockview.env-floating.<envId>` keys on task deletion.

## Verification

```bash
cd apps/web && pnpm vitest run lib/state/dockview-env-switch.test.ts lib/state/dockview-layout-builders.test.ts lib/state/dockview-panel-actions.test.ts lib/state/dockview-floating-store.test.ts lib/state/dockview-store.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/state/dockview-floating.ts` (`restoreFloatingAfterLayout`; invoked from every completion point)
- `apps/web/lib/state/dockview-env-switch.ts` (replacement mapping + winner rule; fast + slow path restore invocation)
- `apps/web/components/task/dockview-layout-restore.ts` (initial three branches)
- `apps/web/components/task/dockview-layout-setup.ts` (restore completion points; `handleMaximizeExitOnLastClose` interplay)
- `apps/web/lib/state/dockview-store.ts` (`toggleRightPanels` completion point, reset-as-docking, maximize exit-on-float)
- `apps/web/lib/state/dockview-layout-builders.ts` (`focusOrAddFloatingOrGridPanel`)
- `apps/web/lib/state/dockview-panel-actions.ts`, `dockview-terminal-panel-actions.ts` (route all add actions through the resolver)
- `apps/web/lib/local-storage.ts` (`cleanupTaskStorage`)
- Existing tests updated for the floating branches

## Inputs

- Spec: Restore call sites, Session replacement, Add-panel routing, State machine, Failure modes (reset, maximize, partial failure), Scenarios (task switch, add-panel duplicate prevention, maximize→float, reset, mid-transaction close).
- Existing flows: `restoreEnvLayout`/`tryRestoreLayout` (`dockview-layout-restore.ts:198-320`), `replaceStaleSessionPanels` (`dockview-env-switch.ts:132-180`), `applyLayoutFixups`, `persistEnvLayoutNow`, `toggleRightPanels` (`dockview-store.ts:523-567`), `resetToEffectiveDefault` (`dockview-store.ts:1060-1073`).

## Dependencies and Risks

- Depends on task-03.
- Risk: one missed completion point re-exposes the reload blocker; the spec's call-site table is the checklist and each entry has a test.
- Risk: `reconcile`/`releaseByEnv` exclusion sets must be per-env or a same-id panel in another env fails to release on switch-away.
- Risk: the duplicate-prevention refactor must not change grid behavior for non-floated panels; existing add-panel tests are the regression net.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
