---
spec: docs/specs/fix-duplicated-tab-stale-data/spec.md
created: 2026-08-16
status: complete
---

# Implementation Plan: Reload on Frozen-Snapshot Restore

## Overview

Chrome's Duplicate tab restores the Kandev page from a frozen browser snapshot
(back/forward-style restore; the duplicated tab's navigation type is
`back_forward`) instead of performing a fresh load. The restored page keeps the
frozen JS heap and DOM, so data that changed after the snapshot (e.g., a task
archived in the meantime) shows as stale until a manual refresh.

Root cause chain, with evidence:

1. Fresh loads are always correct: the SPA shell and the boot-payload paths set
   `Cache-Control: no-store` (`apps/backend/internal/webapp/handler.go`,
   `dev_handler.go`), and the frontend data fetches pass `cache: "no-store"`
   (`apps/web/lib/api/domains/...`, `loadBootPayload` in
   `apps/web/src/boot-payload.ts`). A real navigation therefore re-reads
   current state.
2. Tab duplicate is not a fresh load. Chromium tracks it as a `back_forward`
   navigation (bfcache-dev discussion,
   https://groups.google.com/a/chromium.org/g/bfcache-dev/c/Cs5ISWbKhKU), and
   Chrome has been rolling out bfcache admission for `Cache-Control: no-store`
   pages (https://developer.chrome.com/docs/web-platform/bfcache-ccns).
3. The app has no restore handling: no `pageshow`/`pagehide`/`freeze`/`resume`
   handlers exist anywhere in `apps/web`. The `useForegroundRefresh` hooks
   refresh only subsets of surfaces and do not distinguish restores from
   focus/visibility events.
4. Verified in real Chrome (headless, via a test page): a bfcache restore
   fires `pageshow` with `persisted === true`, and the frozen timers resume.
   Platform guidance for this class of bug is to handle `pageshow` with
   `event.persisted === true` and refresh or reload the page (web.dev bfcache
   article, https://web.dev/articles/bfcache).

## Fix

Add a small, testable bootstrap module that reloads the page when it is
restored from a frozen snapshot:

- `apps/web/src/bfcache-restore-reload.ts` exports
  `installBfcacheRestoreReload(options?)` returning an uninstall function. It
  listens for `pageshow` and reloads when `event.persisted === true`. That is
  the only reliable frozen-restore signal: the navigation type
  `back_forward` also covers cold history traversals and session-restored
  tabs, which load fresh and must NOT be reloaded a second time (a redundant
  reload would discard restored scroll/UI state on the common
  click-out-and-back flow). Fresh loads (`navigate`), manual refreshes
  (`reload`), and SPA soft navigations never reload.
- The module follows the `apps/web/src/vite-preload-recovery.ts` pattern:
  injected `target` and `reload` for testability.
- `apps/web/src/main.tsx` installs it at module scope next to
  `installVitePreloadRecovery()`, before React mounts.

No backend change. No new user-facing copy (no i18n impact).

## Tests

Unit tests, `apps/web/src/bfcache-restore-reload.test.ts` (vitest + happy-dom,
mirroring `vite-preload-recovery.test.ts` conventions):

- `pageshow` with `persisted === true` triggers the reload.
- `pageshow` with `persisted === false` does not reload — including on a cold
  `back_forward` traversal and on a manual refresh (regression: the
  navigation type must not be treated as a restore signal).
- An event carrying no `persisted` flag does not reload.
- The uninstall function removes the listener.

E2E test, `apps/web/e2e/tests/layout/bfcache-restore-reload.spec.ts`
(Playwright + real backend fixture, per `apps/web/e2e/README.md`):

- Load the app; assert the navigation type is `navigate` and no reload fired.
- Dispatch a real `pageshow` event with `persisted === true` from the page
  context; retry (navigation-race safe) until the navigation type becomes
  `reload` (the app reloaded through its own installed handler).
- The synthetic-signal approach is deliberate: the e2e backend page holds an
  open WebSocket, which in current Chrome makes a `no-store` page ineligible
  for bfcache on back/forward navigations, so `page.goBack()` would reload
  even without the fix (false positive). The browser's restore machinery is
  covered by the real-Chrome manual verification below.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run src/bfcache-restore-reload.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint` (changed files only)
- E2E: `cd apps/web && pnpm e2e:run tests/layout/bfcache-restore-reload.spec.ts`
- Real-Chrome manual verification (user or implementer, on a non-headless
  Chrome): open Kandev, archive a task, right-click the tab → Duplicate. The
  duplicated tab must reload and show the task as archived. This run is the
  MERGE BLOCKING gate (instrumentation checklist and decision rule in
  `task-01-reload-on-bfcache-restore.md`). If a current Chrome version
  restores without firing `pageshow` (no persisted signal), record it in the
  task Results and follow up with a WS-close-based fallback
  (reload on unexpected WS close when the navigation type is `back_forward`),
  gated on that evidence.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Reload on bfcache restore](task-01-reload-on-bfcache-restore.md)

Sequential. Single task; no parallel candidates.
