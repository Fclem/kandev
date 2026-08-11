---
title: Repair workspace-scoped integration settings links
status: building
created: 2026-08-11
owner: kandev
---

# Repair workspace-scoped integration settings links

## Problem

Integration settings links that appear inside workspace-scoped surfaces pointed
at the global `/settings/integrations/<slug>` pages even when a workspace
context was available, and workspace-scoped paths were built by hand with
inconsistent encoding.

- The GitHub, GitLab, Jira, Linear, Azure DevOps and Sentry "not
  connected"/"disabled" notices and the Jira/Linear/Sentry error-message
  reconnect CTAs linked to the global settings page instead of the active
  workspace's integration settings.
- Workspace settings navigation (`/settings/workspaces/<id>/...`) was
  hand-assembled in several places with the raw workspace id, so ids with
  reserved URL characters broke the route, and the encoding form differed
  between call sites (raw vs `encodeURIComponent`).

After the settings menu restructure (`#2322`), the settings tree itself is a
static data-driven menu (no route-driven accordion), so the navigation
expansion defects previously reported for the old tree no longer apply; the
surviving defect is the inconsistent, unscoped integration settings links.

## Desired behavior

- Every integration settings link rendered from a workspace-scoped surface
  (page clients, dialogs, notices, error CTAs) targets
  `/settings/workspaces/<encoded-id>/integrations/<slug>` when a workspace
  context exists, and falls back to the global `/settings/integrations/<slug>`
  when it does not.
- All workspace settings paths are built with the shared
  `workspaceSettingsHref(workspaceId, tab)` helper
  (`apps/web/lib/settings/workspace-settings-tabs.ts`), which encodes the id;
  no surface hand-assembles the route.
- The change covers page clients (GitHub, GitLab, Jira, Linear, Azure DevOps,
  Sentry), their dialogs, the automations pages/editor/table, the task session
  error action, and the e2e page helpers.

## Regression scenarios

- **GIVEN** a workspace-scoped integration page or dialog, **WHEN** the
  not-connected/disabled notice or a reconnect CTA renders, **THEN** its link
  points at `/settings/workspaces/<encoded-id>/integrations/<slug>`.
- **GIVEN** no workspace context, **WHEN** the notice or CTA renders, **THEN**
  the link points at the global `/settings/integrations/<slug>`.
- **GIVEN** a workspace id with reserved URL characters, **WHEN** any
  workspace settings link is built, **THEN** the id is percent-encoded and the
  route round-trips through the router's decoder.
- **GIVEN** automations navigation (list, editor, table, page redirects),
  **WHEN** a workspace is present, **THEN** the destination uses the encoded
  workspace path via `workspaceSettingsHref`.

## Constraints

- Use the canonical `workspaceSettingsHref` helper; do not add a parallel
  route builder.
- The settings-discovery catalog and command palette keep their global
  routes by design; only workspace-scoped surfaces retarget.
- No new user-facing copy; workspace ids and integration slugs are user/brand
  data and are never translated.

## Out of scope

- The settings menu information architecture (superseded by `#2322`).
- The task-create dialog's `ConnectProvidersBanner`, which keeps its global
  link because the dialog is a global surface that may target any workspace.
