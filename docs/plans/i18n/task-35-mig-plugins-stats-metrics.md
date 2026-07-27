---
id: "35-mig-plugins-stats-metrics"
title: "Migrate: plugins + stats + system metrics/health"
status: pending
wave: 4
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 35: Migrate plugins / stats / system metrics

Externalize per [task-07 playbook](task-07-migration-playbook.md). Plugin-provided
strings (from plugin manifests/runtime) are plugin data — do not wrap; wrap only
the host chrome around them.

## Scope (edit ONLY these)
- `apps/web/components/plugins/**` (10)
- `apps/web/app/stats/**` (9)
- `apps/web/components/system-metrics/**` (4)
- `apps/web/components/system-health/**` (1)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
