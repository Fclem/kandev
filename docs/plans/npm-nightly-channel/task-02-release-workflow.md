---
id: "02-release-workflow"
title: "Scheduled release workflow"
status: completed
wave: 2
depends_on: ["01-version-and-publisher"]
plan: "plan.md"
spec: "../../specs/npm-nightly-channel/spec.md"
---

# Task 02: Scheduled release workflow

- **Acceptance:** `release.yml` schedules exact noon UTC and skips unchanged/already-published
  commits before building.
- **Acceptance:** Schedule reaches only web/runtime builds and nightly npm publication; every stable
  GitHub/Desktop/GHCR/Homebrew mutation remains dispatch-only.
- **Acceptance:** Publication uses the exact checked-out SHA and is serialized with stable release.
- **Acceptance:** The locked publish step skips when Stable or the observed Nightly tag moved while
  bundles were building, preventing stale or backward tag movement.
- **Acceptance:** An older scheduled rerun is skipped before building when the published Nightly
  commit is the same or newer; unresolvable or divergent tag history fails closed.
- **Verification:** `python3 .github/scripts/release-workflow-contract_test.py`
- **Verification:** `node --test scripts/release/npm-view-version.test.mjs`
- **Verification:** `cd apps && pnpm --filter kandev exec vitest run src/release-config.test.ts`
- **Verification:** `make test-scripts`
- **Files likely touched:** `.github/workflows/release.yml`,
  `.github/scripts/release-workflow-contract_test.py`, `apps/cli/src/release-config.test.ts`,
  `scripts/release/npm-view-version.sh`, `scripts/release/npm-view-version.test.mjs`, `Makefile`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential because the workflow consumes Task 01's interface.
- **Inputs:** spec schedule/publication scenarios; existing `prepare`, `build-web`,
  `build-bundles`, and `publish-npm` jobs.
- **Risks:** an incomplete event gate could trigger stable side effects from cron.

## Verification results

- `python3 .github/scripts/release-workflow-contract_test.py` — passed, 20 tests.
- `node --test scripts/release/npm-view-version.test.mjs` — passed, 3 tests.
- `cd apps && pnpm --filter kandev exec vitest run src/release-config.test.ts` — passed, 12 tests.
- `make test-scripts` — passed, including both release workflow suites.
