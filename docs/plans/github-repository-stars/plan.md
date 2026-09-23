---
created: 2026-09-17
status: draft
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
system_design:
  - ../../specs/integrations/system-design/github-repository-stars.md
legacy_specs: []
---
# Implementation Plan: GitHub repository stars

## Overview

Add a dedicated OAuth App-backed personal GitHub starring boundary and reuse it from every Settings > Plugins surface that displays a plugin repository. Implement the credential capability and persistence together with the provider/API behavior, then the repository/workspace/account-keyed frontend cache and controls, then prove desktop, phone, and coarse-pointer flows. The feature remains optional: provider failures do not block plugin browsing or management.

## Scope

### In scope

- Dedicated per-workspace/user OAuth App connection requesting `public_repo`, with encrypted token persistence, normalized scope metadata, generation-aware replacement, and reconnect/re-consent status.
- Strict-personal GitHub status, star, and unstar operations through that connection.
- Explicit `workspace_id` on all API requests, bounded raw-URL status hydration, layered workspace authorization, and independent backend URL validation.
- Shared frontend cache used by marketplace entries, installed rows, installed-plugin detail, and Canvases detail.
- Localized, accessible star controls with responsive non-mobile fine-pointer 24px compact inline sizing and touch-context 44px sizing.
- Unit, backend integration, and Playwright coverage for success, stale responses, account/workspace switches, reduced scope, and silent failure.
- Public plugin marketplace documentation updates for the new Settings > Plugins action.

### Out of scope

- Reusing the existing GitHub App user-to-server personal flow for starring.
- Persisting stars in Kandev or changing marketplace ranking.
- Non-GitHub providers, ratings, reviews, telemetry, or repository permissions.
- Changes to existing PR personal-write routing, plugin installation, lifecycle, or manifest schemas.

## Technical approach

Add the dedicated OAuth App configuration and separate star flow/state store, with a `github_personal_star_connection` row keyed by workspace/user. The deployment-configured star OAuth service requests `public_repo`, stages generation-specific encrypted token secret IDs, atomically CAS-writes metadata and pointers, compensates staged secrets on metadata failure, and cleans superseded secrets after commit. Reconnect preserves an existing valid connection when replacement consent fails. Expose a dedicated `PersonalStarClient` capability for this flow; do not widen `apps/backend/internal/github.Client` or add star methods to PAT/CLI implementations. The isolated strict-personal star resolver returns only that capability and bypasses only the workspace-repository registration check for this public-repository operation. Register bounded JSON status and mutation routes beside existing `/api/v1/github/…`

## ASCII UI preview

### UI-01: Marketplace entry with star state (desktop, phone, and coarse pointer)

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

Pixel 5 phone, coarse pointer (44px, stacked rows, no horizontal overflow)
┌──────────────────────┐
│ [icon] Plugin name   │
│ ★ 12  [ ☆ Star ]     │
│ [ Install ] [ ↗ Repo ]│
└──────────────────────┘
```

The star button is separate from repository navigation and row overlay links, uses `aria-pressed` only when capability is known, keeps a stable accessible name while busy, exposes localized `role="status"` feedback, and never asserts unstarred for Connect or Reconnect states. The drawing is structural, not pixel-exact. Task 02 component tests cover each state and Task 03 E2E tests cover desktop fine/coarse and touch geometry.

## Tests

- Dedicated OAuth App authorization tests cover `scope=public_repo`, token exchange scope extraction, normalized scope persistence, refresh preservation, generation replacement, unknown historical scope, reduced/declined re-consent, and status/reconnect exposure.
- GitHub client command construction and response mapping cover status, PUT, DELETE, and provider failure.
- Strict-personal routing tests cover App-backed workspaces with unregistered public repositories, missing/inactive/insufficient dedicated personal identity, App-principal rejection, and foreign-workspace denial without changing PR-write routing.
- Service-layer authorization tests cover missing and foreign workspace IDs before credential/provider calls; controller tests cover the same transport errors.
- Strict raw URL parser tests cover lookalike hosts, credentials, ports, extra paths, encoded separators, traversal-like segments, query/fragment, and `.git` canonicalization.
- Controller tests cover required workspace IDs, bounded status requests, duplicates, over-limit input, partial unknown results, mutations, and invalid repository URLs.
- Frontend hook/API tests cover scoped cache hydration, workspace/account changes, shared-key updates, failed mutation rollback, omitted status keys, and stale status-after-mutation ordering.
- Responsive control tests cover 24px non-mobile fine-pointer compact sizing, 44px mobile fine-pointer sizing, 44px coarse-pointer sizing at desktop width, keyboard activation, and busy state.

## E2E tests

- `apps/web/e2e/tests/auth/plugin-repository-stars.spec.ts` covers marketplace, installed-row, installed-plugin detail, and Canvases detail load, star, unstar, reload state, Connect/Reconnect outcomes, and silent provider failure under a dedicated authenticated CI job using the existing `auth` project.
- `apps/web/e2e/tests/settings/mobile-plugin-repository-stars.spec.ts` covers Pixel 5 touch/coarse hit areas across marketplace, installed row, installed detail, and Canvases detail, accessible state, and no horizontal overflow.
- `apps/web/e2e/tests/settings/desktop-coarse-pointer-plugin-repository-stars.spec.ts` covers the desktop-width coarse-pointer context in its own project and is omitted from the mobile and fine-pointer manifests.
- `apps/web/e2e/tests/settings/mobile-fine-pointer-plugin-repository-stars.spec.ts` covers the 767px fine-pointer context separately and is included in the normal shard manifest.

## Work orders

- [x] [Task 01: Add personal star auth and API](task-01-github-api.md)
- [x] [Task 02: Add plugin settings star controls](task-02-plugin-settings-controls.md)
- [x] [Task 03: Add browser star flows](task-03-plugin-stars-e2e.md)

## Verification results

All three work orders are implemented and verified on `feature/sync-plugin-reposito-673` at
`12c6a5d6f`; each work order's `## Results` carries its own commits and gates.

Passing at head: the Task 01 verification block (gates 1-11, including the Windows cross-build, the
Linux-only daemon build, the runtime-bundle contract, and the `-race` unsupported-target filter);
`go test` across `internal/github`, `internal/backendapp`, `internal/secrets`,
`internal/system/deploymentsecret`, `internal/profiles` and `internal/launcher` (one ambient
exception below); `golangci-lint` 0 issues on the touched packages; web `typecheck`, eslint and
`i18n:check`; 216 files / 1911 frontend tests; the five star E2E specs (14/3/2/2/2) and the E2E
project, shard-planner and fixture regressions (16 workflow-contract cases); and the public-docs and
specification validators.

Known residual gaps, recorded rather than hidden:
- The PostgreSQL star-store lifecycle test ships but skips without `KANDEV_TEST_POSTGRES_DSN`, so
  Postgres has not executed here.
- The capable-path E2E cohort depends on the e2e profile's
  `KANDEV_DEPLOYMENT_SECRET_TARGET=supported` override inside containerised CI jobs; that path has
  not yet run in a real container.
- `docs/specs/integrations/system-design/github-repository-stars.md` has roughly 195 bytes of
  headroom against its 32768-byte limit.
- `internal/launcher` `TestRunStartBuildVersion` fails on this workstation because the live kandev
  service holds port 38429; it is environmental, reproducible with the port variables unset.

## Risks

- Existing GitHub App workspaces require an additional dedicated OAuth App consent for personal stars.
- Existing historical connections may lack scope metadata; unknown capability must fail closed and require re-consent.
- Multiple catalog entries can reference one repository, so cache identity must be scoped by repository, workspace, authenticated user, and credential generation.
- Existing plugin rows have overlay navigation and mobile action geometry; the control must not intercept or break those paths.
- Public documentation and all five locale catalogs must remain synchronized with the new user-facing action.
