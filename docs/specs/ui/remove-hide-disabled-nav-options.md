---
status: draft
created: 2026-08-12
owner: platform
---

# Remove Hide-Disabled Left-Panel-Nav Options

## Why

The "Hide disabled integrations from left panel navigation" and "Hide
disabled agent profiles from left panel navigation" settings filter
tree/list entries out of the left panel navigation. The left-panel
navigation no longer renders those tree/list surfaces, so the settings
no longer control anything a user can see. Keeping them means two
obsolete rows on the integrations and agents settings pages plus a
`localStorage`-backed filter chain that nothing observable consumes.

## What

- The integrations index page (`/settings/integrations` and its
  workspace-scoped equivalent) SHALL no longer render the "Hide disabled
  integrations from left panel navigation" row. The six per-integration
  enable/disable toggles on that page are unchanged.
- The agents settings page (`/settings/agents`) SHALL no longer render
  the "Hide disabled agent profiles from left panel navigation" row.
- Nav visibility of integrations SHALL be gated by configuration only: a
  configured integration always appears in the sidebar Integrations
  section, the mobile-menu integrations group, and the Settings left
  panel's per-workspace Integrations branch, regardless of its
  enable/disable toggle state.
- A disabled agent profile SHALL always appear in the Settings left
  panel's Agents tree (with its "Disabled" badge), regardless of the
  profile's `enabled` state.
- The old storage keys (`kandev:integrations:hideDisabledInNav:v1`,
  `kandev:agents:hideDisabledInNav:v1`) SHALL no longer be read or
  written. Leftover values in a user's `localStorage` are inert and
  require no migration.
- Nothing else gated on an integration's or profile's `enabled` state
  changes: pickers, import popovers, task buttons, session/handoff
  selectors, and the profile "no compatible agent" empty states keep
  their existing gating.

## Data model

Two `localStorage` keys become inert:

| Key | Type | Notes |
|---|---|---|
| `kandev:integrations:hideDisabledInNav:v1` | boolean | No longer read or written; leftover values ignored. |
| `kandev:agents:hideDisabledInNav:v1` | boolean | No longer read or written; leftover values ignored. |

No backend/database state exists for either setting; no migration is
needed. The shared `useLocalStorageBoolean` primitive and the two domain
hooks that wrap these keys are removed with the settings.

## Failure modes

None new. The removal deletes the only readers of these storage keys, so
no storage failure mode remains.

## Scenarios

- **GIVEN** a configured but disabled integration (e.g. GitHub's toggle
  off), **WHEN** the user opens the sidebar Integrations section or the
  mobile menu, **THEN** the integration's nav entry is visible; no
  "hide disabled" control exists anywhere on the integrations settings
  page to change that.
- **GIVEN** a disabled agent profile, **WHEN** the user opens the
  Settings left panel's Agents tree in a tree menu mode, **THEN** the
  profile is listed with its "Disabled" badge, and no "hide disabled"
  control exists on `/settings/agents`.
- **GIVEN** an integration with no credentials saved, **WHEN** the user
  opens the sidebar Integrations section, **THEN** it stays hidden
  exactly as today (configuration still gates nav visibility).
- **GIVEN** a user whose browser previously stored either
  `kandev:integrations:hideDisabledInNav:v1` or
  `kandev:agents:hideDisabledInNav:v1` as `"true"`, **WHEN** they load
  the app after this change, **THEN** the stored value has no effect on
  any nav surface.

## Out of scope

- The per-integration enable/disable toggles themselves (Azure DevOps,
  GitHub, GitLab, Jira, Linear, Sentry) and everything they gate outside
  left-panel nav.
- The profile `enabled` toggle and the `DisabledBadge` convention.
- The settings-menu tree modes (`accordion`/`persistent`) and the flat
  menu; the tree branches keep listing every integration and every
  profile, unfiltered.
- Any backend, HTTP, or WS change.

## Open questions

(none)
