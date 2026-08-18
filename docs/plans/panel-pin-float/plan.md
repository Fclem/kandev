# Plan: Floating (unpinned) workbench panels

Spec: [`docs/specs/ui/panel-pin-float.md`](../../specs/ui/panel-pin-float.md)

## Summary

Add a per-group pin toggle to the dockview workbench group headers (left of the
maximize control, message-queue pin icons). Unpinning floats the group over
the workbench; it collapses to an edge title bar when unfocused and re-docks
on pin click. State persists per task environment in sessionStorage, mirroring
the existing env layout / maximize persistence. Revision 8 incorporates the
round-7 adversarial review (this package has been adversarially reviewed every
round): per-panel divergence salvage, a single authoritative
`rightPanelsVisible` (= live right-column presence), a transaction
coordinator with busy-rejection, a `recoverFloatingJournalOnce` gate ahead of
every restore, a size-budget preflight, session-replacement placement
normalization (center column identity), maximize-restore ordering, an
auditable owned-layer callsite inventory with a host/plugin API, and a named
mobile retained-path scenario.

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
4. **Transactions: one coordinator + journal + phase model.** A single
   coordinator (`floating-transaction.ts`) owns the per-env
   operation/generation and phase state via explicit `begin → advance →
   settle`; while busy, float/dock/reset/toggle-right actions are **rejected
   (pin disabled)** — no re-entrant transactions. A per-env **operation
   journal** (`kandev.dockview.env-floating-journal.<envId>`) is written
   **before** mutation with complete before/after blob+layout snapshots;
   **`recoverFloatingJournalOnce(api, envId)` is the single idempotent
   pre-restore gate** running before every restore entry (initial mount,
   env fast/slow, maximize, presets, toggle, reset) and recovers
   deterministically (after-state if the mutation completed, else
   before-state; cleared after both keys validate). **Size budget preflight**
   (96 KB default per env: blob + journal) fails non-destructively before
   mutation. Recovery runs the phase model `mutating → restoring →
   portals-adopted → persist-recovered → settled`; only portals-adopted
   persists the journaled layout (status-returning APIs — the current
   `setEnvLayout`/`persistEnvLayoutNow` swallow errors); token cleanup is
   generation-guarded at settled. Journal-free fallback is idempotent with
   **per-panel salvage** (live identity is the authority for conflicting
   panels; non-conflicting tabs are retained and re-docked per the collision
   policy — nothing disappears silently). One transaction-aware unload
   handler replaces the existing listener (journaled pre-transaction layout
   during a transaction, normal live flush otherwise). **Single storage
   policy: fail closed** on every blob-write failure — rollback to pinned; no
   ephemeral floating mode.
5. **Owned-region focus with refcounted, same-frame-leased pending collapse.**
   One module-level coordinator (pattern: `hooks/use-panel-search.ts`) with
   owned regions = floating window subtree + any Radix layer opened from
   within it (`useFloatingOwnedLayer`; idempotent unregister on dismiss AND
   React cleanup; **mandatory inventory of layer owners in every
   floating-capable panel** — chat, plan, terminal, files, changes, diff,
   plugins, editors). Collapse on: window-capture pointerdown outside all
   owned regions (**pending while the window's owned-layer refcount is above
   zero; application deferred past the microtask and held by a same-frame
   lease that re-checks window generation, pending generation, refcount, and
   new-registration — same-turn and same-frame Radix replacements never
   collapse; longer task gaps may, and that boundary is documented**);
   `focusout` to outside (relatedTarget after a microtask); Escape on the
   **bubble** phase honoring `event.defaultPrevented`. Only the
   focused/last-interacted expanded window collapses.
6. **Reset is an id-aware docking merge, session-aware, with a canonical
   session fallback.** The reset layout owns group/column placement; the
   floating definition owns the panel payload and saved tab order; the active
   panel is merged explicitly — scoped to valid definitions (`session:*` defs
   validated against the active session set; stale dropped; reset chat
   placeholder retained when none remain; **a valid active session with no
   center column lands in the first column's first group with the active
   session set, so the auto-session hook never double-inserts**). Reset-default
   panels are reused by id (a floated ordinary terminal keeps its real
   terminal id); floating state clears only after the merged grid is
   committed; groups do not re-float.
7. **Restore call sites (exhaustive).** `recoverFloatingJournalOnce(api,
   envId)` runs before every restore entry, then `restoreFloatingAfterLayout`
   (idempotent) runs before `isRestoringLayout` clears at: initial mount
   `restoreEnvLayout` (all three branches: saved env layout /
   `tryRestoreMaximizeOnly` / default+route-intent — `dockview-layout-restore.ts`),
   env-switch fast and slow paths (`dockview-env-switch.ts`), **maximize
   restore (defined ordering: journal first, floating session entries
   reconciled with `floatingSessionWinner` written before the auto-session
   hook, floating state restored against the saved `preMaximizeLayout`
   without mutating the two-column overlay)**, preset/custom apply,
   `toggleRightPanels` (`dockview-store.ts:523-567`), and reset/default
   build. One focused test per call site.
8. **Identity coordination (session + right panels).** Session replacement is
   one coordinator over grid + floating entries; the winner is written to a
   store-owned, **memory-only** `floatingSessionWinner: { sessionId, envId,
   generation } | null` atomically with replacement and consumed one-shot via
   atomic **compare-and-clear** `consumeFloatingSessionWinner(...)` from
   `shouldSkipPanelEnsure` (skips only the winner id; unrelated siblings
   ensured as today; stale winners cleared on generation/env transition and
   terminal paths, never clearing a newer generation). Replacement also
   **normalizes placement**: center-kind floating entries have `columnId`/
   `columnIndex` rewritten to the live center column (a panel-derived
   `session:<id>` column id must never force a new root column on dock).
   `rightPanelsVisible` is **exactly live right-column presence** — derived
   everywhere (restore, float, dock, toggle, preset/default assignment), never
   persisted, never an independent intent; toggle follows from presence (show
   with zero pinned groups and hide without a column are no-ops). The
   `toggleRightPanels` show path is floating-aware (**every** floating id
   excluded), removes empty groups, and drops the right column when no
   pinned-right panels remain (legal serialized tree). The enforcement gate
   and the toggle share one authoritative live pinned-right-column detector.
   No panel id ever exists in both surfaces.

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
  custom fallback + **session-replacement placement normalization**),
  materializer (missing column, no-center, tree-path insert, tree+flat
  round-trip with an existing nested tree), maximize→float ordering +
  **maximize-restore ordering (reload + task switch + simultaneous maximize +
  stale floating chat)**, last-group-in-column re-dock, transaction journal
  recovery (**full matrix: float/dock/restore × both crash directions**,
  throw-on-remove, throw-on-materialize, blob-write failure → fail-closed
  rollback to pinned, journal before/after validation, **`recoverFloatingJournalOnce`
  precedes every restore branch incl. maximize-only and route-intent**,
  **size-budget preflight**, timer scheduled before transaction start, unload
  during transaction writes journaled layout exactly once, **re-entrancy
  rejection across all phases**), per-panel divergence salvage (one
  conflicting + one non-conflicting tab; nothing disappears), display/active
  setters, persistence round-trip + type-guard rejection (undefined/cyclic/
  oversized params, negative/oversized numerics, duplicate ids, order
  exhaustion at 9 999/10 000/next, stale `nextOrder` normalization preserving
  the high-water mark), env save/restore, reset-as-merge (payload-wins
  collisions incl. floated ordinary terminal; stale session defs dropped;
  chat placeholder retained; valid-session-without-center-column fallback),
  stacking allocation.
- `apps/web/components/task/dockview-floating-panel.test.tsx` (new) — expanded
  window, owned-region collapse (outside pointer, focusout, portaled Radix
  layer open = owned), pointerdown deferral (outside click while a menu is
  open stays expanded until dismissal; pending collapse cancelled on
  owned-region click; layer closed via unmount/navigation decrements; two
  layers closing in both orders; **close-then-open replacement same-turn and
  same-frame; cross-frame boundary documented**; successor window inherits no
  stale pending collapse), Escape rules (defaultPrevented wins; nested
  AlertDialog/DropdownMenu), vertical/horizontal bar, tablist semantics +
  roving tabindex + Arrow/Home/End, title-click expand+activate, pin re-dock,
  stacking/offsets, empty-group removal, reactive title update on plugin
  re-registration while detached, layer-inventory primitives (one real test
  per Radix family incl. a plugin-panel layer).
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

- **Bidirectional journal consistency:** the operation journal, its recovery
  gate (`recoverFloatingJournalOnce` before EVERY restore entry), and the
  phases are the whole crash story; the journal must be written before
  mutation and cleared only after both keys validate.
- **Re-entrancy:** one transaction coordinator with busy-rejection (pin
  disabled); a second transaction mid-phase must be impossible.
- **Size budget:** blob + journal must preflight against the shared
  sessionStorage quota or valid floats silently fail at write time.
- **Single unload handler:** one idempotent transaction-aware handler (never
  a second listener); a duplicated flush can overwrite the journaled layout
  with the mutated grid.
- **Same-frame lease:** pending collapse must survive same-turn and same-frame
  Radix replacements; the cross-frame boundary must be documented, not
  silently claimed.
- **Winner lifecycle:** compare-and-clear consumption, memory-only field,
  stale-winner cleanup on every terminal path without clearing a newer
  generation; written before the auto-session hook in BOTH env-switch and
  maximize-restore branches.
- **Placement normalization:** session replacement must rewrite center-kind
  column identity or a panel-derived `session:<id>` column id forces a new
  root column on dock.
- **Right-visibility derivation:** `rightPanelsVisible` is exactly live
  right-column presence everywhere (no intent bit); one authoritative live
  pinned-right detector shared by the toggle and enforcement.
- **Reset session fallback:** valid sessions without a center column must land
  in the first column's first group, or the auto-session hook double-inserts.
- **Portal-safe rollback:** journal recovery must run as a restore-gated,
  exclusion-set transaction or rollback itself releases portals.
- **Empty-right legality:** excluding floating ids must also drop empty groups
  and the empty right column, or the serializer emits an illegal branch that
  corrupts the next restore.
- **Owned-layer inventory:** the callsite table and host/plugin API must be
  supplied (task-03 deliverable); an unregistered layer is a collapse bug.
- **Registry consume-once:** consuming a registration for a `fromJSON` removal
  during restore must not break `reconcile`'s post-restore behavior or a float
  whose removal never fires; settle-clear must still run.
- **Persistence guard completeness:** the transaction token must gate
  `persistNow`, the debounce callback, the event handler, AND `beforeunload`,
  and cancel/hold an already-scheduled timer — one missed entry point
  re-exposes the partial-layout persistence the round-4 review found.
- **Session identity:** a floating winner must be visible to
  `shouldSkipPanelEnsure` before the hook's ensure effect, or the incoming
  session id lands in both surfaces; the toggle-show path must not re-add
  floating right ids.
- **Detach registry leaks:** if a registered id is never removed from the grid
  (transaction aborted before removal), the registration must still be cleared
  on settle or the next real close of that tab skips cleanup. Consume-once +
  settle-clear are both required; test the abort path.
- **Materializer divergence:** the delta-LayoutState insert must update both
  `column.tree` and `column.groups` atomically and go through the existing
  serializer/`applyLayoutAndSet`, or dockview's tree invariants silently break
  and `serializePanels` drops the group's definitions.
- **Enforcement gate:** missing the live-pinned-right-column check lets
  `enforcePinnedTargets` resize the center to the right target when all right
  groups float; the gate and its tests are mandatory.
- **Restore ordering:** one missed completion point (e.g. `toggleRightPanels`
  or the initial maximize-only branch) re-exposes the reload blocker; the
  call-site table in the spec is the checklist and each entry has a test.
- **Maximize interplay:** float of the maximized group must derive placement
  from `preMaximizeLayout` and sequence removals so the trailing restore rAF
  does not reassert the overlay.
