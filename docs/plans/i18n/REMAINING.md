# i18n — remaining externalization work

Generated from `pnpm run lint:i18n`. **52 findings across 30 files.**

These are what the two codemods deliberately declined, not misses. Each needs a
judgement call rather than a mechanical rewrite:

1. **Sentence fragments** — JSX text interleaved with expressions/inline markup.
   Needs one `<Trans i18nKey>` covering the whole sentence (see
   `scripts/wrap-trans.mjs`); the tool skipped these as too thin (<2 prose words)
   or as nested candidates.
2. **English plural hacks** — `{n} comment{n > 1 ? "s" : ""}`. The suffix is
   English morphology, not data. Convert to a real i18next plural key
   (`key_one`/`key_other`) and pass `count`.
3. **Logic sentinels** — a literal also compared with `===` or used as a map key.
   Must stay untranslated, or be decoupled by keying off a stable id and
   translating at render. See FOLLOWUPS.md item 2.

Six files were reverted from `<Trans>` wrapping because their tests assert on
text that `<Trans>` splits across markup; their plain strings are already
externalized. Re-wrapping them means updating those assertions in the same change.

## Why the last 94 need a human

Each of these is a case where running the codemod produces *worse* output than
leaving the string alone, so the tools now decline them by design:

- **Conditional JSX children.** `wrap-trans` refuses any element whose direct
  child expression is not a plain identifier/member — a conditional cannot become
  a `values` entry without being duplicated into both the attribute and the
  children. Observed duplicating a `data-testid`, which broke a query.
- **Text assembled by a child component.** Some hints (e.g. the passthrough
  composer's `Ctrl+Shift+Y`) are not literals in the file the guard flags; they
  are composed a level down. Wrapping the parent disturbs them at a distance, so
  these need the child localized first.
- **Button accessible names.** A `<Trans>` around an icon+label folds the icon
  into the name and breaks `getByRole("button", { name })`. The externalizer now
  treats a childless icon sibling as contributing no text, which fixes most of
  these, but a few need the markup reshaped by hand.
- **Irregular plurals.** `fix-plural-hacks` refuses stems where "+s" is wrong
  (entry/category/repository); they need hand-written `_one`/`_other` values.

### Recommended procedure per file
1. `node scripts/wrap-trans.mjs <file>` (dry run) and read what it declines.
2. Localize the innermost component that actually owns the text.
3. Where a test asserts on now-split text, switch it to a function matcher:
   `getByText((_, el) => el?.textContent === "...")` — in the same commit.
4. Re-run `pnpm exec vitest run <that suite>`, `pnpm lint`, `pnpm run i18n:check`.

## Files

| Findings | File |
|---:|---|
| 5 | `components/task/add-workspace-sources/workspace-change-consequences.tsx` |
| 5 | `components/task/passthrough-toolbar.tsx` |
| 4 | `components/github/github-app-import-guide.tsx` |
| 3 | `components/azure-devops/azure-devops-settings.tsx` |
| 2 | `components/diff/unanchored-findings-banner.tsx` |
| 2 | `components/jira/jira-settings.tsx` |
| 2 | `components/task/chat/messages/review-comments-attachment.tsx` |
| 2 | `components/task/chat/messages/sender-task-badge.tsx` |
| 2 | `components/task/mobile/mobile-terminal-keybar.tsx` |
| 2 | `components/task/mobile/session-mobile-top-bar-dialog-parts.tsx` |
| 2 | `components/task/new-session-dialog.tsx` |
| 2 | `components/task/share/share-snapshot-preview.tsx` |
| 2 | `components/vcs/vcs-dialogs.tsx` |
| 1 | `app/office/agents/[id]/components/instruction-file-list.tsx` |
| 1 | `app/office/components/new-task-dialog.tsx` |
| 1 | `app/office/workspace/costs/cost-overview.tsx` |
| 1 | `components/azure-devops/azure-devops-filters.tsx` |
| 1 | `components/github/github-app-import-form.tsx` |
| 1 | `components/github/pr-ci-popover.tsx` |
| 1 | `components/review/review-comments-overview.tsx` |
| 1 | `components/review/review-findings-overview.tsx` |
| 1 | `components/settings/plugins/plugin-detail.tsx` |
| 1 | `components/settings/plugins/plugin-row.tsx` |
| 1 | `components/settings/sprites-settings.tsx` |
| 1 | `components/task/chat/context-popover.tsx` |
| 1 | `components/task/chat/messages/todo-message.tsx` |
| 1 | `components/task/chat/messages/tool-subagent-message.tsx` |
| 1 | `components/task/simple/components/agent-turn-panel.tsx` |
| 1 | `components/task/simple/components/session-timeline-entry.tsx` |
| 1 | `components/task/simple/task-chat.tsx` |
