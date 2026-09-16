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
  `apps/web/lib/prompt-history.ts` — `#N` ordinal from `promptIndex`
  (absent/0 renders no ordinal), agent-sent flag from `senderTaskId`, and
  duration = floor of the earlier of turn `completedAt` and the
  chronologically next (newer) prompt's `createdAt` — the preceding
  element of the facade's newest-first array, not `index + 1` — clamped at
  zero, whole seconds; missing bounds yield no duration; durations are
  suppressed until turns hydrate (the core `turnsHydrated` gate); plus the
  plugin-local `formatPromptDuration` mirror (`h m s` unit labels from the
  translation catalog) matching `apps/web/lib/prompt-history.ts`
  `formatPromptDuration`. Ordering is the facade's page order (older pages
  append; live updates arrive already ordered by the host); derive does
  not re-sort. The vitest suite includes a case with two identical
  `createdAt` values whose ids are seeded so ascending-id order
  contradicts the facade page order, to pin that derive preserves page
  order rather than sorting by timestamp.
- `ui/src/panel.tsx`: consumes
  `conversation.history.useSessionMessages({ sessionId, taskId,
  authorTypes: ["user"], sort: "desc", pageSize: 20 })` (passing
  `pageSize: 20` explicitly; the host facade's `query.pageSize ?? 20`
  default is a cross-check) and
  `conversation.history.useSessionTurns(sessionId, taskId)`; per-row
  `useMessageFavorite` with the favorite highlight; `host.ui
  .PromptMentionText` alias rendering in both the truncated and expanded
  views; `host.i18n.t` / `host.i18n.useTranslation()` for every
  user-facing string (the plugin-scoped catalog namespace);
  `host.utils.formatRelativeTime` for the send time; the agent-sent
  indicator (inline SVG glyph, since `host.ui` exposes no icon primitive);
  truncation with overflow detection and a distinct expand control;
  expanded box capped at 40% of the panel height with its own scroll;
  expansion state keyed by message id so live reordering keeps the right
  row expanded.
- States: initial loading, empty, error with the retry surface only when no
  rows are committed (the core `fetchFailed && entries.length === 0`
  condition; with committed rows the rows render without a retry affordance),
  `loadingMore` indicator while `hasMore`, `removed` (the Host facade's
  terminal state, not a parity-reference state; the core unmounts with the
  task, so no pagination and no live updates while committed rows remain),
  and the passthrough degraded state for
  `sessionKind === "passthrough"` (the same localized empty copy as the
  empty state, no controls).
- Pagination mirrors the core: paging stops when the first prompt (`#1`) is
  rendered, a minimum 400 ms loading-indicator display window, floating vs
  in-flow indicator by measured scrollability, stick-to-bottom while loading,
  and the sentinel with `rootMargin: "0px 0px 200px 0px"` (the Host facade
  joins concurrent older-page loads; the sentinel must not re-issue a load
  already in flight).
- Row selection calls `conversation.openMessage(messageId)`; an
  `unavailable` outcome is consumed without error surfacing.
- `ui/src/panel-state.ts`: the pure seam for the state determination
  and `openMessage` outcome handling above, consumed by `panel.tsx`
  and tested against `test-host` (the vitest suite stays logic-only,
  mirroring `kdlbs/kandev-plugin-voice`, whose `ui/package.json` has
  no react runtime or renderer dependency and whose tests never import
  a react-importing module - `panel.tsx` is rendered only by the
  throwaway parity spec).
- The panel registers with panel key `prompt-history` (layout id
  `plugin:kandev-plugin-prompt-history:prompt-history`), a `titleKey`,
  `mobileEnabled: true` (the mobile Panels picker and bottom nav filter on
  it), a bundled icon component, and no `visible` predicate (the host's
  `registrationIsVisible` gates both the menu entry and the panel body, so
  omitting it keeps the passthrough state reachable), and uses `ph-plugin-`
  test ids distinct from the core panel's ids. Accessibility mirrors the
  parity reference where it exists: `role="status" aria-live="polite"` on
  the loading indicator (the core renders it on every loading render),
  the row bubble's 44 px mobile minimum with its desktop release (the
  core's `min-h-11 md:min-h-0`) and a focusable full-row navigate
  `<button>` (the core's `min-h-11`), both supplied by `ui/plugin.css`,
  and
  `aria-describedby` pointing at an `sr-only` row label whose text is the
  row `aria-label`, and a real `<button>` expand control with
  `aria-expanded` and a catalog `aria-label` (the parity spec's role-based
  queries and 44 px tap-target assertions
  depend on these); deliberate delta: the plugin also puts
  `role="status"` on the empty state (the core's empty and passthrough
  states are plain divs with no role).
- `ui/src/strings.ts`: translation catalogs for en plus every supported
  locale and the pseudo locale (`en` required; `pt-pt`, `zh-cn`, `zh-tw`,
  `zh-hk`, `pseudo`), flat keys matching `^[a-z][a-zA-Z0-9_-]*$` (no dots or
  nesting), at most 1000 messages per locale and 4096 characters per
  message (a violation throws at `initialize` and aborts every
  registration); registered through
  `registry.registerTranslations` in a repeatable `initialize`; the
  vitest suite in `ui/src/strings.test.ts` asserts every catalog (`en`,
  `pt-pt`, `zh-cn`, `zh-tw`, `zh-hk`, `pseudo`) carries exactly the same
  key set.
- `ui/plugin.css`: the plugin-owned stylesheet (declared as
  `ui.styles: ["/ui/plugin.css"]` in the manifest; the Task 01 placeholder
  is replaced here). The bundle is built in a separate repository and
  imported at runtime from `/api/plugins/{id}/bundle`, so the host's
  build never sees it and the stylesheet owns every class it renders (the
  host's `@source` globs in `apps/web/app/globals.css` cover only
  `apps/web/components/**` and `apps/packages/ui/src/**`); namespaced class
  names and kandev CSS custom properties for theme fidelity, mirroring
  `kandev-plugin-voice` (`ui/plugin.css` + `ui.styles`).
- `ui/build.mjs` (esbuild, no bundled React, `react` and
  `react/jsx-runtime` aliased to `ui/src/react-shim.ts`, the
  host-delegating shim) and `ui/src/test-host.ts` (the Host mock for the
  vitest suite, installed through `setHost`).
- `ui/pnpm-workspace.yaml`: allowlists esbuild's build script
  (`onlyBuiltDependencies: [esbuild]`, mirroring `kandev-plugin-voice`;
  pnpm 10 does not run dependency lifecycle scripts unless allowlisted,
  and the monorepo pins pnpm 9.15.9, so nothing else in this package
  models the pnpm 10 default).
- `ui/src/host.ts`: re-exports the `@kandev/plugin-sdk` types (the
  `file:../../kdlbs-kandev/apps/packages/plugin-sdk` dependency in
  `ui/package.json`) instead of restating the contract, so
  `tsc --noEmit` typechecks the panel against the pinned SDK, and,
  mirroring `kdlbs/kandev-plugin-voice`, owns the module-scoped host
  handle (`host()`, `maybeHost()`, `hostReact()`) that
  `ui/src/react-shim.ts` and the panel import, and its write side
  (`setHost(host)`, `clearHost()`) that `ui/src/index.tsx` calls from
  `initialize` and `destroy`.
- Makefile: `ui-install`, `ui` (the esbuild build), `typecheck` (tsc
  --noEmit), and `test-ui` (vitest) targets mirroring
  `kdlbs/kandev-plugin-voice`, the `package`/`package-host` staging
  switch from the template's `cp -r ui` to the voice `stage_common`
  (staging only `manifest.yaml`, the built `ui/bundle.js`, and
  `ui/plugin.css`), so the `make package-host` smoke stages a clean
  tree once `ui/src` and `ui/node_modules` exist, and the
  `verify-package` and `verify-package-host` assertions: `ui/plugin.css`
  present, and `ui/src`, `ui/node_modules`, and `ui/package.json` absent
  from the archive, adding `ui/bundle.js` to the `clean` target (Task 01
  renames its archive glob), and updating the README's "Both stage
  `manifest.yaml` + `ui/`" sentence and its "make package" inline comment
  ("packs manifest + ui/ + binaries") to the `stage_common` file set, and
  updating the `ui.bundle` comment in `manifest.yaml` (esbuild output, no
  longer hand-written).
- CI: each workflow gains the `kandev-plugin-voice` UI steps -
  `Set up Node` (node 24), `Set up pnpm` (v10), and `make ui-install` -
  anchored to its verification step, not to packaging (all three have
  no Node steps after Task 01's strip): `ci.yml` gains them immediately
  before its `Test` step, then `make typecheck`, `make test-ui`, and
  `make ui`; `build.yml` gains them before its `make build` step;
  `release.yml` gains them before its `Verify` step, which runs
  `make test` - not merely before `make verify-package` - since the
  `test` target includes `test-ui` and the Makefile `package` target
  depends on `ui`.
- `ci.yml`'s `Test` step runs `make test-backend` (mirroring
  `kandev-plugin-voice`'s `make test-go`; the Makefile `test` target
  becomes `test-backend test-ui` for local runs - it is `test-backend`
  after Task 01's recipe strip - so the UI suite runs exactly once, in
  its own step after typechecking).
- `.gitignore`: add `/ui/bundle.js` and `/ui/node_modules/` (mirroring
  `kandev-plugin-voice`'s generated-output entries), and run
  `git rm --cached ui/bundle.js` when the esbuild output replaces the
  hand-written placeholder from Task 01.

## Out of scope

- Cross-platform packaging and the parity proof (Task 03).
- Any monorepo change, including the core panel, the fixture plugin, and its
  E2E specs.

## Acceptance

- `make test` passes in the plugin repo, including vitest coverage of
  ordering, ordinals, duration bounds, the turns-hydration gate, the
  agent-sent flag (derive.ts), state determination, `openMessage` outcome
  handling, and page-order preservation with identical `createdAt`
  values (ids seeded to contradict page order; the rendered favorite
  distinction, the agent-sent indicator, and the states are proven by the
  throwaway parity spec - the pinned harness is logic-only).
- `make package-host` produces a bundle whose panel registration matches
  the parity reference's feature set (user-prompt rows, `#N`, alias
  rendering, duration, send time, favorite highlight, agent-sent
  indicator, expand with the 40% cap, older-page auto-load, navigation),
  declares `mobileEnabled: true`, and declares no `visible` predicate.
- No copy is hardcoded: every user-facing string resolves through the plugin
  translation catalog with the English fallback.

## ASCII UI preview

See [UI-01 in the plan](plan.md#ascii-ui-preview) (desktop panel, expanded
row, phone composition, and shared states).

## Verification

```bash
cd ../kandev-plugin-prompt-history   # sibling of the monorepo worktree
make ui-install
make vet test
test -z "$(gofmt -l .)"              # make fmt is advisory (lists, exits 0)
make typecheck
make ui
make package-host
```

## Files likely touched

- `kdlbs/kandev-plugin-prompt-history/ui/src/index.tsx`
- `kdlbs/kandev-plugin-prompt-history/ui/src/panel.tsx`
- `kdlbs/kandev-plugin-prompt-history/ui/src/panel-state.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/derive.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/strings.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/host.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/react-shim.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/test-host.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/derive.test.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/panel.test.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/src/strings.test.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/plugin.css`
- `kdlbs/kandev-plugin-prompt-history/ui/build.mjs`
- `kdlbs/kandev-plugin-prompt-history/ui/package.json`
- `kdlbs/kandev-plugin-prompt-history/ui/tsconfig.json`
- `kdlbs/kandev-plugin-prompt-history/ui/vitest.config.ts`
- `kdlbs/kandev-plugin-prompt-history/ui/pnpm-lock.yaml`
- `kdlbs/kandev-plugin-prompt-history/ui/pnpm-workspace.yaml`
- `kdlbs/kandev-plugin-prompt-history/.gitignore`
- `kdlbs/kandev-plugin-prompt-history/Makefile` (UI toolchain targets and
  the `stage_common` staging switch)

## Dependencies

- Task 01 (repository and installable skeleton).

## Risks

- The Host `host.conversation` DTO shapes are pinned by the installed
  host; `ui/src/host.ts` re-exports the SDK types from the pinned
  `@kandev/plugin-sdk` (`file:../../kdlbs-kandev/apps/packages/plugin-sdk` in
  `ui/package.json`), so `tsc --noEmit` catches DTO drift at build time,
  and the vitest `test-host` mock must track the same SDK types, not an
  older snapshot.
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
