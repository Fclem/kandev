---
id: "34-mig-integrations-automations"
title: "Migrate: integrations + vcs + workspace-source-picker + automations"
status: pending
wave: 4
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 34: Migrate integrations shell + automations

Externalize per [task-07 playbook](task-07-migration-playbook.md).

## Scope (edit ONLY these)
- `apps/web/components/integrations/**` (19)
- `apps/web/components/vcs/**` (7)
- `apps/web/components/workspace-source-picker/**` (5)
- `apps/web/components/automations/**` (28)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
