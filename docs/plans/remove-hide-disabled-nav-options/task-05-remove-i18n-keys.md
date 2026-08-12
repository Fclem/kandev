---
id: "05-remove-i18n-keys"
title: "Remove hide-disabled i18n keys"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/remove-hide-disabled-nav-options.md"
---

# Task 05: Remove hide-disabled i18n keys

Remove the four `hideDisabled*FromNav*` keys from every locale catalog.

- **Acceptance:**
  1. `hideDisabledAgentProfilesFromNav`,
     `hideDisabledAgentProfilesFromNavDescription`,
     `hideDisabledIntegrationsFromNav`, and
     `hideDisabledIntegrationsFromNavDescription` are absent from
     `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json`.
  2. `pnpm run i18n:check` and `pnpm run i18n:ratchet` pass from
     `apps/web`.
- **Verification:**
  ```bash
  cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
  ```
- **Files likely touched:**
  - `apps/web/src/locales/en/settings.json`
  - `apps/web/src/locales/pseudo/settings.json`
  - `apps/web/src/locales/pt-pt/settings.json`
  - `apps/web/src/locales/zh-cn/settings.json`
- **Dependencies:** None.
- **Parallelism:** parallel-safe (disjoint files; no shared config).

## Change

Delete the four keys from each of the four catalogs (they are adjacent
lines — `integrationDescriptionSentry` … `invalidJson` — keep the
surrounding keys and alphabetical order).

## Inputs

- Spec: What bullets 1, 2 (the removed rows' copy).
- Plan: Frontend > i18n.

## Output contract

Summary, files changed, exact commands run and outcomes, blockers/risks,
task/plan status update in the same conversation.

## Results

- Removed the 4 `hideDisabled*FromNav*` keys from `en`, `pseudo`, `pt-pt`, `zh-cn` `settings.json`.
- `cd apps/web && pnpm run i18n:check` → all checks pass; `pnpm run i18n:ratchet` → 0 added, clean.
