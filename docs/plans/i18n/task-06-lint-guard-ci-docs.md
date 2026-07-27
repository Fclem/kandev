---
id: "06-lint-guard-ci-docs"
title: "eslint guard (warn) + CI extract check + docs"
status: done
wave: 1
depends_on: ["01-deps-config"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 06: eslint guard + CI extract check + docs

Add the anti-regression tooling. The eslint rule starts as a **warning** so the
in-progress migration doesn't break the build; task 40 flips it to error.

## Acceptance
- `apps/web/eslint.config.mjs`: add `@lingui/eslint-plugin` with
  `@lingui/no-unlocalized-strings` set to `"warn"`, scoped to
  `apps/web/{components,app}/**` (and `src` UI where relevant). Configure the
  rule's `ignore`/allowlist for: brand/proper nouns (`Kandev`, `GitHub`, `GitLab`,
  `Jira`, `Linear`, `Slack`, `Sentry`, `Azure DevOps`), code identifiers, URLs,
  single symbols/punctuation, `data-testid`/`aria` role tokens, and className-like
  strings. Exclude `**/*.test.ts(x)` and `e2e/**`.
- CI catalog-sync check: a step/script (e.g. `scripts/i18n-extract-check` or an
  npm script `extract:check`) that runs `lingui extract` and fails if
  `messages.po` changes (git diff non-empty). Wired into the web CI workflow
  alongside lint/typecheck.
- Docs: `docs/i18n.md` (or a section in `apps/web/CLAUDE.md`) documenting: how to
  add a translatable string (`<Trans>` vs `t`), how to run extract/compile, the
  pseudo-locale QA flow, the allowlist policy, and the "catalog must be committed"
  rule. Add a pointer from `apps/web/CLAUDE.md`.

## Verification
- `cd apps/web && pnpm lint` → the rule reports warnings (not errors) on existing
  literals; build still passes with `--max-warnings` relaxed for the migration
  window (confirm the lint script tolerates warnings during migration — adjust
  `--max-warnings 0` handling; task 40 restores strictness).
- Run the extract-check script on a clean tree → passes; on an intentionally
  edited source string without re-extract → fails.

## Files likely touched
- `apps/web/eslint.config.mjs`
- `.github/workflows/*` (web CI), `apps/web/package.json` (scripts),
  `scripts/i18n-extract-check` (new, optional)
- `docs/i18n.md` (new) + `apps/web/CLAUDE.md` pointer

## Dependencies
Task 01 (Lingui + eslint plugin installed).

## Parallelism
sequential within Wave 1 (touches eslint config + CI + package.json).

## NOTE on --max-warnings
`apps/web` lint script is `eslint --max-warnings 0`, so warnings fail CI. During
migration, either (a) run the i18n rule under a separate non-blocking lint script,
or (b) set the rule to `"warn"` AND temporarily allow warnings for that rule only
via an override that task 40 removes. Choose (a) if simpler; document the choice.

## Inputs
- Spec "What" (lint guard, CI extract), "Out of scope" (allowlist of non-translatable).
- `apps/web/eslint.config.mjs` current structure.

## Output contract
Summary, files changed, tests run, blockers, risks; mark `done`, update `plan.md`.
