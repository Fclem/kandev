---
id: "31-mig-gitlab"
title: "Migrate: gitlab integration (components + app)"
status: pending
wave: 4
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 31: Migrate GitLab integration

Externalize per [task-07 playbook](task-07-migration-playbook.md). "GitLab" and
API-sourced names are domain data.

## Scope (edit ONLY these)
- `apps/web/components/gitlab/**` (59)
- `apps/web/app/gitlab/**` (2)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
