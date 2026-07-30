---
spec: docs/specs/platform/i18n.md
created: 2026-07-27
status: building
---

# Implementation Plan: Internationalization (i18n)

## Overview
Introduce Lingui-based i18n to `apps/web` in two phases. **Phase 1 (Foundation)**
is one reviewable unit: dependencies, Lingui + Vite config, the i18n runtime and
`<I18nProvider>`, cookie/boot-payload locale plumbing (incl. the Go shell's
`<html lang>`), locale-aware formatting to replace `timeAgo`, the Settings
language switcher, the pseudo-locale, the `@kandev/ui` overridable-string
strategy, the eslint guard, and the CI extract check. **Phase 2 (Migration)**
externalizes every user-facing string across `apps/web` in directory-partitioned,
parallel-safe batches, each independently verifiable by activating the
pseudo-locale and confirming no un-accented user-facing text remains. Order:
foundation must land first (it provides macros, provider, and the pseudo oracle);
migration batches then run in any order because their file sets are disjoint.

The eslint guard is introduced as a **warning** during migration and flipped to
**error** only after the sweep completes, so the codebase can't regress.

---

## Backend

### Go shell `<html lang>` (`apps/backend/internal/webapp/shell.go`)
- `RenderShellHTML` rewrites the `lang="en"` attribute of the shell HTML to the
  request locale. Add a `Locale` field to `BootPayload` (default `"en"`); the
  handler reads the `kandev_locale` cookie, validates against the supported set
  (`en`, `pseudo`), coerces unknown values to `en`, and passes it through.
- `apps/web/index.html` and `embedded/fallback/index.html` keep `lang="en"` as
  the static default; the shell substitutes at render time.
- Cookie is set client-side; the backend only reads it. No new endpoint.
- Tests: `shell_test.go` / `handler_test.go` assert `<html lang="pseudo">` when
  the cookie is `pseudo` and `en` when the cookie is absent or invalid.

---

## Frontend

### i18n runtime (`apps/web/lib/i18n/`)
- `index.ts` — construct and export the Lingui `i18n` instance;
  `SUPPORTED_LOCALES` (`["en", "pseudo"]`, `pseudo` dev/e2e only),
  `DEFAULT_LOCALE = "en"`, `activateLocale(locale)` (dynamic-import the compiled
  catalog, `i18n.loadAndActivate`, set `document.documentElement.lang`, write the
  `kandev_locale` cookie).
- `provider.tsx` — `<I18nProvider>` wrapping `@lingui/react`'s `I18nProvider`,
  mounted at the top of `apps/web/src/app-shell.tsx` (outermost, above
  `ThemeProvider`), activating the boot locale before first paint.
- `boot.ts` — read initial locale from `BootPayload.runtime.locale` → cookie →
  `DEFAULT_LOCALE`; called from `main.tsx`/`app-shell` before render.

### Locale plumbing
- `apps/web/src/boot-payload.ts` — `BootRuntime` gains optional `locale?: string`;
  `readRuntime` parses it.
- `apps/web/lib/i18n/cookie.ts` — `readLocaleCookie()` / `writeLocaleCookie()`
  (name `kandev_locale`, 1-year expiry, `SameSite=Lax`).

### Formatting (`apps/web/lib/i18n/formats.ts`)
- `formatRelative(dateStr)` replaces `apps/web/lib/utils/time.ts` `timeAgo`
  (behavior-compatible under `en`: `just now`, `Nm/Nh/Nd ago`) using the active
  locale; `formatDate`, `formatTime`, `formatNumber` wrap `Intl` with the active
  locale; `date-fns` call sites pass the active `date-fns` locale object.
- Delete `lib/utils/time.ts` after migrating its ~9 call sites (or re-export the
  new formatter from it to keep imports stable — decided in task 03).

### Settings language switcher (`components/settings/`)
- A `LanguageSettings` control in the appearance/general settings surface lists
  `SUPPORTED_LOCALES` (pseudo hidden in production builds via
  `import.meta.env.PROD`), calls `activateLocale`, persists via the cookie, and
  registers with `useSettingsSaveContributor` per the settings-save convention.
- Wire it into `settings-routes.tsx` / the appearance settings page.

### `@kandev/ui` overridable strings (`apps/packages/ui/src/`)
- Each primitive with a baked-in English string (`carousel`, `dialog`, `sheet`,
  `pagination`, `sidebar`, `spinner`, `breadcrumb`) exposes the string as an
  overridable prop with an English default (no runtime i18n dependency).
- `apps/web` passes translated values (via `t` macro) at the call sites it owns,
  or through a small `UIStringsProvider` context in the package consumed by the
  app. Package stays framework-agnostic and works standalone.

### Migration (`apps/web/components/**`, `apps/web/app/**`)
- Every user-facing string wrapped in `<Trans>` (JSX children) or `t` macro
  (attributes, toasts, variables), per the playbook in
  [task-07](task-07-migration-playbook.md). Partitioned into disjoint batches
  (Waves 2–4 below) so batches never edit the same file.

---

## Tests

- **Go shell lang** — `apps/backend/internal/webapp/shell_test.go`: table test
  over cookie values (`en`, `pseudo`, absent, garbage) → asserted `<html lang>`.
- **Locale bootstrap** — `apps/web/lib/i18n/boot.test.ts`: precedence
  (payload > cookie > default) and coercion of invalid locale to `en`.
- **Cookie helpers** — `apps/web/lib/i18n/cookie.test.ts`: round-trip read/write
  (use the jsdom `document.cookie` setter-intercept trick from `apps/web/CLAUDE.md`).
- **Formats** — `apps/web/lib/i18n/formats.test.ts`: `formatRelative` matches the
  old `timeAgo` outputs under `en` for the boundary buckets (<60s, <60m, <24h,
  ≥24h); `formatNumber`/`formatDate` change with locale.
- **activateLocale** — `apps/web/lib/i18n/index.test.ts`: sets
  `document.documentElement.lang`, writes cookie, loads catalog; invalid locale
  coerced.
- **Language switcher** — `components/settings/language-settings.test.tsx`:
  selecting a locale calls `activateLocale`; pseudo hidden when `import.meta.env.PROD`.
- **@kandev/ui override** — `apps/packages/ui`: a primitive renders its default
  English when no override, and the override when provided.
- **Catalog sync (CI)** — a script/test asserting `lingui extract` produces no
  diff; documented in task 06 and wired into CI.

## E2E Tests

- **Language switch persists** — `apps/web/e2e/i18n/language-switch.spec.ts`:
  GIVEN default UI, WHEN user selects pseudo in Settings, THEN visible chrome
  renders accented glyphs, `<html lang>` = `pseudo`, and the choice survives
  reload.
- **Pseudo-locale coverage gate** — `apps/web/e2e/i18n/pseudo-coverage.spec.ts`:
  a smoke crawl of key screens (dashboard, tasks, settings, a task detail) under
  the pseudo-locale asserting no plain-ASCII-only user-facing text nodes remain
  in a curated set of containers (the automated completeness oracle). Expanded as
  batches land.

---

## Implementation Waves And Parallel Candidates

Foundation (Wave 1) is sequential and must land before any migration. Migration
batches (Waves 2–4) are parallel-safe: each owns a disjoint directory set and
touches no shared config after Wave 1. They are grouped only to bound
concurrency; there is no ordering dependency among them. The eslint guard flips
warn→error in the final task after all batches report clean.

```
Wave 1 — Foundation (sequential):
- [x] [task-01-deps-config](task-01-deps-config.md)
- [x] [task-02-runtime-provider-locale](task-02-runtime-provider-locale.md)
- [x] [task-03-formats](task-03-formats.md)
- [x] [task-04-language-switcher-pseudo](task-04-language-switcher-pseudo.md)
- [x] [task-05-ui-package-strings](task-05-ui-package-strings.md)
- [x] [task-06-lint-guard-ci-docs](task-06-lint-guard-ci-docs.md)  (guard added as non-blocking `lint:i18n`; folded into main config at task-40)
- [x] [task-07-migration-playbook](task-07-migration-playbook.md)   (reference doc; macro/extract/pseudo pipeline validated by the foundation build)

Wave 2 — Migration batch A (parallel candidates, disjoint dirs):
- [ ] [task-10-mig-shared-toplevel](task-10-mig-shared-toplevel.md)        components/*.tsx (top-level) + shared/ + routing/ + theme/ + icons/
- [ ] [task-11-mig-sidebar-statusbar](task-11-mig-sidebar-statusbar.md)    app-sidebar/ + app-status-bar/ + quick-chat/ + command-panel
- [ ] [task-12-mig-settings-components](task-12-mig-settings-components.md) components/settings/ (221)
- [ ] [task-13-mig-settings-app](task-13-mig-settings-app.md)              app/settings/ (90)
- [x] [task-14-mig-kanban](task-14-mig-kanban.md)                          components/kanban/ (30)

Wave 3 — Migration batch B (parallel candidates, disjoint dirs):
- [ ] [task-20-mig-task-a](task-20-mig-task-a.md)                          components/task/ subset A
- [ ] [task-21-mig-task-b](task-21-mig-task-b.md)                          components/task/ subset B
- [ ] [task-22-mig-task-c](task-22-mig-task-c.md)                          components/task/ subset C
- [ ] [task-23-mig-office-app](task-23-mig-office-app.md)                  app/office/ (210)
- [ ] [task-24-mig-review-diff-editors](task-24-mig-review-diff-editors.md) review/ + diff/ + editors/

Wave 4 — Migration batch C (integrations + long tail, parallel candidates):
- [ ] [task-30-mig-github](task-30-mig-github.md)                          components/github/ + app/github/
- [ ] [task-31-mig-gitlab](task-31-mig-gitlab.md)                          components/gitlab/ + app/gitlab/
- [ ] [task-32-mig-jira-linear](task-32-mig-jira-linear.md)                jira/ linear/ + app/jira app/linear
- [ ] [task-33-mig-azure-sentry-slack](task-33-mig-azure-sentry-slack.md)  azure-devops/ sentry/ slack/ + app/azure-devops/
- [ ] [task-34-mig-integrations-automations](task-34-mig-integrations-automations.md) integrations/ vcs/ workspace-source-picker/ automations/
- [ ] [task-35-mig-plugins-stats-metrics](task-35-mig-plugins-stats-metrics.md) plugins/ + app/stats/ + system-metrics/ + system-health/
- [ ] [task-36-mig-tasks-auth-demo-longtail](task-36-mig-tasks-auth-demo-longtail.md) app/tasks/ app/t/ app/actions/ app/auth/ app/demo/ + watches/ release-notes/ agent/ search/ config-chat/ onboarding/ session/

Wave 5 — Close-out (sequential, after all M-* clean):
- [ ] [task-40-lint-flip-e2e-coverage](task-40-lint-flip-e2e-coverage.md)  flip eslint guard warn→error; expand pseudo-coverage e2e; final extract
```

Default execution is sequential in the primary conversation; the migration waves
are authorized for multi-agent orchestration by the user for this feature.

---

## Notes on parallel-safety
- Every M-* task edits only files under its named directories. No two M-* tasks
  share a directory.
- Shared config (`lingui.config.ts`, `vite.config.ts`, `eslint.config.mjs`,
  `package.json`, catalog files) is touched **only** in Wave 1 and Wave 5, never
  by an M-* task — so migration agents never contend on lockfiles or config.
- Catalog regeneration (`lingui extract`) is run once in Wave 5 (and by the
  developer as needed); individual M-* tasks do **not** commit catalog changes,
  avoiding merge conflicts on `messages.po`.


---

## Actual state (updated after the sweep)

The plan above described the original Lingui buildout. What shipped:

- **Library**: react-i18next, keyed `namespace:key` (superseded Lingui — see
  `docs/specs/platform/i18n.md` "Resolved decisions").
- **Frontend**: 3,721 catalog entries across 22 namespaces. `lint:i18n` findings
  went 2,674 -> 596; the remainder is inventoried in
  [REMAINING.md](REMAINING.md) with the reason each was declined.
- **Backend**: `apps/backend/internal/i18n` — locale resolution
  (cookie -> Accept-Language -> `en`), embedded catalogs, `pseudo` generated from
  `en`, and `Normalize` shared with `<html lang>`. Scope is the copy Go renders
  directly to a browser; diagnostic API errors stay English by design
  (contract in `docs/i18n.md`).
- **Gates**: `pnpm run i18n:check` (key/catalog drift), `pnpm run lint:i18n`
  (hardcoded strings), the pseudo-locale oracle, and
  `lib/i18n/confirm-tokens.test.ts` (sentinel safety).

### Tools kept for the remaining work
- `scripts/externalize-strings.mjs` — raw literal -> `t("ns:key")`. Declines
  logic sentinels and sentence fragments.
- `scripts/wrap-trans.mjs` — mixed-content sentence -> `<Trans i18nKey>`.
  Declines English plural hacks and thin sentences.
- `scripts/generate-pseudo-locale.mjs`, `scripts/check-i18n-keys.mjs`.

### Not done
- The 596 findings in [REMAINING.md](REMAINING.md).
- The three display-strings-as-sentinels cases in [FOLLOWUPS.md](FOLLOWUPS.md#2).
- No second human language ships; `en` + `pseudo` only (as specced).
