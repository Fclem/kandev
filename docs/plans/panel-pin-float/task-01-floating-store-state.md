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
- **Transactions: one coordinator + digest journal + phase model:** a single coordinator (`floating-transaction.ts`) owns the per-env operation/generation and phase state (`begin → advance → settle`); while busy, **every public layout-mutation boundary** (float/dock/reset/toggle, the add-panel resolver, `buildDefaultLayout`, preset/custom apply, maximize/exit, programmatic actions) **and restore/recovery paths** (`recoverFloatingJournalOnce`, `restoreFloatingAfterLayout` enter the same coordinator — a restore completion during a busy phase is skipped with a retained pending-floating-restore marker, never executed from a stale snapshot) **reject/skip** non-destructively, and all three pin surfaces (grid header, floating header, collapsed bar) render disabled via `isFloatingTransactionBusy(envId)`; re-entrancy tests cover mutation, portals-adopted, rollback, and stale-cleanup phases, per surface and per programmatic action/restore entry. A per-env **operation journal** (`kandev.dockview.env-floating-journal.<envId>`) is written **before** mutation holding `{envId, transactionId, phase, before/after digests, raw before/after strings}`. **Digest protocol:** values are serialized once; digests are SHA-256 (`version: 1`) over the exact raw bytes written to sessionStorage (never a re-stringified parse), with an explicit absent sentinel; journal writes are status-returning and read-back verified, and the write/verify ordering is journal → blob → layout → phase → read-back verify → clear, with a specified phase-write-throw-after-mutation recovery. **`recoverFloatingJournalOnce(api, envId)`** uses digest comparison (four partial-write orderings + **no-op equality = settled**), with a recovery cache keyed by `(envId, transactionId, api instance)` — env-isolated, never one global generation. **Size budgets:** per-env cap (96 KB) and a **global floating allocation budget** (384 KB) enforced through a **validated owned-key index** (built on load/recovery, updated on budget-changing writes — no full prefix scan per toggle); quota races after a passing preflight fail closed via the journal rollback (the backstop). Recovery runs the phase model `mutating → restoring → portals-adopted → persist-recovered → settled`; only the portals-adopted phase persists the journaled layout through **status-returning APIs**; token cleanup is generation-guarded at settled. Journal-free restore is idempotent with **per-panel salvage** and **`allocateUniqueGroupId`** (saved id reused only when absent/owned by the exact group; otherwise `group-floating-<n>` from a dedicated namespace reserved against `api.groups` + same-operation allocations, attempt cap 64 — exhaustion fails non-destructively; saved→live mapping re-derived from saved ids against live ids on every reload). A store-owned transaction token gates every persistence entry point through one `canPersistLayout()`; the unload path is **one idempotent transaction-aware handler** writing the journaled pre-transaction layout. **Single storage policy: fail closed** on every blob-write failure — rollback to pinned; no ephemeral floating mode.
- **Persistence:** versioned `EnvFloatingState` blob under `kandev.dockview.env-floating.<envId>` with a type guard (mirroring `isEnvMaximizeState`) that validates each panel definition independently (JSON-safe values, params ≤ 64 KB serialized, depth-bounded recursive validation), enforces finite nonnegative numeric bounds (`order`/`nextOrder` ≤ 10 000, `size` ≤ 100 000, `columnIndex` ≤ 64, `columnWidth`/`columnMinWidth`/`columnMaxWidth` ≤ 100 000 with min ≤ width ≤ max when both present), **normalizes `nextOrder` on load to max(raw nextOrder, max(accepted group orders) + 1)** — the persisted high-water counter is preserved even when the highest-order group was removed (no accepted groups never resets to 1); exceeding the cap makes allocation fail non-destructively — rejects duplicate panel/group ids deterministically (first wins), defaults to `{}`; `nextOrder` monotonic counter with a documented **exhaustion policy** (at the cap, a new float fails non-destructively and the group stays pinned — no clamping or reuse); saved on env switch (`saveOutgoingEnv`), restored after layout restore, cleared by reset and `cleanupTaskStorage`.
- Width/height sync (`syncPinnedWidthsFromApi`, `enforcePinnedTargets`) does not re-insert or corrupt pinned widths while a group is floating.

## Verification

```bash
cd apps/web && pnpm vitest run lib/state/dockview-floating-store.test.ts lib/layout/panel-portal-manager.test.ts lib/local-storage.test.ts lib/state/dockview-store.test.ts lib/state/dockview-pinned-enforce.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/state/dockview-store.ts` (state, actions, detach registry, transaction token + begin/end, env-switch save/restore, reset-as-merge, maximize interplay, width-sync guard)
- `apps/web/lib/state/floating-transaction.ts` (new: transaction coordinator — per-env operation/generation, begin/advance/settle, busy-rejection, journal write/recovery, phase model)
- `apps/web/lib/state/dockview-floating.ts` (new: placement classifier, materializer, `restoreFloatingAfterLayout`, `recoverFloatingJournalOnce`, journal helpers, `EnvFloatingState` type guard, order allocation, size-budget preflight)
- `apps/web/lib/local-storage.ts` (`DOCKVIEW_ENV_FLOATING_PREFIX`, get/set/remove, `cleanupTaskStorage`)
- `apps/web/components/task/dockview-layout-setup.ts` (detach-registry checks in `setupPortalCleanup`; `canPersistLayout` gate in `setupLayoutPersistence` incl. `beforeunload`)
- `apps/web/lib/layout/panel-portal-manager.ts` (reconcile/releaseByEnv exclusion-set param)
- `apps/web/lib/state/layout-manager/types.ts` + `serializer.ts` (root-column geometry capture/update: pinned/width/min/max retention through apply/restore; `captureRootColumnGeometry`/`applyRootColumnGeometry`)
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
