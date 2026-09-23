---
id: "02-plugin-settings-controls"
title: "Add plugin settings star controls"
status: pending
wave: 2
depends_on:
  - 01-github-api
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.3
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.5
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.7
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.9
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.10
system_design:
  - ../../specs/integrations/system-design/github-repository-stars.md
---
# Task 02: Add plugin settings star controls

## Summary

Add a cache scoped by the explicitly supplied workspace ID, authenticated user ID, personal GitHub credential generation, and canonical repository, then compose the shared star control into marketplace entries, installed plugin rows, installed plugin detail, and Canvases detail. Every composition passes its owning workspace ID into the shared hook/control; Canvases passes the selected canvas workspace, never implicitly `workspaces.activeId`. Hydrate on page load, fence stale responses with the full workspace/user/generation scope, update only after confirmed GitHub mutations, and leave existing plugin flows usable when the optional provider call fails.

## In scope

- Typed GitHub API methods carrying `workspace_id` and raw repository URLs.
- Cache hook with per-key request epochs and workspace/user/account invalidation. A late response for workspace A/user A/generation 1 must not populate workspace A/user B/generation 1.
- One strict frontend repository URL parser with backend conformance vectors.
- Shared localized control with `aria-pressed`, stable accessible name, localized `role="status"`, keyboard behavior, and responsive sizing from the existing responsive hook: `isTouchContext = isMobile || !isFinePointer`; 24px compact inline only when non-mobile and fine-pointer, minimum 44px for all touch contexts.
- Marketplace, installed row, installed-plugin detail, `canvas-marketplace-detail.tsx`, and `CanvasInstallDialog` composition, including overlay z-order. `available=false` omits star/CTA and blocks start; available but inactive/incapable renders localized Connect/Reconnect instead of asserting unstarred; capable reconnect-required keeps star controls plus reconnect CTA. Dialog uses `review.metadata.repo_url` and its selected `workspaceId`.
- Connection-status hydration before repository status, full workspace/user/generation/capability/availability fencing, reconnect/account-switch invalidation, and start/disconnect outcome handling. Start uses the authenticated endpoint with a validated same-origin return path; unavailable deployments issue no start request or socket/provisioning side effect.
- Focused frontend tests for all five surfaces, selected-workspace A/B scope switching, dialog review URL, provider-unknown omission, and `available=false` omission with no start request. Include no connection, reduced scope, declined consent, successful replacement, disconnect, null/omitted status keys, failed hydration, stale/duplicate/unknown outcomes, pointer geometry, busy, and scope transitions; never render `aria-pressed=false` when unavailable.

## Out of scope

- Backend routes or GitHub client behavior.
- New plugin permissions or persistent Kandev state.
- Public documentation, owned by Task 03.

## Acceptance

- Valid GitHub repository URLs with connection status `available=true` show a shared star control in marketplace, installed-row, installed-plugin detail, and Canvases detail surfaces; other URLs do not. With `available=false`, all surfaces omit the star control and actionable CTA and issue no start request. Missing or insufficient dedicated connection while available shows Connect/Reconnect and never asserted unstarred. Callback outcome values trigger only a scoped refetch and cannot produce an asserted result independently.
- Non-mobile fine-pointer controls use the existing 24px compact inline token; mobile fine-pointer and all coarse-pointer controls meet 44px targets in idle and busy states.

## ASCII UI preview

```text
Non-mobile fine-pointer desktop, capable (24px compact inline control)
┌──────────────────────────────────────────────┐
│ [icon] Plugin name  v1.2              [Install]│
│ categories   ★ 12   [☆] [↗ Repo]              │
└──────────────────────────────────────────────┘

Desktop coarse/touch context, capable + reconnect required (44px)
┌──────────────────────────────────┐
│ [icon] Plugin name        [Install]│
│ ★ 12 [ ☆ Star ] [ Reconnect ]    │
│              [ ↗ Repo ]           │
└──────────────────────────────────┘

Connect or Reconnect required, no asserted star state
┌────────────────────────────┐
│ [icon] Plugin name          │
│ [ Connect GitHub ]          │
│ [ ↗ Repo ]                  │
└────────────────────────────┘

Busy mutation, stable accessible name
┌────────────────────────────┐
│ [icon] Plugin name          │
│ [ ☆ Star ... ]  role=status │
└────────────────────────────┘

Provider unknown or failed hydration (omit star control, never `aria-pressed=false`)
┌────────────────────────────┐
│ [icon] Plugin name          │
│ [↗ Repo]                   │
└────────────────────────────┘
```

The star button remains separate from repository navigation and row overlay links, uses `aria-pressed` only when capability is known, keeps a stable accessible name while busy, exposes localized `role="status"` feedback, and never asserts unstarred for Connect, Reconnect, or unavailable states. The drawing is structural, not pixel-exact. See [plan UI-01](plan.md#ui-01-marketplace-entry-with-star-state-desktop-phone-and-coarse-pointer).

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/settings/plugins/plugin-repository-star.test.ts components/settings/plugins/canvas-install-dialog.test.tsx components/settings/plugins/marketplace-entry-row.test.tsx components/settings/plugins/plugin-row.test.tsx components/settings/plugins/plugin-repo-link.test.tsx components/settings/plugins/canvas-marketplace-detail.test.tsx components/settings/plugins/canvas-marketplace.test.ts lib/api/domains/github-api.test.ts hooks/domains/plugins/use-plugin-repository-stars.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint -- components/settings/plugins/marketplace-entry-row.tsx components/settings/plugins/plugin-row.tsx components/settings/plugins/plugin-detail.tsx components/settings/plugins/plugin-repository-star.tsx components/settings/plugins/plugin-repo-link.tsx components/settings/plugins/canvas-marketplace-detail.tsx components/settings/plugins/canvas-marketplace.tsx components/settings/plugins/canvas-install-dialog.tsx hooks/domains/plugins/use-plugin-repository-stars.ts)
(cd apps/web && pnpm run i18n:check)
```

## Files likely touched

- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/hooks/domains/plugins/use-plugin-repository-stars.ts` (new)
- `apps/web/components/settings/plugins/plugin-repository-star.tsx` (new)
- `apps/web/components/settings/plugins/plugin-repository-star.test.ts` (new)
- `apps/web/hooks/domains/plugins/use-plugin-repository-stars.test.ts` (new)
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/components/settings/plugins/marketplace-entry-row.tsx`
- `apps/web/components/settings/plugins/plugin-row.tsx`
- `apps/web/components/settings/plugins/plugin-detail.tsx`
- `apps/web/components/settings/plugins/marketplace-entry-row.test.tsx`
- `apps/web/components/settings/plugins/plugin-row.test.tsx`
- `apps/web/components/settings/plugins/plugin-repo-link.test.tsx`
- `apps/web/components/settings/plugins/canvas-marketplace-detail.test.tsx`
- `apps/web/components/settings/plugins/canvas-marketplace.test.ts`
- `apps/web/components/settings/plugins/canvas-install-dialog.test.tsx`
- `apps/web/components/settings/plugins/plugin-repo-link.tsx`
- `apps/web/src/locales/*/plugins.json`
- `apps/web/components/settings/plugins/canvas-marketplace-detail.tsx`
- `apps/web/components/settings/plugins/canvas-marketplace.tsx`
- `apps/web/components/settings/plugins/canvas-install-dialog.tsx`
- `apps/web/components/settings/plugins/canvas-install-dialog.test.tsx`

## Dependencies

Task 01 API contract.

## Risks

- Overlay links and mobile layouts can swallow or crowd the new control.
- React StrictMode and workspace/user/account switches can expose stale asynchronous responses.
- Applying `isFinePointer` without the mobile branch would regress touch targets on narrow fine-pointer devices; derive `isTouchContext` explicitly.

## Parallelism

`sequential`

## Inputs

- Task 01 API contract.
- Existing plugin marketplace, installed-row, and detail components.
- Mobile control sizing and i18n conventions.

## Results

Status: complete. Feature commits `238cce660`, `286611f46`, `544e5b3e3`, `0e4f2d1bd` (plus the
identity fixture alignment in `0e4f2d1bd`).

Delivered: the typed star API client and the canonical repository-URL parser shared with the backend
grammar; the `pluginStars` slice with per-key epochs and a connection tuple keyed by workspace,
authenticated user, credential generation, and capability; the `usePluginRepositoryStars` hook that
reads connection status first, hydrates only for a capable tuple, fences status and mutation
responses, and clears visible state when the workspace or authenticated account changes; the
`PluginRepositoryStar` control implementing the status truth table (omit when unavailable, Connect,
Reconnect, controls plus a separate reconnect CTA) with 24px compact and 44px touch geometry; wiring
into the marketplace row, installed row, installed-plugin detail, Canvases detail, and the Canvas
install dialog; the navigation-only callback-outcome handler that removes `star_connection_outcome`
with `history.replaceState`; and all copy in five locales plus pseudo.

Gates:
- `pnpm run typecheck` clean; eslint clean on the touched files.
- `pnpm run i18n:check` green (keys, `<Trans>` indices, plurals, module-scope `t()`, em dashes,
  non-JSX copy; pt-pt, zh-cn, zh-hk, zh-tw complete and pseudo in sync).
- `pnpm exec vitest run components/settings/plugins app/settings/plugins hooks/domains/plugins
  lib/state/slices lib/plugins lib/api/domains e2e/scripts` -> 216 files / 1911 tests pass.
- The star suites specifically: parser 51, hook + slice + api + control suites green, including the
  account-switch, stale-response, superseded-epoch, and unknown-state cases.

Residual gap: none specific to this work order; see the plan-level residuals (the coarse-pointer and
mobile geometry is asserted by the Task 03 E2E cohort, not by unit tests).
