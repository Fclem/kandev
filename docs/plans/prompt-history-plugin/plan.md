---
created: 2026-09-15
status: draft
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-001
  - REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-002
  - REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-003
system_design:
  - "../../specs/plugins/system-design/prompt-history-plugin.md"
legacy_specs: []
---

# Implementation Plan: Prompt History Plugin

## Overview

Create `kdlbs/kandev-plugin-prompt-history` from
`kdlbs/kandev-plugin-template`, implement the Prompt History task panel with
feature parity to the shipped core panel, package the production artifact, and
prove parity once against a disposable development instance on desktop and
mobile. The monorepo is not changed: the core panel, the test-only fixture
plugin, and its E2E specs stay untouched. Work orders run in dependency order
01 -> 02 -> 03; they share one repository and one package artifact, so
parallel execution is not appropriate.

## Scope

### In scope

- The `kdlbs/kandev-plugin-prompt-history` repository with the production
  plugin: manifest identity, least-privilege capabilities, no-op Go backend,
  UI bundle with the parity panel, translation catalogs, and packaging.
- One-shot parity proof against a disposable development instance using the
  monorepo e2e harness as the driver (throwaway spec, not committed).
- Package verification: archive contents, checksums, staging leak check, and
  cross-platform executables.

### Out of scope

- Saved-panel-identifier migration and removal of the core Prompt History
  panel (later monorepo packages, per the Step 2 ordering in
  [issue #3567](https://github.com/kdlbs/kandev/issues/3567)).
- Marketplace publication: release tagging and the
  `plugin-registry/plugins.yaml` catalog entry.
- A permanent monorepo E2E regression for the production package.
- Host, SDK, or backend changes; the plugin consumes the Step 1 contracts as
  merged in PR #3588.
- The test-only fixture plugin and its E2E specs remain test support.

## Technical approach

### Checkout layout

The plugin repository is checked out as a sibling of the monorepo worktree
(e.g. `../kandev-plugin-prompt-history`). Its `go.mod` `replace`, its
`KANDEV_SDK` Makefile variable, and its CI checkout point at the worktree's
`apps/backend` and `apps/packages/plugin-sdk` (the template assumes a sibling
directory literally named `kandev`, so the references are updated to the
worktree's directory name). All verification commands below assume this
layout.

### Task 01: Repository bootstrap and installable skeleton

Create the public repository from the template, keeping its packaging,
test, and release safeguards. Rename the identity in all four places
(manifest `id`, `go.mod` module, Makefile `BIN`/`PKG_OUT`/`VERSION`,
`window.registerKandevPlugin` id) to `kandev-plugin-prompt-history`;
`display_name: "Prompt History"`, `author: "kandev"`, `repo_url` to the new
repository. Replace the template demo behavior:

- Manifest: `api_version: 2`, `min_kandev_version: "0.95.0"` (the first
  release carrying the #3588 browser conversation facade; confirm at release
  cut), `capabilities: { api_read: ["messages"] }`, `ui.bundle: "/ui/bundle.js"`,
  all five platform executables. Remove webhooks, actions, `config_schema`,
  events, `state`, `secrets`, `agent_invoke`, providers, and agent tools.
- `server/`: no-op `pluginsdk.UnimplementedPlugin` (the browser facade needs
  no backend logic; AC-PLUGINS-PROMPT-HISTORY-HOST-002.8).
- `ui/bundle.js`: minimal registration of a placeholder task panel
  (`registerTaskPanel`, `mobileEnabled: true`) plus the translation-catalog
  skeleton (en, pt-pt, zh-cn, zh-hk, zh-tw, pseudo).
- Pinned SDK reference: bump the template's pinned kandev ref (CI checkout
  `ref:`, currently `f218880e`, which predates the facade) and the local
  sibling checkout to the PR #3588 merge commit (`2b1d0cf7d`) or later, so
  the pinned `@kandev/plugin-sdk` carries the conversation types.

### Task 02: Parity panel implementation

Adopt the official-plugin toolchain from `kdlbs/kandev-plugin-voice`:
TypeScript sources under `ui/src/` built by `ui/build.mjs` (esbuild, no
bundled React, host-delegating JSX shim) into `ui/bundle.js`, with collocated
vitest tests against a `test-host` mock.

- `ui/src/derive.ts`: pure row derivation mirroring
  `apps/web/lib/prompt-history.ts` — newest-first, `#N` from `promptIndex`,
  agent-sent flag from `senderTaskId`, duration = floor of the earlier of
  turn `completedAt` and the next prompt's `createdAt`, clamped at zero,
  seconds; durations suppressed until turns hydrate (the core
  `turnsHydrated` gate); plus the plugin-local `formatPromptDuration` mirror
  (`h m s` unit labels from the translation catalog).
- `ui/src/panel.tsx`: consumes
  `conversation.history.useSessionMessages({ sessionId, taskId,
  authorTypes: ["user"], sort: "desc" })` (pageSize omitted, host default 20)
  and `conversation.history.useSessionTurns(sessionId, taskId)`; per-row
  `useMessageFavorite`, `host.ui.PromptMentionText` alias rendering,
  `host.utils.formatRelativeTime` send time (documented parity delta: full
  Intl phrase instead of the core compact ladder), the agent-sent indicator
  (inline SVG glyph, since `host.ui` exposes no icon primitive), truncation
  with overflow detection and a distinct expand control, expanded box capped
  at 40% of the panel height with its own scroll, expansion state keyed by
  message id. States: initial loading, empty, error with retry only when no
  rows are committed (the core `fetchFailed && entries.length === 0`
  condition), `loadingMore` indicator while `hasMore`, `removed` terminal
  state, passthrough degraded state (`sessionKind === "passthrough"`).
  Pagination mirrors the core: paging stops when the first prompt (`#1`) is
  rendered, a minimum 400 ms loading-indicator window, floating vs in-flow
  indicator by measured scrollability, stick-to-bottom while loading, and the
  sentinel with `rootMargin: "0px 0px 200px 0px"` rejoining in-flight
  requests. Row selection calls `conversation.openMessage(messageId)`;
  `unavailable` outcomes are consumed without error surfacing. The panel is
  registered with panel key `prompt-history` and uses `ph-plugin-` test ids
  distinct from the core panel's.
- `ui/src/strings.ts`: translation catalogs for en plus every supported
  locale and pseudo; `registerTranslations` in `initialize`.

### Task 03: Package and parity proof

`make package` + `make verify-package` in the plugin repo (the Makefile
stages only the built `ui/bundle.js`, mirroring `kdlbs/kandev-plugin-voice`'s
`stage_common`; `verify-package` additionally asserts `ui/src`,
`ui/node_modules`, and `ui/package.json` are absent from the archive), then
the one-shot parity proof against a disposable development instance:

1. Build the e2e test-base backend and web assets
   (`make -C apps/backend e2e-plugin-package`; `pnpm e2e:run` build step).
2. A throwaway spec (created for the run under
   `apps/web/e2e/tests/plugins/`, deleted after) installs the production
   `kandev-plugin-prompt-history-0.1.0.tar.gz` through the existing upload
   flow, inlined with its own plugin id and package path (the shared
   `installFixturePlugin` helper hardcodes the fixture's `kandev-plugin-e2e`
   id and package), and drives the parity checks, targeting the production
   panel's `ph-plugin-` test ids: prompt ordering, `#N` ordinals, alias
   rendering, durations, favorite distinction, agent-sent indicator,
   older-page auto-loading, live transitions, navigation, empty and error
   states, and desktop plus mobile placement (mirroring
   `prompt-history-plugin.spec.ts` and
   `mobile-prompt-history-plugin.spec.ts`).
3. Re-run the core prompt-history E2E specs to confirm the core panel and
   fixture remain behaviorally unchanged.

## ASCII UI preview

### UI-01: Desktop panel (dockview, via "+" menu)

```text
+----------------------------+
| Prompt History             |
+----------------------------+
| #3 fix the login flow ...  | 12 minutes ago  (AC-002.2, .4)
|            [expand]        | 1m 23s    (AC-002.3, .4)
|                            |
| #2 why did the deploy ...  | 1 hour ago
|            [expand]        | 2m 05s
|                            |
| #1 set up the project ...  | 3 days ago
|                            |
+----------------------------+
| Loading older messages     |  floating chip only when the
+----------------------------+  panel scrolls; in-flow otherwise
                                  (AC-002.6)
```

### UI-01E: Expanded row

```text
+----------------------------+
| #3 fix the login flow      |
|   (wrapped text inside a   |  cap: 40% of panel height,
|   box with its own scroll) |  own scroll area (AC-002.3)
|            [collapse]      |
+----------------------------+
```

### UI-01M: Phone (Panels picker, full height)

```text
+--------------------------+
| Prompt History            |
+--------------------------+
| #3 fix the login flow ... | 12 minutes ago
|            [expand]       | 1m 23s
|                           |  rows: min 44px tap targets;
| #2 why did the deploy ... | 1 hour ago   expansion is a distinct
|            [expand]       | 2m 05s   control (AC-002.1, .3)
+--------------------------+
```

### States (shared, both presentations)

- Loading: neutral "Loading" row, no rows, no arrows.
- Empty: localized empty text, no controls.
- Error: retry control only when no rows are committed; committed rows
  render without a retry affordance (AC-002.9).
- Passthrough: the same localized empty copy as the empty state (the core
  renders the same string with no controls), no rows and no controls.
- Removed: committed rows remain, no pagination, no live updates.

Fixed regions: panel header, panel chrome. Scrolling region: the prompt rows.
The preview is structural; spacing is not a pixel specification. Copy is
plugin-localized (AC-002.10).

## Tests

| Criterion | Evidence |
| --- | --- |
| AC-002.2, .3, .4, .9 | `ui/src/derive.test.ts` and `ui/src/panel.test.tsx` in the plugin repo (vitest against `test-host`): ordering, ordinals, duration bounds, states |
| AC-001.2, .3 | Manifest assertions in `server/` or `ui/` tests plus `make verify-package` |
| AC-002.1, .5, .6, .7, .8, .10 and AC-003.1, .2, .3 | Throwaway parity spec from Task 03 against the disposable instance (desktop + mobile) |
| Go backend no-op contract | `server/plugin_test.go` (template-derived) |

## E2E tests

One-shot, throwaway (not committed): `prompt-history-parity-check.spec.ts`
(desktop, chromium project) and the mobile equivalent (mobile-chrome /
Pixel 5), both driving the production tarball and mapping to
AC-PLUGINS-PROMPT-HISTORY-PLUGIN-003.1, AC-003.2, and the AC-002.1, .2, .5,
.6, .7, .8 behaviors listed in the Task 03 work order. Core preservation is
confirmed by re-running the existing `e2e/tests/task/prompt-history-panel.spec.ts`
and `mobile-prompt-history-panel.spec.ts`.

## Work orders

- [ ] [Task 01: Bootstrap repo and installable skeleton](task-01-bootstrap-repo-skeleton.md)
- [ ] [Task 02: Implement the parity panel](task-02-implement-parity-panel.md)
- [ ] [Task 03: Package and prove parity](task-03-package-and-prove-parity.md)

## Verification results

Pending.

## Risks

- Host contract drift: the plugin pins `min_kandev_version` to the first
  release carrying the facade (0.95.0 at writing time); a later host release
  changing the `host.conversation` DTO shapes would break the bundle. The
  SDK typecheck against the pinned `@kandev/plugin-sdk` catches most drift at
  build time, which is why the pinned ref must include PR #3588.
- One-shot proof only: per the settled scope, no permanent monorepo
  regression covers the production package; the follow-up migration package
  must add one when it removes the core panel.
- Parity delta: send time uses `host.utils.formatRelativeTime` (full Intl
  phrase) instead of the core compact ladder; this is host-contract-mandated
  and documented in the system design, not a defect.
- Disposable instance availability: the parity proof needs the monorepo e2e
  harness and a working browser; environment-sensitive e2e failures must be
  attributed before treating them as plugin defects.
- Repository creation is an external GitHub action; the repository must be
  created before Task 01 proceeds, and the template must be the source of
  the initial tree.

## Open questions

None.
