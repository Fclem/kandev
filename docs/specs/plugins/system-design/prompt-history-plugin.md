---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-001
  - REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-002
  - REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-003
---
# Prompt History Plugin System Design

## Purpose and boundaries

This design covers the production Prompt History plugin: the package in
`kdlbs/kandev-plugin-prompt-history` and the one-shot parity proof against a
disposable development instance. The plugin owns presentation and derivation
only. Conversation access, authorization, sanitization, pagination
transport, live reconciliation, lifecycle fencing, and transcript navigation
belong to the Host and are owned by
[the Host prerequisites system design](prompt-history-extraction-host.md);
this design consumes those contracts and does not restate them. The core
Prompt History panel remains the parity reference; its behavior is owned by
the UI system and is not modified here.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-001` | [Package and identity](#package-and-identity) |
| `REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-002` | [UI bundle architecture](#ui-bundle-architecture), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |
| `REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-003` | [Parity proof](#parity-proof), [Verification](#verification) |

## Package and identity

The repository is created from `kdlbs/kandev-plugin-template` and keeps the
template's packaging, test, and release safeguards. Identity is synchronized
across the manifest `id`, the `go.mod` module, the Makefile `BIN`/`PKG_OUT`/
`VERSION`, and the `window.registerKandevPlugin` id:
`kandev-plugin-prompt-history`.

Manifest fields, per the [manifest reference](../../../public/plugins-manifest.md):

- `id: "kandev-plugin-prompt-history"`, `api_version: 2`,
  `version: "0.1.0"`, `display_name: "Prompt History"`, `author: "kandev"`,
  `repo_url: "https://github.com/kdlbs/kandev-plugin-prompt-history"`.
- `min_kandev_version` — the first release carrying the browser
  conversation facade (PR #3588). `0.91.1` is only the `messages`
  capability audit floor (`MinimumMessagesCapabilityVersion`); as of this
  writing no release carries the facade (latest release 0.94.0, 2026-09-09;
  #3588 merged 2026-09-15), so the manifest declares the first stable
  release cut after the merge (0.95.0 at writing time; confirm at release
  cut).
- `runtime.type: binary` with all five platform executables under `server/`.
  The installer requires a managed runtime even for a UI-focused plugin; the
  executable is a no-op `pluginsdk.UnimplementedPlugin` server (the Host
  prerequisites requirements
  [AC-PLUGINS-PROMPT-HISTORY-HOST-002.8](prompt-history-extraction-host.md#req-plugins-prompt-history-host-002-browser-conversation-host-boundary)
  confirm the browser facade needs no plugin backend).
- `capabilities: { api_read: ["messages"] }` and only that capability.
- `ui: { bundle: "/ui/bundle.js" }`. No `ui.pages`, `ui.styles`,
  `ui.keybindings`, webhooks, actions, `config_schema`, or provider
  declarations.
- Packaging stages only `manifest.yaml`, the built `ui/bundle.js`, and the
  five `server/plugin-<goos>-<goarch>` executables (mirroring
  `kdlbs/kandev-plugin-voice`'s `stage_common`); the `ui/` source tree,
  `ui/package.json`, and `ui/node_modules` are never staged, and the
  plugin's `verify-package` additionally asserts their absence from the
  archive.
- The template's `go.mod` `replace` and CI checkout pin a sibling kandev
  checkout (`f218880e`, which predates the facade); the repository bumps
  both to the PR #3588 merge commit (`2b1d0cf7d`) or later, so the pinned
  `@kandev/plugin-sdk` carries the conversation types.

## UI bundle architecture

The bundle follows the official-plugin toolchain used by
`kdlbs/kandev-plugin-voice`: TypeScript sources under `ui/src/` compiled by
`ui/build.mjs` (esbuild) into the single ES module `ui/bundle.js`, with
collocated vitest tests against a `test-host` mock. No React is bundled; the
build aliases `react` and `react/jsx-runtime` to a host-delegating shim.

Module layout:

- `ui/src/index.tsx` — `window.registerKandevPlugin` entry;
  `initialize(registry, host)` registers the task panel and translations,
  repeatable across enable/disable cycles.
- `ui/src/panel.tsx` — the `PromptHistoryPanel` component, registered with
  panel key `prompt-history` (layout id
  `plugin:kandev-plugin-prompt-history:prompt-history`); renders rows
  (including the agent-sent indicator as an inline SVG glyph, since
  `host.ui` exposes no icon primitive), loading, empty, error, passthrough,
  and removed states; owns expansion state keyed by message id. Test ids use
  a `ph-plugin-` prefix distinct from the core panel's ids.
- `ui/src/derive.ts` — pure entry derivation from the Host DTOs: newest-first
  ordering, `#N` ordinal from `promptIndex`, agent-sent flag from
  `senderTaskId`, and duration bounded by the earlier of turn completion and
  the next prompt's send time (floored to seconds, clamped at zero). Mirrors
  `buildPromptHistoryEntries` in
  `apps/web/lib/prompt-history.ts` so parity assertions compare the same
  arithmetic, and provides the plugin-local `formatPromptDuration` mirror
  (`h m s` unit labels from the translation catalog) matching
  `apps/web/lib/prompt-history.ts` `formatPromptDuration`.
- `ui/src/strings.ts` — translation catalogs (en plus every supported locale
  and the pseudo locale), registered through
  `registry.registerTranslations`.
- `ui/src/test-host.ts` — Host mock for the vitest suite: fake
  `useSessionMessages`/`useSessionTurns` state machines, `openMessage`
  outcomes, `host.ui.PromptMentionText`, and `host.utils.formatRelativeTime`.

## Data and contracts

The panel component receives `PluginTaskPanelProps`
(`taskId`, `sessionId`, `sessionKind`, `presentation`, `panelId`,
`conversation`) and consumes only:

- `conversation.history.useSessionMessages({ sessionId, taskId, authorTypes:
  ["user"], sort: "desc" })` — `pageSize` omitted so the host facade default
  (20) applies, matching the parity reference (`OLDER_PROMPT_PAGE_LIMIT = 20`
  in `apps/web/hooks/use-lazy-load-prompts.ts`, `limit: 20` in
  `apps/web/hooks/domains/session/use-session-prompts.ts`).
  State: `messages`, `loading`, `hydrated`, `loadingMore`, `error`, `hasMore`,
  `removed`, `loadMore()`, `retry()`.
- `conversation.history.useSessionTurns(sessionId, taskId)` — turn
  `startedAt`/`completedAt` for duration derivation, with state `loading`,
  `hydrated`, `error`, `removed`, `retry`. Durations render only after turns
  hydrate; until then rows show no duration, mirroring the core
  `turnsHydrated` gate.
- `conversation.history.useMessageFavorite(sessionId, messageId)` — read-only
  favorite state per row.
- `conversation.openMessage(messageId)` — transcript navigation; the
  `{ status: "accepted" | "unavailable" }` outcome is consumed without error
  surfacing for `unavailable`.
- `host.ui.PromptMentionText` — custom-prompt alias rendering and preview.
- `host.utils.formatRelativeTime` — locale-aware send-time rendering.
  Intentional parity delta: the core row uses the catalog-backed compact
  ladder (`formatRelativeCompact`, e.g. "5m"); plugins must use the host
  `formatRelativeTime` (full Intl phrase, e.g. "5 minutes ago") so timestamps
  follow the user's locale. Accepted delta: the core row's absolute-time
  `title` tooltip (`formatDateTime`) has no `host.utils` counterpart, so the
  plugin omits it.

DTO fields used: `id`, `turnId`, `createdAt`, `updatedAt`, `promptIndex`,
`senderTaskId`, `content`, `authorType` (messages); `id`, `startedAt`,
`completedAt`, `updatedAt` (turns). No field beyond the sanitized
`PluginConversationMessage`/`PluginConversationTurn` shapes is read.

## Control flow

1. The Host mounts the panel for the active task/session/presentation and
   hands the component generation-scoped props.
2. The panel calls the two conversation hooks; the Host facade owns
   snapshot/subscription ordering, deterministic cursors, live
   reconciliation, reconnects, and generation fencing — the panel never
   touches WebSocket frames.
3. `derive.ts` maps DTOs to rows on every state change; React re-renders rows
   keyed by message id. Expansion state survives row reordering because it is
   keyed by message id, not row index.
4. Row selection calls `conversation.openMessage(messageId)`; the Host
   validates the target and drives the native desktop or mobile navigation
   owner.
5. Unmount, session switch, or plugin generation change aborts the panel's
   work through the Host facade; the panel holds no module-level conversation
   state of its own.

## Pagination and reveal

The panel mirrors the core's pagination and reveal behavior:

- Paging stops when the first prompt (`#1`) is rendered: the older-page
  trigger is active only while `hasMore` and no rendered entry has
  `promptNumber === 1`.
- A minimum 400 ms loading-indicator display window keeps back-to-back
  auto-loads readable as one indicator.
- The loading indicator floats or renders in-flow based on measured
  scrollability, and the view sticks to the bottom while older pages load.
- The sentinel uses `rootMargin: "0px 0px 200px 0px"` and rejoins in-flight
  older-page requests instead of duplicating them.

## Failure and recovery

- The messages hook's `error` renders the retry surface only when no rows
  are committed (mirroring the core `fetchFailed && entries.length === 0`
  condition); with committed rows, the rows render without a retry
  affordance. `retry()` re-runs the Host facade's recovery. The turns hook's
  `error` is not surfaced.
- `removed` (terminal session removal) stops pagination and live updates;
  committed rows remain visible and `hasMore` is false.
- `loading` renders the initial loading state; `loadingMore` renders the
  older-page loading indicator while `hasMore` is true.
- Duration derivation uses whichever bound exists (turn completion or the
  next prompt's send time); a row with no bound shows no duration, matching
  the core `durationSeconds === null` branch.

## Persistence

The plugin owns no durable state. Favorites, custom-prompt aliases, and
conversation rows are host-owned; the no-op backend executable stores
nothing. Uninstall removes the panel registration and the Host-managed
plugin record; no plugin data directory is written.

## Security

- Least privilege: only `api_read: ["messages"]` is declared; the server-side
  capability gate on `/api/plugins/{id}/conversation/...` enforces it.
- Same-origin native plugin: the bundle runs in the Kandev origin with Host
  store access; it does not read `host.store`, import `apps/web` modules, or
  call unscoped `/api/v1` routes.
- No secrets, webhooks, or inbound routes. No operator settings.

## Parity proof

The one-shot proof (per
[REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-003](../requirements/prompt-history-plugin.md))
runs against a disposable development instance:

1. `make package-host` in the plugin repo produces
   `kandev-plugin-prompt-history-0.1.0.tar.gz`; `make verify-package-host`
   checks contents, checksums, and leak-free staging.
2. The monorepo e2e test-base backend and Playwright harness are used as the
   disposable instance and driver. A throwaway spec (not committed to the
   monorepo) inlines the upload flow with its own plugin id and package path
   (the shared `installFixturePlugin` helper hardcodes the fixture's
   `kandev-plugin-e2e` id and package) and drives the same behavioral
   checks as `apps/web/e2e/tests/plugins/prompt-history-plugin.spec.ts` and
   `mobile-prompt-history-plugin.spec.ts`, addressing the production panel
   by its `ph-plugin-` test ids and layout identity, distinct from the core
   panel's ids.
3. The core panel and the fixture plugin are exercised alongside to confirm
   they remain behaviorally unchanged.

## Verification

- Plugin repo: `make test` (Go tests + vitest suite), `make vet`,
  `test -z "$(gofmt -l .)"` (the template's `make fmt` lists unformatted
  files but exits 0, so it is advisory), `make package` (cross-platform),
  `make verify-package` (archive contents, checksums, staging leak check,
  absence of `ui/src`, `ui/node_modules`, `ui/package.json`), CI equivalent.
- Parity: the throwaway desktop and mobile Playwright runs from the parity
  proof section, plus `make -C apps/backend e2e-plugin-package` and the
  existing core prompt-history E2E specs to confirm core preservation.

## Related decisions

- [Browser plugins read session conversations through a typed Host
  facade](../../../decisions/2026-09-06-browser-plugin-conversation-facade.md)
  (proposed; this plugin is the first consumer).
- [ADR 0047: Plugin host conversation reads](../../../decisions/0047-plugin-host-conversation-reads.md)
  — the Go-side reads this plugin does not use.
- [Host prerequisites requirements](../requirements/prompt-history-extraction-host.md)
  and [Host prerequisites system design](prompt-history-extraction-host.md).
