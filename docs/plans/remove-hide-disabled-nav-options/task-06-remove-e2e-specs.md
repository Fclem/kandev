---
id: "06-remove-e2e-specs"
title: "Remove hide-disabled E2E specs"
status: pending
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

Delete both spec files. Do not add replacement E2E — the removal's
observable contract is covered by the updated unit tests (task 03) and
the existing sidebar/settings navigation specs.

## Inputs

- Spec: Scenarios.
- Plan: E2E Tests.

## Output contract

Summary, files changed, exact commands run and outcomes, blockers/risks,
task/plan status update in the same conversation.

## Results

Pending.
