---
id: "13-contract-e2e-release-marketplace"
title: "Cross-repository contract E2E, release, and marketplace"
status: in_progress
wave: 4
depends_on: ["03-protocol-manifest-actions", "04-frontend-plugin-registry", "05-dynamic-composer-reference-sources", "06-plugin-owned-task-lifecycle", "07-provider-neutral-git-credentials", "08-native-repository-provider", "09-native-link-review-surfaces", "10-cloud-dc-domain-auth", "11-plugin-workflows-watches", "12-plugin-ui-native-registrations"]
plan: "plan.md"
spec: "../../specs/bitbucket-plugin/spec.md"
---

# Task 13: Cross-repository contract E2E, release, and marketplace

## Intent

Prove packaged host/plugin integration and publish only after every required capability
and security contract passes. Marketplace registry is final mutation.

## Owned paths

- `apps/backend/cmd/plugin-fixture/`
- `apps/backend/internal/plugins/runtime/testdata/fixtureplugin/main.go`
- `apps/web/e2e/tests/plugins/bitbucket-plugin-contract.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-bitbucket-plugin-contract.spec.ts`
- `plugin-registry/plugins.yaml`
- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md`
- `docs/public/plugins-marketplace.md`
- `docs/public/integrations.md`
- Attached `kdlbs/kandev-plugin-bitbucket` release/package/docs paths.

## Dependencies

Tasks 03 through 12, integrated.

## Acceptance

1. Desktop/mobile E2E covers install/enable, authenticated connection action,
   repository picker, PR import, **Link → Bitbucket Pull Request**, plugin native
   review, composer `#` selection/submission, watch-created task, unload/reload, and
   secret-free errors.
2. Containers E2E proves real HTTPS clone/push through helper leases for host and
   remote executor paths. Packaged plugin installs against released minimum host
   version.
3. Public signed plugin release and checksums precede final
   `plugin-registry/plugins.yaml` entry; public docs accurately describe setup,
   security, compatibility, and live `api_write` behavior.

## Verification

```sh
make -C apps/backend test
make -C apps/backend lint
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e --grep "Bitbucket plugin contract"
cd apps/web && pnpm e2e --grep "mobile Bitbucket plugin contract"
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers --grep "Bitbucket plugin contract"
node --test plugin-registry/build-index.test.mjs
```

## Risks

Marketplace publication before host release/package/container evidence breaks minimum-
version compatibility guarantees. Treat every capability-matrix row and all secret
leakage checks as release gates.

## Progress and remaining gates

- The packaged fixture passes the desktop and native-mobile host contract, including
  authenticated action dispatch, repository selection, native Link and review
  surfaces, `#` search with submit-time rejection, plugin-owned task provenance, and
  disable/re-enable cleanup.
- Plugin unit/race/vet/build checks and host/all-platform package checksum verification
  pass. Public docs and the marketplace index baseline validate.
- Container credential specs are present for Docker and SSH, but this workspace lacks
  `KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL`; both tests therefore skip rather than
  claiming real HTTPS clone/push evidence.
- Packaged plugin E2E fails closed without `KANDEV_PLUGIN_E2E_URL`. The manifest also
  intentionally omits `min_kandev_version` until these host contracts ship in a named
  release.
- A signed release is not yet trustworthy: the host has no production verifier/trust
  policy and the release has no approved signing key. Keep the plugin unreleased and
  absent from `plugin-registry/plugins.yaml` until those decisions and external gates
  are resolved.
