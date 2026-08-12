---
id: "02-remove-agent-profiles-setting"
title: "Remove agent-profiles hide-disabled setting"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/remove-hide-disabled-nav-options.md"
---

# Task 02: Remove agent-profiles hide-disabled setting

Remove the "Hide disabled agent profiles from left panel navigation"
row from the agents settings page and delete its component.

- **Acceptance:**
  1. `/settings/agents` (`apps/web/app/settings/agents/page.tsx`) renders
     no element with id `hide-disabled-agent-profiles-in-nav` and no
     longer imports `HideDisabledAgentProfilesSetting`.
  2. The files
     `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.tsx`
     and `.test.tsx` are deleted.
  3. `apps/web` typechecks and lints clean with the unused-import
     removal.
- **Verification:**
  ```bash
  cd apps/web && pnpm run typecheck
  cd apps && pnpm --filter @kandev/web lint -- app/settings/agents app/settings/integrations
  ```
- **Files likely touched:**
  - `apps/web/app/settings/agents/page.tsx`
  - `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.tsx` (delete)
  - `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.test.tsx` (delete)
- **Dependencies:** None.
- **Parallelism:** parallel-safe (disjoint files; no shared config).

## Change

1. `page.tsx`: remove the `HideDisabledAgentProfilesSetting` import and
   its `<HideDisabledAgentProfilesSetting />` render. Keep the header
   `<Separator />` that precedes it (it is the header/content divider,
   matching the integrations page).
2. Delete the setting component and its test file.

## Inputs

- Spec: What bullets 2, 4; Scenarios 2.
- Plan: Frontend > Settings-page rows.

## Output contract

Summary, files changed, exact commands run and outcomes, blockers/risks,
task/plan status update in the same conversation.

## Results

- Deleted `hide-disabled-agent-profiles-setting.tsx` + `.test.tsx` (`git rm`); removed the import and `<HideDisabledAgentProfilesSetting />` render from `apps/web/app/settings/agents/page.tsx` (header `<Separator />` kept).
- Typecheck/lint covered by final `make typecheck` / `make lint` (both exit 0).
