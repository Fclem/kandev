---
id: "06-star-actor-controls"
title: "Disclose star actor in plugin controls"
status: pending
wave: 6
depends_on:
  - 05-star-actor-resolution
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.3
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.3
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.5
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.7
system_design:
  - ../../specs/integrations/system-design/github-repository-star-actors.md
  - ../../specs/integrations/system-design/github-repository-stars.md
---
# Task 06: Disclose star actor in plugin controls

## Summary

Consume the extended connection status from Task 05 in the shared Settings > Plugins star control:

- Map the extended truth table.
- Scope the cache by actor source and login.
- Disclose the acting GitHub login, including when it is shared.
- Route App-workspace Connect through the existing personal-connection flow.
- Refetch status after a failed mutation.

Geometry, surfaces, and the dedicated flow behavior stay unchanged.

## In scope

- `apps/web/lib/types/github-star.ts`: add `actor: {source, login, shared} | null`, `connect: "personal_star_oauth" | "github_app_user" | null`, and `remediation: "token_scope" | "app_permission" | null`.
- `apps/web/lib/plugins/plugin-star-connection.ts`:
  - `resolvePluginStarControlMode` follows the actor design truth table. A resolved but incapable actor maps to `omit`; `connect` selects the CTA flow.
  - `pluginStarKey` adds `actor.source` and `actor.login`.
- `apps/web/hooks/domains/plugins/use-plugin-repository-stars.ts`:
  - `connect` dispatches to `startGitHubStarConnection` or `startGitHubPersonalConnect` (`github-auth-api.ts`) by `status.connect`.
  - A failed mutation triggers one connection-status refetch.
  - Expose `actor` to the control.
- `apps/web/components/settings/plugins/plugin-repository-star.tsx`: add a localized tooltip on fine pointers plus `aria-description` with "Starring as @{{login}}" or the shared variant. The button's accessible name stays unchanged.
- `apps/web/src/locales/*/plugins.json`: new keys in all seven locales and pseudo. Use `pnpm run i18n:zh-hant` for the Traditional Chinese pair. The login is a placeholder, never concatenated, and the copy has no em dash.

## Exclusions

- Backend changes, E2E specs, and public docs (Task 07).
- New surfaces, geometry changes, or a remediation UI for operators.

## Acceptance

1. Unit tests map every actor-design truth-table row to `omit`, `stars`, or the correct `connect` CTA flow. An incapable resolved actor never asserts unstarred and renders no CTA.
2. Hook tests show that a source or login switch with an unchanged generation neither displays nor accepts responses for the previous actor key. Clicking the App-workspace CTA calls `startGitHubPersonalConnect` and navigates to its URL. A failed mutation refetches connection status exactly once and shows no toast.
3. Control tests assert that the disclosure text for shared and personal actors appears as `aria-description` and a tooltip, and that the accessible name stays unchanged while busy.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/plugins/plugin-star-connection.test.ts hooks/domains/plugins/use-plugin-repository-stars.test.tsx components/settings/plugins/plugin-repository-star.test.tsx lib/api/domains/github-star-api.test.ts lib/state/slices)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint -- lib/plugins/plugin-star-connection.ts hooks/domains/plugins/use-plugin-repository-stars.ts components/settings/plugins/plugin-repository-star.tsx lib/types/github-star.ts)
(cd apps/web && pnpm run i18n:check)
```

## Likely files

`apps/web/lib/types/github-star.ts`, `apps/web/lib/plugins/plugin-star-connection{,.test}.ts`, `apps/web/hooks/domains/plugins/use-plugin-repository-stars{.ts,.test.tsx}`, `apps/web/components/settings/plugins/plugin-repository-star{.tsx,.test.tsx}`, the plugin-stars store slice if its connection entry stores the tuple, and `apps/web/src/locales/*/plugins.json`.

## Risks

- `aria-description` support varies by screen reader; also add `title` on fine pointers only, so touch contexts do not depend on hover.
- The personal-connection callback lands on GitHub settings, not Settings > Plugins; the user returns manually, and the status reloads on page load.
