---
id: "23-mig-office-app"
title: "Migrate: app/office (210)"
status: pending
wave: 3
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 23: Migrate app/office

Externalize per [task-07 playbook](task-07-migration-playbook.md). LARGE —
orchestrator may sub-partition by subdirectory (`agents/`, `workspace/`, `tasks/`,
`components/`, `setup/`, `projects/`, `routines/`, `inbox/`, `lib/`).

## Scope (edit ONLY these)
- `apps/web/app/office/**` (210)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to `app/office/**`. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
