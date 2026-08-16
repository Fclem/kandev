---
id: "01-reload-on-bfcache-restore"
title: "Reload on bfcache restore"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/fix-duplicated-tab-stale-data/spec.md"
---

# Task 01: Reload on BFCache Restore

## Acceptance

- `apps/web/src/bfcache-restore-reload.ts` exports `installBfcacheRestoreReload`
  which reloads the page on a `pageshow` event when `event.persisted === true`
  OR the current document navigation type is `back_forward`, does nothing for
  normal loads (`navigate`/`reload`), and returns an uninstall function.
- `apps/web/src/main.tsx` installs it at module scope next to
  `installVitePreloadRecovery()`, before React mounts.
- Unit tests cover: persisted restore reloads; persisted=false +
  `back_forward` reloads; persisted=false + `navigate` does not reload;
  persisted=false + `reload` does not reload; missing Navigation Timing API
  degrades to persisted-only without throwing; uninstall removes the listener.
- E2E test dispatches a real persisted `pageshow` in the running app and
  asserts the page reloads (navigation type becomes `reload`); a normal load
  does not reload.
- No user-facing copy added (no i18n ratchet impact).

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run src/bfcache-restore-reload.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e:run tests/layout/bfcache-restore-reload.spec.ts
```

Manual, on a real (non-headless) Chrome: archive a task, right-click the
Kandev tab → Duplicate; the duplicated tab must reload and show the task as
archived. Record the outcome in Results.

## Files Likely Touched

- `apps/web/src/bfcache-restore-reload.ts` (new)
- `apps/web/src/bfcache-restore-reload.test.ts` (new)
- `apps/web/src/main.tsx` (wire install call)
- `apps/web/e2e/tests/layout/bfcache-restore-reload.spec.ts` (new)

## Dependencies

None.

## Parallelism

Sequential. Single task; no parallel candidates.

## Inputs

- Repair spec: `docs/specs/fix-duplicated-tab-stale-data/spec.md`.
- Existing pattern: `apps/web/src/vite-preload-recovery.ts` and its test
  (injected `target`/`reload`, defensive storage reads).
- Bootstrap entry: `apps/web/src/main.tsx` (`installVitePreloadRecovery()`
  call site).

## Risks

- Chrome duplicate-tab restore event delivery may vary by version. The
  `back_forward` navigation-type check covers state-clone restores where
  `persisted` is false. If a real-Chrome duplicate test shows no reload,
  follow up with the WS-close fallback described in the plan (reload on
  unexpected WS close when the navigation type is `back_forward`), gated on
  that evidence; do not add it speculatively.
- jsdom lacks `PageTransitionEvent` and Navigation Timing; tests inject the
  `persisted` flag on a plain `Event` and pass a fake navigation-type reader.
- E2E must use the synthetic persisted-`pageshow` signal (see plan): with an
  open WebSocket, current Chrome does not bfcache `no-store` pages on
  back/forward, so `page.goBack()` would pass trivially. Use `expect.poll`
  with default timeouts; no sleeps.

## Output Contract

Report the root-cause evidence summary, files changed, exact tests run
(unit, typecheck, lint, E2E), the real-Chrome duplicate-verification outcome,
task and plan status updates, and any follow-up decision on the WS-close
fallback.

## Results

- Added `apps/web/src/bfcache-restore-reload.ts`: a `pageshow` handler that
  reloads the page when `event.persisted === true` or the navigation type is
  `back_forward`, with a guarded navigation-type reader and an uninstall
  function (pattern: `vite-preload-recovery.ts`).
- Wired `installBfcacheRestoreReload()` into `apps/web/src/main.tsx` at module
  scope next to `installVitePreloadRecovery()`, before React mounts.
- Added `apps/web/src/bfcache-restore-reload.test.ts` (7 tests) and
  `apps/web/e2e/tests/layout/bfcache-restore-reload.spec.ts`.
- Checks passed:
  - `cd apps && pnpm --filter @kandev/web test -- --run src/bfcache-restore-reload.test.ts`
    (7/7 tests).
  - `cd apps/web && pnpm run typecheck` (clean).
  - `cd apps/web && pnpm exec eslint src/bfcache-restore-reload.ts
    src/bfcache-restore-reload.test.ts src/main.tsx
    e2e/tests/layout/bfcache-restore-reload.spec.ts` (clean).
  - `cd apps/web && pnpm e2e:run tests/layout/bfcache-restore-reload.spec.ts`
    (1/1; the first attempt's assertion polled `performance` nav type during
    the in-flight reload and hit the destroyed execution context — the reload
    itself fired; the assertion was made navigation-race safe with
    `expect(...).toPass` catching context-destroyed errors).
- Real-Chrome duplicate-tab verification (non-headless, user's environment)
  remains outstanding: archive a task, right-click the tab → Duplicate, expect
  the duplicated tab to reload and show the task as archived. If a Chrome
  version restores without the `pageshow`/`back_forward` signals, add the
  plan-documented WS-close fallback gated on that evidence; none was needed
  for the signals observed here.
