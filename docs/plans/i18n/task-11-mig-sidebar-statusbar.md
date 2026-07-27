---
id: "11-mig-sidebar-statusbar"
title: "Migrate: app-sidebar + app-status-bar + quick-chat"
status: pending
wave: 2
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 11: Migrate sidebar / status bar / quick-chat

Externalize per [task-07 playbook](task-07-migration-playbook.md).

## Scope (edit ONLY these)
- `apps/web/components/app-sidebar/**` (43)
- `apps/web/components/app-status-bar/**` (15)
- `apps/web/components/quick-chat/**` (15)
- `apps/web/components/config-chat/**` (6)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
