---
id: "04-e2e-coverage"
title: "Desktop and mobile E2E coverage"
status: done
wave: 3
depends_on: ["02-backend-issue-workflow", "03-frontend-dialog-and-mobile"]
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
acceptance_criteria:
  - AC-WORKSPACES-IMPROVE-KANDEV-001.1
  - AC-WORKSPACES-IMPROVE-KANDEV-001.3
  - AC-WORKSPACES-IMPROVE-KANDEV-001.4
  - AC-WORKSPACES-IMPROVE-KANDEV-001.5
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
---

# Task 04: Desktop and mobile E2E coverage

## Acceptance

- Desktop Playwright proves intro dismissal persistence, direct reopen,
  report-only task creation, and EMU implementation-versus-issue gating. Seed an
  existing dedicated workspace in the dismissal/reopen scenario so it tests
  direct task creation; Task 06 covers the missing-workspace choice gate.
- Mobile Playwright opens the feature through **Open menu** → **Improve
  Kandev**, proves 44px touch targets, reaches **Open issue**, and detects no
  document horizontal overflow.
- Both specs pass against the production Vite build served by the Go backend.

## Verification

The first managed `pnpm e2e:run` command rebuilds the backend binary, E2E web bundle, and E2E plugin UI/package. The mobile command intentionally uses `--no-build` to reuse those artifacts immediately afterward; do not run it alone after changing source files. See the [runner implementation](../../../apps/web/e2e/scripts/run-e2e.sh) and [E2E documentation](../../../apps/web/e2e/README.md).

```bash
(cd apps/web && pnpm e2e:run tests/improve-kandev.spec.ts)
(cd apps/web && pnpm e2e:run --no-build tests/mobile-improve-kandev.spec.ts -- --project=mobile-chrome)
```

## Files likely touched

- `apps/web/e2e/tests/improve-kandev.spec.ts`
- `apps/web/e2e/tests/mobile-improve-kandev.spec.ts`

## Dependencies

Tasks 02 and 03.

## Parallelism

Sequential. These specs share the Improve Kandev route mocks and production
build.

## Inputs

- All spec scenarios for dismissal, issue reporting, fork restrictions, auth,
  and mobile access.
- Plan: **E2E Tests** and **Continuation Snapshot**.
- Existing fixture APIs in `apps/web/e2e/helpers/api-client.ts`.

## Completed implementation notes

- Desktop tests for persistence, issue task workflow, and revised EMU behavior
  are already drafted. The focused persistence and issue-submission tests
  passed individually.
- The mobile spec is drafted. Its initial sidebar-based path failed because the
  desktop footer is hidden on phones; the test now uses the mobile menu.
- The mobile utility entry was rebuilt and verified after the initial sidebar
  path was corrected to use the native mobile menu.

## Recorded verification

- `cd apps/web && pnpm e2e:run tests/improve-kandev.spec.ts` — passed (7 desktop tests).
- `cd apps/web && pnpm e2e:run --no-build tests/mobile-improve-kandev.spec.ts -- --project=mobile-chrome` — passed (1 mobile test).
- The first rebuilt mobile run caught a 43px preference target; the intro
  checkbox label was increased to `min-h-12`, then the rebuilt mobile run
  passed with the required touch target and no horizontal overflow.

Task 04 is complete. Continue with Tasks 06–07, then Task 05 for repository-wide
verification and the final commit.

## Risks

- Route mocks must return both workflow IDs, and the submitted issue workflow
  must exist in the E2E backend before asserting its start step.
- Do not increase Playwright timeouts; inspect error context if the mobile
  drawer-to-dialog handoff races.

## Output contract

The desktop and mobile Playwright suites prove the shared flow, touch-target
requirements, report-only workflow, and mobile containment behavior.

Update this task and `plan.md`, report both exact commands and outcomes, include
fresh failure-artifact paths if any, and state whether rendered mobile
containment/touch behavior is proven.
