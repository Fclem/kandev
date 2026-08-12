---
id: "03-remove-nav-filtering"
title: "Remove nav filtering by enabled state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/remove-hide-disabled-nav-options.md"
---

# Task 03: Remove nav filtering by enabled state

Make nav visibility configuration-only: integrations show when
configured regardless of their enable/disable toggle; the Settings-tree
branches list every integration and every profile unconditionally.

- **Acceptance:**
  1. `useNavAvailability` no longer imports or calls any `use*Enabled`
     hook or `useHideDisabledIntegrationsInNav`; each `AvailabilityMap`
     key resolves to its `configured` boolean directly.
  2. `useSettingsMenuBranches` and `settings-menu-branches` no longer
     reference `hideDisabled`, `visibleIntegrationSlugs`, or
     `useVisibleIntegrationSlugs`; `buildWorkspacesBranch` and
     `buildAgentsBranch` are called without filter args.
  3. The updated unit suites pass (see Tests).
- **Verification:**
  ```bash
  cd apps && pnpm --filter @kandev/web test -- hooks/use-nav-availability.test.ts components/app-sidebar/sections/settings/settings-menu-branches.test.ts components/app-sidebar/sections/settings/use-settings-menu-branches.test.ts components/app-sidebar/sections/settings/settings-tree-render.test.tsx components/integrations/integrations-menu.test.ts
  cd apps/web && pnpm run typecheck
  ```
- **Files likely touched:**
  - `apps/web/hooks/use-nav-availability.ts`
  - `apps/web/hooks/use-nav-availability.test.ts`
  - `apps/web/components/app-sidebar/sections/settings/use-settings-menu-branches.ts`
  - `apps/web/components/app-sidebar/sections/settings/use-settings-menu-branches.test.ts`
  - `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.ts`
  - `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.test.ts`
  - `apps/web/components/app-sidebar/sections/settings/settings-tree-render.test.tsx`
  - `apps/web/components/integrations/integrations-menu.test.ts`
- **Dependencies:** None (must land before task 04, which deletes the
  hooks).
- **Parallelism:** parallel-safe (disjoint files; no shared config).

## Change

1. `use-nav-availability.ts`: drop the `use*Enabled` reads, the
   `useHideDisabledIntegrationsInNav` read, and the
   `visible = configured && (!hideDisabled || enabled)` helper; return
   each key's `configured` boolean. Update the module doc comment to say
   visibility is configuration-only.
2. `use-nav-availability.test.ts`: remove the
   `use-hide-disabled-integrations-in-nav` mock, the enabled-hook mocks
   and their `vi.mock` blocks, `setEnabled`, `ENABLED_MOCK_BY_KEY`, and
   the `decoupling enabled from nav visibility` `describe.each`; drop
   `mocks.hideDisabled` from the hoisted mock. Keep the
   `getGitHubIntegrationStatus` unit tests, the workspace-scoping tests,
   and configured/not-configured visibility assertions
   (`setConfigured(key, true)` ⇒ visible, `setConfigured(key, false)` ⇒
   hidden).
3. `use-settings-menu-branches.ts`: delete `useVisibleIntegrationSlugs`,
   the `useHideDisabledAgentProfilesInNav` read, and their imports
   (`useAzureDevOpsEnabled`, `useGitHubEnabled`, `useGitLabEnabled`,
   `useJiraEnabled`, `useLinearEnabled`, `useSentryEnabled`,
   `useHideDisabledIntegrationsInNav`,
   `useHideDisabledAgentProfilesInNav`, `IntegrationSlug`,
   `WORKSPACE_INTEGRATIONS`); call `buildWorkspacesBranch(workspaces,
   activeWorkspaceId)` and `buildAgentsBranch(orderedAgents,
   detectedNames)`.
4. `settings-menu-branches.ts`: remove the `visibleSlugs` param from
   `integrationNodes` (and its filter), the `visibleIntegrationSlugs`
   param from `buildWorkspacesBranch`, and the `hideDisabled` param from
   `buildAgentsBranch` (and its `.filter((profile) => !hideDisabled ||
   (profile.enabled ?? true))`). Update the doc comments. Keep the
   exported `IntegrationSlug` type (it types the node `integrationSlug`
   field consumed by `integration-enabled.tsx`).
5. Test updates:
   - `settings-menu-branches.test.ts`: in the "buildWorkspacesBranch
     integration visibility" describe, drop the cases that pass a visible
     set; keep/rename the default case asserting all six slugs are
     listed.
   - `use-settings-menu-branches.test.ts`: remove the `hideDisabled` and
     `integrationEnabled` hoisted state and their mocks; keep an
     accordion case asserting all six integrations are listed and the
     flat-mode case asserting `{}` (no branches).
   - `settings-tree-render.test.tsx`: remove `HIDE_DISABLED_AGENT_KEY`,
     the `beforeEach`/`afterEach`
     `window.localStorage.removeItem(HIDE_DISABLED_AGENT_KEY)` lines, and
     the two tests that toggle the setting; keep "badges a disabled
     profile, which stays listed so it can be re-enabled".
   - `integrations-menu.test.ts`: remove the
     `use-hide-disabled-integrations-in-nav` `vi.mock` and the comment
     paragraph about the decoupling.

## Inputs

- Spec: What bullets 3, 4; Scenarios 1, 3.
- Plan: Frontend > Nav filtering; Tests.

## Output contract

Summary, files changed, exact commands run and outcomes, blockers/risks,
task/plan status update in the same conversation.

## Results

- `cd apps && pnpm --filter @kandev/web test -- hooks/use-nav-availability.test.ts components/app-sidebar/sections/settings/settings-menu-branches.test.ts components/app-sidebar/sections/settings/use-settings-menu-branches.test.ts components/app-sidebar/sections/settings/settings-tree-render.test.tsx components/integrations/integrations-menu.test.ts` — first run 78 passed / 1 failed: a stale `buildAgentsBranch` hide-filter test in `settings-menu-branches.test.ts`; rewritten to the no-filter contract. Final: 5 files, 78 passed.
- Final touched-suite run (incl. `app/settings/integrations/page.test.tsx`): 6 files, 82 passed.
- Typecheck/lint covered by final `make typecheck` / `make lint` (both exit 0).
