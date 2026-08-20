---
spec: docs/specs/deepseek-credits-plugin/spec.md
created: 2026-08-19
status: draft
---

# Implementation Plan: DeepSeek Credits Plugin

## Overview

Build the official `kandev-deepseek-credits` plugin in its dedicated
repository, mirroring the `kandev-plugin-provider-usage` anatomy: a Go backend
(DeepSeek balance client + authenticated `balance.get` action + warm-snapshot
poller) and a no-build `ui/bundle.js` (chat-top-bar pill + hover/tap panel).
Order: bootstrap the plugin repository from the official template, implement
the backend, implement the UI, finalize the manifest/config/package, then prove
the packaged artifact against a disposable dev instance.

The reference implementation is `kdlbs/kandev-plugin-provider-usage` (manifest,
Makefile, `server/` poller + webhook pattern, `ui/bundle.js` top-bar pill and
hover-panel mechanics). The contract difference is deliberate: this plugin uses
the authenticated `actions:` surface (`host.api.invokeAction`), not public
webhooks, so balance data is never exposed over an unauthenticated route.

## Backend

Plugin repository `kandev-plugin-deepseek-credits`; SDK resolved via the
template's `replace github.com/kandev/kandev => ../kandev/apps/backend`
(sibling checkout of this monorepo).

- `server/main.go` — `pluginsdk.Serve(newPlugin())`, mirroring provider-usage.
- `server/balance.go` — DeepSeek balance client. `fetchBalance(ctx, apiKey)`
  calls `GET https://api.deepseek.com/user/balance` with `Authorization: Bearer`
  and a 10 s timeout; enforces a 1 MiB response cap; parses `is_available` +
  `balance_infos` (currency, total/granted/topped-up as strings); classifies
  failures into `invalid_key` (401), `insufficient_balance` (402),
  `rate_limited` (429), `timeout`, `network`, `http` (other non-2xx),
  `bad_response` (malformed/oversized).
  The client takes an injectable base URL for tests.
- `server/plugin.go` — `plugin` struct embedding `pluginsdk.UnimplementedPlugin`;
  `SetHost` starts the poller once; `HandleAction` serves `balance.get`
  (workspace-scoped, body `{ "refresh": bool }`, response JSON per the spec's
  action contract); config reads: `api_key` (secret) → `DEEPSEEK_API_KEY` env
  fallback, `poll_minutes` (default 5, floor 1), `warn_below` (default 10);
  snapshot store with `pollOnce`/`snapshotForRead` semantics copied from
  provider-usage (`pollMu` serialization, `maxAge` dedupe), with TWO
  deliberate deviations: (1) the action returns `status: loading` immediately
  (non-blocking) while the initial poll is pending or in flight — including
  the brief pre-`SetHost` window, where config is not yet readable and the
  action is also `loading` — instead of provider-usage's blocking synchronous
  first build; that branch is what makes `loading` observable; (2) a `refresh`
  that arrives while a poll is in flight JOINS that poll (singleflight)
  instead of provider-usage's `pollOnce(ctx, 0)` refresh, which skips the
  `maxAge > 0` dedupe guard and re-fetches after the lock — EXCEPT while the
  initial poll is pending/in flight, where the refresh returns `status:
  loading` immediately — the join bounds an action to at most one 10 s fetch,
  under the host's 15 s action deadline.
- No `capabilities`, no host state, no data dir, no child processes.

## Frontend

`ui/bundle.js` — hand-written no-build ES module (`window.registerKandevPlugin`),
same pattern as provider-usage:

- Registers only the `chat-top-bar` component. Receives
  `{ taskId, taskTitle, workspaceId, activeSessionId, sessionIds }` slotProps.
- Pill: DeepSeek monogram chip (brand-hue rounded square, e.g. `#4D6BFE` with
  a `Ds` monogram; no hand-drawn whale) + `Intl.NumberFormat` currency
  formatting of the primary currency (`currencyDisplay: "narrowSymbol"` so CNY
  renders `¥`, USD `$`); compact notation when the amount would overflow the
  pill. Colors: calm indigo default, amber below `warn_below` (server-sent
  threshold), muted coral when `is_available` is false.
- Hover/tap panel: `position: fixed` anchored below the trigger rect (copy
  provider-usage's `usagePopoverPosition` + open/close timers + padding bridge
  so the mouse never leaves the hover area); click toggles; touch targets
  ≥ 44 px on phones (`@media (max-width: 639px)` sizing like provider-usage).
  Content: total, granted/topped-up breakdown, every currency entry,
  `is_available` status line, last-updated via `host.utils.formatRelativeTime`,
  Refresh button, and the unconfigured/error guidance states from the spec.
- Data: `host.api.invokeAction("balance.get", { workspaceId, body: { refresh } })`
  — a forced refresh travels in the action `body` (`{ refresh: true }`), because
  the host action envelope (`ActionInput`) carries only workspace/task/session/
  repository selectors plus `body`; the refresh flag must not be passed as a
  top-level input key. A 60 s silent re-read interval serves the cached
  snapshot (no body); `destroy` clears timers and removes injected styles.

## Tests

Plugin-repo conventions (mirror provider-usage; the template's CI runs the same
targets):

- **Balance client** (`server/balance_test.go`): `httptest` server cases —
  golden payload, `401`, `402` (classified `insufficient_balance`), `429`,
  5xx, malformed body, oversized body, timeout; verifies the Bearer header
  is sent and never echoed in errors.
- **Plugin action + poller** (`server/plugin_test.go`): action response shape
  for `ok` / `unconfigured` / `error`; `refresh: true` forces a rebuild while a
  plain call serves the cache; poll failure retains the last snapshot; config
  precedence (NON-EMPTY secret > NON-EMPTY env > unconfigured; an
  empty or whitespace-only stored `api_key` or `DEEPSEEK_API_KEY` counts as
  unset);
  `poll_minutes` floor; `warn_below`
  parsing. Use an injected fake client, no network.
- **UI bundle** (`test/bundle.test.mjs`, `node --test` + `vm`, host mock like
  provider-usage's bundle test): registers exactly the `chat-top-bar` slot;
  pill renders the formatted primary-currency balance; amber below the
  server-sent `warn_below`; coral when unavailable (coral wins when both
  apply); loading state while the initial poll is in flight, with a failed
  initial poll rendering the error state instead; panel lists breakdown +
  status + last-updated;
  unconfigured guidance; error keeps last-known render; click toggles the
  panel; `invokeAction` receives `{ workspaceId }` and no body on the silent
  interval, and `{ workspaceId, body: { refresh: true } }` on Refresh.
- **Manifest contract** (`server/manifest_test.go`, owned by task 06 — plain
  YAML parse of `../manifest.yaml`, since the SDK manifest package is
  `internal` to the kandev module): asserts id/`display_name`/`repo_url`/
  `version` (non-empty SemVer shape, not an exact value — the release
  workflow bumps it and re-runs tests on the tag)/`min_kandev_version:
  0.88.0`/`categories: ["analytics"]`/exactly one action (`balance.get`,
  `scope: workspace`, `max_body_bytes: 1024`)/`config_schema` with `api_key`
  marked `secret: true` + `format: password` (the host's only secret markers —
  dropping them stores the key in cleartext config) and the `poll_minutes` /
  `warn_below` defaults, and the absence of `webhooks`/`capabilities`. This is
  load-bearing because `plugin-pack`/Install validate structure only and the
  base-floor CI job pins the SDK by a hardcoded ref decoupled from the
  manifest declaration, so a floor regression compiles clean and goes
  undetected without the test.

## E2E Tests

Plugin repositories do not run the monorepo Playwright suite; the
`create-kandev-plugin` verification contract is bundle tests plus a
packaged-artifact disposable-instance smoke test. Task 07 covers the
user-facing flow end to end against a real dev host: install the tarball,
enable the plugin, and drive the browser to verify the pill, the hover/tap
panel, Refresh, the unconfigured state, and disable/uninstall cleanup. No
`apps/web/e2e` file is produced; the smoke test is the E2E evidence for this
plugin-only change.

## Implementation Waves And Task Files

Sequential by default; only the user may authorize subagents.

```
Wave 0:
- [x] [task 02 — plugin repository bootstrap](task-02-plugin-repository-bootstrap.md)

Wave 1:
- [x] [task 03 — DeepSeek balance client](task-03-backend-balance-client.md)

Wave 2:
- [x] [task 04 — balance.get action and poller](task-04-backend-action-poller.md)

Wave 3:
- [x] [task 05 — chat-top-bar pill and panel](task-05-ui-pill-panel.md)

Wave 4:
- [x] [task 06 — manifest, config, package](task-06-manifest-config-package.md)

Wave 5:
- [x] [task 07 — disposable-instance smoke test](task-07-instance-smoke-test.md)
```

Task 01 (design package) is complete: this plan, the spec, and the task files.

## Verification Results

Pending. On completion, synchronize this section with each task's `## Results`
(exact commands and outcomes, artifact paths, instance teardown evidence).

## Open Questions

- `kdlbs/kandev-plugin-deepseek-credits` creation needs a maintainer with `kdlbs`
  org rights; fallback is an author-owned public repo created from the template
  with a later transfer request. See task 02.
