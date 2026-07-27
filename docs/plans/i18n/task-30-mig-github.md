---
id: "30-mig-github"
title: "Migrate: github integration (components + app)"
status: pending
wave: 4
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 30: Migrate GitHub integration

Externalize per [task-07 playbook](task-07-migration-playbook.md). "GitHub", PR
titles, branch/repo names, and CI check names from the API are domain data — do
not wrap; wrap chrome only.

## Scope (edit ONLY these)
- `apps/web/components/github/**` (99)
- `apps/web/app/github/**` (2)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
