---
id: "10-mig-shared-toplevel"
title: "Migrate: components top-level + shared/routing/theme/icons"
status: pending
wave: 2
depends_on: ["07-migration-playbook"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 10: Migrate shared / top-level components

Externalize every user-facing string in these directories per
[task-07 playbook](task-07-migration-playbook.md).

## Scope (edit ONLY these)
- `apps/web/components/*.tsx` (top-level, ~95 files)
- `apps/web/components/shared/**`
- `apps/web/components/routing/**`
- `apps/web/components/theme/**`
- `apps/web/components/icons/**`

## Acceptance
- All user-facing strings wrapped with `<Trans>`/`t` per the playbook; domain
  data and allowlisted literals untouched.
- `pnpm run typecheck` clean; pseudo-locale spot check clean for these surfaces;
  `no-unlocalized-strings` warnings → 0 for these files (allowlist aside).

## Verification
See task-07 "Batch verification". Do NOT commit `src/locales/**`.

## Dependencies
Wave 1 foundation (tasks 01–07).

## Parallelism
parallel-safe (disjoint dirs; no shared config).

## Output contract
Per task-07 output contract; mark `done`, update `plan.md`.
