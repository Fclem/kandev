---
id: "33-mig-azure-sentry-slack"
title: "Migrate: azure-devops + sentry + slack"
status: pending
wave: 4
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 33: Migrate Azure DevOps + Sentry + Slack

Externalize per [task-07 playbook](task-07-migration-playbook.md). "Azure DevOps",
"Sentry", "Slack" and API-sourced names are domain data.

## Scope (edit ONLY these)
- `apps/web/components/azure-devops/**` (16)
- `apps/web/components/sentry/**` (21)
- `apps/web/components/slack/**` (1)
- `apps/web/app/azure-devops/**` (1)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
