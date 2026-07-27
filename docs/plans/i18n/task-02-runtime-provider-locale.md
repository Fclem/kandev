---
id: "02-runtime-provider-locale"
title: "i18n runtime, provider, cookie + boot-payload locale plumbing, Go shell lang"
status: done
wave: 1
depends_on: ["01-deps-config"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 02: i18n runtime, provider, locale plumbing

Stand up the runtime that activates a locale before first paint, persists it via
a cookie, and drives `<html lang>` both client-side and from the Go shell.

## Acceptance
- `apps/web/lib/i18n/index.ts`: exports `i18n` (from `@lingui/core`),
  `SUPPORTED_LOCALES = ["en", "pseudo"] as const`, `DEFAULT_LOCALE = "en"`,
  `isSupportedLocale()`, and `async activateLocale(locale)` that dynamic-imports
  the compiled catalog (`../../src/locales/${locale}/messages`), calls
  `i18n.loadAndActivate({ locale, messages })`, sets
  `document.documentElement.lang = locale`, and writes the cookie.
- `apps/web/lib/i18n/cookie.ts`: `readLocaleCookie()` / `writeLocaleCookie(locale)`
  (`kandev_locale`, `SameSite=Lax`, 1-year max-age). Invalid values ignored.
- `apps/web/lib/i18n/boot.ts`: `resolveInitialLocale(payload)` with precedence
  `payload.runtime.locale` → cookie → `DEFAULT_LOCALE`, coercing unknown → `en`.
- `apps/web/lib/i18n/provider.tsx`: `<I18nProvider>` wrapping `@lingui/react`
  `I18nProvider` with the shared `i18n`; mounted as the OUTERMOST provider in
  `apps/web/src/app-shell.tsx` (above `ThemeProvider`). Initial locale activated
  before render (in `main.tsx` or an async boot gate) so there is no English flash.
- `apps/web/src/boot-payload.ts`: `BootRuntime` gains `locale?: string`;
  `readRuntime` reads it.
- **Go shell**: `apps/backend/internal/webapp/shell.go` — add `Locale string` to
  `BootPayload`; `RenderShellHTML` rewrites the shell's `lang="en"` to the payload
  locale (default `en`). The webapp handler reads the `kandev_locale` cookie,
  validates against `{en,pseudo}` (unknown → `en`), and sets `payload.Locale`.

## Verification
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm --filter @kandev/web test -- lib/i18n/boot.test.ts lib/i18n/cookie.test.ts lib/i18n/index.test.ts`
- `make -C apps/backend test` (shell/handler lang tests) — see task Tests in plan
- Manual: `cd apps && pnpm --filter @kandev/web dev`, confirm app renders in `en`
  and `document.documentElement.lang === "en"`.

## Files likely touched
- `apps/web/lib/i18n/{index,cookie,boot,provider}.ts(x)` (new) + their `*.test.ts`
- `apps/web/src/app-shell.tsx`, `apps/web/src/main.tsx`, `apps/web/src/boot-payload.ts`
- `apps/backend/internal/webapp/shell.go`, `payload.go`/`handler.go` (+ `*_test.go`)

## Dependencies
Task 01.

## Parallelism
sequential (foundation; app-shell + boot are shared entrypoints).

## Inputs
- Spec: "What", "API surface", "Failure modes" (invalid locale → `en`, missing
  catalog → id fallback), "Persistence guarantees".
- `apps/web/src/app-shell.tsx` provider stack, `boot-payload.ts` `readRuntime`,
  `apps/backend/internal/webapp/shell.go` render path.

## Output contract
Summary, files changed, tests run, blockers, risks; mark `done` and update `plan.md`.
