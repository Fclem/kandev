---
status: draft
created: 2026-08-19
owner: kandev
---

# DeepSeek Credits Plugin

## Why

Users running DeepSeek models through Kandev have no in-product visibility into
their remaining API balance; running out mid-session is a silent failure. The
official `kandev-plugin-provider-usage` proves the presentation: a pill in the
session top bar whose hover/tap panel carries the detail. This plugin applies
that same surface to the balance DeepSeek's own API exposes.

## What

- Kandev ships the official `kandev-deepseek-credits` plugin from its dedicated
  public repository `kdlbs/kandev-plugin-deepseek-credits`, built from the
  official plugin template. The manifest pins `min_kandev_version: "0.88.0"`,
  the first released host version carrying authenticated plugin actions
  (PR #2117), and the initial release follows the current unsigned marketplace
  trust contract like the other analytics plugins: generated internal
  `checksums.txt`, no cryptographic publisher-provenance claim.
- The plugin reads the account's remaining balance from DeepSeek's
  `GET /user/balance` endpoint using the operator's DeepSeek API key. The key
  comes from the plugin's settings (`api_key`, a `secret: true` config field,
  stored in Kandev's encrypted vault) or, when unset, from the
  `DEEPSEEK_API_KEY` environment variable the plugin subprocess inherits from
  Kandev. The settings secret wins; an EMPTY or WHITESPACE-ONLY `api_key`
  (trimmed) is treated as unset and falls through to `DEEPSEEK_API_KEY` (no
  empty-Bearer fetch, no spurious 401 — the settings form drops only `""`,
  so a stray-space paste survives serialization and the trim rule is
  load-bearing). The key is TrimSpace'd once, and the TRIMMED value is BOTH
  the emptiness-check input AND the Bearer credential: a real key with
  stray surrounding whitespace (e.g. `" sk-abc "`, the common copy-paste
  artifact with a trailing space/newline) authenticates as `Bearer sk-abc`,
  never as a whitespace-padded raw value that would 401-loop. The same
  NON-EMPTY rule applies to the env source: a
  `DEEPSEEK_API_KEY` set to an empty or whitespace-only string counts as
  unset too, so a dotenv-template mistake never produces an empty-Bearer
  fetch — and the same trim-once rule applies: a non-empty
  `DEEPSEEK_API_KEY` authenticates as the TRIMMED value
  (`DEEPSEEK_API_KEY=" sk-abc "` sends `Bearer sk-abc`, never the
  whitespace-padded raw value). The STORED key value never appears in any
  action response, plugin log, host API response, or request after save
  (masked as `********`, vault reference in the config file); the
  operator-entered value exists only transiently in the settings form and its
  single save request (the env-var source never touches the browser at all).
- A pill renders in the `chat-top-bar` plugin slot (the session top bar, beside
  the CPU/DB metrics): a DeepSeek monogram chip plus the formatted total
  balance of the primary currency, e.g. `¥104.32` or `$5.20`. The primary
  currency is the first entry of `balance_infos`, preserving the order DeepSeek
  returns; DeepSeek does not document that ordering, so a multi-currency
  account may see the primary vary between responses (accepted assumption —
  single-currency accounts are the norm, and the panel always lists every
  entry). The pill is calm indigo by default; it turns amber when the primary
  total is below the configured `warn_below` threshold (default `10`, in
  primary-currency units); it turns muted coral while DeepSeek reports
  `is_available: false`. When both apply (low balance and unavailable), the
  coral unavailable state wins over amber.
- Hovering the pill (desktop) or clicking/tapping it (all surfaces) opens a
  panel anchored below the trigger: total balance of the primary currency,
  the granted/topped-up breakdown, every currency entry when the account has
  several, the `is_available` status line, the last-updated time, and a
  **Refresh** control that forces the backend to re-fetch (subject to the 5 s
  anti-amplification cooldown: a Refresh within 5 s of the last completed
  fetch attempt serves the current state — the warm snapshot when one
  exists, otherwise the cached error state). Nothing required is
  hover-only: the click/tap toggle opens the same panel, with touch targets of
  at least 44 px on phones.
- The pill and panel consume one authenticated plugin action,
  `balance.get` with `scope: workspace`, invoked via
  `host.api.invokeAction("balance.get", { workspaceId, body: { refresh } })`;
  `chat-top-bar` slot props carry `workspaceId`. A forced refresh travels
  in the bounded action `body` (`{ "refresh": true }`), because the host
  action envelope carries only the resource selectors plus `body`
  (`ActionInput` has no free-form keys). When the slot reports no workspace
  (`workspaceId` is null/empty — the host type is `string | null`), the
  component renders nothing and issues no fetch: the action is
  workspace-scoped and there is no workspace to authorize, so no rejected-
  request (400) retry loop occurs. The plugin declares no webhooks, so no
  balance data is reachable over an unauthenticated route.
- The backend keeps a warm in-memory snapshot. A background poller refreshes it
  every `poll_minutes` (default `5`, minimum `1`); the UI silently re-reads the
  snapshot every 60 s so an open panel keeps up without forcing a DeepSeek
  round trip; the panel's **Refresh** requests a forced rebuild
  (`refresh: true`).
- When no API key is configured, the pill shows a neutral unavailable
  indicator and the panel explains that the key must be set in
  Settings → Plugins → DeepSeek Credits or via `DEEPSEEK_API_KEY`, with no
  other failure noise.
- When a fetch fails after a successful one, the panel shows the reason but the
  pill and panel keep rendering the last-known balance; the next poll retries
  and recovery is automatic. A transient failure never blanks the pill.
- The API key is never logged, never returned in any action response, and never
  embedded in URLs or child-process environments (the plugin spawns no child
  processes). Logs and returned errors redact the `Authorization` header.
- Disabling the plugin removes the pill from the top bar and stops all polling;
  re-enabling restarts it. Upgrade and reinstall require no state migration
  because the plugin keeps no durable state beyond the vault-stored key.

## API surface

### DeepSeek balance endpoint

`GET {apiBase}/user/balance` (apiBase is `https://api.deepseek.com`), header
`Authorization: Bearer <api-key>`. Documented response:

```json
{
  "is_available": true,
  "balance_infos": [
    {
      "currency": "CNY",
      "total_balance": "110.00",
      "granted_balance": "10.00",
      "topped_up_balance": "100.00"
    }
  ]
}
```

`currency` is `CNY` or `USD`; the three balance fields are strings in that
currency. `total_balance` is the total available balance, documented as
including the granted and topped-up components (the docs do not state an
arithmetic identity, and the client does not validate the sum). The client
enforces a 10 s timeout and a 1 MiB response-size cap; `401` means the key
was rejected, `429` a rate limit, `402` "Insufficient Balance" (documented in
DeepSeek's error-code table; if the balance endpoint ever returns it, the
plugin surfaces `error.code: insufficient_balance` rather than folding it
into generic `http`), other 4xx/5xx upstream failures.

### Manifest contract

```yaml
id: "kandev-deepseek-credits"
display_name: "DeepSeek Credits"
description: "DeepSeek API balance and remaining credits in the session top bar."
author: "kandev"
repo_url: "https://github.com/kdlbs/kandev-plugin-deepseek-credits"
api_version: 1
version: "0.1.0"
min_kandev_version: "0.88.0"
categories: ["analytics"]
runtime:
  type: binary
  executables:
    linux-amd64: "server/plugin-linux-amd64"
    linux-arm64: "server/plugin-linux-arm64"
    darwin-amd64: "server/plugin-darwin-amd64"
    darwin-arm64: "server/plugin-darwin-arm64"
    windows-amd64: "server/plugin-windows-amd64.exe"
actions:
  - key: balance.get
    scope: workspace
    max_body_bytes: 1024
ui:
  bundle: "/ui/bundle.js"
config_schema:
  type: object
  properties:
    api_key:
      type: string
      format: password
      secret: true
      title: "DeepSeek API key"
      description: "DeepSeek API key used to read the account balance. Leave empty or blank to fall back to the DEEPSEEK_API_KEY environment variable."
    poll_minutes:
      type: number
      default: 5
      minimum: 1
      title: "Refresh interval (min)"
      description: "How often the balance refreshes in the background. Default 5, minimum 1."
    warn_below:
      type: number
      default: 10
      title: "Warn threshold"
      description: "The pill turns amber when the primary-currency total balance falls below this amount. Default 10."
```

Every config property carries `title` (+ `description`): the Settings form
labels fields with `title ?? key`, and the reference plugin titles every
field; raw snake_case keys are not acceptable user-facing labels.

No `capabilities` (no events, state, or `api_read` usage) and no `webhooks`.

### Action payload

Request body is bounded JSON `{ "refresh": boolean }` (omitted = cached
snapshot), delivered inside the action envelope's `body`:
`host.api.invokeAction("balance.get", { workspaceId, body: { refresh: true } })`.
The body is attacker-controlled (any authenticated visible-workspace user can
POST up to 1024 arbitrary bytes). The HOST rejects a non-JSON envelope body
with `400` ("invalid plugin action payload") and a body over the declared
`max_body_bytes` with `413` BEFORE the plugin is invoked — those bytes never
reach the plugin. What reaches `HandleAction` is always valid JSON up to the
cap; a valid-JSON body that is not the expected shape (e.g. a non-boolean
`refresh`), or raw bytes injected directly at unit level, is treated as
`refresh: false` (the current state served — the warm snapshot when one
exists, otherwise the cached error state — or `loading`/`unconfigured` per
the window rules) — the plugin never panics, returns a non-200, or invents a
new error code for a bad body. A `refresh: true` arriving within a small
cooldown (5 s) of the last COMPLETED FETCH ATTEMPT — successful OR failed;
the attempt time is recorded separately from `fetched_at`, which tracks only
successes — serves the CURRENT state (the warm snapshot when one exists,
otherwise the cached error state) without a new DeepSeek round trip. The
body is attacker-controlled and sequential refreshes would otherwise burn the
operator's DeepSeek rate quota (singleflight dedupes only concurrent
fetches); anchoring on ATTEMPTS matters because in a persistent error state
(invalid key, 429 storm, network blackout) there is no successful fetch, and
a success-only anchor would leave the cooldown never satisfied and the
amplification unbounded. The cooldown rarely affects the UI's Refresh in
practice (human click rate is far below 1/5 s); a Refresh clicked inside the
window still serves the current state without a round trip. Precedence when
both rules could fire (a
periodic poll starting within 5 s of a completed attempt): the COOLDOWN is
checked first — within 5 s of the last completed attempt the action serves
the current state immediately, even if another poll is in flight; otherwise
a refresh joins an in-flight poll. The host authenticates the caller, verifies
`workspaceId`, and delivers `VerifiedActionContext{WorkspaceID}` to the
plugin. The plugin returns HTTP
**200** for every `status` value below — domain failures (invalid key, rate
limit, network, unconfigured) are encoded in the BODY, never in the HTTP
status. This is load-bearing: the host forwards plugin statuses verbatim and
the UI's `fetchJson` throws on non-2xx, so a non-200 would discard the body
and degrade the invalid-key/rate-limit surfaces to the transient path.
Response body:

```json
{
  "status": "ok" | "loading" | "unconfigured" | "error",
  "error": { "code": "invalid_key" | "insufficient_balance" | "rate_limited" | "timeout" | "network" | "http" | "bad_response", "message": "..." } | null,
  "fetched_at": "2026-08-19T12:00:00Z" | null,
  "is_available": true | false | null,
  "balance_infos": [
    { "currency": "CNY", "total_balance": "110.00", "granted_balance": "10.00", "topped_up_balance": "100.00" }
  ] | null,
  "warn_below": 10 | null
}
```

`status: ok` carries a snapshot from a successful fetch (possibly stale);
`loading` is the window before the first successful fetch while the initial
poll is pending or in flight — the pending flag is set synchronously in
`SetHost` before the poller goroutine starts, so the flag defines the start of
the window from the moment `SetHost` runs and there is no gap in the loading
classification from that moment onward. `SetHost` is called from a
background goroutine after the plugin handshake, so an action may arrive
before it: a not-yet-started poller (no pending flag, no snapshot) is also
treated as `status: loading`; `unconfigured`
carries no balance data; `error` carries the last successful snapshot when one
exists — and `balance_infos: null` when the first fetch never succeeded — plus
a machine-readable `error.code` and a human-readable, secret-free
`error.message`. A failed initial poll transitions `loading` to `error`, so an
invalid key surfaces its 401 reason even with no prior snapshot. `fetched_at`
is the time of the last successful fetch, `null` until the first success.
`warn_below` is the effective amber threshold (default `10`, in
primary-currency units) so the UI can color the pill. The plugin owns the
default: `GetConfig` returns an empty map when the operator never opened
Settings, so the plugin applies the `10` default itself and the response
carries the effective numeric value whenever `status` is not `unconfigured`
AND `SetHost` has run (config readable); `warn_below` is `null` when
`status: unconfigured` OR in the pre-`SetHost` loading window (the Host RPC
is not yet available, so the operator's override is unreadable and the plugin
cannot truthfully report it — the UI ignores `warn_below` while loading, so
no user-visible regression). In every other state the effective value is
present. A configured `warn_below` of zero or less is treated as the `10`
default (matching the reference plugin's positive-only rule; a non-positive
threshold would never trigger amber). The UI never falls back to its own
threshold. `is_available` is `boolean` when a snapshot exists, `null`
otherwise.

## Permissions

The host authenticates every action call and verifies the caller has
visibility into the claimed workspace — in the current host model that is the
workspace owner (`OwnerID == userID`, or any user for ownerless workspaces);
other authenticated callers receive `404`, so workspace visibility, not a
membership concept, gates `balance.get`. The balance is operator-level: the
DeepSeek API key and therefore the reported balance are identical for every
user and workspace of this Kandev install. The plugin does not receive
per-user identity beyond `ActorID` and never renders user-specific data, so
this visibility model is a documented property, not a per-user privacy
boundary.

## Failure modes

- **No key configured** → `status: unconfigured`; the poller performs no
  DeepSeek round trip (no empty-key fetch or 401 noise); the panel names the
  two ways to provide a key (settings secret, `DEEPSEEK_API_KEY`). The pill
  renders its neutral unavailable indicator — but note the pre-`SetHost`
  window is `loading` for EVERY install (keyed or not, since config is not
  yet readable), so an unconfigured install settles to the unavailable
  indicator only once `SetHost` runs and the poller resolves
  `unconfigured`. Saving the key in Settings restarts the plugin (host
  config-save behavior), which re-runs `SetHost`: the pending flag +
  immediate fetch apply, so the transition to a balance happens without
  manual Refresh.
- **Key configured, no snapshot yet** → `status: loading` from `SetHost`
  through the initial poll's completion — and also in the brief pre-`SetHost`
  window, where config is not yet readable and the action is likewise
  `loading`; the pill shows a neutral checking indicator and the panel a
  loading state, never a fabricated balance. A failed initial poll
  transitions to `status: error` with `balance_infos: null` and the
  machine-readable code (e.g. `invalid_key`), so a bad key from the start
  still surfaces its reason; the next poll or an explicit Refresh retries. In
  that error-with-no-snapshot state the PILL shows the same neutral
  unavailable indicator as unconfigured (distinct from loading's checking
  state); the panel carries the reason and settings path.
- **Key rejected (`401`)** → `error.code: invalid_key`; the panel shows the
  reason and the settings path; the poller retries on the next interval, and
  a corrected key takes effect on the next config-save restart (host
  behavior — the plugin never re-reads config mid-run), so recovery is
  automatic without manual Refresh.
- **Rate limited (`429`)** → `error.code: rate_limited`; retry on the next
  interval; the panel keeps the last-known snapshot.
- **Timeout / network failure / upstream 5xx** → `error.code: timeout |
  network | http`; the snapshot is retained and re-rendered; the next poll
  retries with no backoff growth beyond the fixed interval.
- **Malformed or oversized response** → treated as `bad_response`; the response
  is discarded and the previous snapshot retained. An EMPTY `balance_infos`
  array is NOT `bad_response`: it is a valid snapshot for accounts with no
  balance data (schema-legal `object[]`; empty arrays are assumed common for
  never-funded accounts, though the docs do not state a frequency). The pill
  renders icon-only in the `is_available`-derived color
  (coral when false) with no currency amount, and the panel shows the
  unavailable status without amounts — no `Intl.NumberFormat` call is made
  without a primary currency. A response missing `is_available` or otherwise
  violating the documented shape remains `bad_response`.
- **DeepSeek endpoint removed or balance unavailable** → surfaces as `http` /
  `bad_response` in the panel; the plugin never fabricates a balance.
- **Host action deadline** → the action route enforces a 15 s host timeout and
  answers `504` (`{"error":"plugin action timed out"}`) on expiry. A `refresh`
  that arrives while a poll is already in flight JOINS that poll instead of
  starting a second DeepSeek round trip — EXCEPT while the initial poll is
  pending or in flight, where the refresh returns `status: loading`
  immediately with no new round trip (the in-flight initial poll continues;
  the action does NOT join it) — so an action spans at most
  one 10 s client fetch and stays under the host deadline. A `504` or other
  non-contract response is treated by the UI as a transient error: the
  last-known render is kept and the next poll/refresh retries.
- **Host RPC unavailable (`SetHost` never fires)** → the poller never
  starts, no snapshot is produced, and the action remains `status: loading`
  indefinitely (the broker dial is retried in a background goroutine with a
  bounded timeout; `SetHost` fires only on dial success). This is a
  host-runtime failure, not a plugin bug: it is recovered by a plugin
  restart, and the panel's loading state is the observable symptom.
- Poller and UI failures are independent: a UI fetch error never clears the
  pill's last render, and a failed poll never evicts the snapshot.

## Persistence guarantees

The plugin itself keeps no durable state: the snapshot lives only in the
plugin process memory and is re-fetched on every plugin restart (startup fetch
runs immediately). Disable preserves the plugin's vault-stored API key and
config, so re-enabling recovers without re-entering the key. Uninstall purges
the package, the plugin record, the vault namespace (both config-backed and
SetSecret-owned secret entries), plugin user state, and plugin state, so a
reinstall starts clean. Upgrade never needs migration: the snapshot re-fetches
on restart and the vault key survives the version change.

## Scenarios

- **GIVEN** the plugin is installed, enabled, and an API key is configured,
  **WHEN** a session top bar renders, **THEN** the pill shows the DeepSeek
  monogram and the formatted total balance of the primary currency.
- **GIVEN** the pill is rendered, **WHEN** the user hovers it (desktop) or
  clicks/taps it (all surfaces), **THEN** a panel opens below the pill showing
  total, granted, and topped-up balance, the `is_available` status, the
  last-updated time, and a Refresh control.
- **GIVEN** the primary total balance is below the `warn_below` threshold,
  **WHEN** the pill renders, **THEN** the pill is amber.
- **GIVEN** DeepSeek reports `is_available: false`, **WHEN** the pill renders,
  **THEN** the pill is muted coral and the panel leads with the
  insufficient-balance status.
- **GIVEN** the primary total is below `warn_below` AND DeepSeek reports
  `is_available: false`, **WHEN** the pill renders, **THEN** the pill is muted
  coral (coral wins over amber).
- **GIVEN** DeepSeek returns `is_available: false` with an empty
  `balance_infos`, **WHEN** the pill renders, **THEN** the pill is icon-only
  in muted coral (no currency amount formatted) and the panel shows the
  insufficient-balance status without amounts.
- **GIVEN** no API key is configured, **WHEN** the pill renders, **THEN** the
  pill shows a neutral unavailable indicator and the panel explains how to set
  the key (Settings → Plugins → DeepSeek Credits, or `DEEPSEEK_API_KEY`).
- **GIVEN** an API key is configured and the initial poll is in flight (no
  snapshot exists yet), **WHEN** the pill renders, **THEN** the pill shows a
  neutral checking indicator and the panel a loading state, never an error or
  a fabricated balance.
- **GIVEN** the API key is invalid from the start (no prior snapshot), **WHEN**
  the initial poll fails, **THEN** the panel shows the 401 reason
  (`status: error`, `balance_infos: null`) rather than an endless loading
  state.
- **GIVEN** an authenticated user with visibility into a workspace (the owner,
  or any user for an ownerless workspace), **WHEN** they invoke `balance.get`
  for that workspace, **THEN** the host authorizes on workspace visibility and
  the plugin returns the operator-level balance (identical for every user);
  an authenticated non-owner receives `404`.
- **GIVEN** the API key is invalid, **WHEN** the poller or Refresh runs,
  **THEN** the panel shows the 401 reason and the last-known balance remains
  rendered if one exists.
- **GIVEN** a fetch succeeded earlier and a later fetch fails on the network,
  **WHEN** the panel is open, **THEN** the panel shows the failure reason while
  the pill and panel keep rendering the last-known balance.
- **GIVEN** the account reports several `balance_infos` entries, **WHEN** the
  pill and panel render, **THEN** the pill shows the first entry and the panel
  lists every entry with its currency.
- **GIVEN** the user activates Refresh, **WHEN** the action completes,
  **THEN** the backend re-fetches from DeepSeek — or returns `loading`
  immediately while the initial poll is still pending/in flight, with no new
  round trip (the in-flight initial poll continues; the action does not join
  it), or serves the current state without a round trip when the last
  completed fetch attempt is under 5 s old (cooldown — the warm snapshot when
  one exists, otherwise the cached error state) — and the pill and panel
  update with the resulting state.
- **GIVEN** a fetch attempt completed less than 5 s ago (successful or
  failed), **WHEN** a `refresh: true` arrives, **THEN** the action serves the
  CURRENT state — the warm snapshot when one exists, otherwise the cached
  error state (e.g. a failed initial poll with `balance_infos: null`) — with
  zero new DeepSeek round trips (cooldown), regardless of whether the attempt
  succeeded.
- **GIVEN** the plugin is disabled, **WHEN** the session top bar re-renders,
  **THEN** the pill is gone and no further balance requests are issued.
- **GIVEN** the plugin is upgraded to a newer version, **WHEN** it activates,
  **THEN** it re-fetches balance immediately, the vault-stored API key still
  resolves, and no operator state migration is required.
- **GIVEN** the plugin is uninstalled and reinstalled, **WHEN** it activates,
  **THEN** the uninstall purged the vault namespace, so the operator re-enters
  the API key; no other state migration is required.
- **GIVEN** an unauthenticated caller, **WHEN** they call
  `POST /api/plugins/kandev-deepseek-credits/actions/balance.get`, **THEN**
  the host returns `401`; no webhook route exists for this plugin.

## Out of scope

- Marketplace catalog submission and the `plugin-registry/plugins.yaml` PR;
  those follow a valid released tarball and remain a separate maintainer action.
- Notifications or alerts when balance runs low.
- Spending history, invoices, top-up flows, or multi-account management; the
  API key is operator-level.
- Per-workspace or per-user keys.
- Per-model spend tracking (covered by `kandev-plugin-session-cost`).

## Open questions

- Repository ownership: the authenticated GitHub account has no `kdlbs` org
  membership, so creating `kdlbs/kandev-plugin-deepseek-credits` may require a
  maintainer action. The bootstrap task carries the exact creation request and
  a fallback (author-owned public repo, later transfer). This does not block
  the spec, plan, or local implementation.
