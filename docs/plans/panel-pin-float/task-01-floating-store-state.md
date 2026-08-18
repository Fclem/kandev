---
id: "01-floating-store-state"
title: "Floating-group store, persistence, and non-destructive removal"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 01: Floating-group store, persistence, and non-destructive removal

## Acceptance

- `useDockviewStore` exposes `floatingGroups: Record<string, FloatingGroupState>` (groupId, columnId, columnKind, treePath, edge, orientation, size, panels as full `FloatingPanelDef[]`, activePanelId, display, order) and actions `floatGroup(groupId)`, `dockGroup(groupId)`, `setFloatingDisplay(groupId, display)`, `setFloatingActivePanel(groupId, panelId)`.
- `floatGroup` is one transaction: exits maximize first when the target is the maximized group (placement/geometry derived from `preMaximizeLayout`, never the overlay's live dimensions), captures placement + panel definitions + size from the live `LayoutState`, marks the group's panel ids in the floated-id set, removes the panels from the grid, records the entry, clears the set, persists the floating blob, then calls `persistEnvLayoutNow`.
- `dockGroup` materializes the group back at its remembered column/tree path (reusing the column when it still has groups; `fallbackGroupPosition`-style resolution otherwise), re-adds tabs in saved order, restores the active tab, clears the entry, persists, then `persistEnvLayoutNow`. Docking the last group in an emptied side column recreates a side-column group, never a center fallback.
- **Non-destructive removal:** while the floated-id set is non-empty, `setupPortalCleanup`'s `onDidRemovePanel` skips `panelPortalManager.release`, terminal park/stop, vscode stop, and `handleMaximizeExitOnLastClose` for those ids; `panelPortalManager.reconcile` and `releaseByEnv` skip the current env's floated ids. A real user close after re-dock still releases (proven by test).
- Persistence: versioned `EnvFloatingState` blob under `kandev.dockview.env-floating.<envId>` with a type guard (mirroring `isEnvMaximizeState`) that drops invalid entries and defaults to `{}`; saved on env switch (`saveOutgoingEnv`), restored after layout restore, cleared by reset-layout and `cleanupTaskStorage`.
- Width/height sync (`syncPinnedWidthsFromApi`, `enforcePinnedTargets`) does not re-insert or corrupt pinned widths while a group is floating.

## Verification

```bash
cd apps/web && pnpm vitest run lib/state/dockview-floating-store.test.ts lib/layout/panel-portal-manager.test.ts lib/local-storage.test.ts lib/state/dockview-store.test.ts lib/state/dockview-pinned-enforce.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/state/dockview-store.ts` (state, actions, floated-id set, env-switch save/restore, reset clears, maximize interplay, width-sync guard)
- `apps/web/lib/state/dockview-floating.ts` (new: placement capture/derivation, materialization, `restoreFloatingAfterLayout`, `EnvFloatingState` type guard)
- `apps/web/lib/local-storage.ts` (`DOCKVIEW_ENV_FLOATING_PREFIX`, get/set/remove, `cleanupTaskStorage`)
- `apps/web/components/task/dockview-layout-setup.ts` (floated-id suppression in `setupPortalCleanup`)
- `apps/web/lib/layout/panel-portal-manager.ts` (reconcile/releaseByEnv floated-id exclusion)
- `apps/web/lib/state/dockview-floating-store.test.ts` (new), `apps/web/lib/layout/panel-portal-manager.test.ts` (new), existing store/pinned-enforce/local-storage tests

## Inputs

- Spec: Data model, State machine (float/dock/restore transactions), Non-destructive removal contract, Failure modes (storage, reset, maximize, env-scoped release), Persistence guarantees.
- Existing patterns: `buildMaximizeActions` (`dockview-store.ts:934-1000`), `saveOutgoingEnv` (`:779-831`), `isEnvMaximizeState` (`lib/local-storage.ts:380-431`), `fallbackGroupPosition` (`dockview-layout-builders.ts:272`), `isCenterCandidateGroupId` (`layout-manager/applier.ts`), `setupPortalCleanup` (`dockview-layout-setup.ts:488-502`).

## Dependencies and Risks

- None before this task.
- Risk: the floated-id set must clear on every settle path (success, error, unmount) or user closes silently stop releasing portals.
- Risk: materialization must reuse `applyLayout`/`fallbackGroupPosition` machinery, not a hand-rolled group path.
- Risk: `handleMaximizeExitOnLastClose` runs inside `onDidRemovePanel`; floating the last grid group must not trigger maximize exit.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
