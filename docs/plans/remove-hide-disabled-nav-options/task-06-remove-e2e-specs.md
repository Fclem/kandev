---
id: "06-remove-e2e-specs"
title: "Remove hide-disabled E2E specs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/remove-hide-disabled-nav-options.md"
---

# Task 06: Remove hide-disabled E2E specs

Delete the two Playwright specs that exercised the removed settings.

- **Acceptance:**
  1. `apps/web/e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts`
     and
     `apps/web/e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts`
     are deleted.
  2. `grep -rn "hide-disabled-integrations-nav\|hide-disabled-agent-profiles-nav" apps/web` returns no hits (historical
     `docs/plans/**` command transcripts are implementation records and
     stay).
  3. `apps/web` typechecks (no spec imports anything from the deleted
     files; the shared `e2e/helpers/settings-menu.ts` stays — the
     settings-menu-mode spec still uses it).
- **Verification:**
  ```bash
  grep -rn "hide-disabled-integrations-nav\|hide-disabled-agent-profiles-nav" apps/web
  cd apps/web && pnpm run typecheck
  ```
- **Files likely touched:**
  - `apps/web/e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts` (delete)
  - `apps/web/e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts` (delete)
- **Dependencies:** None.
- **Parallelism:** parallel-safe (disjoint files; no shared config).

## Change

Delete both spec files (their setting-toggle scenarios are obsolete).
The post-removal contract is covered by three replacement specs added
during review round 1 (commit `c96721e43`):
`e2e/tests/integrations/disabled-integration-stays-in-nav.spec.ts`,
`e2e/tests/integrations/mobile-disabled-integration-stays-in-nav.spec.ts`,
and `e2e/tests/settings/disabled-profile-stays-in-nav.spec.ts`.

## Inputs

- Spec: Scenarios.
- Plan: E2E Tests.

## Output contract

Summary, files changed, exact commands run and outcomes, blockers/risks,
task/plan status update in the same conversation.

## Results

- Deleted both specs via `git rm`.
- `grep -rn "hide-disabled-integrations-nav\|hide-disabled-agent-profiles-nav" apps/web` → no matches (historical `docs/plans/**` transcripts untouched).
- Replacement specs added in review round 1 (commit `c96721e43`): desktop + mobile integration-stays-visible and disabled-profile-stays-listed; all pass (chromium 2/2, mobile-chrome 1/1).
