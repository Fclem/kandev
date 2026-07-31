---
spec: docs/specs/bitbucket-plugin/spec.md
created: 2026-07-31
status: in_progress
---

# Implementation Plan: Bitbucket Connector Plugin

## Overview

Release generic host seams before plugin behavior: authenticated actions, plugin
registry contracts, dynamic composer sources, richer task ownership, and a provider-
neutral credential broker. Build Cloud/Data Center adapters and plugin workflows only
after those seams land, then prove native task, review, composer, desktop, mobile, and
credential behavior through cross-repository acceptance.

The host remains provider-neutral. `kdlbs/kandev-plugin-bitbucket` owns Bitbucket
payloads, auth, product probes, watches, and UI. The required native hooks remain
**Link → Bitbucket Pull Request**, plugin-rendered native review panels, and composer
`#` source search with submit-time authorization.

## Host contracts

- Add additive plugin RPC/SDK/manifest shapes and authenticated action dispatch.
- Add revocable repository-provider, task-action, and review-provider frontend
  registrations; migrate built-ins through the same registry.
- Register/unregister dynamic backend composer reference sources and reauthorize every
  reference at submission.
- Extend plugin-owned task lifecycle and provider-neutral descriptor validation.
- Replace GitHub-only broker mechanics with composite short-lived credential leases.

## Plugin repository

- Bootstrap `kdlbs/kandev-plugin-bitbucket` from official template in its attached
  Kandev workspace; do not nest a clone under this repository.
- Implement separate Cloud and Data Center adapters, workspace-scoped encrypted auth,
  health, capabilities, task/link/Git/watch workflows, and native registrations.
- Publish only after source/package, host/plugin desktop/mobile, and container broker
  acceptance pass; marketplace mutation is last.

## Tests

- Host protocol/action tests cover declaration, normal authentication, resource
  authorization, body caps, timeout/cancellation, response headers, and SDK
  compatibility.
- Registry/task/review tests cover ownership collisions, unload cancellation, complete
  provider descriptors, Link-group actions, layout aliases, desktop/mobile review
  selection, and plugin teardown.
- Composer tests cover dynamic source lifecycle, canonical identity, workspace
  isolation, search and submission authorization, disabled plugins, and generic
  presentation.
- Credential tests cover exact scope, expiry/revocation, refreshed resolution,
  leakage, custom host/path, and local/remote executor helper use.
- Plugin tests cover Cloud/DC fixture mapping, auth rotation/PKCE, capability probes,
  health/retry, watch concurrency/recovery, ownership-safe deletion, and responsive UI.

## E2E tests

- Install/enable fixture package; call a declared authenticated action without exposing
  a webhook as a browser RPC.
- Select a plugin provider in native task creation, import/inspect a PR, invoke
  **Link → Bitbucket Pull Request**, then open the plugin native review panel on
  desktop and mobile.
- Select a `#` Bitbucket pull request and prove submitted metadata is authorized.
- Verify watch-created task, unload/reload cleanup, secret-free failure UI, and real
  HTTPS clone/push in the containers project.

## Implementation waves and task files

Wave 0 (parallel-safe, docs/external repository only):

- [x] [task 01 — design package](task-01-design-package.md)
- [x] [task 02 — plugin repository bootstrap](task-02-plugin-repository-bootstrap.md)

Wave 1 (parallel-safe after task 01):

- [x] [task 03 — protocol, manifest, authenticated actions](task-03-protocol-manifest-actions.md)
- [x] [task 04 — frontend plugin registry contracts](task-04-frontend-plugin-registry.md)

Wave 2 (parallel-safe after contract foundations):

- [x] [task 05 — dynamic composer reference sources](task-05-dynamic-composer-reference-sources.md)
- [x] [task 06 — plugin-owned task lifecycle](task-06-plugin-owned-task-lifecycle.md)
- [x] [task 07 — provider-neutral git credential broker](task-07-provider-neutral-git-credentials.md)
- [x] [task 08 — native repository provider task creation](task-08-native-repository-provider.md)
- [x] [task 09 — native Link and review surfaces](task-09-native-link-review-surfaces.md)

Wave 3 (plugin repository after declared dependencies):

- [x] [task 10 — Cloud/DC domain and authentication](task-10-cloud-dc-domain-auth.md)
- [x] [task 11 — task, Git, linking, and watch workflows](task-11-plugin-workflows-watches.md)
- [x] [task 12 — plugin UI and native registrations](task-12-plugin-ui-native-registrations.md)

Wave 4:

- [ ] [task 13 — contract E2E, release, and marketplace](task-13-contract-e2e-release-marketplace.md)

## Current status

Tasks 01–12 are implemented. The packaged generic host contract passes on desktop and
mobile. The actual package passes its unconfigured action, disable/re-enable, desktop,
and mobile lifecycle checks; its canonical composer reference now rehydrates the
repository/PR identity and performs live submit-time authorization. Plugin unit, race,
build, and five-platform archive checks pass. Task 13 remains in progress until a
configured disposable Cloud/Data Center target, a compatible disposable host, and a
container-reachable HTTPS credential-broker URL are available, the host changes ship
in a release that can be named by `min_kandev_version`, and an explicit signing
key/trust policy exists. No plugin tag, release, signature claim, or marketplace entry
is created before those gates pass.

## Risks

- Full parity is complete only when every capability-matrix row passes; alpha sideloads
  may be partial but must be marked partial.
- BYO OAuth registration and Data Center network/version variation require clear
  failure states, fixture coverage, and a release-time compatibility matrix.
- Generic host seams must pass a second-provider test: Bitbucket types, URL parsing,
  and auth rules in host code are a design failure.
- Cross-repository release ordering is strict: host contract release precedes plugin
  compatibility gate, package release, and marketplace entry.
