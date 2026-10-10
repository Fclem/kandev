---
id: "04-protected-deployment-secret-store"
title: "Protect deployment secret store"
status: pending
wave: 4
depends_on:
  - 01-github-api
  - 03-plugin-stars-e2e
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.11
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-STARS-001.12
system_design:
  - ../../specs/integrations/system-design/deployment-secret-store.md
---
# Task 04: Protect deployment secret store

## Summary

Move deployment identity and `deployment-secrets.sqlite` out of the backend-writable data directory and into the root-owned layout in the [protected deployment secret store design](../../specs/integrations/system-design/deployment-secret-store.md). The backend gets group read access to its own installation and validates root ownership along the full path. No component reads, imports, or falls back to the unreleased data-directory layout.

## In scope

- `internal/secrets`: add a single resolver for the state root, binding key, `installations/<installation_id>/`, and full-path ownership and mode validation. Use `Lstat`, open with `O_NOFOLLOW`, require regular files with one link, and check that the private key matches the stored public key. Add typed errors. Remove `DeploymentIdentityDir(dataDir)` and `DeploymentSecretStorePath(dataDir)`. Move `BootstrapDeploymentIdentity`, `LoadDeploymentIdentity`, `LoadDeploymentRootPrivateKey`, and both store openers onto the resolver.
- Open the store with rollback journaling (`journal_mode=DELETE`) for its single writer and read it with `mode=ro`. Do not use `db.OpenSQLite`'s WAL DSN for this store.
- Make the root CLI bootstrap atomic and idempotent, add `--service-user` with group-ownership handling, and clean orphaned temporary directories. Update the help text.
- Point the supervisor (`cmd/kandev-deployment-secretd`), the backend wiring (`internal/backendapp/github_personal_star_deployment.go`), and `cmd/deployment-secret-seed` at the resolver.
- Add the E2E-only `KANDEV_E2E_DEPLOYMENT_SECRET_ROOT` seam, honored only when `KANDEV_E2E_MOCK=true` and checked against the effective UID. Rewrite `apps/web/e2e/fixtures/personal-star-deployment.ts` to produce the same layout, and update `apps/web/e2e/README.md` § Deployment-capability fixtures.
- Update `docs/public/{cli,configuration,run-as-a-service,security}.md` to cover the layout, bootstrap order, `--service-user`, recovery, data-directory relocation, and backups.

## Exclusions

- Migrating, validating, or deleting the unreleased data-directory files.
- Changing the `<dataDir>/master.key` contract, the supervisor socket protocol, OAuth token storage, or frontend or star behavior.
- Adding a production path setting.

## Acceptance

1. A backend-UID process cannot write, rename, or replace any file or directory in the protected layout or its ancestors. Resolution rejects symlinked, hardlinked, non-root-owned, or group/other-writable components, a mismatched binding or directory ID, and a mismatched key pair. Each rejection leaves `available=false`.
2. Fresh bootstrap followed by provision resolves through the backend's read-only connection. Revocation through the supervisor remains visible to the backend. A crash between the installation-directory rename and the binding rename leaves no binding, and the next bootstrap succeeds.
3. When the data directory contains `deployment/` and `deployment-secrets.sqlite` but no binding exists, the capability reports unavailable. Bootstrap does not read or modify those files.

## Verification

```bash
(cd apps/backend && TMPDIR=/tmp go test ./internal/secrets/... ./internal/system/deploymentsecret/... ./internal/launcher/cli/... ./internal/backendapp/... ./cmd/kandev-deployment-secretd/... ./cmd/deployment-secret-seed/...)
(cd apps/backend && sudo env "PATH=$PATH" TMPDIR=/tmp go test -run 'Protected|NonRoot' ./internal/secrets/... ./cmd/kandev-deployment-secretd/...)
(cd apps/backend && golangci-lint run ./internal/secrets/... ./internal/system/deploymentsecret/... ./internal/launcher/... ./internal/backendapp/... ./cmd/...)
(cd apps/web && pnpm e2e:raw --project=auth tests/auth/plugin-repository-stars.spec.ts)
(cd apps/web && pnpm e2e:raw --project=chromium tests/settings/plugin-repository-stars.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

The root-only command runs root-gated tests: these spawn a helper as UID 65534, which tries to replace a file, rename a directory, and swap an ancestor, and they also exercise crash injection between the installation-directory and binding renames. If no root shell is available, record that gap in Results instead of claiming the tests passed.

## Likely files

`apps/backend/internal/secrets/deployment_{identity,secret_store}.go` plus a new resolver file and its tests, `apps/backend/internal/launcher/cli/deployment_secret{,_client}.go`, `apps/backend/cmd/kandev-deployment-secretd/supervisor_linux.go`, `apps/backend/cmd/deployment-secret-seed/main.go`, `apps/backend/internal/backendapp/github_personal_star_deployment{,_test}.go`, `apps/web/e2e/fixtures/personal-star-deployment.ts`, `apps/web/e2e/README.md`, and `docs/public/*.md`.

## Risks

- A WAL DSN in the protected store breaks backend reads; tests must open as a non-root reader.
- The E2E seam must stay unreachable outside the `e2e` profile. Add a test that the production profile ignores it.
- The ownership walk must not trust `filepath.EvalSymlinks` on backend-controlled input; the binding key uses lexical cleaning only.
