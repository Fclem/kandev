---
created: 2026-09-17
status: draft
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002
system_design:
  - ../../specs/integrations/system-design/github-repository-stars.md
  - ../../specs/integrations/system-design/github-repository-star-actors.md
  - ../../specs/integrations/system-design/deployment-secret-store.md
legacy_specs: []
---
# Implementation Plan: GitHub repository stars

## Overview

Add a dedicated OAuth App-backed personal GitHub starring boundary and reuse it from every Settings > Plugins surface that displays a plugin repository. Implement the credential capability and persistence together with the provider/API behavior, then the repository/workspace/account-keyed frontend cache and controls, then prove desktop, phone, and coarse-pointer flows. The feature remains optional: provider failures do not block plugin browsing or management.

Tasks 05-07 widen the star actor from the dedicated OAuth App to every builtin GitHub authentication scheme ([actor design](../../specs/integrations/system-design/github-repository-star-actors.md), [ADR-2026-10-18-github-star-actor-resolution](../../decisions/2026-10-18-github-star-actor-resolution.md)): the user's dedicated connection, then their GitHub App personal connection, then the workspace PAT, `gh` CLI, or legacy shared credential. The dedicated OAuth App and Task 04 stay, as an optional source.

## Scope

### In scope

- Dedicated per-workspace/user OAuth App connection requesting `public_repo`, with encrypted token persistence, normalized scope metadata, generation-aware replacement, and reconnect/re-consent status.
- Star status, star, and unstar operations through one resolved actor across every builtin scheme (Tasks 05-07), with per-source capability, actor disclosure, and `starring: write` on new App registrations.
- Explicit `workspace_id` on all API requests, bounded raw-URL status hydration, layered workspace authorization, and independent backend URL validation.
- Shared frontend cache used by marketplace entries, installed rows, installed-plugin detail, and Canvases detail.
- Localized, accessible star controls with responsive non-mobile fine-pointer 24px compact inline sizing and touch-context 44px sizing.
- Unit, backend integration, and Playwright coverage for success, stale responses, account/workspace switches, reduced scope, and silent failure.
- Public plugin marketplace documentation updates for the new Settings > Plugins action.

### Out of scope

- Granting GitHub permissions or token scopes on the operator's behalf; Kandev reports `remediation` only.
- Persisting stars in Kandev or changing marketplace ranking.
- Non-GitHub providers, ratings, reviews, telemetry, or repository permissions.
- Changes to existing PR personal-write routing, plugin installation, lifecycle, or manifest schemas.

## Technical approach

Add the dedicated OAuth App configuration and separate star flow/state store, with a `github_personal_star_connection` row keyed by workspace/user. The deployment-configured star OAuth service requests `public_repo`, stages generation-specific encrypted token secret IDs, atomically CAS-writes metadata and pointers, compensates staged secrets on metadata failure, and cleans superseded secrets after commit. Reconnect preserves an existing valid connection when replacement consent fails. Expose a dedicated `PersonalStarClient` capability for this flow; do not widen `apps/backend/internal/github.Client` or add star methods to PAT/CLI implementations. The isolated strict-personal star resolver returns only that capability and bypasses only the workspace-repository registration check for this public-repository operation. Register bounded JSON status and mutation routes beside existing `/api/v1/github/…`

Tasks 05-07 add `StarActorResolver` in front of the existing star client. It consults the dedicated connection first, then `CredentialResolver.Resolve` with a new `CredentialPurposeStar`, rejects App principals, and filters through a process-local `StarCapabilityCache`: an `X-OAuth-Scopes` probe for token sources, a registration `starring: write` check for App users, and a per-generation `403` denial record. Every source issues the same `/user/starred` requests through `PersonalStarHTTPClient`. The connection status gains `actor`, `connect`, and `remediation`; the frontend keys its cache by actor source and login, discloses the acting login, routes App-workspace Connect to the existing personal-connection flow, and refetches status after a failed mutation.

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

### UI-02: Actor disclosure and App-workspace connect (Tasks 06-07)

```text
Desktop, shared workspace credential (gh / PAT / legacy)
┌──────────────────────────────────────────────┐
│ [icon] Plugin name  v1.2              [Install]│
│ categories   ★ 12   [☆ Star] [↗ Repo]          │
│                     └ tooltip: Starring as @octocat (shared workspace account)
└──────────────────────────────────────────────┘

Desktop, personal actor (dedicated OAuth or App personal connection)
│ categories   ★ 12   [★ Unstar] [↗ Repo]        │
│                     └ tooltip: Starring as @alice

App-installation workspace, no personal connection
┌────────────────────────────┐
│ [icon] Plugin name          │
│ [ Connect GitHub ]          │  → personal-connection flow, lands on GitHub settings
│ [ ↗ Repo ]                  │
└────────────────────────────┘

Actor resolved but incapable (missing scope / App permission): no control, no CTA
┌────────────────────────────┐
│ [icon] Plugin name   ★ 12   │
│ [ ↗ Repo ]                  │
└────────────────────────────┘

Pixel 5 phone, shared actor (44px; disclosure via aria-description, no tooltip reliance)
┌──────────────────────┐
│ [icon] Plugin name   │
│ ★ 12  [ ☆ Star ]     │
│ [ Install ] [ ↗ Repo ]│
└──────────────────────┘
```

The disclosure never changes the button's accessible name; it is an `aria-description` plus a tooltip on fine pointers. Geometry is unchanged from UI-01.

## Tests

- Dedicated OAuth App authorization tests cover `scope=public_repo`, token exchange scope extraction, normalized scope persistence, refresh preservation, generation replacement, unknown historical scope, reduced/declined re-consent, and status/reconnect exposure.
- GitHub client command construction and response mapping cover status, PUT, DELETE, and provider failure.
- Strict-personal routing tests cover App-backed workspaces with unregistered public repositories, missing/inactive/insufficient dedicated personal identity, App-principal rejection, and foreign-workspace denial without changing PR-write routing.
- Service-layer authorization tests cover missing and foreign workspace IDs before credential/provider calls; controller tests cover the same transport errors.
- Strict raw URL parser tests cover lookalike hosts, credentials, ports, extra paths, encoded separators, traversal-like segments, query/fragment, and `.git` canonicalization.
- Controller tests cover required workspace IDs, bounded status requests, duplicates, over-limit input, partial unknown results, mutations, and invalid repository URLs.
- Frontend hook/API tests cover scoped cache hydration, workspace/account changes, shared-key updates, failed mutation rollback, omitted status keys, and stale status-after-mutation ordering.
- Responsive control tests cover 24px non-mobile fine-pointer compact sizing, 44px mobile fine-pointer sizing, 44px coarse-pointer sizing at desktop width, keyboard activation, and busy state.
- Actor resolution tests (Task 05) cover each source and the precedence order, App-principal rejection, no lower-source fallback after an incapable or failed higher source, scope-probe outcomes (classic with/without `repo`/`public_repo`, missing header, probe failure), registration `starring` permission, `403` denial recording and invalidation on generation change, the connect-path rules, the extended status contract, and import-policy tolerance of a missing `starring` permission.
- Frontend tests (Task 06) cover the extended truth table, actor-keyed cache isolation across source/login switches, the App-workspace CTA call, actor disclosure copy, and the post-failure status refetch.

## E2E tests

- `apps/web/e2e/tests/auth/plugin-repository-stars.spec.ts` covers marketplace, installed-row, installed-plugin detail, and Canvases detail load, star, unstar, reload state, Connect/Reconnect outcomes, and silent provider failure under a dedicated authenticated CI job using the existing `auth` project.
- `apps/web/e2e/tests/settings/mobile-plugin-repository-stars.spec.ts` covers Pixel 5 touch/coarse hit areas across marketplace, installed row, installed detail, and Canvases detail, accessible state, and no horizontal overflow.
- `apps/web/e2e/tests/settings/desktop-coarse-pointer-plugin-repository-stars.spec.ts` covers the desktop-width coarse-pointer context in its own project and is omitted from the mobile and fine-pointer manifests.
- `apps/web/e2e/tests/settings/mobile-fine-pointer-plugin-repository-stars.spec.ts` covers the 767px fine-pointer context separately and is included in the normal shard manifest.
- Task 07 extends `apps/web/e2e/tests/auth/plugin-repository-stars.spec.ts` with a workspace-PAT actor (shared disclosure, star/unstar/reload), an App-installation workspace without a personal connection (Connect GitHub CTA), an App personal-connection actor, an incapable actor (no control), and a mid-session `403` that removes the control silently.

## Work orders

- [x] [Task 01: Add personal star auth and API](task-01-github-api.md)
- [x] [Task 02: Add plugin settings star controls](task-02-plugin-settings-controls.md)
- [x] [Task 03: Add browser star flows](task-03-plugin-stars-e2e.md)
- [ ] [Task 04: Protect deployment secret store](task-04-protected-deployment-secret-store.md)
- [ ] [Task 05: Resolve star actors across GitHub auth schemes](task-05-star-actor-resolution.md)
- [ ] [Task 06: Disclose star actor in plugin controls](task-06-star-actor-controls.md)
- [ ] [Task 07: Prove multi-scheme star flows](task-07-star-actor-e2e.md)
- [x] [Task 08: Synchronize repository star counts](task-08-star-count-synchronization.md)

## Verification results

All three work orders are implemented and verified on `feature/sync-plugin-reposito-673`; each work
order's `## Results` carries its own commits and gates, including the corrections that followed it.

Task 04 is pending. Round 11 found that the backend service account can replace deployment secret-store files through its writable data-directory parent. Task 04 moves identity and the store to the root-owned, installation-ID-keyed layout under `/var/lib/kandev-deployment-secrets/` and makes a clean cutover from the unreleased data-directory layout without migration. It claims no implementation verification.

Tasks 05-07 are pending design work orders from the multi-scheme decision; they claim no implementation verification.

Passing at head: the Task 01 verification block (gates 1-11, including the Windows cross-build, the
Linux-only daemon build, the runtime-bundle contract, and the `-race` unsupported-target filter);
`go test` across `internal/github`, `internal/backendapp`, `internal/secrets`,
`internal/system/deploymentsecret`, `internal/profiles` and `internal/launcher` (one ambient
exception below); `golangci-lint` 0 issues on the touched packages; `go run ./cmd/sqlguard ./internal`
exits 0; web `typecheck`, eslint and `i18n:check`; 216 files / 1911 frontend tests; the five star
E2E specs (14/3/2/2/2) and the E2E project, shard-planner and fixture regressions (16
workflow-contract cases); and the public-docs and specification validators.

Known residual gaps, recorded rather than hidden:
- Four packages fail only on this workstation. Two of them,
  `internal/agentctl/server/config` (`TestCollectAgentEnvGitHubCLIShimSurvivesLoginShell`) and
  `internal/common/config` (`TestConfigSourceLoadsStableYAMLFields`), were re-run at the pre-change
  base commit `57bbae10a` in a separate worktree and fail there identically, so they are
  pre-existing on this host (its login shell puts a real `gh` ahead of the test shim, and its
  ambient config wins the discovery assertions). The other two,
  `internal/agent/settings/handlers` and `internal/system/deploymentsecret`, fail only under
  `make`'s package parallelism: the first with `database is locked` timeouts, the second with the
  fail-closed socket test losing a scheduling race, and both pass isolated (`-count=3` and
  `-count=20` respectively). The second race is now tolerated by that test; the first is a load
  artifact of this host.
- The PostgreSQL star-store lifecycle test ships but skips without `KANDEV_TEST_POSTGRES_DSN`, so
  Postgres has not executed here.
- The capable-path E2E cohort depends on the e2e profile's
  `KANDEV_DEPLOYMENT_SECRET_TARGET=supported` override inside containerised CI jobs; that path has
  not yet run in a real container.
- `docs/specs/integrations/system-design/github-repository-stars.md` has roughly 240 bytes of
  headroom against its 32768-byte limit; actor resolution lives in `github-repository-star-actors.md`.
- `internal/launcher` `TestRunStartBuildVersion` fails on this workstation because the live kandev
  service holds port 38429; it is environmental, reproducible with the port variables unset.
- The supervisor's socket path is bounded by the platform's unix socket limit (107 bytes on Linux).
  The shipped default (`/run/kandev/deployment-secret.sock`) and the unit's `--socket` value always
  fit, and the supervisor, the operator CLI, and the unit generator now refuse a longer path before
  binding. That limit only bites a deliberate `--socket` override, including one under a deep data
  root, which is how this branch's own test harness hit it. Relocating the socket to a shorter
  system path, or deriving it from the data root deliberately, is deliberately out of scope here: it
  would touch the installer, the unit, and the CLI path derivation together.
- Tests whose temp root is deeper than roughly 46 characters cannot bind a unix socket under
  `t.TempDir()`, so the deployment-secret suite must be run with a short `TMPDIR` (and note that
  `GOTMPDIR` overrides `TMPDIR` for test processes under `make`).

## Risks

- Shared workspace credentials attribute every member's star to one GitHub account; the status and control disclose it (AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.5).
- Fine-grained PAT and App user capability is learned from a `403`; the first attempt for such an actor can fail silently before the control disappears.
- GitHub has community reports of App user tokens with `Starring` still receiving "Resource not accessible by integration"; Task 05 must treat that as a denial, and Task 07 cannot prove real GitHub behavior.
- Adding `starring` to the App manifest must not flag existing Apps in import or lifecycle policy checks.
- Existing historical connections may lack scope metadata; unknown capability must fail closed and require re-consent.
- Multiple catalog entries can reference one repository, so cache identity must be scoped by repository, workspace, authenticated user, and credential generation.
- Existing plugin rows have overlay navigation and mobile action geometry; the control must not intercept or break those paths.
- Public documentation and all seven locale catalogs must remain synchronized with the new user-facing action.

## UI-03: Confirmed star count update

Task 08 implements `AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.13` using a canonical-repository count projection in the existing frontend store. The projection adjusts the catalog baseline only on an accepted mutation transition; hydration, errors, and stale or duplicate responses do not alter it. No provider/auth changes or live total fetching.

```text
Desktop: ★ 12 [☆ Star] -> pending: ★ 12 [☆ Star] -> confirmed: ★ 13 [★ Unstar]
Phone:   ★ 12 [ ☆ Star ]  (44px target, existing row wrapping)
         -> ★ 13 [ ★ Unstar ]
Failure: ★ 12 [☆ Star] -> ★ 12 [☆ Star]
```

Task 08 is independent of Tasks 04-07 and depends only on the implemented Task 02 cache. Implemented and verified: 204 affected unit/component tests, 14 desktop browser tests, 2 phone browser tests, typecheck, ESLint, i18n, public-docs, and specification checks passed. Interactive desktop smoke observed 12 -> 13 -> 12; phone rendering retained a 44px toggle and no horizontal overflow. See the work order for exact commands and results. Catalog staleness and other GitHub users' activity remain outside this session-local projection.
