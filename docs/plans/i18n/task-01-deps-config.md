---
id: "01-deps-config"
title: "Dependencies + Lingui/Vite config + catalog scaffolding"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 01: Dependencies + Lingui/Vite config + catalog scaffolding

Add Lingui and wire it into the Vite/Babel/TS build so `<Trans>`/`t` macros
compile and catalogs extract/compile.

## Acceptance
- `@lingui/core`, `@lingui/react`, `@lingui/macro`, `@lingui/vite-plugin`,
  `@lingui/cli`, `@lingui/babel-plugin-lingui-macro` (+ `@lingui/format-po`,
  `@lingui/eslint-plugin` — the eslint plugin is wired in task 06) added to
  `apps/web/package.json` at current stable versions; `pnpm install` succeeds.
- `apps/web/lingui.config.ts` exists: `locales: ["en", "pseudo"]`,
  `pseudoLocale: "pseudo"`, `sourceLocale: "en"`, `catalogs` →
  `apps/web/src/locales/{locale}/messages`, `format: "po"`, `compileNamespace: "es"`.
- `apps/web/vite.config.ts` adds `@lingui/vite-plugin` and the Babel macro plugin
  to `@vitejs/plugin-react` (`babel.plugins: ["@lingui/babel-plugin-lingui-macro"]`).
- `package.json` scripts: `"extract": "lingui extract"`, `"compile": "lingui compile"`,
  `"extract:clean": "lingui extract --clean"`.
- Empty `en` and `pseudo` catalogs generated (`lingui extract` runs without error
  and `lingui compile` produces compiled output); catalogs committed.
- `apps/web/tsconfig` picks up Lingui macro types (no TS errors on a sample
  `<Trans>` usage).

## Verification
- `cd apps && pnpm install`
- `cd apps/web && pnpm extract && pnpm compile`
- `cd apps/web && pnpm run typecheck` (add a throwaway `<Trans>` in a scratch file
  to confirm macro types resolve, then remove it)

## Files likely touched
- `apps/web/package.json`
- `apps/web/lingui.config.ts` (new)
- `apps/web/vite.config.ts`
- `apps/web/src/locales/en/messages.po`, `apps/web/src/locales/pseudo/messages.po` (new)
- `apps/web/tsconfig*.json` (if macro types need referencing)
- `apps/pnpm-lock.yaml`

## Dependencies
None.

## Parallelism
sequential (touches package.json + lockfile + build config).

## Inputs
- Spec: "What", "Data model" (catalog locations), "Resolved decisions".
- Existing `apps/web/vite.config.ts` and `apps/web/package.json` scripts.

## Output contract
Summary of deps/versions added, the config files, confirmation extract+compile
run clean, tests run, blockers, risks. Mark this task `done` and check it in
`plan.md`.
