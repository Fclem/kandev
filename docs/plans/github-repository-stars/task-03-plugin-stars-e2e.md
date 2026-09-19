---
id: "03-plugin-stars-e2e"
title: "Add plugin repository star flows"
status: pending
wave: 3
depends_on:
  - 02-plugin-settings-controls
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.3
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.5
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.9
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.10
system_design:
  - ../../specs/integrations/system-design/github-repository-stars.md
---
# Task 03: Add plugin repository star flows

## Summary

Prove the user-visible star flow against existing Settings > Plugins fixtures on non-mobile fine-pointer desktop, mobile fine-pointer, and coarse-pointer tablet/desktop. Include marketplace, installed-row, installed-detail, and Canvases detail surfaces, connection-status hydration, Connect/Reconnect CTA and callback outcomes, confirmed mutation state, reload hydration, shared repository state, workspace/account scope changes, and silent provider failure without broadening plugin-management scope.

## In scope

- Playwright desktop fine-pointer, desktop coarse-pointer, mobile fine-pointer, and mobile coarse-pointer scenarios.
- Assertions for stable accessible names, `aria-pressed`, localized status feedback, causal waits, responsive 24px non-mobile fine-pointer/44px touch-context geometry, connection-status hydration, Connect/Reconnect navigation, accepted/declined/reduced-scope/failed callback outcomes, and no horizontal overflow. The desktop coarse-pointer scenario runs at desktop width with coarse pointer/touch emulation and asserts the 44px target, touch activation, busy-state geometry, star action, and non-interception of row/repository navigation.

## Out of scope

- New provider implementations or unit-test behavior.
- Product screenshots or unrelated marketplace documentation.

## Acceptance

- Desktop admin and authenticated workspace member contexts can star, unstar, reload, and observe confirmed state in marketplace, installed row, installed detail, Canvases detail, and `CanvasInstallDialog`. The member owns a distinct dedicated OAuth connection and has active membership for the target workspace.
- The member can hydrate and mutate repository stars on visible surfaces, while plugin-management controls (install, enable, disable, uninstall) remain absent; foreign-workspace member requests are denied before credential/provider resolution. Service and controller tests cover active-member allow and foreign-workspace deny.
- CanvasInstallDialog coverage uses its selected workspace, exercises star/unstar and provider-unknown omission, and proves selected workspace B remains isolated from global active workspace A.
- Public plugin marketplace documentation describes the new optional action and dedicated personal connection/reconnect behavior in `docs/public/plugins-marketplace.md`; the authoritative `docs/public/integrations.md` routing table also documents repository stars as a dedicated personal OAuth App exception with no PAT/CLI/App fallback, and the same page's provider overview no longer claims GitHub supports exactly one personal identity per user/workspace. `docs/public/feature-status.md` records the stars capability for the GitHub/plugin surfaces without overstating availability.
- The authenticated spec runs in a dedicated serial CI job with `KANDEV_FEATURES_AUTH=true`, uses `backendFixture` rather than `testPage`, `apiClient`, `seedData`, or `test-base` automatic setup/cleanup, explicitly calls `beforeAll backend.restart({ KANDEV_FEATURES_AUTH: "true" })` after baseline spawn, creates explicit admin/browser contexts and fixture data after restart, asserts the authenticated backend rejects unauthenticated access before exercising the flow, and calls `afterAll backend.restart()` teardown. The job uploads an always-present `blob-report-auth` artifact (the `blob-report-*` prefix the merge job globs at `.github/workflows/e2e-tests.yml:1216-1220`, matching the convention documented at `:938-940`) and is included in `e2e-report` and `e2e-gate` dependencies/results.
- The auth job is appended after the `e2e-gate:` job so every block sliced by `.github/scripts/e2e-tests-workflow-contract_test.py` (`desktop-e2e` -> `e2e-report`, `e2e-report` -> `e2e-gate`) keeps its asserted contents: `runs-on: ubuntu-latest`, no `setup-node`/`pnpm/action-setup`/`apt-get`/`sudo`/`rustup toolchain install` bootstrap, no `runner_plan`/`KANDEV_CI_*` reference, and unchanged `timeout-minutes: 35` in the `e2e` block. That contract test is updated in the same change if the placement or job set differs.
- The normal manifest includes all four normal contexts: `chromium` desktop fine-pointer, `desktop-coarse-pointer`, `mobile-chrome` coarse/mobile, and `mobile-fine-pointer`; shard-planner regressions prove catalog and manifest retention for both added projects, and fixture regressions prove desktop-width coarse emulation plus 767px fine-pointer context while preserving Pixel 5 mobile behavior.
- The mobile project matchers are mutually exclusive: `mobile-chrome` ignores `mobile-fine-pointer-*.spec.ts`, and the desktop-coarse-pointer project owns only its named coarse-pointer files, so each spec produces exactly one normal project/file catalog unit.
- E2E README and workflow gates document the four-project normal cohort, separate authenticated job, fine-pointer routing, desktop-coarse-pointer routing, and routing cohort isolation.

## ASCII UI preview

See [plan UI-01](plan.md#ui-01-marketplace-entry-with-star-state-desktop-phone-and-coarse-pointer).

## Verification

```bash
(cd apps/web && pnpm e2e:run --project auth e2e/tests/auth/plugin-repository-stars.spec.ts)
(cd apps/web && pnpm e2e:run --project desktop-coarse-pointer e2e/tests/settings/desktop-coarse-pointer-plugin-repository-stars.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-plugin-repository-stars.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-fine-pointer e2e/tests/settings/mobile-fine-pointer-plugin-repository-stars.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

The authenticated desktop spec runs in a dedicated gated auth job with `KANDEV_FEATURES_AUTH=true`; it uses `backendFixture`, explicitly restarts with that override after baseline spawn, creates admin/browser contexts after restart, avoids `testPage`/`apiClient`/`seedData`/`test-base` automatic cleanup, and tears down with `afterAll backend.restart()`. The mobile project matchers are mutually exclusive, and the fine-pointer/Pixel 5 commands are separate. Public-doc validators are required for the marketplace documentation change.

## Files likely touched

- `apps/web/e2e/tests/auth/plugin-repository-stars.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-plugin-repository-stars.spec.ts` (new)
- `apps/web/e2e/tests/settings/desktop-coarse-pointer-plugin-repository-stars.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-fine-pointer-plugin-repository-stars.spec.ts` (new)
- `apps/web/e2e/playwright.config.ts`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/fixtures/test-base.test.ts` (new)
- `apps/web/e2e/scripts/plan-shards.ts`
- `apps/web/e2e/scripts/*plan-shards*.test.ts`
- `apps/web/e2e/README.md`
- `.github/workflows/e2e-tests.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`
- `docs/public/integrations.md`
- `docs/public/feature-status.md`
- `docs/public/plugins-marketplace.md`

## Dependencies

Task 02 frontend controls.

## Risks

- GitHub API calls need deterministic fixture stubs; tests must not use a real GitHub account.
- Authenticated and mobile projects use different fixtures and must be run separately.
- Coarse-pointer emulation must be explicit; width alone is not sufficient to prove touch geometry.

## Parallelism

`sequential`

## Inputs

- Task 02 rendered control and test IDs.
- Existing authenticated and mobile plugin settings fixtures.
- Public plugin marketplace documentation.

## Results

Pending.
