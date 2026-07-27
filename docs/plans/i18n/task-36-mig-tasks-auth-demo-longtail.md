---
id: "36-mig-tasks-auth-demo-longtail"
title: "Migrate: app/tasks + auth + demo + long-tail components"
status: pending
wave: 4
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 36: Migrate app/tasks + auth + long tail

Externalize per [task-07 playbook](task-07-migration-playbook.md). This batch
sweeps the remaining directories so NO user-facing literal is left outside the
translation layer.

## Scope (edit ONLY these)
- `apps/web/app/tasks/**` (13), `apps/web/app/t/**` (1), `apps/web/app/actions/**` (6)
- `apps/web/app/auth/**` (3), `apps/web/app/demo/**` (4)
- `apps/web/components/watches/**`, `release-notes/**`, `agent/**`, `search/**`,
  `onboarding/**`, `session/**`
- Any `apps/web/components/*` file or dir not claimed by tasks 10–35 (verify none
  remain — this is the mop-up batch).

## Acceptance
- As task 10. ADDITIONALLY: after this batch, running the eslint
  `no-unlocalized-strings` rule across ALL of `apps/web/{components,app}` reports
  no un-allowlisted user-facing literals (feeds task 40's flip to error).

## Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
