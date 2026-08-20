---
spec: docs/specs/ui/prompt-history-panel.md
created: 2026-08-20
status: done
---

# Implementation Plan: Floating prompt-history loading message

## Overview

Move the prompt-history panel's "Loading older messages..." indicator out of the
content flow and pin it as a floating overlay at the bottom of the panel. The
indicator keeps its `prompt-history-loading-older` test id and the
`task:loadingOlderMessages` copy; only its presentation and layout
participation change. Sentinel, pagination, and store logic are untouched.

## Confirmed root cause

`apps/web/components/task/prompt-history-panel-content.tsx` renders the
loading-older row **in-flow** as the last child of the scrollable `PanelRoot`,
immediately above the sentinel (`data-testid="prompt-history-sentinel"`):

```tsx
{isLoadingMore && (
  <div data-testid={LOADING_OLDER_TEST_ID} className="py-2 text-center text-xs ...">
    {t("task:loadingOlderMessages")}
  </div>
)}
<div ref={sentinelRef} data-testid={SENTINEL_TEST_ID} aria-hidden="true" />
```

The panel's sentinel is configured with `rearmWhileIntersecting: true` and
`joinInFlightWhileLoading: true` (`use-lazy-load-sentinel.ts`), so after every
positive page load the still-intersecting sentinel fires the next older-page
request immediately. `isLoadingMore` therefore flaps true→false once per page
while pages stream in, and each in-flow mount/unmount of the ~28 px row changes
the scrollable content height and the sentinel's geometry. The content reflow
shifts the sentinel across the intersection boundary and jitters the scroll
position, which is the visible flicker.

## Fix

Render the indicator as a floating overlay anchored to the panel viewport:

- The `PanelRoot` in the rows branch and in the empty `isLoadingMore` branch
  gains `relative`, making it the positioned containing block (the panel root
  is already the `IntersectionObserver` root, so anchoring to it keeps the
  overlay pinned to the panel's visible bottom regardless of scroll).
- The indicator becomes an absolutely positioned, `pointer-events-none`
  centered chip at the panel bottom (`absolute inset-x-0 bottom-2 z-10 flex
  justify-center`), so it never participates in content layout and never
  intercepts row clicks or scroll.
- Conditions unchanged: shown only while `shouldPaginate && isLoadingMore`
  (rows branch) or while `isLoadingMore` with zero entries (empty branch, where
  older-page loading keeps precedence over neutral loading and the empty
  state). Passthrough stays an unconditional no-controls empty state.
- No new copy: reuse `task:loadingOlderMessages`; no locale changes.

## Task waves

- **Wave 1 (single task, one component + its tests + one e2e spec):**
  `task-01-loading-overlay.md`.

## Tasks

- `task-01-loading-overlay.md` — floating overlay in
  `prompt-history-panel-content.tsx` with unit + e2e regression coverage.

## Global validation

From `apps/web`:

```bash
pnpm vitest run components/task/prompt-history-panel-content.test.tsx
pnpm run typecheck
pnpm run lint
```

Targeted e2e (desktop auto-load + mobile panel):

```bash
pnpm e2e:raw -- e2e/tests/task/prompt-history-auto-load.spec.ts
pnpm e2e:raw -- e2e/tests/task/mobile-prompt-history-panel.spec.ts
```

## Risks

- The overlay must stay below the panel toolbar (none here) and above rows
  (`z-10`); `pointer-events-none` keeps wheel/touch gestures and row
  interactions intact.
- Sentinel geometry is now constant while loading, which changes re-arm timing
  slightly (no height shift mid-flight); pagination behavior is otherwise
  identical and covered by the existing sentinel unit tests, which must stay
  green.
- Out of scope: the transcript's in-flow loading-older row
  (`message-list-shared.tsx`) stays as-is.
