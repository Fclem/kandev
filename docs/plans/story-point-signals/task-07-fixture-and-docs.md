---
id: "07-fixture-and-docs"
title: "Fixture plugin, E2E, and documentation"
status: completed
wave: 5
depends_on:
  - "02-activity-reporter"
  - "06-change-request-feedback"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-EFFORT-SIGNALS-005
acceptance_criteria:
  - AC-PLUGINS-EFFORT-SIGNALS-005.2
  - AC-PLUGINS-EFFORT-SIGNALS-005.3
  - AC-PLUGINS-EFFORT-SIGNALS-005.4
system_design:
  - ../../specs/plugins/system-design/effort-signals-host.md
---

# Task 07: Fixture plugin, E2E, and documentation

## Summary

Prove the contract with a fixture plugin that uses only public SDK surfaces to
read every new signal, store a derived value in plugin state, and render a
route, task panel, and card indicator with `host.ui` primitives. A Playwright
test focuses a real page and reads the recorded time through the fixture.
Document the reads in the public plugin docs.

## In scope

- Extend the single packaged e2e fixture plugin (`apps/backend/Makefile`
  `e2e-plugin-package` target, `cmd/plugin-fixture`, UI source under
  `apps/web/e2e/fixtures/plugins/prompt-history-plugin/`) with backend reads,
  state, `summary` and `estimate` actions, and a manifest declaring
  `session_activity`. The capability raises the manifest's minimum version.
  Update `apps/web/scripts/write-e2e-plugin-identity.mjs` to validate and record
  the manifest's actual `min_kandev_version`, and
  `apps/web/e2e/global-setup.ts` to compare the identity against that value
  instead of a pinned version. The Make target already invokes the identity
  script and is listed as affected.
- Playwright desktop spec: the flag is set through the spec's environment and
  visibility is explicit. Perform an explicit `pointerdown` on an inert task
  surface immediately before the capture window, keep the page focused about
  20 s for two 15 s beats, and call the fixture summary action to capture the
  resulting `ActiveMs` baseline. The summary action must make a fresh
  `Sessions().UserActivity` Host read on every invocation, not return persisted
  plugin state. Then open a second page and bring it to front; assert the first
  page is hidden, wait 30 s (two beat periods), call summary again and assert
  the fresh Host `ActiveMs` is unchanged. The 60 s idle stop is covered by the
  task 02 hook test, not here. A Pixel 5 spec
  `mobile-effort-signals-plugin.spec.ts` covers the route, panel, and badge.
- `docs/public/plugins-manifest.md`, `docs/public/plugins-authoring.md`,
  `docs/plans/plugins/PLUGIN-API.md` updates: explicitly document every new RPC
  and read/resource name, bounds, failure states, and minimum version. Document
  `PullRequestsPartial` presence semantics: true means partial, false means all
  sources were read, and absent means an older Host did not report completeness.
  AC 005.4 is verified by a manual checklist against the proto and manifest plus
  the documentation, recorded in Results; the public-doc validator checks
  structure and links, not whether each API is described.

## Out of scope

- The real estimator plugin and any new core slot.

## Acceptance

- The fixture feedback view renders provider-specific availability and the
  returned feedback items. Task 06's backend test verifies partial provider
  failure while retaining successful-provider items.
- The desktop Playwright flow produces an explicit interaction, reads a fresh
  Host activity aggregate while focused, then backgrounds that page and verifies
  through another fresh Host read that `ActiveMs` remains unchanged for two beat
  periods.
- Route, panel, and badge render on desktop and Pixel 5 without horizontal
  overflow.
- Spec lint and documentation checks pass; AC 005.4's manual content checklist
  records each new RPC/resource checked against the implementation contract.

## Verification


```bash
(cd apps/backend && go test ./cmd/plugin-fixture/... ./internal/plugins/...)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/plugins/effort-signals-plugin.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-effort-signals-plugin.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check -- docs
```

## Files likely touched

- `apps/backend/cmd/plugin-fixture/**`, `apps/backend/Makefile` fixture targets
- `apps/web/e2e/fixtures/plugins/prompt-history-plugin/`,
  `apps/web/scripts/write-e2e-plugin-identity.mjs`,
  `apps/web/e2e/global-setup.ts`
- `apps/web/e2e/tests/plugins/effort-signals-plugin.spec.ts`, `mobile-effort-signals-plugin.spec.ts`, and capability-pinning plugin specs
- `docs/public/plugins-manifest.md`, `docs/public/plugins-authoring.md`, `docs/plans/plugins/PLUGIN-API.md`

## Dependencies

Tasks 02 and 06.

## Risks

- Fixture package must be rebuilt by `e2e-plugin-ui` before E2E.

## Parallelism

`sequential`

## Inputs

- Design section: Reference plugin design. Pattern: existing prompt-history
  fixture under `apps/web/e2e/fixtures/plugins/`.

## Results

Completed. Fixture plugin reads only public Go SDK surfaces (summary, sessions, activity, turns, code stats, human-filtered messages, live feedback, usage) and follows pages. `summary` makes fresh Host reads on every call. `estimate` persists only derived `active_ms`/`human_prompts` and `fixture-count-v1` estimate fields, never feedback bodies or logins. Route `/plugins/e2e-effort`, task panel and card indicator use public host APIs. Manifest floor 0.97.0 with `session_activity`, `change_requests`, `usage`. The identity writer records the manifest's actual minimum (was pinned at 0.91.1).

Behavioral RED then GREEN: `TestEffortSignalsActionsRequireHost` (summary/estimate dispatch); `TestEffortSignalsFreshReadAndDerivedState` (private feedback and login were persisted; fixed to persist derived features only; fresh Host `active_ms` rises 15000 to 30000); Node identity test (pinned 0.91.1 rejected actual 0.97.0; now passes). Two incidental action-order assertions were removed, not repinned.

Acceptance amendment: the work order's "bring second page to front, assert first hidden" step cannot establish hidden state in headless Chromium (`bringToFront` leaves the first page visible and focused). Per the parent decision, the E2E uses visibility/focus emulation in that test only.

Verification (parent rerun): `(cd apps/backend && go test -count=1 ./cmd/plugin-fixture/ -run TestEffortSignals)` PASS; `node --test scripts/write-e2e-plugin-identity.test.mjs` 1/1 PASS; `node scripts/validate-public-docs.mjs` 47 pages valid; `python3 scripts/list-docs.py validate` 348 decisions, 1333 specs; `python3 scripts/lint-spec-files.py --all` PASS. Child-reported: `make fmt lint` 0 issues; `go test ./cmd/plugin-fixture/... ./internal/plugins/...` and `./internal/plugins/... ./pkg/pluginsdk/...` PASS; eslint on E2E files PASS; vitest global-setup 30/30; public-doc test 62/62.

E2E (observed, both projects PASS, retries 0). Desktop chromium: `AC-PLUGINS-EFFORT-SIGNALS-005.2: fresh Host activity stops under emulated visibility and resumes`, 1 passed (1.3m). Command, from apps/web: `TMPDIR=/tmp mise exec -- pnpm e2e:run --no-build tests/plugins/effort-signals-plugin.spec.ts -- --retries=0`. Mobile-chrome (Pixel 5): `AC-PLUGINS-EFFORT-SIGNALS-005.3: effort route panel and card badge fit Pixel 5`, 1 passed (22.4s). Command: `TMPDIR=/tmp mise exec -- pnpm e2e:run --no-build --project mobile-chrome tests/plugins/mobile-effort-signals-plugin.spec.ts -- --retries=0`. Evidence record: `/home/clem/.kandev/tasks/investigate-potentia_cw2zw7bk/task07-e2e-verification.txt`. Retained screenshot: `apps/web/e2e/test-results/plugins-mobile-effort-sign-296fe--and-card-badge-fit-Pixel-5-mobile-chrome/effort-mobile.png`. The desktop screenshot was removed by managed cleanup.

Deviation: the prescribed desktop command failed in task setup (POST /api/v1/tasks, 500), before reporter assertions. Cause: the seed repository inherited a TMPDIR under `.kandev/tasks/` and was correctly rejected as a Kandev task worktree. This is a setup failure, not a reporter failure. Artifacts were built under the prescribed TMPDIR/GOCACHE; both specs then ran `--no-build` with browser-only `TMPDIR=/tmp`, passing normal global-setup freshness and fixture identity checks. No freshness override, production guard change, or shared fixture change. The original backend attachment was removed by managed cleanup; its diagnostic is kept in the evidence record. The literal combined mobile build+run command was not run.

Limitation (verbatim): this proves the reporter's gating logic under emulated visibility/focus, not native background throttling in a real browser window.

AC 005.4 manual checklist (child-reported, checked against proto, SDK and manifest): every new RPC and resource is documented in the public authoring, manifest and PLUGIN-API docs, including bounds, failure states and minimum version. `PullRequestsPartial` tri-state is documented.

PostgreSQL not run: `KANDEV_TEST_POSTGRES_DSN` unset. Native-background throttling and the literal combined mobile build/run remain unclaimed.

Amendment: the fixture manifest floor is now 0.98.0 (see Task 04 amendment). The production estimator plugin lives in the external repository `Fclem/kandev-plugin-story-points`.
