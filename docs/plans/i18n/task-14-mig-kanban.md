---
id: "14-mig-kanban"
title: "Migrate: components/kanban (30) — playbook pilot"
status: done
wave: 2
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 14: Migrate components/kanban (playbook pilot)

Run FIRST as the pilot that validates the [task-07 playbook](task-07-migration-playbook.md)
end-to-end (typecheck, extract-without-commit, pseudo spot check, lint→0). Once
green, task 07 is marked `done` and the remaining M-* batches proceed.

## Scope (edit ONLY these)
- `apps/web/components/kanban/**` (30)

## Acceptance
- Playbook validated on a real batch; kanban board renders fully in pseudo with no
  plain-English chrome; typecheck clean; lint warnings → 0 for these files.

## Verification / Parallelism / Output
As task 10, scoped to `components/kanban/**`. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation (01–06); this task validates 07.
