---
id: "05-ui-pill-panel"
title: "chat-top-bar pill and hover/tap panel"
status: done
wave: 3
depends_on: ["04-backend-action-poller"]
plan: "plan.md"
spec: "../../specs/deepseek-credits-plugin/spec.md"
---

# Task 05: chat-top-bar pill and hover/tap panel

## Intent

Implement `ui/bundle.js`: a hand-written, no-build ES module registering a
single `chat-top-bar` component (pill + hover/tap panel) that consumes the
`balance.get` action, plus the `node --test` bundle test suite. Model the
mechanics on `kandev-plugin-provider-usage/ui/bundle.js` (top-bar pill,
`position: fixed` anchored panel, open/close timers, silent re-read interval).

## Owned paths

- `ui/bundle.js` (replace template demo content; keep `registerKandevPlugin`)
- `test/bundle.test.mjs` (new; host-mock via `vm` like provider-usage)
- `Makefile` (test target: add `node --test test/bundle.test.mjs`,
  `node --check ui/bundle.js`; drop removed recipe targets if task 02 left them)

## Dependencies

Task 04 (action contract). Re-read `docs/plans/plugins/PLUGIN-API.md` for the
current `chat-top-bar` slot props and `host.api.invokeAction` signature before
implementing.

## Acceptance

1. Registers exactly the `chat-top-bar` component; slotProps
   `{ taskId, taskTitle, workspaceId, activeSessionId, sessionIds }` are read
   defensively. When `workspaceId` is null/empty the component renders
   NOTHING and issues no `invokeAction` (the action is workspace-scoped; a
   fetch with no selector would be rejected with 400 and retried forever).
   Pill content: DeepSeek monogram chip (brand-hue rounded
   square, e.g. `#4D6BFE`, `Ds` monogram — no hand-drawn whale) plus the
   primary currency's total balance formatted with `Intl.NumberFormat`
   (`style: "currency"`, `currencyDisplay: "narrowSymbol"` so CNY renders `¥`
   and USD `$`; compact notation when the amount would overflow the pill).
2. Pill color states: calm indigo by default; amber when the primary total is
   below the server-sent `warn_below`; muted coral while `is_available` is
   false; when both apply, coral wins over amber. Unconfigured shows a neutral
   unavailable indicator.
3. Hover (desktop) and click/tap (all) open the panel; `position: fixed`
   anchored below the trigger rect with the padding-bridge + close-timer
   pattern from provider-usage so the mouse never leaves the hover area. Panel
   content: primary total, granted/topped-up breakdown, every currency entry,
   `is_available` status line — which LEADS the panel when `is_available` is
   false (the spec scenario "the panel leads with the insufficient-balance
   status") — last-updated via
   `host.utils.formatRelativeTime`, a Refresh control, the `loading` state
   (key set, no snapshot yet — neutral, never an error), and the
   unconfigured / error guidance states (401 reason, settings path, env-var
   fallback). No required info is hover-only. Phone sizing keeps the pill and
   touch targets ≥ 44 px (`@media (max-width: 639px)`), matching
   provider-usage.
4. Data: `host.api.invokeAction("balance.get", { workspaceId, body: { refresh } })`
   — the forced-refresh flag travels in the action `body` because the host
   envelope (`ActionInput`) has no free-form keys; a 60 s silent re-read
   interval serves the cached snapshot with no body; the panel's Refresh
   passes `body: { refresh: true }`. The pill colors from the server-sent
   `warn_below` in the response. A fetch error never clears the last rendered
   data. `destroy` clears timers/intervals and removes injected styles.

## Verification

```sh
node --check ui/bundle.js
node --test test/bundle.test.mjs
make test-backend
make vet
```

`test/bundle.test.mjs` (host mock): registers only `chat-top-bar`; pill renders
the formatted primary-currency balance; amber below the server-sent
`warn_below`; coral when unavailable; coral wins when low AND unavailable;
neutral loading state before the first snapshot; unconfigured guidance; error
keeps the last-known render; `status: error` with `balance_infos: null`
renders the neutral unavailable pill (distinct from loading's checking state)
and the panel carries the reason; panel lists breakdown + status +
last-updated; with several `balance_infos`
entries the pill shows the first entry and the panel lists every entry with
its currency; with an EMPTY `balance_infos` the pill renders icon-only colored
by `is_available` (no `Intl.NumberFormat` call, no RangeError); hover
(`mouseenter`) opens and (`mouseleave`) schedules close
via the padding-bridge timer; click toggles; null/empty `workspaceId` renders
nothing and issues no `invokeAction`; `invokeAction` receives
`{ workspaceId }` with no body on the silent interval and
`{ workspaceId, body: { refresh: true } }` on Refresh; a non-2xx
`invokeAction` rejection (mock throws) is treated as a transient error —
last-known render kept, next interval retries — documenting the contract
boundary that the backend always answers 200 with body-encoded errors;
`destroy` clears the silent re-read timer and removes injected styles.

## Risks

- Slot props and `host.api.invokeAction` shape can drift; verify against
  `apps/web/lib/plugins/types.ts` / `host-api.ts` in the monorepo at
  implementation time.
- No second React/Radix runtime may be bundled; use `host.jsx`, `host.ui`, and
  plain elements only.
- i18n: bundle copy follows the reference convention — `kandev-plugin-provider-usage`
  ships hardcoded English copy and never calls `host.i18n` (the monorepo i18n
  ratchet does not cover plugin repos). Hardcoded English is accepted for v1.
  If `host.i18n.t` is used anyway, an `en` catalog must be registered via
  `registry.registerTranslations` first: without a registered catalog `t()`
  falls back to the raw key (`host-api.ts` `defaultValue: options?.defaultValue ?? key`).

## Results

Completed 2026-08-20 (plugin repo commit `ed3ddbc`).

TDD: `test/bundle.test.mjs` (22 tests) written against `ui/bundle.js` (red:
bundle was still the template demo), then the bundle rewritten (green). Host
contracts verified against the monorepo at implementation time:
`PluginActionInput` (`apps/packages/plugin-sdk/src/index.ts`) = `{ workspaceId?,
taskId?, sessionId?, repositoryId?, body?: unknown }` — the forced-refresh flag
travels in `body`; `chat-top-bar` slotProps `{ taskId, taskTitle, workspaceId,
activeSessionId, sessionIds }` (PLUGIN-API.md).

Implementation notes:

- Registers exactly `chat-top-bar`; null/empty `workspaceId` renders nothing
  and issues no `invokeAction`.
- Pill: `Ds` monogram chip on brand-hue (`#4D6BFE`) rounded square + primary
  total via `Intl.NumberFormat` (`narrowSymbol`: ¥/$; compact notation above
  1e6). Colors: calm indigo `#8085e6`, amber `#e0a95e` below server-sent
  `warn_below`, muted coral `#d97b6c` when `is_available` false (coral wins).
  Neutral loading (pulsing) vs unavailable (static) distinction via
  `data-deepseek-state` + injected keyframes. Icon-only (no Intl call, no
  RangeError) for empty `balance_infos`, colored by `is_available`.
- Panel: `position: fixed` below the trigger (`usagePopoverPosition` copied
  from provider-usage), padding-bridge + 260 ms close timer, click/tap toggles
  (nothing hover-only); total/granted/topped-up, every currency entry,
  unavailable status LEADS when `is_available` false, `Updated …` via
  `host.utils.formatRelativeTime`, Refresh control, unconfigured guidance
  (Settings path + `DEEPSEEK_API_KEY`), loading text, error reason + settings
  path. Phone sizing 44 px via `@media (max-width:639px)`.
- Data: 60 s silent re-read with `{ workspaceId }` (no body); Refresh sends
  `{ workspaceId, body: { refresh: true } }`; a rejected `invokeAction` is
  transient (last render kept, next interval retries); `destroy` clears the
  interval and removes injected styles.
- Hardcoded English copy per the reference convention (plugin repos are not
  covered by the monorepo i18n ratchet).

Exact verification (plugin worktree):

```sh
node --check ui/bundle.js          # ok
node --test test/bundle.test.mjs   # 22/22 pass
make test-backend                  # ok — go test ./server/...
make vet                           # ok
gofmt -l .                         # no output
make test                          # ok — backend + bundle
```

Case coverage: exactly one slot (`chat-top-bar`); desktop/phone style
geometry; pill balance formatting; amber below warn_below; coral when
unavailable; coral-wins-over-amber; neutral loading before the first snapshot;
unconfigured guidance; loading panel text; error-no-snapshot neutral pill +
reason; error keeps last-known render; panel breakdown/status/last-updated;
multi-currency (pill first entry, panel every entry); EMPTY balance_infos
icon-only; hover open + mouseleave close timer + padding-bridge cancel; click
toggle; null/empty workspaceId no-render no-fetch; invokeAction arg shape
(silent no-body, Refresh body, open-load plain); transient rejection keeps
render and retries; destroy cleanup; formatBalance (¥/$/compact); popover
position + clamping.
