---
id: "21-mig-task-b"
title: "Migrate: components/task subset B (top-level files, 318)"
status: pending
wave: 3
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 21: Migrate components/task — subset B (top-level)

Externalize per [task-07 playbook](task-07-migration-playbook.md). LARGE — the
orchestrator MUST sub-partition the 318 top-level files by filename range (e.g.
`[a-f]*`, `[g-m]*`, `[n-s]*`, `[t-z]*`) into disjoint sub-batches that still edit
only top-level files of `components/task/`.

## Scope (edit ONLY these)
- `apps/web/components/task/*.tsx` and `apps/web/components/task/*.ts` (top-level
  only — NOT the subdirectories owned by tasks 20 and 22)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to top-level `components/task` files. Do NOT commit
`src/locales/**`. Do NOT edit `components/task/*/` subdirs.

## Dependencies
Wave 1 foundation.
