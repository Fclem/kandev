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

- The rows and the sentinel live in a content wrapper; scrollability is
  measured from the wrapper alone against the root's scrollable content box
  (`wrapper.scrollHeight > root.clientHeight - verticalPadding`, padding read
  from `getComputedStyle`; re-measured after every commit AND on external root
  resize via ResizeObserver), so the indicator can never flip the mode and a
  width-only dockview drag cannot leave the mode stale.
- When the prompt rows overflow the panel (panel scrolls) and the loading
  message is shown, it renders as a floating overlay pinned to the bottom of
  the panel root: the indicator keeps `data-testid="prompt-history-loading-older"`
  and the `task:loadingOlderMessages` copy, is absolutely positioned within the
  panel root (`absolute`, not in flow), is `pointer-events-none`, and sits
  above the rows (`z-10`); the panel root is the positioned containing block
  (`relative`).
- When the rows fit (panel does not scroll) and in the zero-entries branch
  (which can never scroll), the loading message is an in-flow row directly
  under the last message (after the sentinel), and the panel root is not a
  containing block.
- Flicker: the loading message stays mounted for a 400 ms grace window after a
  page settles, so consecutive auto-loads render one continuous indicator; it
  disappears once a settle is not followed by another load within the window
  or pagination ends. The grace is scoped to the active session: switching
  sessions within the window hides the indicator for the new session (it has no
  in-flight load of its own). With zero entries, the loading message shows
  instead of the neutral `task:loading` text or the empty state. Passthrough
  sessions remain an unconditional no-controls empty state.
- Sentinel behavior, pagination, and the shared lazy-load hooks are untouched;
  all existing sentinel/loading unit tests in
  `prompt-history-panel-content.test.tsx` stay green unchanged.

## Verification

Targeted unit suite (fails before the change, passes after):

```bash
cd apps/web && pnpm vitest run components/task/prompt-history-panel-content.test.tsx
```

Add/extend regression tests in `prompt-history-panel-content.test.tsx`:

1. Mode test: with default geometry (content does not overflow), the loading
   element is in-flow (`py-2`, not `absolute`) and the panel root has no
   `relative`; stub the content wrapper's `scrollHeight` above the panel
   root's `clientHeight` (via the file's `setGeometry` helper, extended with
   `scrollHeight`) and rerender: the loading element carries the overlay
   contract (`absolute`, `pointer-events-none`, `z-10`), the panel root is
   `relative`, and in both modes the sentinel's `previousElementSibling` is
   not the loading element.
2. Grace test (fake timers): after `isLoadingMore` settles, the loading
   element stays mounted through the 400 ms window and across a load that
   starts and settles within it; it disappears once the grace expires without
   a new load.
3. The zero-entries + `isLoadingMore` case keeps asserting the panel's
   `textContent` is exactly "Loading older messages..." with the sentinel
   present, and asserts the in-flow (non-floating) placement.

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
request, capture `scrollHeight` (and whether the panel scrolls); after
`prompt-history-loading-older` is visible, assert `scrollHeight` is unchanged
when the panel scrolls (the floating overlay must not alter content layout).
The existing "visible while held" assertion stays valid.

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
