---
id: "01-remove-integrations-setting"
title: "Remove integrations hide-disabled setting"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/remove-hide-disabled-nav-options.md"
---

# Task 01: Remove integrations hide-disabled setting

Remove the "Hide disabled integrations from left panel navigation" row
from the integrations index page, keeping the six per-integration
enable/disable toggles.

- **Acceptance:**
  1. `IntegrationsIndexPage` renders exactly six `switch` elements (one
     per integration) and no element with id
     `hide-disabled-integrations-in-nav`.
  2. `apps/web/app/settings/integrations/page.test.tsx` passes with the
     hide-disabled draft test and the
     `kandev:integrations:hideDisabledInNav:v1` localStorage cleanup
     removed, and the switch-count assertion updated 7 → 6.
- **Verification:**
  ```bash
  cd apps && pnpm --filter @kandev/web test -- app/settings/integrations/page.test.tsx
  cd apps/web && pnpm run typecheck
  ```
- **Files likely touched:**
  - `apps/web/components/integrations/integrations-index-page.tsx`
  - `apps/web/app/settings/integrations/page.test.tsx`
- **Dependencies:** None.
- **Parallelism:** parallel-safe (disjoint files; no shared config).

## Change

1. `integrations-index-page.tsx`: delete the
   `HideDisabledIntegrationsSetting` component and its
   `<HideDisabledIntegrationsSetting />` render; remove the now-unused
   imports `useHideDisabledIntegrationsInNav`,
   `useDraftedIntegrationEnabled`, `Switch`, and `Label` (the cards use
   `Link`, `CardContent`, and the per-integration control components; the
   header `Separator` and `useTranslation` stay).
2. `page.test.tsx`: delete the
   "renders the hide-disabled-in-nav setting off by default…" test and
   the `beforeEach` `window.localStorage.removeItem(...hideDisabledInNav...)`
   line; change the slider-count assertion from 7 to 6.

## Inputs

- Spec: What bullets 1, 3; Scenarios 1, 4.
- Plan: Frontend > Settings-page rows; Tests.

## Output contract

Summary, files changed, exact commands run and outcomes, blockers/risks,
task/plan status update in the same conversation.

## Results

- `cd apps && pnpm --filter @kandev/web test -- app/settings/integrations/page.test.tsx` — Red: 1 failed (7 switches rendered, expected 6). Green after removing the setting from `integrations-index-page.tsx`: 4 passed.
- Typecheck/lint covered by final `make typecheck` / `make lint` (both exit 0).
