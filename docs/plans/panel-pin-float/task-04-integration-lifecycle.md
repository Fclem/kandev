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
- **Session replacement (single coordinator over grid + floating + auto-session hook):** `replaceStaleSessionPanels` returns an old→new mapping applied to floating entries (panelIds, `FloatingPanelDef`s, portal params/title, `activePanelId`). The deterministic **winner** is the group's active stale panel, else the first stale floating panel in saved tab order. The winner is written to a store-owned field — `floatingSessionWinner: { sessionId, envId, generation } | null` — atomically with replacement and consumed by `shouldSkipPanelEnsure` (`dockview-session-tabs.ts`) before the always-mounted `useAutoSessionTab` hook's ensure effect; the hook skips only the winner id for grid insertion/activation (unrelated current-session siblings are ensured as today via `addCurrentSessionSiblings`/`ensureSiblingPanels`, with an explicit anchor when the winner floats). A winner in the grid drops the floating stale copy. Other stale floating panels are dropped; `activePanelId` points to the winner's new id (or the first surviving panel). No panel id ever exists in both surfaces. Tested through the real desktop hook, not only direct `replaceStaleSessionPanels` calls.
- **Right-width enforcement gate + floating-aware toggle:** `enforcePinnedTargets` restores the right target only when a live pinned-right column exists (`hasPinnedRightColumn`-style check); with all right groups floated, the center column is never resized to the right target. The `toggleRightPanels` **show path** is floating-aware: floating ids are excluded from the re-added default right column (or the toggle docks them explicitly), **groups with zero remaining panels are removed, and the right column is dropped entirely when no pinned-right panels remain** — the serialized tree is always legal (no empty branch/leaf); `rightPanelsVisible` stays an independent intent/visibility bit. Tested with one and both right groups floated, hide→show while floating, and container resize, asserting a legal serialized tree.
- **Add-panel routing:** `focusOrAddPanel` becomes `focusOrAddFloatingOrGridPanel`; every single-instance/add action family (plan, changes, files, terminal, preview, review, plugin, session, deferred actions — including direct `api.getPanel()` checks in `dockview-panel-actions.ts` and `dockview-terminal-panel-actions.ts`) routes through it. Adding a panel that already floats expands/focuses the floating window with that tab active; no duplicate grid panel is created for a floated panel; grid behavior for non-floated panels is unchanged (existing add-panel tests are the regression net).
- **Maximize:** floating windows render above a maximized grid; `floatGroup` on the currently maximized group exits maximize first with placement derived from `preMaximizeLayout`, sequenced so the trailing restore rAF does not reassert the overlay (tested deterministically).
- **Closing tabs:** closing all tabs of a floating group removes its floating entry (no orphan edge bar); the group's pin state resets to pinned. A user close of a floated tab mid-transaction drops it from the floating definitions and releases its portal (no leak).
- **Reset is an id-aware docking merge, session-aware:** the reset layout owns group/column **placement**; the floating definition owns the panel **payload** (component, params, tabComponent) and saved tab order; the active panel is merged explicitly — **scoped to valid definitions**. `session:*` floating defs are validated against the active task/session set and env: stale/deleted/absent-session definitions are dropped and the reset chat placeholder/default behavior is retained when no valid session remains. Reset-default panels are reused by id (never duplicated); a floated ordinary terminal keeps its real terminal id rather than being retargeted to the default `terminal-default` params. Floating state (memory + storage) clears only after the merged grid is committed and persisted; groups do not re-float after reset. `cleanupTaskStorage` removes `kandev.dockview.env-floating.<envId>` keys on task deletion.

## Verification

```bash
cd apps/web && pnpm vitest run lib/state/dockview-env-switch.test.ts lib/state/dockview-layout-builders.test.ts lib/state/dockview-panel-actions.test.ts lib/state/dockview-floating-store.test.ts lib/state/dockview-store.test.ts lib/state/dockview-pinned-enforce.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/state/dockview-floating.ts` (`restoreFloatingAfterLayout`; invoked from every completion point)
- `apps/web/lib/state/dockview-env-switch.ts` (replacement coordinator + winner rule + `floatingSessionWinner` write; grid-insertion suppression; fast + slow path restore invocation)
- `apps/web/components/task/dockview-session-tabs.ts` + `dockview-desktop-layout.tsx` (`shouldSkipPanelEnsure` consumes `floatingSessionWinner`)
- `apps/web/components/task/dockview-layout-restore.ts` (initial three branches)
- `apps/web/components/task/dockview-layout-setup.ts` (restore completion points; `handleMaximizeExitOnLastClose` interplay; `canPersistLayout` gate)
- `apps/web/lib/state/dockview-pinned-enforce.ts` (live-pinned-right-column gate)
- `apps/web/lib/state/dockview-store.ts` (`toggleRightPanels` floating-aware show path, reset-as-merge, maximize exit-on-float)
- `apps/web/lib/state/dockview-layout-builders.ts` (`focusOrAddFloatingOrGridPanel`)
- `apps/web/lib/state/dockview-panel-actions.ts`, `dockview-terminal-panel-actions.ts` (route all add actions through the resolver)
- `apps/web/lib/local-storage.ts` (`cleanupTaskStorage`)
- Existing tests updated for the floating branches

## Inputs

- Spec: Restore call sites, Session replacement, Add-panel routing, State machine, Failure modes (right-width enforcement, reset merge, maximize, partial failure), Scenarios (task switch, add-panel duplicate prevention, maximize→float, reset merge, no-duplicate-id, mid-transaction close).
- Existing flows: `restoreEnvLayout`/`tryRestoreLayout` (`dockview-layout-restore.ts:198-320`), `replaceStaleSessionPanels` (`dockview-env-switch.ts:132-180`) + `restoreMissingSessionPanel` (`:584-594`), `enforcePinnedTargets` (`dockview-pinned-enforce.ts:105-127`), `applyLayoutFixups`, `persistEnvLayoutNow`, `toggleRightPanels` (`dockview-store.ts:523-567`), `resetToEffectiveDefault` (`dockview-store.ts:1060-1073`), `setupLayoutPersistence` (`dockview-layout-setup.ts:346-403`).

## Dependencies and Risks

- Depends on task-03.
- Risk: one missed completion point re-exposes the reload blocker; the spec's call-site table is the checklist and each entry has a test.
- Risk: the session coordinator must be visible to `useAutoSessionTab` BEFORE its ensure effect, or the incoming id lands in both surfaces; test through the real desktop hook.
- Risk: the `toggleRightPanels` show path must exclude floating ids (or dock explicitly), or default right panels duplicate identity while floating.
- Risk: the enforcement gate must check a live pinned-right column, not `rightPanelsVisible` alone; with all right groups floated the center column is `sv.length - 1`.
- Risk: the duplicate-prevention refactor must not change grid behavior for non-floated panels; existing add-panel tests are the regression net.
- Risk: `canPersistLayout` must gate `persistNow`, the debounce callback, the event handler, AND `beforeunload`, and hold an already-scheduled timer; one missed entry point re-exposes partial-layout persistence.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
