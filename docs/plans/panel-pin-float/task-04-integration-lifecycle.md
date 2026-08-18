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

- **Restore call sites (exhaustive):** `recoverFloatingJournalOnce(api, envId)` runs before every restore entry (and enters the transaction coordinator — a restore during a busy phase is skipped with a retained pending-floating-restore marker), then `restoreFloatingAfterLayout` (in `lib/state/dockview-floating.ts`, idempotent) runs before `isRestoringLayout` clears at every completion point, each with one focused test:
  1. initial mount `restoreEnvLayout` — after saved-env-layout, maximize-only, and default/route-intent branches (`components/task/dockview-layout-restore.ts`);
  2. env-switch fast path and slow path (`lib/state/dockview-env-switch.ts`), including `applyInitialRouteLayout`;
  3. **maximize restore — one shared coordinator `restoreFloatingMaximize`** (in `lib/state/dockview-floating.ts`) for BOTH callers (`tryRestoreMaximizeOnly` initial-mount and `restoreMaximizeFromStorage` env-switch): journal first, floating session entries reconciled (winner written before the auto-session hook in this branch), the two-column overlay never mutated, a per-env pending-floating-restore marker (store state) set, materialization only after `exitMaximizedLayout`'s rAF settles; marker consumption token/generation-guarded, cleared only after the post-exit rAF AND floating restore settle; **skipped-restore settle drain** (recheck envId/generation/api/marker before invoking restore); exit + immediate reload re-evaluates journal + marker on next mount;
  4. preset and custom layout apply (`applyLayout`/`applyLayoutAndSet`);
  5. `toggleRightPanels` (`lib/state/dockview-store.ts`), where the restore is a no-op if already applied;
  6. reset/default build.
  No docked flash appears after any restore.
- **Session replacement (single coordinator over grid + floating + auto-session hook):** `replaceStaleSessionPanels` returns an old→new mapping applied to floating entries (panelIds, `FloatingPanelDef`s, portal params/title, `activePanelId`). The deterministic **winner** is the group's active stale panel, else the first stale floating panel in saved tab order. The winner is written to a store-owned, **memory-only** field — `floatingSessionWinner: { sessionId, envId, generation } | null` — atomically with replacement and consumed **one-shot** via atomic compare-and-clear `consumeFloatingSessionWinner(sessionId, envId, generation)` from `shouldSkipPanelEnsure` (`dockview-session-tabs.ts`) before the always-mounted `useAutoSessionTab` hook's ensure effect — written before the hook in **both** env-switch and maximize-restore branches; the hook skips only the winner id (unrelated current-session siblings are ensured as today via `addCurrentSessionSiblings`/`ensureSiblingPanels`, with an explicit anchor when the winner floats). Stale winners (generation/env mismatch) are cleared on generation/env transition and every terminal path (ensure failure, unmount, env switch); a newer generation is never cleared by an older cleanup; repeated/StrictMode effects cannot double-skip. **Placement normalization (post-apply hook, floating-winner edge):** replacement also runs a
  post-apply hook — after the synchronous layout/session replacement AND
  incoming-session insertion (fast, slow, route-intent, maximize-restore,
  reload paths alike) — resolving the root column by **direct live group
  membership + index**, never `fromDockviewApi`'s panel-derived ids and never
  `findCenterGroupId`'s fabricated fallback; center-kind entries' `columnId`/
  `columnIndex` are rewritten to the real live center column when
  `isCenterKnown`. **When the incoming session is the floating winner (no
  live grid center), the entry's center intent is preserved and normalization
  is deferred until materialization/dock** (resolved from the live grid or
  the winner's saved/pre-max layout) — never downgraded to the custom
  fallback. A winner in the grid drops the floating stale copy. Other stale floating panels are dropped; `activePanelId` points to the winner's new id (or the first surviving panel). No panel id ever exists in both surfaces. Tested through the real desktop hook, not only direct `replaceStaleSessionPanels` calls.
- **Right-width enforcement gate + floating-aware toggle:** `rightPanelsVisible` is exactly **`hasLivePinnedRightColumn`** — pinned canonical right-column presence defined by one shared predicate (pinned/width metadata + canonical right-group ids), **never** any side column: plan/preview/vscode unpinned side columns return false (matching today's preset assignments) and are never hidden as right-panel toggles. The bit is derived everywhere (restore, float, dock, toggle, preset/default assignment), never persisted; show with no pinned-right panels and hide without one are no-ops. `enforcePinnedTargets`, the toggle, and `dockview-layout-setup`'s detection share the predicate; with all right groups floated, the center column is never resized to the right target. The `toggleRightPanels` **show path** is floating-aware: **every** floating panel id (not only the default `files`/`changes`/`terminal-default` set — custom and ordinary right panels included) is excluded from the re-added default right column (or the toggle docks them explicitly), **groups with zero remaining panels are removed, and the right column is dropped entirely when no pinned-right panels remain** — the serialized tree is always legal (no empty branch/leaf). **Exclusion is tree-aware**: a single tree+flat filtering helper updates both the column's `groups` and its nested `tree` (the current `removeRightPanelTabs` filters only flat groups while the serializer prefers `tree`); nested right columns, empty leaves, and ordinary/custom floating ids are tested. **Busy guards:** all public layout-mutation boundaries (add-panel resolver, `buildDefaultLayout`, preset/custom apply, maximize/exit, programmatic actions) return a non-destructive no-op while `isFloatingTransactionBusy(envId)`; restore/recovery paths enter the same coordinator and skipped restores drain at settle. **Deletion cancellation:** task deletion/mount teardown invalidates the env's transaction generation before `cleanupTaskStorage` removes the floating + journal keys, and a late settle can never rewrite them (tested with an incomplete journal and an in-flight transaction). Tested with one and both right groups floated (incl. an ordinary terminal id), hide→show while floating, float-last-right → hide → show → reload, plan/preview/vscode/compact presets (predicate + bit values asserted), and container resize, asserting a legal serialized tree.
- **Add-panel routing:** `focusOrAddPanel` becomes `focusOrAddFloatingOrGridPanel`; every single-instance/add action family (plan, changes, files, terminal, preview, review, plugin, session, deferred actions — including direct `api.getPanel()` checks in `dockview-panel-actions.ts` and `dockview-terminal-panel-actions.ts`) routes through it. Adding a panel that already floats expands/focuses the floating window with that tab active; no duplicate grid panel is created for a floated panel; grid behavior for non-floated panels is unchanged (existing add-panel tests are the regression net).
- **Maximize:** floating windows render above a maximized grid; `floatGroup` on the currently maximized group exits maximize first with placement derived from `preMaximizeLayout`, sequenced so the trailing restore rAF does not reassert the overlay (tested deterministically).
- **Closing tabs:** closing all tabs of a floating group removes its floating entry (no orphan edge bar); the group's pin state resets to pinned. A user close of a floated tab mid-transaction drops it from the floating definitions and releases its portal (no leak).
- **Reset is an id-aware docking merge, session-aware:** the reset layout owns group/column **placement**; the floating definition owns the panel **payload** (component, params, tabComponent) and saved tab order; the active panel is merged explicitly — **scoped to valid definitions**. `session:*` floating defs are validated against the active task/session set and env: stale/deleted/absent-session definitions are dropped and the reset chat placeholder/default behavior is retained when no valid session remains. **Valid session insertion is canonical:** every valid active session gets a session tab — via the merger's center-chat replacement when a center column exists, and via the documented fallback (first column's first group, active session set) when it does not, so the auto-session hook never double-inserts. Reset-default panels are reused by id (never duplicated); a floated ordinary terminal keeps its real terminal id rather than being retargeted to the default `terminal-default` params. Floating state (memory + storage) clears only after the merged grid is committed and persisted; groups do not re-float after reset. `cleanupTaskStorage` removes `kandev.dockview.env-floating.<envId>` **and the journal key** `kandev.dockview.env-floating-journal.<envId>` on task deletion (tested with an incomplete journal).

## Verification

```bash
cd apps/web && pnpm vitest run lib/state/dockview-env-switch.test.ts lib/state/dockview-layout-builders.test.ts lib/state/dockview-panel-actions.test.ts lib/state/dockview-floating-store.test.ts lib/state/dockview-store.test.ts lib/state/dockview-pinned-enforce.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/lib/state/dockview-floating.ts` (`restoreFloatingAfterLayout`; invoked from every completion point)
- `apps/web/lib/state/dockview-env-switch.ts` (replacement coordinator + winner rule + `floatingSessionWinner` write + compare-and-clear; grid-insertion suppression; fast + slow path restore invocation; journal recovery before restore)
- `apps/web/components/task/dockview-session-tabs.ts` + `dockview-desktop-layout.tsx` (`shouldSkipPanelEnsure` consumes `floatingSessionWinner` one-shot)
- `apps/web/components/task/dockview-layout-restore.ts` (initial three branches; journal recovery first)
- `apps/web/components/task/dockview-layout-setup.ts` (restore completion points; `handleMaximizeExitOnLastClose` interplay; `canPersistLayout` gate; single transaction-aware unload handler; shared live-right detector)
- `apps/web/lib/state/dockview-pinned-enforce.ts` (shared live-pinned-right-column gate)
- `apps/web/lib/state/dockview-store.ts` (`toggleRightPanels` floating-aware show path with empty-right removal, `rightPanelsVisible` derivation on restore, reset-as-merge with session fallback, maximize exit-on-float)
- `apps/web/lib/state/dockview-layout-builders.ts` (`focusOrAddFloatingOrGridPanel`)
- `apps/web/lib/state/dockview-panel-actions.ts`, `dockview-terminal-panel-actions.ts` (route all add actions through the resolver)
- `apps/web/lib/local-storage.ts` (`cleanupTaskStorage` incl. the journal key)
- Existing tests updated for the floating branches

## Inputs

- Spec: Restore call sites, Session replacement, Add-panel routing, State machine, Failure modes (right-width enforcement, reset merge, maximize, partial failure), Scenarios (task switch, add-panel duplicate prevention, maximize→float, reset merge, no-duplicate-id, mid-transaction close).
- Existing flows: `restoreEnvLayout`/`tryRestoreLayout` (`dockview-layout-restore.ts:198-320`), `replaceStaleSessionPanels` (`dockview-env-switch.ts:132-180`) + `restoreMissingSessionPanel` (`:584-594`), `enforcePinnedTargets` (`dockview-pinned-enforce.ts:105-127`), `applyLayoutFixups`, `persistEnvLayoutNow`, `toggleRightPanels` (`dockview-store.ts:523-567`), `resetToEffectiveDefault` (`dockview-store.ts:1060-1073`), `setupLayoutPersistence` (`dockview-layout-setup.ts:346-403`).

## Dependencies and Risks

- Depends on task-03.
- Risk: one missed completion point re-exposes the reload blocker; the spec's call-site table is the checklist and each entry has a test; `recoverFloatingJournalOnce` runs before all restore sites, and maximize-restore has its own ordering (journal → session reconcile → pre-max placement).
- Risk: the session coordinator must be consumed one-shot via compare-and-clear before the hook's ensure in BOTH env-switch and maximize-restore branches, or repeated/StrictMode effects double-skip or leak the winner.
- Risk: placement normalization must rewrite center-kind column identity on session replacement, or a panel-derived `session:<id>` column id forces a new root column on dock.
- Risk: the `toggleRightPanels` show path must exclude every floating id (custom/ordinary right panels too) and remove empty groups/columns, or duplicate identity or an illegal serialized tree results.
- Risk: `rightPanelsVisible` is exactly live right-column presence everywhere (no intent bit); the enforcement gate and toggle share the live-right detector, or the center is resized to the right target.
- Risk: the duplicate-prevention refactor must not change grid behavior for non-floated panels; existing add-panel tests are the regression net.
- Risk: `canPersistLayout` must gate `persistNow`, the debounce callback, the event handler, AND `beforeunload` (one transaction-aware handler), and hold an already-scheduled timer; one missed entry point re-exposes partial-layout persistence.
- Risk: the operation journal must be written before mutation and cleared only after both keys validate; a lost journal falls back to the per-panel salvage rules, which are not the primary guarantee.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
