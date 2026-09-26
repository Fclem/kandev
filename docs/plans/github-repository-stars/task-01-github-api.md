---
id: "01-github-api"
title: "Add personal star auth and API"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.6
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.7
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.8
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.10
system_design:
  - ../../specs/integrations/system-design/github-repository-stars.md
---
# Task 01: Add personal star auth and API

## Summary

Add the dedicated OAuth App-backed personal GitHub star connection and expose status, star, and unstar operations through an isolated strict-personal path and authenticated HTTP controller. Preserve existing PR personal-write routing, bypass only the workspace-repository registration requirement for public catalog repositories, require explicit workspace identity, and reject malformed raw URLs before invoking GitHub.

## In scope

- Add typed dedicated OAuth App configuration to the startup catalog: `github.personalStarOAuth.clientId`, `github.personalStarOAuth.clientSecretRef`, and `github.personalStarOAuth.callbackURL` with `KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CLIENT_ID`, `KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CLIENT_SECRET_REF`, and `KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CALLBACK_URL` aliases, YAML-overridden by environment, validation, sensitive reference resolution through the deployment-owned `deployment_secret_refs` store and `DeploymentSecretResolver`, and fail-closed provider startup behavior.
- Add the public configuration reference entry and config/catalog/source tests covering precedence, required-together validation, production HTTPS callback validation, immutable deployment installation-ID/class checks, unscoped startup resolution, cross-installation same-class rejection, missing/foreign/user-owned/deleted/revoked reference rejection, provisioning-only create/revoke behavior, attempted provenance/class/reference mutation rejection, secret rotation/restart, and startup ownership.
- Add `DeploymentMasterKeyring.OpenExisting(cfg.ResolvedDataDir())` as a read-only no-generation wrapper exposing stable ID `legacy-master-key-v1`; preserve the existing `MasterKeyProvider` contract, do not rotate the master key, fail closed on wrong/missing/invalid roots or IDs without filesystem mutation, and prove CLI/backend cross-process restart resolution. Include the affected key-provider implementation and tests.
- Own canonical Linux-only artifact `kandev-deployment-secretd-linux-x64.tar.gz` / `kandev-deployment-secretd-linux-arm64.tar.gz` (matching the release's existing `linux-x64`/`linux-arm64` tokens) containing `bin/kandev-deployment-secretd`, the generated `systemd/kandev-deployment-secretd.service`, and `checksums.txt`. Add the new `apps/backend/cmd/kandev-deployment-secretd` main package and build it in the Linux release job only, alongside `cmd/kandev`/`cmd/agentctl`; the unit text is generated in Go like `kandev.service` today, and the packaging job writes it into the payload. Keep the runtime bundle daemon-free (`scripts/release/package-bundle.sh` still validates `./cmd/{kandev,agentctl}`; the runtime-bundle test asserts that exact binary set). Add `apps/backend/Makefile` and update the three name-list release seams that otherwise ship nothing silently: the `bundle-*` upload path list (`.github/workflows/release.yml:877-886`), the required-asset check (`:2024-2043`), and the publish `files:` globs (`:2084`), plus the assets tuple in `.github/scripts/release-workflow-contract_test.py:754`. Add exact archive, checksum, installer, desktop/Docker rejection, and lifecycle tests. The `windows-latest` job runs native `go test -race` filtered to `./internal/system/deploymentsecret/...` (the new platform package keeps compile time inside the job's 40-minute cap), while the same `-run` filter over `./internal/launcher/...` and `./internal/backendapp/...` runs in the standard Linux job; Linux `GOOS=windows go test -c` is build-only.
- Wire `DeploymentMasterKeyring.OpenExisting(cfg.ResolvedDataDir())` and `DeploymentSecretResolver` through the backend composition root into `github.Provide`; retain a once-guarded cleanup owner immediately, run it on every later constructor failure, register `Services.GitHubCleanup` immediately after successful handoff, and transfer ownership once. Provider shutdown drains `StarSecretCleanupRunner` before resolver close; test construction failure, post-return failure, normal shutdown, and ordering.
- Add a dedicated E2E mock client/route contract for star state, failure injection, OAuth outcome controls, and `MockClient` conformance; route registration requires both `KANDEV_E2E_MOCK=true` and `MockClient`, so `KANDEV_MOCK_GITHUB=true` without the E2E selector mounts no mock controls. Keep controls unavailable in production.
- Extend E2E reset to clear dedicated star connection/flow/cleanup rows and all generation-specific secret IDs transactionally, with two-workspace queued/terminal isolation tests.
- Update `apps/backend/AGENTS.md` in the same change: document the new deployment-secret package and correct the stale `cmd/kandev/e2e_reset.go` reset reference to `internal/backendapp/e2e_reset.go`.
- Add `github_personal_star_connection` persistence keyed by workspace/user, with non-secret login/scope/capability/status/generation metadata and encrypted token/refresh-token secret IDs.
- Add dedicated flow state, PKCE callback identity, `public_repo` scope request, returned-scope normalization, atomic secret/metadata replacement, refresh scope preservation, and preserve-and-remain-usable reconnect/re-consent behavior.
- Define a dedicated `PersonalStarClient` capability for the dedicated OAuth token path with status/star/unstar methods; do not widen the general GitHub `Client` interface or add star methods to PAT/CLI implementations. The strict-personal resolver returns only this capability and fails closed for every other credential source.
- Add an isolated star-specific strict-personal resolver/purpose that rejects App fallback, inactive/missing personal identities, and personal connections without `public_repo`.
- Add strict raw repository URL parsing and conformance tests.
- Add bounded JSON connection-status, repository status, and PUT/DELETE mutation routes, each requiring `workspace_id`; connection status returns generation, active, capable, reconnect-required, and `available` for the current user without token data. Map missing/invalid OAuth configuration and unsupported daemon targets to `available=false`; controller/frontend tests prove no star/CTA, start request, socket, or provisioning side effect.
- Add a durable secret-cleanup queue with startup/periodic retry, generation-specific ownership, workspace-deletion integration, bounded backoff, transactional lease claims, shutdown drain, restart recovery, and operator-visible terminal failures.
- Add red tests before implementation for configuration, schema/store/secret round-trip, OAuth URL scope and `S256` challenge derivation, 256-bit URL-safe state generation, exact `RawURLEncoding` state validation and digest-only persistence/lookup, rejection vectors for whitespace/padding/standard-Base64/non-ASCII/alternate encodings and percent-encoded URL-safe characters, raw-state absence from rows/logs/errors, literal-plus rejection before digest lookup, opaque `code=%252F` one-decode preservation with the exchange receiving `%2F` rather than `/`, RFC 7636 verifier grammar/length, verifier-to-challenge recomputation, tampered or absent persisted PKCE pair rejection before provider exchange, callback query multimap/cardinality/percent-decoding and accepted-key vectors, denial-before-exchange state consumption, `ClaimForExchange` atomic verifier/challenge read-and-null semantics, committed-row absence of verifier/challenge, two-callback race with exactly one successful claim, token exchange receiving the claimed verifier, no-row start-claim-disconnect-release lifecycle-fence rejection with no connection creation and staged-secret compensation, safe return-path outcome replacement/preservation, callback flow-kind/session/user/session-ID/workspace/PKCE/replay checks in both auth-enabled session mode and auth-disabled synthetic-subject mode, PAT/internal/token-only no-session start rejection, same-user/different-session rejection, token scope extraction, refresh/generation replacement, unknown/reduced/declined re-consent, connection start/disconnect/status transitions, cleanup fail-once/retry/concurrent-claim/shutdown/restart isolation, strict ASCII URL grammar/canonicalization shared vectors, command/API mapping, service/controller authorization, validation, duplicate/over-limit input, partial unknown results, missing/foreign workspace, and failures.
- Add provisioning RPC tests for the full challenge exchange and fixed binary frame: exact scalar encodings and lengths, nonce byte-equality/mismatch, challenge byte substitution rejection, RFC3339Nano expiry, challenge/request/result ordering and half-close, peer/install/nonce binding, length bounds, big-endian fields, EOF/trailing/concatenated/truncated rejection, exact action-specific provision/revoke schemas, canonical envelope/signature coverage including bounded base64 ciphertext payload, RFC 8785 round-trip and duplicate-key rejection, Ed25519 detached-frame verification with the fixed prefix test vector, strict Base64 re-encoding/non-zero-unused-bit rejection for ciphertext/signature, canonical AES-256-GCM sealed-envelope algorithm/key-id/nonce/tag validation, read-only `DeploymentMasterKeyring.OpenExisting` with explicit data-root and no-generation failure, stable legacy key-ID lookup across CLI/backend restart, wrong-root/unknown-key/retired-key rejection without filesystem mutation, cross-process MasterKeyProvider encrypt/decrypt and wrong-key/authentication failure, digest recomputation using lowercase-hex SHA-256, truncation/substitution, noncanonical/padded signature rejection, challenge expiry and atomic single-use replay rejection, SO_PEERCRED root-only denial, wrong socket/path permissions, wrong-installation/copy-key denial, provenance mutation, and HTTP/WebSocket non-dispatch.
- Add HTTP, WebSocket, and credential-selector regressions proving the exact `kandev-runtime:github-personal-star:client-secret:<version>` reference is absent from list/select responses and returns not-found or rejection for reveal, update, delete, copy, move, create, and select attempts.

- Startup config exposes the three canonical YAML keys and environment aliases, uses environment-over-YAML precedence, validates required-together and production HTTPS rules, resolves only protected install-owned secret references, fails closed without preventing backend startup, and documents rotation/restart in `docs/public/configuration.md`.
- Public operator docs cover root-only provisioning/rotation/revocation, Linux service prerequisites/lifecycle, the unavailable target matrix (desktop, Docker, user-systemd, root-run backends, Darwin, Windows), the root-owned socket and deployment-owned secret store trust boundary in `docs/public/security.md`, and Settings > Plugins omission/reconnect behavior without secret values. Public CLI/service docs and validators must be updated in the same change.
- Keep `docs/specs/integrations/system-design/github-repository-stars.md` inside its 32768-byte spec-lint limit: as written it has roughly 250 bytes of headroom, so any added prose must be offset by trimming equivalent text in the same file.

## Out of scope
- Frontend state or controls.
- Persistence of star state, telemetry, or marketplace ranking.
- Reuse or modification of the existing GitHub App user-to-server personal flow for starring.
- Changes to existing PR review/reviewer/merge credential purposes.

## Acceptance

- The deployment-configured star OAuth service is independent of `PersonalAuthService`, does not require a workspace App registration, and rejects callback state with the wrong flow kind, workspace/user identity, lifecycle revision or credential generation, PKCE verifier, callback identity, replay, disconnect fence, or expiry. PAT, `gh_cli`, and GitHub App-backed workspaces remain eligible to complete the dedicated personal star flow.
- Callback requests accept only the complete decoded grammar: one non-empty `state` plus one non-empty `code`, or one non-empty `state` plus one non-empty `error` and at most one non-empty `error_description`/`error_uri`; malformed encoding, duplicate required keys, unknown keys, empty values, and mixed code/error are rejected before state lookup or consumption. Denial consumes state without token exchange and redirects with one safe `star_connection_outcome` value while preserving non-reserved return-path query parameters. Wrong-session and cross-workspace callbacks return authorization errors without redirect.
- Dedicated OAuth App authorization requests include `scope=public_repo`; granted capability metadata round-trips through persistence and refresh, and unknown/reduced-scope connections require reconnect/re-consent without invalidating a prior valid connection or using App fallback.
- Generation replacement stages access/refresh secret IDs, compensates staged secrets on metadata failure or lifecycle-fence rejection, and performs superseded-secret cleanup only after a successful pointer swap; post-commit cleanup failure leaves the new connection usable and retryable.
- Client implementations invoke documented GitHub starring endpoints and map 204/404 status correctly.
- Personal OAuth App requests can star an unregistered public catalog repo in an App-backed workspace; missing/inactive/insufficient personal identity, App principals, missing workspace ID, and foreign workspace requests fail closed.
- Service-layer and controller-layer authorization both reject missing/foreign workspace IDs before credentials or provider calls.
- The status endpoint accepts raw URLs, independently validates them, deduplicates at most 100 canonical keys within a 4 KiB body, and preserves per-key unknown results when a provider call fails.

## Verification

```bash
(cd apps/backend && go test -run 'Test(GHClient.*Star|.*Starred|GitHub.*Star|.*GitHubRepoURL|.*StrictPersonal|.*AuthorizeWorkspace|.*PersonalAuth.*Scope|.*Personal.*Reconsent|.*PersonalStarConnection)' ./internal/github/...)
(cd apps/backend && go test -run 'TestController.*Star|Test.*Starred|Test.*RepositoryURL|Test.*Workspace|Test.*PersonalAuth|Test.*PersonalStarConnection' ./internal/github/...)
(cd apps/backend && go test -run 'Test.*(DeploymentSecret|ConfigCatalog|PersonalStarOAuth)' ./internal/common/config/... ./internal/secrets/... ./internal/github/...)
(cd apps/backend && go test -run 'Test.*(Supervisor|Provision|DeploymentSecret)' ./internal/system/restart/... ./internal/secrets/...)
(cd apps/backend && go test -run 'Test.*(DeploymentSecretDaemon|RootPeer|DeploymentSecretBootstrap|Provider.*DeploymentSecret|OpenExisting|ResolvedDataDir)' ./internal/system/deploymentsecret/... ./internal/launcher/... ./internal/backendapp/... ./internal/github/...)
(cd apps/backend && GOOS=windows GOARCH=amd64 go test -c -o /tmp/deployment-secret-windows.test ./internal/system/deploymentsecret/... && rm -f /tmp/deployment-secret-windows.test) # cross-build only
(cd apps/backend && GOOS=linux GOARCH=amd64 go build -o /tmp/kandev-deployment-secretd ./cmd/kandev-deployment-secretd && rm -f /tmp/kandev-deployment-secretd) # daemon binary builds Linux-only
bash scripts/release/runtime-bundle.test.sh # runtime bundle stays daemon-free
(cd apps/backend && go test -race -run 'Test.*(UnsupportedUserSystemd|UnsupportedContainer|UnsupportedRootRun|UnavailablePlatform|NoProvisioningSideEffect)' ./internal/system/deploymentsecret/...) # native `windows-latest` test-windows step; one package keeps the job within its 40-minute cap
(cd apps/backend && go test -run 'Test.*(UnsupportedUserSystemd|UnsupportedContainer|UnsupportedRootRun|UnavailablePlatform|NoProvisioningSideEffect)' ./internal/launcher/... ./internal/backendapp/...) # same filter, Linux job
(cd apps/backend && go test -run 'Test.*(Secret.*HTTP|Secret.*WebSocket|CredentialSelector.*Protected)' ./internal/secrets/...)
```

## Files likely touched

- `apps/backend/internal/github/client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/service_routing.go`
- `apps/backend/internal/github/auth_resolver.go`
- `apps/backend/internal/github/personal_auth_service.go`
- `apps/backend/internal/github/oauth_flow.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_connections.go`
- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/secrets/crypto.go`
- `apps/backend/internal/secrets/crypto_test.go` (new)
- `apps/backend/internal/secrets/deployment_secret_store.go` (new)
- `apps/backend/internal/secrets/deployment_secret_store_test.go` (new)
- `apps/backend/internal/github/factory.go`
- `apps/backend/internal/github/handlers.go`
- `apps/backend/internal/system/restart/supervisor.go`
- `apps/backend/internal/system/deploymentsecret/server.go` (new)
- `apps/backend/internal/launcher/launcher.go`
- `apps/backend/internal/launcher/cli/help.go`
- `apps/backend/internal/launcher/cli/help_test.go`
- `apps/backend/internal/github/provider.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/secrets/deployment_secret_provisioner.go` (new)
- `apps/backend/internal/secrets/deployment_secret_provisioner_test.go` (new)
- `apps/backend/internal/backendapp/types.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/e2e_reset.go`
- `apps/backend/internal/backendapp/e2e_reset_test.go`
- `apps/backend/internal/github/mock_controller.go`
- `apps/backend/internal/github/mock_client.go`
- `apps/backend/AGENTS.md`
- `.github/workflows/release.yml`
- `apps/backend/internal/launcher/service.go`
- `apps/backend/cmd/kandev-deployment-secretd/main.go` (new)
- `apps/backend/Makefile`
- `apps/backend/internal/launcher/service_deployment_secretd.go` (new; generates the daemon systemd unit the same way `service.go` generates `kandev.service`)
- `.github/workflows/backend-tests.yml`
- `scripts/release/package-bundle.sh`
- `scripts/release/runtime-bundle.test.sh`
- `apps/backend/internal/secrets/handlers.go`
- `docs/public/cli.md`
- `docs/public/run-as-a-service.md`
- `docs/public/configuration.md`
- `docs/public/security.md`
- `scripts/validate-public-docs.mjs`
- `apps/backend/internal/secrets/handlers_test.go`
- `apps/backend/internal/github/*mock*test.go`
- `apps/backend/internal/github/*star*_test.go` (new)
- `apps/backend/internal/github/*personal*auth*_test.go`

## Dependencies

None.

## Risks

- Existing GitHub App workspaces require an additional dedicated OAuth App consent for personal stars.
- Existing personal rows may lack scope metadata; unknown capability must fail closed and require reconnect.
- A failed reconnect must preserve a previously valid connection; replacement must be atomic after full callback verification.

## Parallelism

`sequential`

## Inputs

- GitHub REST starring API documentation.
- GitHub OAuth scope documentation.
- ADR-2026-09-18-github-personal-star-oauth.
- GitHub authentication requirements and existing service-routing/client/auth-flow tests.
- Workspace GitHub authentication requirements and system design, including the strict repository-star resolver exception.

## Results

Status: complete. Feature commits 8f07a841c..12c6a5d6f on `feature/sync-plugin-reposito-673`.

Delivered: the dedicated OAuth App star API behind a strict personal resolver with no App/PAT/CLI
fallback (`b2fcc9c1f`, `d99242c307`); the deployment-secret boundary (protected reference
validation, read-only master-keyring, Linux target policy, root-peer socket authorisation,
JCS frame/envelope protocol with Ed25519 detached signatures, immutable versioned store, single
writer provisioner) in `3523894f8`, `2b2b2f370`, `1aec01f86`, `0f5685e0a`, `6e68a33c3`, `5d9d8467a`,
`eb0eda656`, `21e0c3f2d`, `ae0a5523b`, `dae410dae`; the root-only CLI, daemon and release artifacts in
`acce4be3b`, `d5ae98677`, `ab5dc0bd5`, `44ebc92cc`; provider wiring in `d20d47b06`; public docs in
`b27d7652c`; and the review-loop corrections `abe881ec8`, `e0bac36d0`, `5704f8fc8`, `33afe9cc0`,
`4baf53b78`, `752b8796d`, `c65587367`, `151fd2fe3`, `31fcbc8d9`, `aeb0d0c27`, `d49e0133b`,
`0a42f1f19`, `12c6a5d6f`.

Gates (all re-run at head, `-count=1`):
- The work order's verification block passes in full: gates 1-5 (star/API, controller, config/secrets,
  restart/secrets, daemon filters) ok; gate 6 Windows cross-build of `internal/system/deploymentsecret`
  ok; gate 7 Linux-only `cmd/kandev-deployment-secretd` build ok; gate 8
  `bash scripts/release/runtime-bundle.test.sh` PASS; gate 9 (-race, unsupported-target filter) ok;
  gate 10 (same filter across launcher/backendapp) ok; gate 11 (secrets HTTP/WS filter) ok.
- Package suites: `go test ./internal/github/ ./internal/backendapp/ ./internal/secrets/
  ./internal/system/deploymentsecret/ ./internal/profiles/` all pass; `golangci-lint` on the touched
  packages reports 0 issues.
- `go run ./cmd/sqlguard ./internal` exits 0. The dedicated star table template renders its
  timestamps through `dialect.MustRenderSchema`, so it is recorded in the guard's exemption file
  beside the sibling `createTablesSQL` entries; leaving it out kept CI's `static_checks` job red.
- The one red anywhere is `internal/launcher` `TestRunStartBuildVersion`, caused by this workstation's
  live kandev service holding port 38429 (ambient, reproducible with the port variables unset).

Review-round corrections landed after the work order itself: the first consent commits its row and
its generation marker in one transaction (`32967d871`), the star schema template is recorded in the
SQL guard exemptions (`961b2e8de`), and a cleanup retry is counted only once it is durable, with the
backoff cap matching the schedule the operator documentation promises (`02acfe94a`).

Round-2 review fixes: workspace teardown locks and rereads committed token pointers, then queues
them atomically with row deletion; consent writes hold the lifecycle fence through commit; terminal
refresh-token rejection persists reconnect-required status without replacing the committed grant.
SQLite regressions cover the stale cleanup snapshot, lifecycle advancement before commit, and
terminal versus transient refresh failures. PostgreSQL-gated interleaving tests cover teardown
versus refresh and replacement versus consent advancement.

Verification for these fixes: `go test ./internal/github -count=1`,
`go test ./internal/persistence/storeconformance -count=1`, and
`go run ./cmd/sqlguard ./internal` pass. PostgreSQL race tests skip without
`KANDEV_TEST_POSTGRES_DSN`. Local `go test -race` cannot load `runtime/race` because this Go
installation lacks `src/runtime/race`. Shared Go and linter cache volumes were full; changed-code
`golangci-lint` passed with isolated caches (0 issues), and `go vet ./internal/github` passes.

Round-3 review fixes: connection status reads now share the monotonic plugin-star request epoch, so
a delayed same-user response cannot be accepted after reset and reload. OAuth compensation records
secret kind and generation, enqueues failed staged-secret deletions in the durable cleanup queue,
and returns queueing failures to the caller. Regressions cover deferred status responses, access and
refresh cleanup metadata, and queue-insert failure propagation.

Verification: `go test ./internal/github -count=1`, `go vet ./internal/github`, and
`pnpm exec vitest run lib/state/slices/plugins/plugins-slice.test.ts
hooks/domains/plugins/use-plugin-repository-stars.test.tsx` pass; the Vitest command reports 29
tests passed.

Round-4 review fix: connection insert/replace commit errors now have an explicit unknown-outcome
classification. The OAuth service reconciles the lifecycle-fenced connection row before cleanup,
returns success when the committed token references match, queues only staged IDs no longer
referenced, and retains staged secrets when reconciliation cannot establish state. Tests inject
post-commit acknowledgment loss for initial insert and refresh, a rolled-back commit, and failed
reconciliation.

Verification: `go test ./internal/github -count=1`, `go vet ./internal/github`,
`python3 scripts/list-docs.py validate`, and `python3 scripts/lint-spec-files.py --all` pass.

Residual gaps (recorded for the reviewer, not hidden):
- The PostgreSQL star-store lifecycle test (replace/disconnect/claim) is written but skips without
  `KANDEV_TEST_POSTGRES_DSN`, so Postgres remains unexecuted in this environment.
- The capable-path E2E cohort relies on the e2e-profile `KANDEV_DEPLOYMENT_SECRET_TARGET=supported`
  override so a containerised CI backend can exercise it; that path has not yet run in a real container.
- `docs/specs/integrations/system-design/github-repository-stars.md` is 32763 of 32768 bytes;
  avoid further additions.
