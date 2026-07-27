---
id: "04-language-switcher-pseudo"
title: "Settings language switcher + pseudo-locale"
status: done
wave: 1
depends_on: ["02-runtime-provider-locale"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 04: Settings language switcher + pseudo-locale

Add a user-facing language selector and make the pseudo-locale selectable in
dev/e2e only.

## Acceptance
- `components/settings/language-settings.tsx`: a `LanguageSettings` control (Radix
  `Select` or the settings pattern in the appearance/general page) listing
  `SUPPORTED_LOCALES` with human labels (`English`, `Pseudo (QA)`), current value
  from the cookie/active locale, `onChange` → `activateLocale(locale)`.
  - `pseudo` is only listed when NOT a production build (`!import.meta.env.PROD`).
  - Follows the self-documenting-settings convention (visible copy explaining what
    it changes) and registers with `useSettingsSaveContributor` (or uses
    `SettingsPageTemplate`) — no page-local Save/Cancel.
- Wired into the appearance/general settings surface and reachable via
  `settings-routes.tsx`.
- Its own strings are authored with Lingui macros (it is part of the foundation,
  so it ships already-localized).

## Verification
- `cd apps/web && pnpm --filter @kandev/web test -- components/settings/language-settings.test.tsx`
- `cd apps/web && pnpm run typecheck && pnpm lint`
- Manual (dev): switch to Pseudo → chrome renders accented glyphs, reload keeps it,
  `document.documentElement.lang === "pseudo"`.

## Files likely touched
- `components/settings/language-settings.tsx` (new) + `.test.tsx`
- appearance/general settings page (`components/settings/general-settings.tsx` or
  appearance settings) and `apps/web/src/settings-routes.tsx` if a new entry is added

## Dependencies
Task 02 (`activateLocale`, `SUPPORTED_LOCALES`). Task 01 (pseudo catalog).

## Parallelism
sequential within Wave 1 (can run after 02).

## Inputs
- Spec "What" (switcher, pseudo), Scenarios (pseudo select, production hides pseudo).
- `apps/web/CLAUDE.md` settings-save + self-documenting-settings conventions.

## Output contract
Summary, files changed, tests run, blockers, risks; mark `done`, update `plan.md`.
