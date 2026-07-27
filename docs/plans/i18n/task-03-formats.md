---
id: "03-formats"
title: "Locale-aware formatting; replace timeAgo"
status: done
wave: 1
depends_on: ["02-runtime-provider-locale"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 03: Locale-aware formatting

Provide locale-aware date/time/relative-time/number helpers and retire the
hand-rolled English `timeAgo`.

## Acceptance
- `apps/web/lib/i18n/formats.ts`:
  - `formatRelative(dateStr): string` — locale-aware relative time; under `en`
    produces the SAME buckets as the old `timeAgo` (`""` for empty/invalid,
    `just now` <60s, `${m}m ago` <60m, `${h}h ago` <24h, `${d}d ago` else). Use
    `Intl.RelativeTimeFormat` or `date-fns/formatDistanceToNowStrict` with the
    active `date-fns` locale.
  - `formatDate`, `formatTime`, `formatDateTime`, `formatNumber` — thin wrappers
    over `Intl.*` bound to the active locale (from the `i18n` instance).
  - A helper to resolve the active `date-fns` locale object for existing
    `date-fns` call sites.
- `apps/web/lib/utils/time.ts`: either deleted with all ~9 call sites repointed
  to `formatRelative`, OR kept as a thin re-export of `formatRelative` (pick the
  lower-churn option; if re-exporting, mark it deprecated in a comment).
- No behavior regression for existing `timeAgo` consumers under `en`.

## Verification
- `cd apps/web && pnpm --filter @kandev/web test -- lib/i18n/formats.test.ts`
- `cd apps/web && pnpm run typecheck`
- `grep -rn "utils/time" apps/web` → all resolved to the new path or the re-export.

## Files likely touched
- `apps/web/lib/i18n/formats.ts` (new) + `formats.test.ts`
- `apps/web/lib/utils/time.ts` (delete or re-export)
- ~9 call sites currently importing `timeAgo`

## Dependencies
Task 02 (needs the active `i18n` locale).

## Parallelism
sequential within Wave 1.

## Inputs
- Spec "What" (formatting bullet), Scenario (5m → `5m ago`).
- `apps/web/lib/utils/time.ts` (current behavior to preserve).

## Output contract
Summary, files changed, tests run, blockers, risks; mark `done`, update `plan.md`.
