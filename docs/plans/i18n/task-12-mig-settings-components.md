---
id: "12-mig-settings-components"
title: "Migrate: components/settings (221)"
status: pending
wave: 2
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 12: Migrate components/settings

Externalize per [task-07 playbook](task-07-migration-playbook.md). Large batch —
orchestrator may sub-partition by subdirectory (`account/`, `system/`, `layouts/`,
top-level, etc.); sub-partitions still edit only files under
`apps/web/components/settings/**`.

## Scope (edit ONLY these)
- `apps/web/components/settings/**` (221) — EXCEPT `language-settings.tsx`
  (already localized in task 04).

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to `components/settings/**`. Follow the settings self-documenting
copy convention when rewording is unavoidable (prefer pure wrapping). Do NOT commit
`src/locales/**`.

## Dependencies
Wave 1 foundation.
