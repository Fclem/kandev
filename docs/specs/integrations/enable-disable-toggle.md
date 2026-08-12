---
status: shipped
created: 2026-08-06
owner: platform
---

# Integration Enable/Disable Toggle

## Why

Every third-party integration (Azure DevOps, GitHub, GitLab, Jira, Linear,
Sentry) can be connected with credentials, but only Jira, Linear and Sentry
currently expose a user-facing on/off switch for their UI surfaces. Azure
DevOps, GitHub and GitLab have no such switch. Users who want to temporarily
silence an integration (e.g. while its credentials are being rotated, or
because a workspace doesn't use it) have no consistent, discoverable way to
do that across all six integrations. (Slack was retired from the core app to
an external plugin after this spec was written and is out of scope.)

## What

- Every integration (Azure DevOps, GitHub, GitLab, Jira, Linear, Sentry)
  SHALL expose an enable/disable slider on its own settings page
  (`/settings/integrations/<slug>`, and the per-workspace equivalent
  `/settings/workspace/<id>/integrations/<slug>`). Jira, Linear and Sentry
  already have this; Azure DevOps, GitHub and GitLab gain it.
- The same slider SHALL also appear on each integration's row/card on the
  integrations index page (`/settings/integrations` and its per-workspace
  equivalent), so a user can enable or disable any integration without
  opening its detail page.
- Toggling the slider in either location SHALL keep both locations in sync
  (same underlying per-integration enabled state, install-wide — not
  per-workspace — consistent with the existing Jira/Linear/Sentry
  toggle).
- The enabled state for Azure DevOps, GitHub and GitLab currently affects
  only their settings controls — the own-page slider and the index-page row
  slider — and MUST NOT change whether their existing PR/work-item/board/MR
  features function. It does not gate left-panel navigation: nav visibility
  is configuration-only (see the next bullet), so these three integrations
  have no UI surface gated on an "available" concept beyond their settings
  pages and their settings-page banner.
- Left-panel nav visibility is configuration-only: a configured integration
  always appears in the sidebar Integrations section, the mobile-menu
  integrations group, and the Settings left panel's per-workspace
  Integrations branch regardless of its toggle state. The "Hide disabled
  integrations from left panel navigation" setting that once filtered those
  surfaces was removed —
  see `docs/specs/ui/remove-hide-disabled-nav-options.md`.
- The new setting SHALL NOT change any other behavior gated on an
  integration's existing "available" signal (e.g. Jira/Linear import
  popovers, Kanban external-link buttons, task-top-bar issue buttons) — those
  keep gating on enabled-and-authed exactly as they do today, independent of
  left-panel nav visibility.
- All new/changed toggles persist and sync the same way the existing
  Jira/Linear/Sentry "enabled" toggle does: a `localStorage`-backed,
  install-wide boolean, synced across browser tabs, with a manual-save
  affordance consistent with the rest of the Settings UI (`data-settings-dirty`
  + the shared save bar) where the control lives on a settings page.

## Data model

No new persistent (backend/database) state. All state introduced by this
feature lives in the browser's `localStorage`, matching the existing pattern
in `hooks/domains/integrations/use-integration-enabled.ts`:

| Key | Type | Default | Notes |
|---|---|---|---|
| `kandev:azure-devops:enabled:v1` | boolean | `true` | New. Mirrors `kandev:jira:enabled:v1`. |
| `kandev:github:enabled:v1` | boolean | `true` | New. |
| `kandev:gitlab:enabled:v1` | boolean | `true` | New. |

Existing keys (`kandev:jira:enabled:v1`, `kandev:linear:enabled:v1`,
`kandev:sentry:enabled:v1`) are unchanged.

## API surface

No backend/HTTP/WS changes. This is entirely a frontend feature.

Frontend primitives (new):

- `hooks/domains/azure-devops/use-azure-devops-enabled.ts` — exports
  `useAzureDevOpsEnabled()`, a one-line wrapper over
  `useIntegrationEnabled(storageKey, legacyKeyPrefix, syncEvent)`, mirroring
  `hooks/domains/jira/use-jira-enabled.ts`. No legacy key prefix existed
  before, so the legacy-migration argument is inert (no prior keys to
  migrate) but kept for signature symmetry.
- `hooks/domains/github/use-github-enabled.ts` — same shape, for GitHub.
- `hooks/domains/gitlab/use-gitlab-enabled.ts` — same shape, for GitLab.

Existing primitives reused unchanged:

- `hooks/domains/integrations/use-integration-enabled.ts` (`useIntegrationEnabled`)
- `hooks/domains/jira/use-jira-enabled.ts`, `use-linear-enabled.ts`,
  `use-sentry-enabled.ts`
- `hooks/domains/jira/use-jira-availability.ts`'s `useJiraAuthed` and
  `hooks/domains/linear/use-linear-availability.ts`'s `useLinearAuthed` —
  these already expose the "configured and healthy, independent of the
  enabled toggle" signal and are the building block for decoupled nav
  filtering.
- `components/integrations/use-drafted-integration-enabled.ts` +
  `components/integrations/drafted-integration-enabled-control.tsx` — the
  existing draft/save-wired slider component
  (`<DraftedIntegrationEnabledControl id={slug} enabled={enabled} persist={setEnabled} />`),
  reused as-is for Azure DevOps/GitHub/GitLab's own-page slider (new
  `<AzureDevOpsEnabledControl>`, `<GitHubEnabledControl>`,
  `<GitLabEnabledControl>` wrapper components, one per integration,
  mirroring `components/jira/jira-enabled-control.tsx`) and reused again for
  the index-page per-row sliders (all six integrations).

Modified:

- `hooks/use-nav-availability.ts`'s `useNavAvailability()` — returns each
  integration's `configured` boolean; its return shape (`AvailabilityMap`) is
  unchanged. Nav visibility no longer consults the enable/disable toggle (the
  removed "hide disabled" filter), only credential/health status.

## Permissions

No change. The toggle is a per-browser-profile, install-wide UI preference
with no authorization dimension (same as the existing Jira/Linear/Sentry
toggle).

## Failure modes

- `localStorage` unavailable or throwing (private browsing, quota) — the
  existing `useIntegrationEnabled` implementation already degrades to
  `enabled: true` and swallows write errors; the new hooks reuse this
  behavior.
- No integration ever hides from a *settings* page based on the toggle —
  a user can always reach a disabled integration's own settings page
  directly (by URL, from the index page, or from the sidebar's Settings
  section) and re-enable it.

## Persistence guarantees

State lives only in `localStorage`; it is not part of any backend restart or
task-execution durability guarantee, matching the existing enabled-toggle
convention. It survives a browser restart but not a `localStorage` clear, and
is not synced across devices/browsers.

## Scenarios

- **GIVEN** Jira is disabled, **WHEN** the user opens the Jira import popover
  on a task (a surface gated on Jira's combined enabled+authed "available"
  signal), **THEN** the import popover remains hidden exactly as it does
  today when Jira is disabled — the toggle's non-nav gating is unchanged.
- **GIVEN** the integrations index page, **WHEN** it renders, **THEN** every
  one of the six integration rows/cards shows its own enable/disable
  slider reflecting that integration's current stored state, and toggling a
  slider does not navigate to that integration's detail page.
- **GIVEN** the integrations index page and the Azure DevOps settings page
  open in two browser tabs, **WHEN** the user disables Azure DevOps on the
  index page in tab A, **THEN** tab B's Azure DevOps settings-page slider
  updates to "Disabled" after the next `storage` event (existing
  cross-tab-sync behavior, unchanged mechanism, extended to the three new
  integrations).

## Out of scope

- No backend/database changes; the toggle is a pure frontend,
  `localStorage`-backed preference.
- No change to Azure DevOps/GitHub/GitLab's PR, work-item, board, or MR
  features when disabled — the new toggle for these three integrations only
  controls settings-page/index-page slider state, not functional gating
  (unlike Jira/Linear/Sentry's existing toggle, which already gates other
  surfaces and is unchanged by this feature).
- No change to the Settings-page navigation tree's status-badge convention
  (`components/app-sidebar/sections/settings/workspaces-group.tsx`): its
  per-workspace Integrations list keeps the configured-status badge
  ("Enabled" = credentials/health present), which is unrelated to the
  enable/disable toggle. The list itself is unfiltered — every integration
  is listed there, disabled ones included.
- Sentry has no main-sidebar nav destination today (see
  `lib/navigation/core-destinations.ts` — only Azure DevOps, GitHub, GitLab,
  Jira and Linear are nav-gated). Sentry DOES appear in the Settings left
  panel's per-workspace Integrations tree (all six integrations are always
  listed there); its existing slider (own page + index-page row) still works
  for the enable/disable state itself.
- No new command-palette entries or keyboard shortcuts for the new toggles.

## Open questions

(none)
