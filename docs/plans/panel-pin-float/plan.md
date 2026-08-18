# Plan: Floating (unpinned) workbench panels

Spec: [`docs/specs/ui/panel-pin-float.md`](../../specs/ui/panel-pin-float.md)

## Summary

Add a per-group pin toggle to the dockview workbench group headers (left of the
maximize control, message-queue pin icons). Unpinning floats the group over
the workbench; it collapses to an edge title bar when unfocused and re-docks
on pin click. State persists per task environment in sessionStorage, mirroring
the existing env layout / maximize persistence. Revision 5 incorporates the
round-4 adversarial review (this package has been adversarially reviewed every
round; see `docs/specs/ui/panel-pin-float.md` history): unified
`canPersistLayout` gating incl. `beforeunload`, `useAutoSessionTab`
coordination, pending-collapse refcounts/generations, fail-closed storage
commits, reset collision precedence, floating-aware `toggleRightPanels`, and
an order-exhaustion policy.

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
2. **Detach registry, not a global id set.** The registry is keyed by panel id
   with records `{ envId, token }` (panel ids are unique in the live grid; the
   env tag travels with the record, so lookups never depend on the mutable
   current store env). `setupPortalCleanup`'s `onDidRemovePanel` decision
   order: (1) registered id with the current token → consume the registration
   and skip close side effects, regardless of `isRestoringLayout`; (2)
   unregistered while `isRestoringLayout` → return (existing behavior); (3)
   unregistered otherwise → full cleanup (a user closing a floated tab
   mid-transaction is an ordinary close). `panelPortalManager.reconcile`/
   `releaseByEnv` take an explicit exclusion set derived from the **target
   env's** registered ids. Stale tokens never clear newer registrations; the
   env's registry clears on transaction settle (success/failure/unmount).
3. **Placement classifier + materializer over LayoutState.** Dockview exposes
   no left/right/top/bottom group location, and `isCenterCandidateGroupId`
   misclassifies plan/preview/custom columns (it returns true for every group
   except three constants — `layout-manager/applier.ts:37-39`). Classification
   is a pure function over the live `LayoutState` (root column id/index/
   pinned metadata) plus an **explicit, nullable center identity**
   (`centerColumnId, isCenterKnown`) — `findCenterGroupId` fabricates ids when
   no center exists (`applier.ts:45-55`), so an unknown center classifies as
   the documented custom fallback, never by promoting an arbitrary side
   column. Plan/preview/vscode/compact root columns classify as side/vertical
   on their edge. Re-dock/restore materialize missing columns/groups by
   cloning the live layout, inserting the saved column at `columnIndex` with
   metadata, inserting the group **atomically into both `column.tree` and
   `column.groups`** (one mutation helper — `serializeColumn` prefers `tree`
   while `serializePanels` iterates only `groups`,
   `layout-manager/serializer.ts`), and applying through the existing
   serializer/`applyLayoutAndSet` machinery. `fallbackGroupPosition`
   (`dockview-layout-builders.ts:272`) is the explicit existing-group fallback
   only, never the column-creation mechanism.
4. **Transactions with journal + unified persistence guard.** Every float/dock/
   restore: capture+validate → mutate → commit (store + blob +
   `persistEnvLayoutNow`) → on throw mid-mutation, re-apply the journaled
   LayoutState and drop the partial entry. A store-owned transaction token
   (`floatingTransactionToken` with token-guarded begin/end actions; read by
   `setupLayoutPersistence`, so no module-local flag and no import cycle)
   gates every layout-persistence entry point through one `canPersistLayout()`
   guard: `persistNow`, the debounce callback, the `onDidLayoutChange`
   handler, and the `beforeunload` flush; on transaction begin an already
   scheduled timer is cancelled/held and marked dirty. Persistence runs only
   after commit or journal recovery. **Storage commits fail closed:** if the
   floating blob cannot be written, the transaction rolls back and the group
   stays pinned (worst case: the pin did not stick — never a lost panel).
   Suppression cleanup in `finally`, token-guarded; token reset on unmount.
5. **Owned-region focus with refcounted pending collapse.** One module-level
   coordinator (pattern: `hooks/use-panel-search.ts`) with owned regions =
   floating window subtree + any Radix layer opened from within it
   (`useFloatingOwnedLayer` hook wired to the existing `onOpenChange`
   handlers; idempotent unregister also runs on React cleanup, so a layer
   closed via unmount/navigation still decrements). Collapse on:
   window-capture pointerdown outside all owned regions (**deferred while the
   window's owned-layer refcount is above zero**: pending collapse applied at
   refcount zero, stored with its event generation, cleared on window unmount,
   cancelled if the pointerdown landed inside an owned region); `focusout` to
   outside (relatedTarget after a microtask); Escape on the **bubble** phase
   honoring `event.defaultPrevented` (Radix dismissable layers win). Only the
   focused/last-interacted expanded window collapses.
6. **Reset is an id-aware docking merge with collision precedence.** The reset
   layout owns group/column placement; the floating definition owns the panel
   payload (component, params, tabComponent) and saved tab order; the active
   panel is merged explicitly. Reset-default panels are reused by id (never
   duplicated); floating payloads win (a floated ordinary terminal keeps its
   real terminal id); floating state clears only after the merged grid is
   committed; groups do not re-float after reset.
7. **Restore call sites (exhaustive).** `restoreFloatingAfterLayout`
   (idempotent) runs before `isRestoringLayout` clears at: initial mount
   `restoreEnvLayout` (all three branches: saved env layout /
   `tryRestoreMaximizeOnly` / default+route-intent — `dockview-layout-restore.ts`),
   env-switch fast and slow paths (`dockview-env-switch.ts`), maximize restore,
   preset/custom apply, `toggleRightPanels` (`dockview-store.ts:523-567`), and
   reset/default build. One focused test per call site.
8. **Identity coordination (session + right panels).** Session replacement is
   one coordinator over grid + floating entries **including the always-mounted
   `useAutoSessionTab` hook** (`dockview-desktop-layout.tsx` →
   `ensureSessionPanel`): a floating winner is visible to the hook's
   ensure/reconcile guard so the incoming id is never re-added to the grid; a
   grid winner drops the floating stale copy. The `toggleRightPanels` show
   path is floating-aware (floating ids excluded from the re-added default
   right column, or the toggle docks them explicitly); `rightPanelsVisible`
   is tracked independently of floating right groups. `enforcePinnedTargets`
   restores the right target only when a live pinned-right column exists
   (with all right groups floated, `sv.length - 1` is the center column —
   `dockview-pinned-enforce.ts`). No panel id ever exists in both surfaces.

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
  nested custom + plan-preset side classification + no-center/unknown-center
  custom fallback), materializer (missing column, no-center, tree-path insert,
  tree+flat round-trip with an existing nested tree), maximize→float ordering,
  last-group-in-column re-dock, transaction journal recovery (throw-on-remove,
  throw-on-materialize, storage-write failure → fail-closed rollback to
  pinned, timer scheduled before transaction start, unload during
  transaction), display/active setters, persistence round-trip + type-guard
  rejection (undefined/cyclic/oversized params, negative/oversized numerics,
  duplicate ids, order exhaustion at 9 999/10 000/next), env save/restore,
  reset-as-merge (payload-wins collisions incl. floated ordinary terminal),
  stacking allocation.
- `apps/web/components/task/dockview-floating-panel.test.tsx` (new) — expanded
  window, owned-region collapse (outside pointer, focusout, portaled Radix
  layer open = owned), pointerdown deferral (outside click while a menu is
  open stays expanded until dismissal; pending collapse cancelled on
  owned-region click; layer closed via unmount/navigation decrements; two
  layers closing in both orders; successor window inherits no stale pending
  collapse), Escape rules (defaultPrevented wins; nested
  AlertDialog/DropdownMenu), vertical/horizontal bar, tablist semantics +
  roving tabindex + Arrow/Home/End, title-click expand+activate, pin re-dock,
  stacking/offsets, empty-group removal, reactive title update on plugin
  re-registration while detached.
- `apps/web/components/task/dockview-group-actions.test.tsx` — pin button
  placement/aria/tooltip/click.
- `apps/web/lib/layout/panel-portal-manager.test.ts` — detach-vs-close:
  registered ids survive removePanel + reconcile (synchronous and delayed
  `fromJSON` removals); unregistered close releases; same id across envs;
  exclusion-set semantics.
- `apps/web/lib/state/dockview-env-switch.test.ts` — floating session
  replacement (single stale, multi-stale winner rule, delayed replacement,
  active-tab rewrite, winner-floats-only suppresses grid insertion — no id in
  both surfaces, winner-in-grid drops floating copy), fast + slow path
  restore; **through the real desktop hook**: `useAutoSessionTab` skips
  ensure for a floating winner (tested via `dockview-desktop-layout`-level
  integration, not only direct `replaceStaleSessionPanels` calls).
- `apps/web/lib/state/dockview-panel-actions.test.ts` — duplicate prevention
  for plan/terminal/preview/review/plugin/session actions with floated panels.
- `apps/web/lib/state/dockview-pinned-enforce.test.ts` — right target not
  applied when all right groups float (one and both floated, toggle-right-
  panels hide→show while floating — floating ids excluded from the re-added
  column — and container resize).
- `apps/web/lib/local-storage.test.ts` — floating key helpers + guard +
  `cleanupTaskStorage` + fail-closed rollback on write failure.
- `apps/web/e2e/tests/task/panel-pin.spec.ts` (new, desktop) — full matrix:
  float/collapse/expand/dock, reload recreation, plan-preset orientation,
  maximize→float, task switch with floated chat, terminal liveness, keyboard
  collapse, portaled-menu collapse suppression, two groups on one edge,
  reset-merge, toggle-right-panels with floated right groups, storage-write
  failure (group stays pinned).

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

- **Registry consume-once:** consuming a registration for a `fromJSON` removal
  during restore must not break `reconcile`'s post-restore behavior or a float
  whose removal never fires; settle-clear must still run.
- **Persistence guard completeness:** the transaction token must gate
  `persistNow`, the debounce callback, the event handler, AND `beforeunload`,
  and cancel/hold an already-scheduled timer — one missed entry point
  re-exposes the partial-layout persistence the round-4 review found.
- **Session identity:** a floating winner must be visible to `useAutoSessionTab`
  before its ensure effect runs, or the incoming session id lands in both
  surfaces; the toggle-show path must not re-add floating right ids.
- **Pointerdown deferral cancellation:** pending collapse must be refcounted
  (React cleanup + dismiss), generation-tagged, and cleared on unmount, or a
  stale pending collapse collapses a successor window.
- **Detach registry leaks:** if a registered id is never removed from the grid
  (transaction aborted before removal), the registration must still be cleared
  on settle or the next real close of that tab skips cleanup. Consume-once +
  settle-clear are both required; test the abort path.
- **Materializer divergence:** the delta-LayoutState insert must update both
  `column.tree` and `column.groups` atomically and go through the existing
  serializer/`applyLayoutAndSet`, or dockview's tree invariants (alternating
  branch orientation, pinned columns) silently break and `serializePanels`
  drops the group's definitions.
- **Enforcement gate:** missing the live-pinned-right-column check lets
  `enforcePinnedTargets` resize the center to the right target when all right
  groups float; the gate and its tests are mandatory.
- **Restore ordering:** one missed completion point (e.g. `toggleRightPanels`
  or the initial maximize-only branch) re-exposes the reload blocker; the
  call-site table in the spec is the checklist and each entry has a test.
- **Maximize interplay:** float of the maximized group must derive placement
  from `preMaximizeLayout` and sequence removals so the trailing restore rAF
  does not reassert the overlay.
- **Owned-region focus:** Radix layers portaled to `body` must register with
  the coordinator or the window collapses under an open menu; the
  `useFloatingOwnedLayer` hook is mandatory, and pointerdown must be deferred
  while a layer is open (Radix's own outside handler runs after window
  capture).
