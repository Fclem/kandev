---
id: "01-loading-overlay"
title: "Render the loading-older message as a floating panel-bottom overlay"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/prompt-history-panel.md"
parallelism: sequential
---

# Task 01: Floating loading-older overlay

## Context

`apps/web/components/task/prompt-history-panel-content.tsx` renders the
"Loading older messages..." indicator in-flow as the last child of the
scrollable `PanelRoot`, directly above the pagination sentinel
(`prompt-history-loading-older` / `prompt-history-sentinel`). Because the
panel's sentinel re-arms while intersecting, `isLoadingMore` flaps once per
page during auto-load; each in-flow mount/unmount of the indicator changes the
content height and the sentinel's geometry, producing visible flicker and
scroll jitter. The indicator must instead be a floating message at the bottom
of the panel, out of the content flow. See `plan.md` for the full root-cause
analysis.

## Acceptance

- When `shouldPaginate && isLoadingMore`, the loading message renders as a
  floating overlay pinned to the bottom of the panel root: the indicator keeps
  `data-testid="prompt-history-loading-older"` and the `task:loadingOlderMessages`
  copy, is absolutely positioned within the panel root (`absolute`, not in
  flow), is `pointer-events-none`, and sits above the rows (`z-10`).
- The panel root is the positioned containing block (`relative`) in the rows
  branch and in the empty `isLoadingMore` branch, so the overlay anchors to the
  panel viewport, not to the scroll content; the overlay's presence never
  changes the scrollable content height.
- When `isLoadingMore` is false, no indicator renders. With zero entries and
  `isLoadingMore` true, the panel shows the floating loading message (not the
  neutral `task:loading` text or the empty state) and keeps the sentinel.
  Passthrough sessions remain an unconditional no-controls empty state.
- Sentinel behavior, pagination, and the shared lazy-load hooks are untouched;
  all existing sentinel/loading unit tests in
  `prompt-history-panel-content.test.tsx` stay green unchanged.

## Verification

Targeted unit suite (fails before the change, passes after):

```bash
cd apps/web && pnpm vitest run components/task/prompt-history-panel-content.test.tsx
```

Add/extend regression tests in `prompt-history-panel-content.test.tsx`:

1. Extend "shows the older-page loading row only while shouldPaginate &&
   isLoadingMore" (or add a sibling test): while `isLoadingMore`, assert the
   loading element's class list contains the overlay contract
   (`absolute`, `pointer-events-none`, `z-10`) and that the panel root class
   list contains `relative`; assert the loading element is not the sentinel's
   preceding in-flow sibling (the sentinel's `previousElementSibling` is not
   the loading element). These assert the layout contract that prevents
   reflow-driven flicker.
2. The zero-entries + `isLoadingMore` case keeps asserting the panel's
   `textContent` is exactly "Loading older messages..." with the sentinel
   present.

Global checks:

```bash
cd apps/web && pnpm run typecheck && pnpm run lint
```

E2E (user-facing flow):

```bash
cd apps/web && pnpm e2e:raw -- e2e/tests/task/prompt-history-auto-load.spec.ts
cd apps/web && pnpm e2e:raw -- e2e/tests/task/mobile-prompt-history-panel.spec.ts
```

In `prompt-history-auto-load.spec.ts`, before releasing the held older-page
request, capture `scrollHeight` via `panel.evaluate((el) => el.scrollHeight)`;
after `prompt-history-loading-older` is visible, assert `scrollHeight` is
unchanged (the floating overlay must not alter content layout). The existing
"visible while held" assertion stays valid.

## Files Likely Touched

- `apps/web/components/task/prompt-history-panel-content.tsx` — floating
  overlay + `relative` on the panel root (rows branch and empty `isLoadingMore`
  branch via `emptyEntriesSpec`).
- `apps/web/components/task/prompt-history-panel-content.test.tsx` — regression
  assertions above.
- `apps/web/e2e/tests/task/prompt-history-auto-load.spec.ts` — scrollHeight
  stability assertion.

## Output Contract

Report the exact class strings used for the overlay and the panel root, the
unit assertions added, the e2e assertion added, and results of the targeted
vitest run, typecheck, lint, and e2e specs. Mark this task done in this file
and `plan.md`.
