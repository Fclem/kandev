---
id: "07-migration-playbook"
title: "Migration playbook (shared procedure for all M-* tasks)"
status: done
wave: 1
depends_on: ["02-runtime-provider-locale", "03-formats"]
plan: "plan.md"
spec: "../../specs/platform/i18n.md"
---

# Task 07: Migration playbook

This is the shared procedure every `task-1x/2x/3x-mig-*` batch follows. It is a
reference doc, not a code change (mark `done` once the procedure is validated on
one real batch, e.g. task 14 kanban). Each migration batch externalizes every
user-facing string in its named directories using Lingui macros.

## What counts as a user-facing string (translate)
- JSX text children rendered to the user.
- Attributes: `placeholder`, `aria-label`, `aria-description`, `title`, `alt`.
- Toasts: `toast.success/error/info/warning/message(...)` (sonner) argument text.
- Error/empty-state/confirm/dialog copy, button labels, form/validation messages,
  option labels, tooltip content.

## What NOT to translate (leave as literals)
- User/domain data from the store or boot payload (task titles, workflow/step/
  repo names, chat/transcript/diff content, file paths).
- Code identifiers, enum keys, `data-testid`, event names, class names, CSS,
  `import` paths, WS action strings, query keys.
- Brand/proper nouns and symbols on the allowlist (task 06): `Kandev`, `GitHub`,
  `GitLab`, `Jira`, `Linear`, `Slack`, `Sentry`, `Azure DevOps`, single glyphs.

## How to wrap
- **JSX children** → `<Trans>Create task</Trans>`. With interpolation:
  `<Trans>Deleted {count} tasks</Trans>` (Lingui reads the JSX). For plurals use
  the `Plural` macro: `<Plural value={n} one="# task" other="# tasks" />`.
- **Strings in expressions / attributes / toasts** → `t` macro:
  ```tsx
  import { useLingui } from "@lingui/react/macro";
  const { t } = useLingui();
  toast.success(t`Task created`);
  <input placeholder={t`Search tasks`} aria-label={t`Search`} />
  ```
  For module-scope (non-hook) strings use `import { msg } from "@lingui/macro"`
  and resolve with `i18n._(...)` at use, or lazily inside the component. Prefer
  the hook form inside components.
- Keep ICU/interpolation variables named (`{count}`, `{name}`), not positional.
- Do not change surrounding logic, props, testids, or markup structure. Wrapping
  should be behavior-preserving.

## Per-file steps
1. Read the file; identify user-facing strings per the rules above.
2. Add the macro import(s) actually used.
3. Wrap each string. For components already using hooks, add `useLingui`.
4. Leave allowlisted/domain strings untouched.

## Batch verification (required before marking done)
1. `cd apps/web && pnpm run typecheck` → clean for the batch's files.
2. `cd apps/web && pnpm extract` locally to confirm the batch's strings extract
   without macro errors — **but do NOT commit `messages.po`** (catalog is
   regenerated once in task 40 to avoid cross-batch conflicts). Revert the
   catalog change after confirming: `git checkout -- apps/web/src/locales`.
3. Pseudo-locale spot check (dev build, `pseudo` active): the batch's primary
   screens show accented glyphs with no remaining plain-English user-facing text.
4. Run any existing component tests for the touched files; update snapshot/text
   assertions that now read from the catalog (text content is unchanged under
   `en`, so most assertions pass as-is).
5. `pnpm lint` for the batch files: the `no-unlocalized-strings` warnings for the
   batch should drop to zero (allowlisted exceptions aside).

## Constraints for parallel safety
- Edit ONLY files under this batch's named directories.
- Do NOT touch `lingui.config.ts`, `vite.config.ts`, `eslint.config.mjs`,
  `package.json`, or commit `src/locales/**` — those are Wave 1 / Wave 5 only.
- Do NOT edit files owned by another batch even if tempting (shared helper) —
  note it for the shared/top-level batch instead.

## Output contract (per batch)
List files changed, count of strings externalized, typecheck/lint/test results,
any strings intentionally left as literals (with reason), blockers, risks. Mark
the batch task `done` and check it in `plan.md`.
