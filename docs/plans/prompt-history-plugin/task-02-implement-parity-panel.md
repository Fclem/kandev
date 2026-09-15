---
id: "02-implement-parity-panel"
title: "Implement the parity panel"
status: pending
wave: 2
depends_on:
  - "01-bootstrap-repo-skeleton"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-002
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.1
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.2
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.3
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.4
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.5
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.6
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.7
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.8
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.9
  - AC-PLUGINS-PROMPT-HISTORY-PLUGIN-002.10
system_design:
  - ../../specs/plugins/system-design/prompt-history-plugin.md
---

# Task 02: Implement the Parity Panel

## Summary

Replace the placeholder bundle with the parity panel: TypeScript sources
under `ui/src/` built by `ui/build.mjs` into `ui/bundle.js`, with collocated
vitest tests against a `test-host` mock. The panel mirrors the shipped core
Prompt History panel's observable behavior using only the public Host
contracts.

## In scope

- `ui/src/derive.ts`: pure row derivation mirroring
  `apps/web/lib/prompt-history.ts` — newest-first ordering, `#N` ordinal from
  `promptIndex` (absent/0 renders no ordinal), agent-sent flag from
  `senderTaskId`, and duration = floor of the earlier of turn `completedAt`
  and the next prompt's `createdAt`, clamped at zero, whole seconds; missing
  bounds yield no duration; durations are suppressed until turns hydrate
  (the core `turnsHydrated` gate); plus the plugin-local
  `formatPromptDuration` mirror (`h m s` unit labels from the translation
  catalog) matching `apps/web/lib/prompt-history.ts` `formatPromptDuration`.
- `ui/src/panel.tsx`: consumes
  `conversation.history.useSessionMessages({ sessionId, taskId,
  authorTypes: ["user"], sort: "desc" })` (pageSize omitted, host default 20)
  and `conversation.history.useSessionTurns(sessionId, taskId)`; per-row
  `useMessageFavorite` with the favorite highlight; `host.ui.PromptMentionText`
  alias rendering in both the truncated and expanded views; `host.utils
  .formatRelativeTime` for the send time; the agent-sent indicator (inline
  SVG glyph, since `host.ui` exposes no icon primitive); truncation with
  overflow detection and a distinct expand control; expanded box capped at
  40% of the panel height with its own scroll; expansion state keyed by
  message id so live reordering keeps the right row expanded.
- States: initial loading, empty, error with the retry surface only when no
  rows are committed (the core `fetchFailed && entries.length === 0`
  condition; with committed rows the rows render without a retry affordance),
  `loadingMore` indicator while `hasMore`, `removed` terminal state (no
  pagination, no live updates), and the passthrough degraded state for
  `sessionKind === "passthrough"` (the same localized empty copy as the
  empty state, no controls).
- Pagination mirrors the core: paging stops when the first prompt (`#1`) is
  rendered, a minimum 400 ms loading-indicator display window, floating vs
  in-flow indicator by measured scrollability, stick-to-bottom while loading,
  and the sentinel with `rootMargin: "0px 0px 200px 0px"` rejoining
  in-flight older-page requests.
- Row selection calls `conversation.openMessage(messageId)`; an
  `unavailable` outcome is consumed without error surfacing.
- The panel registers with panel key `prompt-history` (layout id
  `plugin:kandev-plugin-prompt-history:prompt-history`) and uses `ph-plugin-`
  test ids distinct from the core panel's ids.
- `ui/src/strings.ts`: translation catalogs for en plus every supported
  locale and the pseudo locale; registered through
  `registry.registerTranslations` in a repeatable `initialize`.
- `ui/build.mjs` (esbuild, no bundled React, host-delegating JSX shim) and
  `ui/src/test-host.ts` (Host mock for the vitest suite).

## Out of scope

- Cross-platform packaging and the parity proof (Task 03).
- Any monorepo change, including the core panel, the fixture plugin, and its
  E2E specs.

## Acceptance

- `make test` passes in the plugin repo, including vitest coverage of
  ordering, ordinals, duration bounds, the turns-hydration gate, favorite
  distinction, the agent-sent indicator, states, and `openMessage` outcome
  handling.
- `make package-host` produces a bundle whose panel registration matches the
  parity reference's feature set (user-prompt rows, `#N`, alias rendering,
  duration, send time, favorite highlight, agent-sent indicator, expand with
  the 40% cap, older-page auto-load, navigation).
- No copy is hardcoded: every user-facing string resolves through the plugin
  translation catalog with the English fallback.

## ASCII UI preview

See [UI-01 in the plan](plan.md#ascii-ui-preview) (desktop panel, expanded
row, phone composition, and shared states).

## Verification

```bash
cd ../kandev-plugin-prompt-history   # sibling of the monorepo worktree
make vet test
test -z "$(gofmt -l .)"              # make fmt is advisory (lists, exits 0)
cd ui && pnpm install --frozen-lockfile && npx tsc --noEmit && node build.mjs
cd .. && make package-host
```

## Files likely touched

- `kdlbs/kandev-plugin-prompt-history/ui/src/index.tsx`
- `kdlbs/kandev-plugin-prompt-history/ui/src/panel.tsx`
- `kdlbs/kandev-plugin-prompt-history/ui/src/derive.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/strings.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/test-host.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/build.mjs`
- `kdlbs/kandev-plugin-prompt-history/ui/package.json`
- `kdlbs/kandev-plugin-prompt-history/Makefile` (test/build targets for the
  UI toolchain)

## Dependencies

- Task 01 (repository and installable skeleton).

## Risks

- The Host `host.conversation` DTO shapes are pinned by the installed host;
  the vitest `test-host` mock must track the SDK types in
  `apps/packages/plugin-sdk/src/index.ts`, not an older snapshot.
- The 40% cap, the auto-load sentinel, the 400 ms indicator window, and the
  floating vs in-flow indicator have no Host primitive; the implementation
  must re-derive them from the panel element (ResizeObserver and
  IntersectionObserver) while keeping the behavior observable in the vitest
  suite and the later parity proof.
- Duration arithmetic must match the core `buildPromptHistoryEntries`
  semantics (earlier-of bounds, floor, clamp); a divergence would show up in
  the parity proof, not in unit tests, so the vitest cases must seed
  turn/prompt timestamps that exercise both bounds.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/prompt-history-plugin.md)
  REQ-PLUGINS-PROMPT-HISTORY-PLUGIN-002.
- [System design](../../specs/plugins/system-design/prompt-history-plugin.md),
  UI bundle architecture, Data and contracts, Control flow, Pagination and
  reveal, Failure and recovery.
- Parity reference: `apps/web/components/task/prompt-history-panel-content.tsx`,
  `apps/web/components/task/prompt-history-panel-row.tsx`,
  `apps/web/lib/prompt-history.ts`.
- Host contract: `apps/packages/plugin-sdk/src/index.ts`
  (`PluginTaskPanelProps`, `PluginConversationApi`),
  `apps/web/lib/plugins/host-api.ts` (`PromptMentionText`,
  `formatRelativeTime`).
- Toolchain reference: `kdlbs/kandev-plugin-voice` (`ui/build.mjs`,
  `ui/src/test-host.ts`, vitest layout).

## Results

Pending.
