---
id: "05-updates-channel-ui"
title: "Responsive Updates setting"
status: completed
wave: 4
depends_on: ["04-backend-api-apply"]
plan: "plan.md"
spec: "../../specs/npm-nightly-channel/spec.md"
---

# Task 05: Responsive Updates setting

- **Acceptance:** Stable/Nightly uses the shared settings Save/Revert coordinator and persists via
  the typed PATCH API.
- **Acceptance:** server capability reasons disable Nightly for unsupported installs while Desktop
  keeps its signed stable updater.
- **Acceptance:** inline rows are keyboard accessible, at least 44px high on phone, and long target
  versions cannot create document overflow.
- **Acceptance:** a save response cannot overwrite a newer channel draft, and save failures replace
  stale manual-check errors while leaving the draft retryable.
- **Verification:** `cd apps && pnpm --filter @kandev/web exec vitest run lib/api/domains/system-api.test.ts components/settings/system/updates-card.test.tsx`
- **Verification:** `cd apps/web && pnpm run typecheck`
- **Files likely touched:** `apps/web/lib/types/system.ts`, `lib/api/domains/system-api.ts`,
  `hooks/domains/system/use-updates.ts`, `components/settings/system/updates-card.tsx`, an extracted
  channel control if needed, and focused tests.
- **Dependencies:** Task 04 response/API contract.
- **Parallelism:** sequential.
- **Inputs:** plan mobile design contract and spec UI scenarios.
- **Risks:** conditional Desktop composition must not conditionally invoke hooks; failed saves must
  reject and preserve a discardable authoritative baseline.

## Verification results

- `cd apps && pnpm --filter @kandev/web exec vitest run lib/api/domains/system-api.test.ts components/settings/system/updates-card.test.tsx`
  — passed, 47 tests.
- `cd apps/web && pnpm run typecheck` — passed.
- Focused ESLint for the changed frontend unit/API files — passed with no warnings.
