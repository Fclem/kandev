---
id: "04-delete-orphaned-hooks"
title: "Delete orphaned hooks and shared primitive"
status: pending
wave: 2
depends_on: ["01-remove-integrations-setting", "02-remove-agent-profiles-setting", "03-remove-nav-filtering"]
plan: "plan.md"
spec: "../../specs/ui/remove-hide-disabled-nav-options.md"
---

# Task 04: Delete orphaned hooks and shared primitive

Delete the two domain hooks, the shared `useLocalStorageBoolean`
primitive, and their tests — everything left over once no consumer
imports them.

- **Acceptance:**
  1. The six files below are deleted.
  2. `grep -rn "useHideDisabledIntegrationsInNav\|useHideDisabledAgentProfilesInNav\|useLocalStorageBoolean\|hideDisabledInNav" apps/web` returns no hits except the updated comment in
     `apps/web/hooks/local-storage-mock.test-helpers.ts` (the helper
     itself stays — the per-integration enabled-hook tests still use
     it).
  3. `apps/web` typechecks clean (no dangling imports anywhere).
- **Verification:**
  ```bash
  grep -rn "useHideDisabledIntegrationsInNav\|useHideDisabledAgentProfilesInNav\|useLocalStorageBoolean\|hideDisabledInNav" apps/web
  cd apps/web && pnpm run typecheck
  ```
- **Files likely touched:**
  - `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts` (delete)
  - `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.test.ts` (delete)
  - `apps/web/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.ts` (delete)
  - `apps/web/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.test.ts` (delete)
  - `apps/web/hooks/use-local-storage-boolean.ts` (delete)
  - `apps/web/hooks/use-local-storage-boolean.test.ts` (delete)
  - `apps/web/hooks/local-storage-mock.test-helpers.ts` (comment only)
- **Dependencies:** 01, 02, 03 (their deletions/rewrites are what orphan
  these modules).
- **Parallelism:** sequential.

## Change

1. Delete the six hook/primitive files.
2. `local-storage-mock.test-helpers.ts`: update the doc comment so it no
   longer claims to serve the two nav-visibility toggles.

## Inputs

- Spec: Data model.
- Plan: Frontend > Dead code.

## Output contract

Summary, files changed, exact commands run and outcomes, blockers/risks,
task/plan status update in the same conversation.

## Results

Pending.
