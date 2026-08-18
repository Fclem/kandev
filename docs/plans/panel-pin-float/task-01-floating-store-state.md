---
id: "01-floating-store-state"
title: "Floating-group store, detach registry, materializer, and persistence"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 01: Floating-group store, detach registry, materializer, and persistence

## Acceptance

- `useDockviewStore` exposes `floatingGroups: Record<string, FloatingGroupState>` (groupId, columnId, columnIndex, columnKind, columnPinned, treePath, edge, orientation, size, panels as full `FloatingPanelDef[]`, activePanelId, display, order) and actions `floatGroup(groupId)`, `dockGroup(groupId)`, `setFloatingDisplay(groupId, display)`, `setFloatingActivePanel(groupId, panelId)`.
- **Placement classifier:** a pure function over the live `LayoutState` (root column id/index/pinned metadata + live `centerGroupId`) classifies each group as side/vertical (non-center root column, edge by index relative to center) or center/horizontal (center column, bottom edge), with the documented custom fallback. It MUST NOT use `isCenterCandidateGroupId` as the decision; plan/preview/vscode/compact root columns classify as side/vertical on the right edge. Covered by tests for all five presets plus a nested custom layout.
- **Materializer:** re-dock/restore recreate groups by cloning the live layout, inserting the saved root column at `columnIndex` with pinned/width metadata when absent, inserting the group at the saved `treePath` (creating branch nodes), and applying through the existing serializer/`applyLayoutAndSet` machinery. `fallbackGroupPosition` is the explicit existing-group fallback only. Docking the last group in an emptied side column recreates the side column, never a center fallback; missing-column and no-center cases are tested.
- **Detach registry:** `floatingDetachRegistry: Map<envId, Map<panelId, transactionToken>>`. `setupPortalCleanup`'s `onDidRemovePanel` skips `panelPortalManager.release`, terminal park/stop, vscode stop, and `handleMaximizeExitOnLastClose` only for registered ids with the current token, then consumes the registration. Unregistered removals (including a user closing a floated tab mid-transaction) run full cleanup; the existing `isRestoringLayout` early return keeps its precedence. `panelPortalManager.reconcile`/`releaseByEnv` take an explicit per-env exclusion set. A stale token never clears a newer registration; the env registry clears on settle (success, failure, unmount).
- **Transactions with journal:** float/dock/restore capture+validate → mutate → commit (store + blob + `persistEnvLayoutNow`); on a mid-mutation throw, re-apply the journaled LayoutState, drop the partial entry, and keep the previous blob. Suppression cleanup runs in `finally`, token-guarded. Storage-write failure is non-fatal (in-memory authoritative).
- **Persistence:** versioned `EnvFloatingState` blob under `kandev.dockview.env-floating.<envId>` with a type guard (mirroring `isEnvMaximizeState`) that validates each panel definition independently (JSON-safe values, params ≤ 64 KB serialized), drops invalid entries, defaults to `{}`; `nextOrder` monotonic counter; saved on env switch (`saveOutgoingEnv`), restored after layout restore, cleared by reset and `cleanupTaskStorage`.
- Width/height sync (`syncPinnedWidthsFromApi`, `enforcePinnedTargets`) does not re-insert or corrupt pinned widths while a group is floating.

## Verification

```bash
cd apps/web && pnpm vitest run lib/state/dockview-floating-store.test.ts lib/layout/panel-portal-manager.test.ts lib/local-storage.test.ts lib/state/dockview-store.test.ts lib/state/dockview-pinned-enforce.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/state/dockview-store.ts` (state, actions, detach registry, env-switch save/restore, reset, maximize interplay, width-sync guard)
- `apps/web/lib/state/dockview-floating.ts` (new: placement classifier, materializer, `restoreFloatingAfterLayout`, journal helpers, `EnvFloatingState` type guard)
- `apps/web/lib/local-storage.ts` (`DOCKVIEW_ENV_FLOATING_PREFIX`, get/set/remove, `cleanupTaskStorage`)
- `apps/web/components/task/dockview-layout-setup.ts` (detach-registry checks in `setupPortalCleanup`)
- `apps/web/lib/layout/panel-portal-manager.ts` (reconcile/releaseByEnv exclusion-set param)
- `apps/web/lib/state/dockview-floating-store.test.ts` (new), `apps/web/lib/layout/panel-portal-manager.test.ts` (new), existing store/pinned-enforce/local-storage tests

## Inputs

- Spec: Data model, Placement capture, State machine (transactions), Materializer, Non-destructive removal contract, Failure modes, Persistence guarantees.
- Existing patterns: `buildMaximizeActions` (`dockview-store.ts:934-1000`), `saveOutgoingEnv` (`:779-831`), `isEnvMaximizeState` (`lib/local-storage.ts:380-431`), `fallbackGroupPosition` (`dockview-layout-builders.ts:272`), `findCenterGroupId` (`layout-manager/applier.ts`), `setupPortalCleanup` (`dockview-layout-setup.ts:488-502`), `toggleRightPanels` (`dockview-store.ts:523-567`).

## Dependencies and Risks

- None before this task.
- Risk: detach-registry leak — a registered id never removed (aborted transaction) must still be cleared on settle or the next real close of that tab skips cleanup; test the abort path.
- Risk: materializer must go through the existing serializer/`applyLayoutAndSet`, or dockview's tree invariants silently break.
- Risk: `handleMaximizeExitOnLastClose` runs inside `onDidRemovePanel`; floating the last grid group must not trigger maximize exit.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
