---
id: "40-lint-flip-e2e-coverage"
title: "Close-out: flip lint to error, final extract, e2e coverage"
status: pending
wave: 5
depends_on: ["10-mig-shared-toplevel","11-mig-sidebar-statusbar","12-mig-settings-components","13-mig-settings-app","14-mig-kanban","20-mig-task-a","21-mig-task-b","22-mig-task-c","23-mig-office-app","24-mig-review-diff-editors","30-mig-github","31-mig-gitlab","32-mig-jira-linear","33-mig-azure-sentry-slack","34-mig-integrations-automations","35-mig-plugins-stats-metrics","36-mig-tasks-auth-demo-longtail"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 40: Close-out

Run AFTER every migration batch reports clean. Locks in the anti-regression
guarantees and regenerates the committed catalog once.

## Acceptance
- `apps/web/eslint.config.mjs`: `@lingui/no-unlocalized-strings` flipped from
  `"warn"` to `"error"` (and any temporary migration-window `--max-warnings`
  relaxation from task 06 removed). `pnpm lint` passes with the rule as error.
- `cd apps/web && pnpm extract && pnpm compile` run ONCE; the resulting
  `src/locales/en/messages.po` and `src/locales/pseudo/messages.po` (+ compiled
  output) committed — this is the single catalog commit for the whole sweep.
- CI extract-check (task 06) passes on the committed catalog.
- E2E: `apps/web/e2e/i18n/language-switch.spec.ts` and
  `apps/web/e2e/i18n/pseudo-coverage.spec.ts` implemented and passing; the
  coverage spec crawls the key screens under pseudo and asserts no plain-ASCII
  user-facing text remains in the audited containers.
- Spec status → `shipped`; plan status → `done`; `docs/specs/INDEX.md` updated.

## Verification
- `cd apps/web && pnpm lint && pnpm run typecheck && pnpm --filter @kandev/web test`
- `cd apps/web && pnpm e2e -- e2e/i18n`
- `git status` shows exactly one catalog change set.

## Files likely touched
- `apps/web/eslint.config.mjs`, `apps/web/package.json` (lint script)
- `apps/web/src/locales/**` (regenerated, committed here)
- `apps/web/e2e/i18n/*.spec.ts` (new)
- `docs/specs/platform/i18n.md`, `docs/specs/INDEX.md`, `docs/plans/i18n/plan.md`

## Dependencies
All migration tasks (10–36).

## Parallelism
sequential (touches shared config + catalog).

## Output contract
Summary, final lint/typecheck/test/e2e results, catalog commit confirmation;
mark `done`, update `plan.md` and spec status.
