---
id: "05-star-actor-resolution"
title: "Resolve star actors across GitHub auth schemes"
status: pending
wave: 5
depends_on:
  - 01-github-api
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.6
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.10
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.12
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.1
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.3
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.5
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.6
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.7
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-002.8
system_design:
  - ../../specs/integrations/system-design/github-repository-star-actors.md
  - ../../specs/integrations/system-design/github-repository-stars.md
---
# Task 05: Resolve star actors across GitHub auth schemes

## Summary

Replace the dedicated-only star credential path with `StarActorResolver`, which resolves one human actor from the dedicated connection, the GitHub App personal connection, or the workspace PAT, `gh` CLI, or legacy shared credential. Add per-source capability evaluation and the extended connection-status contract. Request `starring: write` on new App registrations without flagging existing ones. All existing routes, the URL parser, batch limits, and the dedicated OAuth flow stay unchanged.

## In scope

- `CredentialPurposeStar` in `validCredentialPurpose` and `resolveAtEpoch` (`auth_resolver.go`). It tries the personal path like `CredentialPurposePersonalRead`, but when no personal row exists it falls through to automation, returning nil personal for PAT/CLI/legacy workspaces.
- `star_actor_resolver.go`: the dedicated connection first (only when the deployment reports it available), then `CredentialResolver.Resolve`, rejecting `AuthPrincipalApp`. It returns `{source, login, shared, credential_generation, token}` plus `connect` and `remediation` per the actor design. The token stays package-private.
- `star_capability_cache.go`:
  - An `X-OAuth-Scopes` probe via `GET /user` for `pat`/`gh_cli`/`legacy_shared`, with a 10-minute TTL and no entry on probe failure.
  - A registration `starring` permission check via `AppClient.GetAuthenticatedApp`, cached per registration credential generation.
  - A `403` denial record keyed by `(workspace_id, source, login, credential_generation, app_registration_id)`.
- `service_personal_star.go`: status, batch status, star, and unstar use the resolved actor through `PersonalStarHTTPClient`, built from the actor token. The single refresh-and-retry stays limited to `personal_star_oauth`. A `403` from any star call records a denial and maps to the existing insufficient-scope error; nothing retries under another actor.
- `PersonalStarConnectionStatus` gains `actor{source,login,shared}`, `connect`, and `remediation`, with `available` computed per the actor design.
- `deployment_app_manifest.go` adds `"starring": "write"`. An explicit optional-permission set is skipped by `importedAppPolicyProblems` and the comparison in `app_registration_helpers.go`.
- Mock provider (`mock_personal_star.go`, `mock_personal_star_controller.go`, E2E-gated as today): key star state by actor, and let E2E seed a workspace source, an App personal connection, scope-probe outcomes, and a `403` injection.
- Counter `github_star_actor_resolutions_total{source,outcome}` and the structured resolution log, with no login, repository, or token.

## Exclusions

- Frontend changes (Task 06) and E2E specs (Task 07).
- Changing the dedicated OAuth flow, its persistence, or Task 04's deployment store.
- Changing `CredentialPurposePersonalWrite`, task Git transport, or any workspace connection lifecycle.

## Acceptance

1. Table tests across the sources prove the precedence order. An App installation never resolves. A higher incapable candidate ends resolution without stepping down. A `403` records a denial that a generation change clears. Missing/foreign workspace and no actor fail before any provider call.
2. Scope-probe tests:
   - A classic header with `repo` or `public_repo` is capable.
   - A classic header without either is denied with `token_scope`.
   - A missing header is unverified and capable.
   - A probe failure is incapable with no remediation, and the next status re-probes.

   An App user without registration `starring: write` is denied with `app_permission`.
3. Status contract tests cover `actor`, `connect` (`personal_star_oauth`, `github_app_user`, null), `remediation`, and `available` for each truth-table row. The manifest test shows `starring: write` on new registrations, and import/lifecycle policy tests show an App without it reports no new problem.

## Verification

```bash
(cd apps/backend && go test ./internal/github/... ./internal/backendapp/...)
(cd apps/backend && go test -race -run 'Star|Personal|Manifest|Import' ./internal/github/...)
(cd apps/backend && golangci-lint run ./internal/github/... ./internal/backendapp/...)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
```

## Likely files

`apps/backend/internal/github/{auth_resolver,auth_principal,service_personal_star,personal_star_resolver,personal_star_client,provider_personal_star,deployment_app_manifest,app_registration_import,app_registration_helpers,mock_personal_star,mock_personal_star_controller}.go`, new `star_actor_resolver.go` and `star_capability_cache.go`, and their `_test.go` files.

## Risks

- `CredentialResolver` caches and workspace epochs must remain the single source of credential invalidation; the capability cache must key on generation, not wall-clock alone.
- `resolveAutomation` can return an App installation for App workspaces; the App-principal rejection must happen before any provider call.
- `gh auth token` latency runs once per resolution; reuse the resolver's existing credential cache rather than calling `gh` per repository.
- The probe consumes a request from the shared credential's rate limit; the TTL bounds it.
