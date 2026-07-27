---
id: "20-mig-task-a"
title: "Migrate: components/task subset A (chat/share/inspector/document)"
status: pending
wave: 3
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 20: Migrate components/task — subset A

Externalize per [task-07 playbook](task-07-migration-playbook.md). Chat renders
user/agent MESSAGE CONTENT — that is domain data and must NOT be wrapped; only the
UI chrome around it (labels, buttons, tooltips, empty states, toasts) is translated.

## Scope (edit ONLY these)
- `apps/web/components/task/chat/**` (195)
- `apps/web/components/task/share/**` (5)
- `apps/web/components/task/inspector/**` (2)
- `apps/web/components/task/document/**` (1)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Extra care: never translate transcript/
message/diff content. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
