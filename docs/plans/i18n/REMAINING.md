# i18n — remaining externalization work

Generated from `pnpm run lint:i18n`. **280 findings across 83 files.**

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

## Files

| Findings | File |
|---:|---|
| 17 | `components/settings/voice-mode-settings.tsx` |
| 17 | `components/settings/workflow-pipeline-editor-step-actions.tsx` |
| 16 | `components/task/passthrough-toolbar.tsx` |
| 14 | `components/settings/terminal-settings.tsx` |
| 13 | `components/settings/sprites-settings.tsx` |
| 11 | `components/settings/system-metrics-settings-card.tsx` |
| 10 | `components/settings/ssh-sessions-card.tsx` |
| 9 | `components/settings/ssh-agent-readiness-card.tsx` |
| 8 | `components/settings/secrets-settings.tsx` |
| 8 | `components/settings/utility-sections.tsx` |
| 8 | `components/settings/workflow-card-dialogs.tsx` |
| 8 | `components/settings/workflow-pipeline-editor-panels.tsx` |
| 8 | `components/settings/workflow-sync-dialog.tsx` |
| 7 | `components/settings/workflow-card.tsx` |
| 6 | `components/settings/utility-agent-dialog.tsx` |
| 6 | `components/task/add-workspace-sources/workspace-change-consequences.tsx` |
| 5 | `components/azure-devops/azure-devops-settings.tsx` |
| 5 | `components/github/github-app-import-guide.tsx` |
| 5 | `components/settings/shell-settings-card.tsx` |
| 5 | `components/ui/data-table-pagination.tsx` |
| 4 | `components/slack/slack-settings.tsx` |
| 3 | `components/settings/settings-floating-save.tsx` |
| 3 | `components/settings/settings-layout-client.tsx` |
| 3 | `components/settings/ssh-connection-card.tsx` |
| 3 | `components/settings/workflow-pipeline-editor-wip-controls.tsx` |
| 3 | `components/task/chat/messages/sender-task-badge.tsx` |
| 2 | `components/diff/unanchored-findings-banner.tsx` |
| 2 | `components/github/github-app-import-form.tsx` |
| 2 | `components/jira/jira-settings.tsx` |
| 2 | `components/settings/ssh-connection-form.tsx` |
| 2 | `components/settings/ssh-fingerprint-trust-block.tsx` |
| 2 | `components/settings/workflow-pipeline-editor.tsx` |
| 2 | `components/task/add-workspace-sources/add-workspace-sources-dialog.tsx` |
| 2 | `components/task/chat/messages/review-comments-attachment.tsx` |
| 2 | `components/task/mobile/mobile-terminal-keybar.tsx` |
| 2 | `components/task/mobile/session-mobile-top-bar-dialog-parts.tsx` |
| 2 | `components/task/new-session-dialog.tsx` |
| 2 | `components/task/share/share-snapshot-preview.tsx` |
| 2 | `components/task/simple/components/agent-turn-panel.tsx` |
| 2 | `components/task/simple/task-chat.tsx` |
| 2 | `components/task/task-archive-confirm-dialog.tsx` |
| 2 | `components/task/task-delete-confirm-dialog.tsx` |
| 2 | `components/ui/data-table.tsx` |
| 2 | `components/vcs/vcs-dialogs.tsx` |
| 1 | `app/linear/linear-page-client.tsx` |
| 1 | `app/office/agents/[id]/components/instruction-file-list.tsx` |
| 1 | `app/office/agents/[id]/dashboard/components/run-activity-chart.tsx` |
| 1 | `app/office/components/new-task-dialog.tsx` |
| 1 | `app/office/workspace/costs/cost-overview.tsx` |
| 1 | `app/office/workspace/routing/components/wake-reason-tier-card.tsx` |
| 1 | `app/settings/agents/[agentId]/profile-mcp-config-card.tsx` |
| 1 | `components/azure-devops/azure-devops-filters.tsx` |
| 1 | `components/diff/hunk-action-bar.tsx` |
| 1 | `components/editors/codemirror/codemirror-code-editor.tsx` |
| 1 | `components/editors/monaco/monaco-editor-toolbar.tsx` |
| 1 | `components/github/pr-ci-popover.tsx` |
| 1 | `components/jira/my-jira/results-pagination.tsx` |
| 1 | `components/release-notes/release-notes-dialog.tsx` |
| 1 | `components/review/review-comments-overview.tsx` |
| 1 | `components/review/review-findings-button.tsx` |
| 1 | `components/review/review-findings-overview.tsx` |
| 1 | `components/review/walkthrough-overlay.tsx` |
| 1 | `components/sentry/sentry-issue-dialog.tsx` |
| 1 | `components/settings/notifications-settings.tsx` |
| 1 | `components/settings/plugins/plugin-detail.tsx` |
| 1 | `components/settings/plugins/plugin-row.tsx` |
| 1 | `components/settings/unsaved-indicator.tsx` |
| 1 | `components/settings/workflow-card-header-actions.tsx` |
| 1 | `components/settings/workflow-cycle-diagnostic.tsx` |
| 1 | `components/settings/workflow-export-dialog.tsx` |
| 1 | `components/settings/workflow-pipeline-editor-agent-profile-select.tsx` |
| 1 | `components/settings/workflow-sync-status-banner.tsx` |
| 1 | `components/settings/workflow-synced-badge.tsx` |
| 1 | `components/slack/slack-secret-fields.tsx` |
| 1 | `components/task/chat/context-popover.tsx` |
| 1 | `components/task/chat/messages/todo-message.tsx` |
| 1 | `components/task/chat/messages/tool-subagent-message.tsx` |
| 1 | `components/task/inspector/annotations-panel.tsx` |
| 1 | `components/task/mobile/mobile-repos-section.tsx` |
| 1 | `components/task/mobile/mobile-sessions-section.tsx` |
| 1 | `components/task/mobile/mobile-terminals-section.tsx` |
| 1 | `components/task/simple/components/session-timeline-entry.tsx` |
| 1 | `components/task/task-detach-confirm-dialog.tsx` |
