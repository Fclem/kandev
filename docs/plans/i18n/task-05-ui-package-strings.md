---
id: "05-ui-package-strings"
title: "@kandev/ui overridable primitive strings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 05: @kandev/ui overridable primitive strings

Make the ~12 baked-in English strings in shared primitives translatable without
the package depending on the app's i18n runtime.

## Acceptance
- Each primitive with a hardcoded user-facing string exposes it as an overridable
  prop with an English default (no import of `@lingui/*` in the package):
  - `carousel.tsx` ("Previous slide"/"Next slide"),
    `dialog.tsx`/`sheet.tsx` ("Close"), `pagination.tsx`
    ("Previous"/"Next"/"Go to previous page"/"More pages"),
    `sidebar.tsx` ("Sidebar"/"Toggle Sidebar"), `spinner.tsx` ("Loading"),
    `breadcrumb.tsx` (any labels).
- Strategy: props default to the current English literal so existing callers and
  standalone use are unchanged. Optionally add a lightweight `UIStringsProvider`
  React context in the package (English defaults) that the app can populate with
  translated values — but the package must render correctly with NO provider.
- Package builds and its existing tests pass; no new runtime dependency added.
- `apps/web` call sites that render these primitives pass `t`-macro translated
  labels where the label is user-visible (done here for the handful of app-owned
  call sites, or deferred to the relevant migration batch — note which).

## Verification
- `cd apps && pnpm --filter @kandev/ui build`
- `cd apps && pnpm --filter @kandev/ui test` (if the package has tests) or add a
  focused test asserting default vs. overridden label.
- `grep -rn "@lingui" apps/packages/ui/src` → no matches (package stays i18n-free).

## Files likely touched
- `apps/packages/ui/src/carousel.tsx`, `dialog.tsx`, `sheet.tsx`, `pagination.tsx`,
  `sidebar.tsx`, `spinner.tsx`, `breadcrumb.tsx`
- optional `apps/packages/ui/src/lib/ui-strings.tsx` (context) + test

## Dependencies
None (can run in parallel with 01–04; does not touch app build config).

## Parallelism
parallel-safe with tasks 01–04 (disjoint files: `apps/packages/ui/**` only).

## Inputs
- Spec "What" (@kandev/ui bullet), "Failure modes" (standalone default).
- Survey list of primitive strings.

## Output contract
Summary, files changed, tests run, blockers, risks; mark `done`, update `plan.md`.
