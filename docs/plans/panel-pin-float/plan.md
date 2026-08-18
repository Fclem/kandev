# Plan: Floating (unpinned) workbench panels

Spec: [`docs/specs/ui/panel-pin-float.md`](../../specs/ui/panel-pin-float.md)

## Summary

Add a per-group pin toggle to the dockview workbench group headers (left of the
maximize control, message-queue pin icons). Unpinning floats the group over
the workbench; it collapses to an edge title bar when unfocused and re-docks
on pin click. State persists per task environment in sessionStorage, mirroring
the existing env layout / maximize persistence. Revision 3 incorporates
round-2 adversarial review: a LayoutState-based placement classifier +
materializer, an env-scoped detach registry, owned-region focus handling,
transaction journaling, an explicit restore call-site table, and reset-as-
docking.

## Architecture

Panel content already lives outside dockview: `PanelPortalHost` renders every
registered panel into persistent portal elements owned by `panelPortalManager`
(`apps/web/lib/layout/panel-portal-manager.ts`); dockview wrappers adopt/release
those elements via `usePortalSlot`. **However, normal panel removal destroys
the portal**: `setupPortalCleanup`'s `onDidRemovePanel` handler
(`apps/web/components/task/dockview-layout-setup.ts:488-502`) calls
`panelPortalManager.release(panel.id)`, parks/stops terminals, stops vscode,
and runs `handleMaximizeExitOnLastClose`. Float therefore needs the explicit
non-destructive detach registry (below), not bare `api.removePanel`.

Maximize is store-driven (`maximizeGroup`/`exitMaximizedLayout` in
`apps/web/lib/state/dockview-store.ts:934-1000`), persisting a
`preMaximizeLayout` LayoutState per env. Floating reuses the same store +
per-env sessionStorage pattern.

### Key invariants

1. **Live env layout always.** The persisted env layout reflects the live
   grid (floated groups absent), unchanged from today. The floating blob
   (`kandev.dockview.env-floating.<envId>`, versioned + type-guarded like
   `isEnvMaximizeState`) carries complete `FloatingPanelDef`s (id, component,
   title, tabComponent, params) + placement metadata (columnId, columnIndex,
   columnKind, columnPinned, treePath, edge, orientation, size, order,
   display), so floated groups can be **materialized** after any reload/env
   switch/layout rebuild and re-floated.
2. **Detach registry, not a global id set.** `floatingDetachRegistry:
   Map<envId, Map<panelId, transactionToken>>` in the store. `setupPortalCleanup`
   skips close side effects only for registered ids with the **current**
   token, then consumes the registration (one expected removal per panel).
   Ordinary closes — including a user closing a floated tab mid-transaction —
   run the full cleanup path. `panelPortalManager.reconcile`/`releaseByEnv`
   take an explicit per-env exclusion set, so same ids in other envs still
   release. Stale tokens never clear newer registrations; the env's registry
   clears on transaction settle (success/failure/unmount).
3. **Placement classifier + materializer over LayoutState.** Dockview exposes
   no left/right/top/bottom group location, and `isCenterCandidateGroupId`
   misclassifies plan/preview/custom columns (it returns true for every group
   except three constants — `layout-manager/applier.ts:37-39`). Classification
   is a pure function over the live `LayoutState` (root column id/index/
   pinned metadata + live `centerGroupId`); plan/preview/vscode/compact root
   columns classify as side/vertical on their edge. Re-dock/restore
   materialize missing columns/groups by cloning the live layout, inserting
   the saved column at `columnIndex` with metadata, inserting the group at the
   saved `treePath` (creating branch nodes), and applying through the existing
   serializer/`applyLayoutAndSet` machinery. `fallbackGroupPosition`
   (`dockview-layout-builders.ts:272`) is the explicit existing-group fallback
   only, never the column-creation mechanism.
4. **Transactions with journal.** Every float/dock/restore: capture+validate
   → mutate → commit (store + blob + `persistEnvLayoutNow`) → on throw
   mid-mutation, re-apply the journaled LayoutState and drop the partial
   entry. Suppression cleanup in `finally`, token-guarded. Storage failure is
   non-fatal (in-memory authoritative, ephemeral).
5. **Owned-region focus.** One module-level coordinator (pattern:
   `hooks/use-panel-search.ts`) with owned regions = floating window subtree +
   any Radix layer opened from within it (`useFloatingOwnedLayer` hook wired to
   the existing `onOpenChange` handlers). Collapse on: window-capture
   pointerdown outside all owned regions; `focusout` to outside (relatedTarget
   checked after a microtask for Radix portals); Escape on the **bubble**
   phase honoring `event.defaultPrevented` (Radix dismissable layers win).
   Only the focused/last-interacted expanded window collapses.
6. **Reset docks.** Reset materializes every floating group into the reset
   target grid (side → right column, center → center column, fallback
   center), clears floating memory+storage only after the grid contains them,
   persists; groups do not re-float after reset.
7. **Restore call sites (exhaustive).** `restoreFloatingAfterLayout`
   (idempotent) runs before `isRestoringLayout` clears at: initial mount
   `restoreEnvLayout` (all three branches: saved env layout /
   `tryRestoreMaximizeOnly` / default+route-intent — `dockview-layout-restore.ts`),
   env-switch fast and slow paths (`dockview-env-switch.ts`), maximize restore,
   preset/custom apply, `toggleRightPanels` (`dockview-store.ts:523-567`), and
   reset/default build. One focused test per call site.

## Files

### Likely touched

- `apps/web/components/task/dockview-group-actions.tsx` — `PinButton` +
  placement in `GroupSplitCloseActionsView` (left of `MaximizeButton`).
- `apps/web/components/task/dockview-header-actions.tsx` — wire pin state +
  `floatGroup`/`dockGroup` into the shared `GroupSplitCloseActions`.
- `apps/web/components/task/dockview-floating-panel.tsx` (new) — floating
  window + collapsed edge bar overlay, rendered inside `DockviewDesktopLayout`
  root; adopts `panelPortalManager` elements; tablist/tab semantics; stacking
  by `(order, groupId)`.
- `apps/web/components/task/dockview-floating-coordinator.ts` (new) —
  module-level owned-region focus/outside-pointer/Escape ownership +
  `useFloatingOwnedLayer`.
- `apps/web/components/task/dockview-desktop-layout.tsx` — mount the floating
  overlay; wire `restoreFloatingAfterLayout` at ready.
- `apps/web/lib/state/dockview-store.ts` — `floatingGroups`, detach registry,
  transaction actions (`floatGroup`, `dockGroup`, `setFloatingDisplay`,
  `setFloatingActivePanel`), env-switch save/restore, reset-as-docking,
  maximize interplay.
- `apps/web/lib/state/dockview-floating.ts` (new) — placement classifier,
  materializer, `restoreFloatingAfterLayout`, journal helpers, versioned
  `EnvFloatingState` type guard (JSON-safe, 64 KB/def params bound), cleanup.
- `apps/web/components/task/dockview-layout-setup.ts` — detach-registry
  checks in `setupPortalCleanup`; restore completion points.
- `apps/web/lib/state/dockview-env-switch.ts` — session replacement returns an
  old→new mapping applied to floating entries (winner rule); restore
  invocation on fast and slow paths.
- `apps/web/lib/state/dockview-layout-builders.ts` — `focusOrAddPanel` becomes
  `focusOrAddFloatingOrGridPanel`.
- `apps/web/lib/state/dockview-panel-actions.ts`,
  `dockview-terminal-panel-actions.ts` — route every single-instance/add
  action through the resolver.
- `apps/web/lib/layout/panel-portal-manager.ts` — `reconcile`/`releaseByEnv`
  accept an exclusion set.
- `apps/web/lib/local-storage.ts` — `DOCKVIEW_ENV_FLOATING_PREFIX`, get/set/
  remove helpers + guard; `cleanupTaskStorage` removes the key.
- Locales: `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
  — `pinPanel` / `unpinPanel` keys.

### Tests

- `apps/web/lib/state/dockview-floating-store.test.ts` (new) — float/dock
  transitions, placement classifier (default/compact/plan/preview/vscode +
  nested custom + plan-preset side classification), materializer (missing
  column, no-center, tree-path insert), maximize→float ordering, last-group-
  in-column re-dock, transaction journal recovery (throw-on-remove,
  throw-on-materialize, storage failure), display/active setters, persistence
  round-trip + type-guard rejection (undefined/cyclic/oversized params),
  env save/restore, reset-as-docking, stacking allocation.
- `apps/web/components/task/dockview-floating-panel.test.tsx` (new) — expanded
  window, owned-region collapse (outside pointer, focusout, portaled Radix
  layer open = owned), Escape rules (defaultPrevented wins; nested
  AlertDialog/DropdownMenu), vertical/horizontal bar, tablist semantics +
  roving tabindex + Arrow/Home/End, title-click expand+activate, pin re-dock,
  stacking/offsets, empty-group removal.
- `apps/web/components/task/dockview-group-actions.test.tsx` — pin button
  placement/aria/tooltip/click.
- `apps/web/lib/layout/panel-portal-manager.test.ts` — detach-vs-close:
  registered ids survive removePanel + reconcile; unregistered close releases;
  same id across envs; exclusion-set semantics.
- `apps/web/lib/state/dockview-env-switch.test.ts` — floating session
  replacement (single stale, multi-stale winner rule, delayed replacement,
  active-tab rewrite), fast + slow path restore.
- `apps/web/lib/state/dockview-panel-actions.test.ts` — duplicate prevention
  for plan/terminal/preview/review/plugin/session actions with floated panels.
- `apps/web/lib/local-storage.test.ts` — floating key helpers + guard +
  `cleanupTaskStorage`.
- `apps/web/e2e/tests/task/panel-pin.spec.ts` (new, desktop) — full matrix:
  float/collapse/expand/dock, reload recreation, plan-preset orientation,
  maximize→float, task switch with floated chat, terminal liveness, keyboard
  collapse, portaled-menu collapse suppression, two groups on one edge,
  reset docks.

## Dependencies

- Store + persistence + detach registry + materializer (`task-01`) before UI
  (`task-02`, `task-03`).
- `task-03` (floating window + coordinator) before integration (`task-04`).
- E2E last (`task-05`).

## Verification

Per task: targeted unit runs (`cd apps/web && pnpm vitest run <file>`),
`pnpm run i18n:check` / `pnpm run i18n:zh-hant` after locale edits, then the
full gate: `make fmt` → `make typecheck` → `make test` → `make lint`. E2E:
`cd apps/web && pnpm e2e:raw tests/task/panel-pin.spec.ts` (mock profile,
`KANDEV_E2E_MOCK=true`).

## Risks

- **Detach registry leaks:** if a registered id is never removed from the grid
  (transaction aborted before removal), the registration must still be cleared
  on settle or the next real close of that tab skips cleanup. Consume-once +
  settle-clear are both required; test the abort path.
- **Materializer divergence:** the delta-LayoutState insert must go through
  the existing serializer/`applyLayoutAndSet`, or dockview's tree invariants
  (alternating branch orientation, pinned columns) silently break.
- **Restore ordering:** one missed completion point (e.g. `toggleRightPanels`
  or the initial maximize-only branch) re-exposes the reload blocker; the
  call-site table in the spec is the checklist and each entry has a test.
- **Maximize interplay:** float of the maximized group must derive placement
  from `preMaximizeLayout` and sequence removals so the trailing restore rAF
  does not reassert the overlay.
- **Owned-region focus:** Radix layers portaled to `body` must register with
  the coordinator or the window collapses under an open menu; the
  `useFloatingOwnedLayer` hook is mandatory, not optional.
