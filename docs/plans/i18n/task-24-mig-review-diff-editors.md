---
id: "24-mig-review-diff-editors"
title: "Migrate: review + diff + editors"
status: pending
wave: 3
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 24: Migrate review / diff / editors

Externalize per [task-07 playbook](task-07-migration-playbook.md). Diff CONTENT
(code lines, file paths) is domain data — never wrap; only chrome (headers,
buttons, comment UI, empty/error states) is translated.

## Scope (edit ONLY these)
- `apps/web/components/review/**` (32)
- `apps/web/components/diff/**` (38)
- `apps/web/components/editors/**` (41)

## Acceptance / Verification / Parallelism / Output
As task 10, scoped to the dirs above. Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation.
