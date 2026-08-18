---
id: "02-header-pin-control"
title: "Pin control in group header"
status: pending
wave: 2
depends_on: ["01-floating-store-state"]
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 02: Pin control in group header

## Acceptance

- Every non-sidebar dockview group header shows a pin toggle immediately to the **left** of the maximize control, using `IconPinned` (pinned) / `IconPin` (unpinned) from `@tabler/icons-react`, matching the message queue pin.
- The button renders `aria-pressed` with the group's pin state (pinned by default) and a localized accessible name/tooltip from new keys `task:pinPanel` / `task:unpinPanel` ("Pin panel" / "Unpin panel").
- Clicking it calls `floatGroup(group.id)` when pinned and `dockGroup(group.id)` when unpinned; when the group is floating (no grid header), the control is instead rendered by the floating window / collapsed bar (task-03) and reports the unpinned state.
- New copy is added in all five real locales plus the generated pseudo-locale; `i18n:check` passes.

## Verification

```bash
cd apps/web && pnpm vitest run components/task/dockview-group-actions.test.tsx components/task/dockview-header-actions.test.tsx
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/task/dockview-group-actions.tsx` (`PinButton` + `GroupSplitCloseActionsView` ordering: pin, maximize, split/close)
- `apps/web/components/task/dockview-header-actions.tsx` (shared `GroupSplitCloseActions` wiring: pin state + `floatGroup`/`dockGroup` handlers)
- `apps/web/components/task/dockview-group-actions.test.tsx`
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/task.json` (`pinPanel`, `unpinPanel`; run `pnpm run i18n:zh-hant` for zh-hk/zh-tw)

## Inputs

- Spec: What (pin placement + icons + aria), first Scenario.
- Existing queue pin for icon/aria/label conventions: `components/task/chat/queued-ghost-panel-header.tsx`.
- `MaximizeButton` and `GroupSplitCloseActionsView` in `dockview-group-actions.tsx`; `GROUP/ACTION_BTN` class constants.

## Dependencies and Risks

- Depends on task-01 (`floatGroup`/`dockGroup`).
- Risk: `GroupSplitCloseActionsView` is unit-tested with a `width` prop; keep the pin unconditional (do not collapse it into the narrow dropdown) so the control is always reachable, per spec.

## Output Contract

Report behavior implemented, files changed, targeted tests run, locale additions, blockers, and update this task plus `plan.md` to done.
