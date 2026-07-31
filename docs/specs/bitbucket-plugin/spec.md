---
status: approved
created: 2026-07-31
owner: kandev
---

# Bitbucket Connector Plugin

## Why

Teams using Bitbucket Cloud or Bitbucket Data Center need repository, pull-request,
and task workflows without putting Bitbucket API knowledge or credentials into the
Kandev host. The connector must feel native where users already create, link, review,
and reference work, while remaining independently releasable as an official plugin.

## What

- Kandev ships the official `kandev-plugin-bitbucket` from its dedicated public
  repository. Its manifest pins `min_kandev_version` to the first released host
  version containing the required generic contracts; it never guesses an unreleased
  version.
- The plugin supports Bitbucket Cloud and Bitbucket Data Center through separate
  adapters behind one Bitbucket domain. Cloud and Data Center have full capability
  parity wherever their APIs provide an equivalent operation. Capability flags hide
  unavailable controls or explain version-specific limits; the UI never presents a
  non-working equivalent.
- A workspace connects with Cloud API token or OAuth 2.0, and with Data Center
  personal/HTTP access token or OAuth 2.0 when its administrator configured an
  incoming OAuth application link. Cloud app passwords are not accepted. OAuth client
  registrations are bring-your-own per workspace; no client secret ships in the public
  plugin.
- The plugin owns Bitbucket REST payloads, OAuth and token rules, product/version
  probes, pagination, rate-limit handling, health polling, secret refresh, connector
  screens, watches, and Bitbucket rendering. The host owns only reusable authenticated
  action, repository-provider, task-action, review-provider, reference-source, task
  ownership, and credential-broker contracts.
- Users browse/search repositories, select branches, inspect pull-request URLs, launch
  tasks from pull requests, link/unlink existing tasks, and create pull requests from
  a task checkout branch. Remote descriptors preserve the exact credential-free clone
  URL, including Data Center context paths.
- Plugin task actions appear in native task surfaces. The required entry is
  **Link → Bitbucket Pull Request**, alongside unlink, open-review, and create-pull-
  request actions when their declared visibility rules allow them. Desktop context/drop-
  downs and visible mobile action menus have the same capability set.
- Bitbucket pull requests appear through the native review-provider registry in desktop
  selectors, dock/detail panels, task center, and mobile review navigation. The plugin
  renders its provider-owned panel in those host surfaces; it is not reduced to a
  Bitbucket-only host branch or an external redirect.
- Composer `#` search consumes the plugin's dynamically registered
  `bitbucket`/`pull_request` reference source. Kandev constructs canonical reference
  identity, and every selected reference is reauthorized by the live plugin at message
  submission so stale, tampered, cross-workspace, or disabled-plugin selections fail
  closed.
- The `/bitbucket` workbench provides connection status, repository/project browse,
  a pull-request queue, review details, Cloud Pipelines or Data Center build status,
  and saved watches. Desktop uses a multi-pane queue/review layout. Mobile uses a
  focused list/detail state with one scroll owner, `100dvh`, safe-area clearance,
  touch controls of at least 44px, and Drawers for filters and actions.
- Watches use authenticated polling, not Bitbucket event webhooks. OAuth callback is
  the only required public webhook in v1. A watch writes a durable `creating`
  reservation before task creation, stamps plugin/watch/external-PR ownership, and
  reconciles unfinished reservations after restart. Reset/delete previews its cascade
  and deletes only tasks owned by this plugin/watch; manually linked or adopted tasks
  remain.
- Clone, fetch, and push resolve through the provider-neutral short-lived credential
  broker. Secrets never appear in clone URLs, task metadata/state, environment
  variables, command arguments, logs, or executor payloads.

## Capability matrix

| Capability | Cloud | Data Center |
|---|---|---|
| API token/PAT connection and health | Required | Required |
| OAuth 2.0 connect and refresh | Required | Required when an incoming OAuth application link exists |
| Repository/project browse and search | Required | Required |
| Native repository picker, branch selection, and PR URL inspection | Required | Required |
| Scoped clone/fetch/push credential broker | Required | Required |
| Launch task, link/unlink, and create PR from task | Required | Required |
| Native desktop/mobile review panel | Required | Required |
| Composer `#` pull-request search and submitted context | Required | Required |
| Queue, files/diff, commits, comments/threads, reviewers, approvals, merge/decline | Required | Required where server API supports thread semantics |
| Status presentation | Pipelines | Commit/build status |
| Watches with deduplicated task creation | Required | Required |
| Bitbucket Issues | Not supported; use Jira | Not supported; use Jira |

## Connection, permissions, and secrets

Connection state is `unconfigured`, `checking`, `connected`, `auth_required`, or
`unavailable`. Workspace-scoped encrypted plugin secrets hold token/PAT credentials,
OAuth registrations, grants, and rotating refresh tokens. State stores only non-secret
connection metadata and an atomically rotated credential generation. Refresh is
singleflight per workspace/generation; logs and returned errors redact headers, query
parameters, and secret values.

The host authenticates browser actions through normal Kandev session middleware and
authorizes every claimed workspace, task, and repository before dispatch. It derives
task-to-workspace relationships server-side. Plugins receive verified actor/resource
context separately from bounded untrusted JSON. The connector can create or cascade-
delete only task trees stamped `metadata.source = "plugin:kandev-plugin-bitbucket"`;
pre-existing task links remain plugin state rather than host-owned task provenance.

Data Center accepts private-network installations intentionally. Production connections
require HTTPS, reject URL credentials, retain redirect origin, and enforce connect/read
timeouts and response-size limits. HTTP is development-only behind an explicit setting.

## API and host contracts

The required generic seams are defined by
[authenticated plugin actions](../../decisions/2026-07-31-authenticated-plugin-actions.md),
[repository-provider extensions](../../decisions/2026-07-31-plugin-repository-provider-extensions.md),
and [provider-neutral git credential brokerage](../../decisions/2026-07-31-provider-neutral-git-credential-broker.md).
Their frozen protocol/UI references are
[`GRPC-CONTRACT.md`](../../plans/plugins/GRPC-CONTRACT.md),
[`HOST-DATA-API.proto`](../../plans/plugins/HOST-DATA-API.proto), and
[`PLUGIN-API.md`](../../plans/plugins/PLUGIN-API.md).

The plugin declares actions including `connection.get`, `connection.save`,
`oauth.start`, `repositories.list`, `pullrequests.get`, and `watches.update`; each
has a resource scope and bounded body. It declares ownership of provider ID
`bitbucket` and the `bitbucket`/`pull_request` reference source. The browser calls
declared actions only through the authenticated host action route; public plugin
webhooks remain reserved for provider callbacks.

## Failure modes

- A disabled, unavailable, or timed-out action plugin returns a bounded actionable
  error; it does not expose the public webhook route as a browser fallback.
- A failed connection, expired credential, or refresh denial transitions visibly to
  `auth_required` or `unavailable`; existing non-secret connection metadata stays
  intact. Health probes run around every 90 seconds with jitter/backoff.
- Cloud rate limits and Data Center transient failures use adapter-owned bounded retry/
  backoff. Unsupported product/version capabilities show an unsupported explanation.
- An invalid or incomplete provider descriptor, host/path mismatch, stale broker
  lease, disabled provider, or submission reauthorization denial fails closed.
- Watch create/recovery failures preserve durable reservations and surface the last
  error. A reset/delete never removes a task whose plugin provenance does not match.

## Persistence guarantees

Plugin state persists connection metadata, capability probe result, watches, cursors,
dedupe keys, reservations, links, and recovery/error state through the Host state API.
Encrypted secrets persist separately. Credential broker leases are short-lived and
non-durable; they are re-resolved from the live plugin and revoked on task/session/
workspace teardown, plugin disable, connection reset, or credential-generation change.
The plugin exposes that generation only as an opaque, non-secret credential binding;
the host checks it before and after each lease redemption and fails closed when absent
or changed. Plugin disable, error, or uninstall also immediately revokes every lease
for its declared provider; exact repository path matching remains case-sensitive.

## Scenarios

- **GIVEN** a connected Cloud or Data Center workspace, **WHEN** a user selects a
  Bitbucket repository in native task creation, **THEN** Kandev persists the complete
  plugin-inspected descriptor and exact credential-free clone URL without host-side
  Bitbucket URL parsing.
- **GIVEN** an eligible task, **WHEN** a user opens a desktop task context/dropdown or
  visible mobile task action menu, **THEN** **Link → Bitbucket Pull Request** is
  available in the Link group and invokes the plugin with verified current context.
- **GIVEN** a Bitbucket pull request linked to a task, **WHEN** the user opens review
  on desktop or mobile, **THEN** the native review surface selects a normalized
  Bitbucket item and mounts the plugin `ReviewPanel` in the normal review location.
- **GIVEN** the plugin is disabled after a review panel, task action, or repository
  provider was registered, **WHEN** Kandev refreshes the plugin registry, **THEN**
  those registrations disappear, in-flight work is aborted, review selections close
  safely, and no host Bitbucket fallback remains.
- **GIVEN** a `#` Bitbucket pull-request result was selected, **WHEN** the message is
  submitted after the plugin was disabled, moved workspaces, or access changed,
  **THEN** submission reauthorization rejects it and no unapproved reference metadata
  reaches the queued message.
- **GIVEN** a watch finds the same pull request concurrently or restarts during task
  creation, **WHEN** recovery runs, **THEN** the durable reservation and plugin-owned
  task metadata result in at most one created task; reset/delete previews and removes
  only that owned tree.

## Out of scope

- Bitbucket Issues, Cloud app passwords, and a generic host Bitbucket API client.
- Bitbucket-specific changes in `agentctl` pull-request automation or a new
  provider-specific WebSocket contract.
- External Bitbucket webhooks for watches; authenticated polling is v1 behavior.
- Pretending Cloud Pipelines and Data Center build statuses have identical APIs.
- Marketplace publication before required host contracts are released, package,
  desktop/mobile, and container credential-flow acceptance passes.

## Implementation plan

[Bitbucket plugin implementation plan](../../plans/bitbucket-plugin/plan.md)
