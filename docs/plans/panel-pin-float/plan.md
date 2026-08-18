# Plan: Floating (unpinned) workbench panels

Spec: [`docs/specs/ui/panel-pin-float.md`](../../specs/ui/panel-pin-float.md)

## Summary

Add a per-group pin toggle to the dockview workbench group headers (left of the
maximize control, message-queue pin icons). Unpinning floats the group over
the workbench; it collapses to an edge title bar when unfocused and re-docks
on pin click. State persists per task environment in sessionStorage, mirroring
the existing env layout / maximize persistence. Revision 2 incorporates the
adversarial review: non-destructive float removals, full panel definitions +
placement metadata in the blob (materialization), a single floating
coordinator for focus/Escape, and explicit persistence after transactions.

## Architecture

Panel content already lives outside dockview: `PanelPortalHost` renders every
registered panel into persistent portal elements owned by `panelPortalManager`
(`apps/web/lib/layout/panel-portal-manager.ts`); dockview wrappers adopt/release
those elements via `usePortalSlot`. **However, normal panel removal destroys
the portal**: `setupPortalCleanup`'s `onDidRemovePanel` handler
(`apps/web/components/task/dockview-layout-setup.ts:488-502`) calls
`panelPortalManager.release(panel.id)`, parks/stops terminals, stops vscode,
and runs `handleMaximizeExitOnLastClose`. Float therefore needs an explicit
non-destructive path (see below), not bare `api.removePanel`.

Maximize is store-driven (`maximizeGroup`/`exitMaximizedLayout` in
`apps/web/lib/state/dockview-store.ts:934-1000`), persisting a
`preMaximizeLayout` LayoutState per env. Floating reuses the same store +
per-env sessionStorage pattern.

### Key invariants

1. **Live env layout always.** The persisted env layout reflects the live
   grid (floated groups absent), unchanged from today. The floating blob
   (`kandev.dockview.env-floating.<envId>`, versioned + type-guarded like
   `isEnvMaximizeState`) carries complete `FloatingPanelDef`s (id, component,
   title, tabComponent, params) + placement metadata (columnId, columnKind,
   treePath, edge, orientation, size, order, display), so floated groups can be
   **materialized** after any reload/env switch/layout rebuild and re-floated.
2. **Non-destructive float.** A store-level floated panel-id set suppresses
   close-side effects in `setupPortalCleanup` (`release`, terminal
   park/stop, vscode stop, `handleMaximizeExitOnLastClose`) and in
   `panelPortalManager.reconcile`/`releaseByEnv` for the current env's floated
   ids, for the duration of the float/dock/restore transaction. Cleared on
   settle; ordinary user closes afterwards behave normally.
3. **Placement from LayoutState, not a group API.** Dockview exposes no
   left/right/top/bottom group location; the mapping comes from the live
   `LayoutState` (`apps/web/lib/state/layout-manager/types.ts`): root column
   id/index, column kind via `isCenterCandidateGroupId`
   (`layout-manager/applier.ts`), tree path, and sizes. Orientation/edge per
   the spec's table; `fallbackGroupPosition`
   (`dockview-layout-builders.ts:272`) covers missing reference groups.
4. **Single store transactions.** float/dock/restore each run as one store
   transaction ending with `persistEnvLayoutNow` (the normal persistence
   callback gates on `isRestoringLayout`). Maximize→float restores the pre-max
   layout and derives geometry from `preMaximizeLayout`, never from the
   overlay's live dimensions.
5. **One floating coordinator.** Module-level focus tracker (pattern:
   `apps/web/hooks/use-panel-search.ts`) owns expansion/collapse: outside
   pointerdown (window capture), `focusout` to outside the owning window, and
   Escape while focus is inside the owning window (nested dialogs consume
   Escape first). Collapsed bars are semantic `tablist`/`tab` with
   `aria-selected`, roving Tab/Arrows, Enter/Space, focus return.

## Files

### Likely touched

- `apps/web/components/task/dockview-group-actions.tsx` — `PinButton` +
  placement in `GroupSplitCloseActionsView` (left of `MaximizeButton`).
- `apps/web/components/task/dockview-header-actions.tsx` — wire pin state +
  `floatGroup`/`dockGroup` into the shared `GroupSplitCloseActions`.
- `apps/web/components/task/dockview-floating-panel.tsx` (new) — floating
  window + collapsed edge bar overlay, rendered inside `DockviewDesktopLayout`
  root; adopts `panelPortalManager` elements; coordinator-driven collapse;
  tablist/tab semantics; stacking by `order`.
- `apps/web/components/task/dockview-floating-coordinator.ts` (new) —
  module-level focus/outside-pointer/Escape ownership + stack ordering.
- `apps/web/components/task/dockview-desktop-layout.tsx` — mount the floating
  overlay; wire `restoreFloatingAfterLayout` at ready.
- `apps/web/lib/state/dockview-store.ts` — `floatingGroups` state, actions
  (`floatGroup`, `dockGroup`, `setFloatingDisplay`, `setFloatingActivePanel`),
  floated panel-id set, transaction helpers, env-switch save/restore, reset
  clears, maximize interplay.
- `apps/web/lib/state/dockview-floating.ts` (new) — `restoreFloatingAfterLayout`
  (materialize + re-float), placement capture/derivation, versioned
  `EnvFloatingState` type guard, cleanup helpers; imported by store +
  setup + env-switch.
- `apps/web/components/task/dockview-layout-setup.ts` — suppress close-side
  effects for floated ids; invoke `restoreFloatingAfterLayout` at restore
  completion points; `reconcile`/`releaseByEnv` exclusion.
- `apps/web/lib/state/dockview-env-switch.ts` — session replacement returns an
  old→new mapping applied to floating entries (panelIds, defs, portal
  params/title, activePanelId); `restoreFloatingAfterLayout` on fast and slow
  paths.
- `apps/web/lib/state/dockview-layout-builders.ts` — `focusOrAddPanel` becomes
  `focusOrAddFloatingOrGridPanel` (floating check first).
- `apps/web/lib/state/dockview-panel-actions.ts`,
  `dockview-terminal-panel-actions.ts` — route every single-instance/add
  action through the identity resolver.
- `apps/web/lib/local-storage.ts` — `DOCKVIEW_ENV_FLOATING_PREFIX`, get/set/
  remove helpers + type guard; `cleanupTaskStorage` removes the key.
- Locales: `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
  — `pinPanel` / `unpinPanel` keys.

### Tests

- `apps/web/lib/state/dockview-floating-store.test.ts` (new) — float/dock
  transitions, placement capture (default/compact/plan/preview/vscode +
  nested custom), maximize→float ordering, last-group-in-column re-dock,
  display/active setters, persistence round-trip + type-guard rejection,
  env save/restore, reset clears, storage-write failure.
- `apps/web/components/task/dockview-floating-panel.test.tsx` (new) — expanded
  window, outside-pointer collapse, focusout collapse, Escape rules (owning
  window only; nested dialog wins), vertical/horizontal bar, tablist
  semantics, title-click expand+activate, pin re-dock, stacking/offsets,
  empty-group removal.
- `apps/web/components/task/dockview-group-actions.test.tsx` — pin button
  placement/aria/tooltip/click.
- `apps/web/lib/layout/panel-portal-manager.test.ts` — detach-vs-close:
  floated ids survive removePanel + reconcile; a real close after re-dock
  still releases.
- `apps/web/lib/state/dockview-env-switch.test.ts` — floating session
  replacement (stale floating tab, delayed replacement, active-tab rewrite),
  fast + slow path restore.
- `apps/web/lib/state/dockview-panel-actions.test.ts` — duplicate prevention
  for plan/terminal/preview/review/plugin/session actions with floated panels.
- `apps/web/lib/local-storage.test.ts` — floating key helpers + guard +
  `cleanupTaskStorage`.
- `apps/web/e2e/tests/task/panel-pin.spec.ts` (new, desktop) — full matrix:
  float/collapse/expand/dock, reload recreation, maximize→float, task switch
  with floated chat, terminal liveness, keyboard collapse, two groups on one
  edge.

## Dependencies

- Store + persistence + non-destructive removal (`task-01`) before UI
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

- **Portal lifecycle:** the floated-id suppression set is the only thing
  standing between float and portal destruction (`release`, `reconcile`,
  `releaseByEnv`). It must be cleared on every settle path (success, error,
  unmount) or user closes silently stop releasing. Terminal-liveness test is
  mandatory.
- **Materialization:** recreating floated panels after reload must reuse the
  proven `applyLayout`/`fallbackGroupPosition` machinery; a hand-rolled group
  creation path risks diverging from dockview's tree invariants.
- **Restore ordering:** `restoreFloatingAfterLayout` runs at every grid-restore
  completion point before `isRestoringLayout` clears, or users see a docked
  flash and the persistence gate skips the settled write.
- **Maximize interplay:** float of the maximized group must derive placement
  from `preMaximizeLayout` and sequence removals so the trailing restore rAF
  does not reassert the overlay.
