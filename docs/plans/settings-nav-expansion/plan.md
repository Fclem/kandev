---
spec: docs/specs/settings-nav-expansion/spec.md
created: 2026-08-11
status: done
---

# Implementation Plan: Workspace-Scoped Integration Settings Links

## Overview

The settings menu restructure (`#2322`, merged after this branch forked)
superseded the original settings-tree work: the new menu is static and
data-driven, so the navigation-expansion defects and the top-level Integrations
shortcut no longer apply. This rebased change keeps the surviving, still-needed
part: consistently scope integration settings links to the active workspace,
built with the canonical `workspaceSettingsHref(workspaceId, tab)` helper
(`apps/web/lib/settings/workspace-settings-tabs.ts`), with global fallbacks.

## Changes

### Notices and error CTAs

- GitHub `NotAuthenticatedNotice`, GitLab `NotConnectedNotice`, Jira
  `NotConfiguredNotice`, Linear `NotConfiguredNotice` + `DisabledNotice`,
  Azure DevOps `NotConfigured`, and the Jira/Linear/Sentry `ErrorMessage`
  reconnect CTAs accept `workspaceId` and build the scoped href via
  `workspaceSettingsHref(workspaceId, "integrations")`; without a workspace
  they keep `/settings/integrations/<slug>`.
- `workspaceId` is threaded through the page clients (`TicketResults`,
  `ResultsArea`) and dialogs (`JiraTicketDialog`, `LinearIssueDialog`,
  `SentryIssueDialog` and their body components).
- Azure DevOps and GitHub notices also adopt the canonical helper (fixing the
  raw-id/plural inconsistencies introduced by the restructure).
- Notice/error components are exported for tests; each test pins the encoded
  scoped href (with a non-URL-safe id `"ws 1/2"`) and the global fallback.

### Automations and navigation

- `automation-editor`, `automations-list-page`, `automations-table`, and
  `ensure-session-error` replace hand-assembled `/settings/workspaces/<id>/...`
  paths with `workspaceSettingsHref(workspaceId, <tab>)`.
- e2e page helpers (`github-auth-settings-page`, `gitlab-settings-page`,
  `linear-settings-page`, `automations-page`, `workflow-settings-page`) use the
  same helper.

## Verification

```bash
cd apps/web
pnpm exec vitest run app/gitlab app/jira app/linear app/github app/azure-devops \
  components/jira components/linear components/sentry components/automations components/task
pnpm run typecheck
pnpm run i18n:ratchet
pnpm exec eslint app/gitlab app/jira app/linear app/github app/azure-devops \
  components/jira components/linear components/sentry components/automations components/task e2e/pages
pnpm e2e:run tests/settings/settings-gear-only.spec.ts tests/workflow/workflow-settings.spec.ts \
  automations-settings.spec.ts
```

## Risks

- Import placement: the helper import must land after `"use client"` /
  directive comments.
- The e2e helper files are not covered by web typecheck, so runtime E2E runs
  are the verification for them.

## Open Questions

None.
