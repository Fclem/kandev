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
- **Placement classifier:** a pure function over the live `LayoutState` (root column id/index/pinned metadata) plus an **explicit nullable center identity** (`centerColumnId, isCenterKnown` — `findCenterGroupId` fabricates ids when no center exists, so an unknown center classifies as the documented custom fallback, never by promoting an arbitrary side column) classifies each group as side/vertical (non-center root column, edge by index relative to center) or center/horizontal (center column, bottom edge). It MUST NOT use `isCenterCandidateGroupId` as the decision; plan/preview/vscode/compact root columns classify as side/vertical on the right edge. Covered by tests for all five presets, a nested custom layout, and no-center/unknown-center cases.
- **Materializer:** re-dock/restore recreate groups by cloning the live layout, inserting the saved root column at `columnIndex` with pinned/width metadata when absent, inserting the group **atomically into both `column.tree` and `column.groups`** (one mutation helper; `serializeColumn` prefers `tree` while `serializePanels` iterates only `groups`, so a tree-only insert loses panel definitions), and applying through the existing serializer/`applyLayoutAndSet` machinery. `fallbackGroupPosition` is the explicit existing-group fallback only. Docking the last group in an emptied side column recreates the side column, never a center fallback; missing-column, no-center, and tree+flat round-trip cases are tested.
- **Detach registry:** keyed by panel id with records `{ envId, token }` (env tag travels with the record; lookups never depend on the mutable current store env). `setupPortalCleanup`'s `onDidRemovePanel` decision order: (1) registered id with the current token → consume the registration and skip `release`, terminal park/stop, vscode stop, and `handleMaximizeExitOnLastClose`, regardless of `isRestoringLayout`; (2) unregistered while `isRestoringLayout` → return (today's behavior); (3) unregistered otherwise → full cleanup (a user closing a floated tab mid-transaction is an ordinary close). `panelPortalManager.reconcile`/`releaseByEnv` take an explicit exclusion set derived from the target env's registered ids. A stale token never clears a newer registration; the env registry clears on settle (success, failure, unmount).
- **Transactions with journal + persistence suppression:** float/dock/restore capture+validate → mutate → commit (store + blob + `persistEnvLayoutNow`); on a mid-mutation throw, re-apply the journaled LayoutState, drop the partial entry, and keep the previous blob. A transaction-scoped suppression flag (distinct from `isRestoringLayout`) defers the debounced layout auto-save and `trackPinnedWidths` capture for the transaction's duration (float removals fire layout-change events that would otherwise persist a partial grid before commit); persistence runs only after commit or journal recovery. Suppression cleanup runs in `finally`, token-guarded. Storage-write failure is non-fatal (in-memory authoritative).
- **Persistence:** versioned `EnvFloatingState` blob under `kandev.dockview.env-floating.<envId>` with a type guard (mirroring `isEnvMaximizeState`) that validates each panel definition independently (JSON-safe values, params ≤ 64 KB serialized, depth-bounded recursive validation), enforces finite nonnegative numeric bounds (`order`/`nextOrder` ≤ 10 000, `size` ≤ 100 000, `columnIndex` ≤ 64), rejects duplicate panel/group ids deterministically (first wins), defaults to `{}`; `nextOrder` monotonic counter; saved on env switch (`saveOutgoingEnv`), restored after layout restore, cleared by reset and `cleanupTaskStorage`.
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
