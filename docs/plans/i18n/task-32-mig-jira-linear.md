---
id: "32-mig-jira-linear"
title: "Migrate: jira + linear (components + app)"
status: pending
wave: 4
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 32: Migrate Jira + Linear integrations

Externalize per [task-07 playbook](task-07-migration-playbook.md). "Jira",
"Linear", issue keys/titles/statuses from the API are domain data.

## Scope (edit ONLY these)
- `apps/web/components/jira/**` (29)
- `apps/web/components/linear/**` (16)
- `apps/web/app/jira/**` (2)
- `apps/web/app/linear/**` (2)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
