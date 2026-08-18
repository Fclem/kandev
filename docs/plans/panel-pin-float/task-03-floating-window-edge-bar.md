---
id: "03-floating-window-edge-bar"
title: "Floating window, collapsed edge bar, and owned-region coordinator"
status: pending
wave: 3
depends_on: ["01-floating-store-state", "02-header-pin-control"]
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 03: Floating window, collapsed edge bar, and owned-region coordinator

## Acceptance

- A floating overlay (`components/task/dockview-floating-panel.tsx`, new) mounted inside the `DockviewDesktopLayout` root renders one entry per `floatingGroups` entry, absolutely positioned over the workbench with z-index above the dockview grid. Expanded windows show a header (tab strip with active tab highlighted + pin control) and the active panel's content adopted from `panelPortalManager` (same portal element the grid uses; content stays alive across float/dock).
- **Owned-region coordinator** (`dockview-floating-coordinator.ts`, pattern: `hooks/use-panel-search.ts`): a floating window's region = its DOM subtree plus any Radix layer opened from within it (`useFloatingOwnedLayer` hook wired to the existing `onOpenChange` handlers, with an **idempotent unregister that also runs on React cleanup** — unmount, navigation, ancestor teardown — so a layer closed without its dismiss callback still decrements the window's owned-layer refcount). Collapse triggers: window-capture pointerdown whose target is in no owned region — **pending while the window's owned-layer refcount is above zero; zero-refcount application is deferred past the microtask and held by a same-frame lease** that re-checks window generation, pending generation, refcount, and new registration before collapsing (same-turn and same-frame Radix close-then-open replacements never collapse; a successor registered by a later effect before the frame boundary cancels the pending collapse, which re-arms only on a fresh outside pointerdown; the cross-frame boundary is documented, not silently claimed); `focusout` whose `relatedTarget` (checked after a microtask, for Radix portal focus moves) is in no owned region; or Escape per the Escape contract. Only the focused/last-interacted expanded window collapses; other floating groups keep their display state.
- **Focusout acceptance (mandatory, three concrete cases):** (1)
  focus INTO an open owned layer is never a collapse candidate
  (relatedTarget inside the layer); (2) focusout to an outside target with
  NO owned layer open collapses immediately; (3) focusout while an owned
  layer is open defers (pending), and when the layer closes while focus
  is still outside, the pending collapse fires at the next frame.
- **Custom (non-Radix) portal ownership:** `plan-selection-popover.tsx`
  287-319 uses the host `useFloatingOwnedLayer` lease with explicit
  open/close lifecycle (inventory row flips to `audited` only after the
  wiring lands).
- **Layer inventory (mandatory, blocking deliverable):** the task ships
  `docs/plans/panel-pin-float/owned-layer-inventory.md` — a callsite table
  where every row reaches **exact file/line or an explicit layer-free proof**
  before task-03 is accepted (`to-wire`/`verify` are audit-baseline states,
  not completion). Rows cover Radix Dialog/Popover/DropdownMenu/ContextMenu/
  HoverCard/Drawer inside chat, plan, terminal, files, changes, diff,
  plugins, editors (the changes-* rows incl. the header Drawer ~293-302 and
  PullDropdown ~411-425 are already source-confirmed). The **host/plugin API
**transport-complete**: `PluginTaskPanelProps` gains a render-bound opaque
  `floatingOwnedLayerCapability` issued from a **portal-instance generation**
  (stable via `useRef` across benign re-renders so an open layer survives;
  revoked on actual portal release or `unregisterPlugin`; **rotated on
  reacquire** so hoarded tokens are rejected; **absent on mobile** where
  floating is desktop-only and host registration is rejected), and
  `host.ui.registerFloatingOwnedLayer(capability,
  openHandlers) => () => void` (per-open onOpenChange handshake; a body-
  portaled root node alone is NOT ownership proof — the plugin spreads the
  returned `onOpenChange` onto its Radix root) is added to the SDK types
  (`apps/packages/plugin-sdk/src/index.ts`, as a callable outside the mapped
  component type), `lib/plugins/host-api.ts` (WeakMap token binding), and the
  contract docs; a hoarded plugin-scoped function cannot be reused across
  renders or after unmount. One real test per primitive family (Dialog, Popover, DropdownMenu,
  ContextMenu, HoverCard, **Drawer, plus AlertDialog via the Dialog
  family**), including
  plugin hoarding, cross-panel rejection, unmount revocation, release-
  reacquire rotation, benign re-render stability, mobile rejection, and
  unregister cleanup.
- **Escape contract:** the coordinator listens on the **bubble** phase and honors `event.defaultPrevented` — a Radix dismissable layer or editor handler that handles Escape wins; otherwise Escape collapses the focused expanded window. No capture-phase claim.
- The collapsed bar is a semantic `tablist` with `tab` roles, `aria-selected`, an accessible group label, **roving tabindex** (one tab stop on the active tab), Arrow Up/Down and Left/Right navigation (both axes accepted in either orientation), Home/End, Enter/Space activation, Escape collapse, and focus return on expand/collapse. It is a **vertical** title bar (titles stacked) for side groups and a **horizontal** title bar (titles in a row) for horizontal groups, showing each tab's title (registry ids via `panelTitle()`, unknown ids via the live portal/dockview title, falling back to the persisted definition title) in tab order with the active tab highlighted, plus the pin control (a separate button in DOM order after the tablist). Title resolution is **reactive**: the overlay subscribes to the plugin registry and portal manager so a plugin re-registration or session replacement re-renders the bar while the panel is detached.
- Clicking a title sets it active and expands the window; clicking the bar's pin control re-docks the group.
- Stacking: `order` allocated from the monotonic per-env counter; render order sorted by `(order, groupId)`; offsets `order × 12px`; z-index `1000 + order`; expanded windows render above collapsed bars of the same edge.
- Test ids: `dockview-pin-btn`, `dockview-floating-window-<groupId>`, `dockview-floating-bar-<groupId>`, `dockview-floating-tab-<groupId>-<panelId>`.

## Verification

```bash
cd apps/web && pnpm vitest run components/task/dockview-floating-panel.test.tsx components/task/dockview-group-actions.test.tsx
cd apps/web && pnpm run typecheck
node scripts/check-owned-layer-inventory.mjs   # executable gate: static AST scan
                                               # using the repo's Babel parseSync
                                               # (typescript + jsx plugins),
                                               # file globs over panel content
                                               # sources, imports canonicalized
                                               # through the Vite aliases
                                               # (@kandev/ui wrappers and
                                               # <X.DropdownMenu> member JSX
                                               # matched), test/generated/mobile
                                               # exclusions, dynamic references
                                               # handled via an explicit
                                               # fixture/allowlist, and failure
                                               # output mapping each discovered
                                               # owner to an inventory row
                                               # (file:line -> row); fails on any
                                               # row still to-wire/verify, on an
                                               # unlisted primitive, or on BROAD
                                               # DIRECTORY-LEVEL rows (every row
                                               # must be an exact
                                               # component/file + line range or
                                               # a layer-free proof); wired
                                               # into CI
```

## Files Likely Touched

- `apps/web/components/task/dockview-floating-panel.tsx` (new; expanded window, collapsed bar, portal adoption, tablist semantics, reactive title subscription)
- `apps/web/hooks/use-panel-active.ts` (extended/replaced: lease-record-backed active authority, `acceptPanelActive` routing, api-null = floating-store-backed virtual active state)
- `apps/web/components/task/dockview-floating-coordinator.ts` (new; owned regions, pointer/focus/Escape ownership with same-frame lease, `useFloatingOwnedLayer`, stacking)
- `apps/web/components/task/dockview-desktop-layout.tsx` (mount overlay)
- `apps/packages/plugin-sdk/src/index.ts` + `apps/web/lib/plugins/types.ts` + `apps/web/lib/plugins/host-api.ts` + `docs/plans/plugins/PLUGIN-API.md` (`host.ui.registerFloatingOwnedLayer(capability, openHandlers)` — SDK type, host implementation with per-open handshake + WeakMap binding, ownership capability, docs; all four change together)
- `apps/web/components/task/plugin-task-panel.tsx` (render-bound `floatingOwnedLayerCapability` injection + cleanup revocation)
- `apps/web/lib/plugins/registry.ts` (`unregisterPlugin` owned-layer cleanup)
- `apps/web/scripts/check-owned-layer-inventory.mjs` (new, with focused
  fixtures/tests: alias imports, member JSX, wrappers, dynamic references,
  generated/mobile exclusions, an intentionally unlisted primitive) + a
  `check:owned-layers` package script + **`.github/workflows/frontend-tests.yml`
  required-job wiring** (exact `pnpm --filter @kandev/web check:owned-layers`
  invocation alongside the existing lint/typecheck/i18n gates; the gate
  fails CI if any row remains `to-wire`/`verify` or an unlisted primitive is
  found)
- `docs/plans/panel-pin-float/owned-layer-inventory.md` (audit completion)
- `apps/web/components/task/dockview-floating-panel.test.tsx` (new; incl. pointerdown deferral while a menu is open, reactive title update on plugin re-registration while detached, real Radix Escape ordering, plugin-panel layer ownership rejection)
- **Inventory deliverables OWNED here: `scripts/generate-owned-layer-
  inventory.mjs` (generator), `apps/web/config/owned-layer-inventory.
  generated.json` (committed artifact), `scan-panel-content`
  (set-equality + lazy-manifest AST validation), and
  `apps/web/config/lazy-panel-manifest.json` are all in Files Likely
  Touched + Verification; CI runs scan → generate → git-diff →
  validate as ONE required step; omitted-lazy-panel + out-of-glob
  fixtures per the inventory contract.**
- **Touched (ownership explicit): `lib/layout/panel-portal-host.tsx` (adds
  `FloatingWindowLeaseProvider` context around each createPortal render +
  portalInstanceKey context transport; React key STAYS the stable
  panelId), `lib/layout/panel-portal-manager.ts` (PortalEntry
  portalInstanceKey + atomic reacquire envId update), the workbench-root
  provider component, plus the reparent/no-null-lease test.**
- Reuse: `panelPortalManager` (`lib/layout/panel-portal-manager.ts`), `usePanelSearch` pointer-outside pattern (`hooks/use-panel-search.ts`), `panelTitle()` (`lib/state/layout-manager/panel-title.ts`), `PinButton` from task-02

## Inputs

- Spec: What (floating window, collapse orientation, bar contents, title click), Focus ownership, Escape contract, Collapsed bar accessibility, Stacking, Scenarios (outside click, Escape, tab-out collapse, portaled menu, two groups same edge, nested dialog Escape).
- `walkthrough-floating-window.tsx` for the overlay precedent; `panel-portal-host.tsx` `usePortalSlot` for element adoption mechanics; `popup-menu.tsx`/`plan-selection-popover.tsx` for the body-portaled layer precedent.

## Dependencies and Risks

- Depends on task-01 (store + detach registry) and task-02 (`PinButton`).
- Risk: the adopted portal element must be appended exactly once; guard against double-append when both the grid wrapper and the floating window could mount it (float removes the grid wrapper first).
- Risk: an unregistered Radix layer (component that opens a menu without `useFloatingOwnedLayer`) collapses the window under the open menu — the hook must be applied to every interactive layer inside floating panels, not just dialogs.
- Risk: pointerdown deferral must be refcounted (React cleanup + dismiss) and generation-tagged, or a stale pending collapse collapses a successor window after unmount/navigation; tested with two layers closing in both orders and unmount-without-dismiss.
- Risk: Escape must be tested with real Radix AlertDialog/DropdownMenu and an editor key handler, not only synthetic `fireEvent.keyDown` ordering.

## Output Contract

Report behavior implemented, files changed, targeted tests run, blockers, residual risks, and update this task plus `plan.md` to done.
