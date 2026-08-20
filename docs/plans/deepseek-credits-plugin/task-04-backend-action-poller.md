---
id: "04-backend-action-poller"
title: "balance.get action and snapshot poller"
status: done
wave: 2
depends_on: ["03-backend-balance-client"]
plan: "plan.md"
spec: "../../specs/deepseek-credits-plugin/spec.md"
---

# Task 04: balance.get action and snapshot poller

## Intent

Implement the plugin backend: `pluginsdk.Serve` entrypoint, the plugin struct
with a warm-snapshot poller, and the authenticated `balance.get` action with
the spec's exact response contract. Replace the template's example
webhook/event/state scaffolding in `server/`.

## Owned paths

- `server/main.go` (rewrite: `pluginsdk.Serve(newPlugin())`)
- `server/plugin.go` (replace template demo with the real plugin)
- `server/plugin_test.go` (replace template tests)
- `server/balance.go` unchanged from task 03

## Dependencies

Task 03 (balance client).

## Acceptance

1. The plugin embeds `pluginsdk.UnimplementedPlugin`; `SetHost` starts a
   background poller that fetches immediately, then every `poll_minutes`
   (default 5, floor 1). The poller is serialized (singleflight-style `maxAge`
   dedupe) and never evicts the last successful snapshot on failure. Mirror
   `kandev-plugin-provider-usage`'s `pollOnce`/`snapshotForRead` mechanics.
2. `HandleAction` serves only action key `balance.get`; the workspace scope is
   host-verified (`VerifiedActionContext.WorkspaceID`); the bounded request
   body (the action envelope's `body`, JSON `{ "refresh": bool }`) forces a
   DeepSeek round trip when `refresh: true`, otherwise the current state is
   served (the warm snapshot when one exists, otherwise the cached error
   state — e.g. a failed initial poll with `balance_infos: null`). The body
   is attacker-controlled (any authenticated visible-
   workspace user can POST up to 1024 arbitrary bytes): an unparseable body
   or a non-boolean `refresh` is treated as `refresh: false` — no panic, no
   non-200, no invented error code. The plugin returns HTTP **200** for every
   body `status` — domain failures are body-encoded, never HTTP statuses (the
   host forwards plugin statuses verbatim and the UI's `fetchJson` throws on
   non-2xx, which would discard the body). Response body exactly matches the
   spec's action contract:
   `status: "ok" | "loading" | "unconfigured" | "error"` (`loading` = key
   configured, initial poll pending or in flight, no snapshot yet; the pending
   flag is set synchronously in `SetHost` before the poller goroutine starts,
   so the window has a defined start and there is no gap in the loading
   classification from the moment `SetHost` runs onward (before `SetHost`,
   with no pending flag and no snapshot, the action is ALSO `loading`); a
   failed initial poll transitions to `error` with
   `balance_infos: null` so a bad key surfaces its reason even with no prior
   snapshot). `HandleAction` returns `loading` IMMEDIATELY (non-blocking)
   when the INITIAL poll is pending or in flight (or before `SetHost`), even
   for `refresh: true` — this deliberately deviates from provider-usage's
   blocking `snapshotForRead` (synchronous first build); the non-blocking
   branch is what makes `loading` observable at all. This scoping matters:
   after a FAILED initial poll the periodic retry also runs with no snapshot,
   but it is no longer the initial poll, so a `refresh: true` arriving during
   that retry JOINS it (per the join rule below) and returns the retry's
   outcome rather than `loading`. A `refresh` that arrives while a poll is in flight JOINS that poll
   (singleflight, no second DeepSeek round trip), so an action spans at most
   one 10 s client fetch — under the host's 15 s action deadline (a host
   504/timeout response is possible and the UI keeps the last-known render).
   The immediate `loading` branch applies to `refresh: true` as well: a
   refresh arriving while the initial poll is pending or in flight also
   returns `status: loading` immediately with no new round trip (the
   in-flight initial poll continues; the action does NOT join it — distinct
   from an in-flight join, which waits for the joined fetch and returns its
   outcome: fresh data on success, the error state on failure).
   Anti-amplification: a
   `refresh: true` arriving within 5 s of the last COMPLETED FETCH ATTEMPT
   — successful OR failed; the attempt time is recorded separately from
   `fetched_at` (successes only) — serves the CURRENT state (the warm
   snapshot when one exists, otherwise the cached error state, e.g. a failed
   initial poll with `balance_infos: null`) without a new DeepSeek round
   trip (the body is attacker-controlled; sequential refreshes would burn the
   operator's rate quota — singleflight dedupes only concurrent fetches —
   and a success-only anchor would leave the cooldown never satisfied during
   persistent error states),
   `error: { code, message } | null` (secret-free, machine-readable codes from
   task 03), `fetched_at` (RFC 3339 of last successful fetch, else null),
   `is_available` (`boolean` with a snapshot, `null` without), `balance_infos`,
   and `warn_below` (operator threshold as float, default 10, `null` when
   `status: unconfigured` OR in the pre-`SetHost` loading window — the Host
   RPC is unavailable there, so the override is unreadable; the UI needs the
   threshold to color the pill and has no other config read path).
3. Config precedence: `api_key` settings secret → `DEEPSEEK_API_KEY` env →
   `status: "unconfigured"`. An EMPTY or WHITESPACE-ONLY stored `api_key`
   (trimmed) counts as unset (falls through to `DEEPSEEK_API_KEY`; no
   empty-Bearer fetch — a direct PATCH `{"api_key": ""}` or `{"api_key":
   "   "}` can store such a value — the settings form drops only `""`, so
   the trim-based presence-vs-emptiness read rule is pinned here). The key
   is TrimSpace'd ONCE, and the TRIMMED value is BOTH the emptiness-check
   input AND the Bearer credential: a real key with stray surrounding
   whitespace authenticates as the trimmed value, never as a whitespace-
   padded raw value that would 401-loop. The same NON-EMPTY rule
   applies to the env source: a `DEEPSEEK_API_KEY` set to an empty or
   whitespace-only string counts as unset too (a dotenv-template mistake
   must never produce an empty-Bearer fetch). While no key is
   configured the poller is a no-op: it performs NO DeepSeek round trip (no
   empty-key fetch, no 401 noise in logs) and the action serves
   `status: unconfigured` until a key exists. The unconfigured → configured transition is the HOST's
   config-save restart: saving a key restarts the plugin (fresh `SetHost`),
   so the pending flag + immediate fetch apply and no mid-run key-detection
   mechanism is needed — the plugin never has to observe the key changing
   within one process lifetime. `warn_below` (default 10) and `poll_minutes` (default 5, floor 1)
   parse per the config contract. The plugin owns the `warn_below` default:
   `GetConfig` returns an empty map when the operator never opened Settings,
   so the plugin applies `10` itself and the response carries the effective
   numeric value whenever a key is configured and `SetHost` has run; `null`
   when `status: unconfigured` OR in the pre-`SetHost` loading window (Host
   RPC unavailable, override unreadable); the UI never falls back to its own
   threshold.
4. The API key is never logged, never included in responses, and never placed
   in child-process environments (the plugin spawns no processes).

## Verification

```sh
make test-backend
make vet
make build
```

`server/plugin_test.go` uses an injected fake client (no network): action
response shape for ok/loading/unconfigured/error; `loading` from the
synchronous `SetHost` pending flag through the in-flight initial poll (no
gap window); a failed initial poll returns `error` with `balance_infos: null`
and the machine-readable code; `warn_below` present in the response with the
plugin-applied 10 default when the operator never set it (and `null` when
unconfigured); `is_available` true/false/null; refresh in the action body
forces a rebuild while a plain call serves the cache; a refresh arriving
during an in-flight poll joins it (single DeepSeek round trip, bounded under
the 15 s host deadline); a `refresh: true` arriving during a POST-FAILURE
periodic retry (no snapshot, initial poll already failed) JOINS the retry and
returns its outcome — error with the machine-readable code when the retry
fails, `ok` when it recovers — NEVER `loading`, zero extra DeepSeek round
trips; a refresh within 5 s of the last completed fetch
ATTEMPT serves the CURRENT state (the warm snapshot when one exists,
otherwise the cached error state) with ZERO new DeepSeek round trips
(cooldown), including when the last attempt FAILED (invalid key / 429 storm —
the cooldown anchors on attempts, not successes); PRECEDENCE: when a refresh
lands inside the cooldown while another poll is in flight, the cooldown wins
(serve current state immediately, `fetched_at` unchanged — no join, no new
round trip); poll failure after a success retains the last
snapshot; config precedence NON-EMPTY secret > env > unconfigured; the
poller performs ZERO DeepSeek fetches while unconfigured (fake client call
counter stays 0, action serves `unconfigured`, no 401 noise); an EMPTY-STRING
stored `api_key` (direct PATCH `{"api_key": ""}`) falls through to
`DEEPSEEK_API_KEY` and, with no env var, serves `status: unconfigured` with
zero fetches and NO empty-Bearer request; a WHITESPACE-ONLY stored `api_key`
(direct PATCH `{"api_key": "   "}`) likewise counts as unset — zero fetches,
no empty-Bearer request, same as the empty-string case; an empty or
whitespace-only `DEEPSEEK_API_KEY` also counts as unset (zero fetches, no
empty-Bearer request); a key with stray surrounding whitespace (`{"api_key":
" sk-abc "}` or env `DEEPSEEK_API_KEY=" sk-abc "`) authenticates with the
TRIMMED value — the fake client asserts the Authorization header is exactly
`Bearer sk-abc`, not the whitespace-padded raw value; `poll_minutes` floor;
`warn_below` parsing (a configured value <= 0 is treated as the 10 default);
unknown action keys rejected; malformed action bodies (`{"refresh":"yes"}`
and non-JSON garbage) return HTTP 200 with a valid body status, treated as
`refresh: false`, with no panic; a `refresh: true` call
arriving while the INITIAL poll is pending/in flight returns `status:
loading` immediately with ZERO extra DeepSeek round trips (distinct from an
in-flight join, which waits for the joined fetch and returns its outcome:
fresh data on success, the error state on failure).
`HandleAction` invoked BEFORE `SetHost` (poller not started, no pending flag,
nil Host) also returns `status: loading` with `warn_below: null` — the
no-gap invariant holds from the moment SetHost runs, and the pre-SetHost
window is loading, never unconfigured/error/panic; a loading response AFTER
`SetHost` has run (pending flag set, key configured, initial poll in flight)
carries the EFFECTIVE `warn_below` (10 default) — null applies ONLY to
unconfigured and pre-SetHost loading.

## Risks

- GetConfig returns the plugin's own secret in cleartext inside the process —
  never marshal the full config object into logs or responses.
- Keep the poller goroutine bounded to the plugin process lifetime; it must not
  outlive `SetHost` teardown expectations (process exit reaps it, as in
  provider-usage).

## Results

Completed 2026-08-20 (plugin repo commit `74c5877`).

TDD: `server/plugin_test.go` (34 tests) written first (red: symbols
undefined), then `server/plugin.go` + `server/main.go` rewritten (green).

Implementation notes:

- `SetHost` sets the initial-loading pending flag synchronously BEFORE the
  poller goroutine starts (no gap in the loading classification); poller
  fetches immediately then every `poll_minutes` (floor 1, enforced at runtime
  because `minimum: 1` is declarative only).
- Singleflight join: `startFetch` starts one fetch or JOINS the in-flight one;
  completion is broadcast via a closed `done` channel (an earlier single-value
  channel design deadlocked the joined waiter — the poll starter consumed the
  only value — caught by the suite, redesigned so every waiter reads the
  stored outcome).
- Cooldown checked FIRST in the refresh path (5 s anchored on last COMPLETED
  attempt, success or failure) and wins over an in-flight join.
- HTTP 200 for every domain status; unknown action keys rejected with 404;
  malformed/non-boolean bodies treated as `refresh: false` (no panic).
- Config precedence NON-EMPTY trimmed secret > NON-EMPTY trimmed
  `DEEPSEEK_API_KEY` > unconfigured; zero-fetch unconfigured poller.
- `warn_below` effective value (10 default; <= 0 → 10) present whenever a key
  is configured and SetHost ran; null only for unconfigured and pre-SetHost
  loading.
- `server/balance.go` unchanged from task 03 (kept client free of plugin
  state; removed the unused `errBalanceCode` helper).

Exact verification (plugin worktree):

```sh
make test-backend   # ok — go test ./server/... (all 34 plugin + 12 client tests pass)
make vet            # ok
make build          # ok — bin/kandev-deepseek-credits
gofmt -l .          # no output (after gofmt -w)
go test -race -count=1 ./server/...   # ok — race-clean
```

Case coverage: status windows (ok/loading/unconfigured/error), pre-SetHost
loading with `warn_below: null`, no-gap loading from the synchronous pending
flag, failed initial poll → error with `balance_infos: null`, poll failure
after success retains the snapshot, config precedence + whitespace trimming
(empty/whitespace stored key and env count as unset; stray-whitespace keys
authenticate trimmed), refresh rebuild vs plain-cache, cooldown (incl.
post-FAILURE cooldown serving the cached error state), join in-flight poll,
join post-failure retry (recovery ok / failure error, never loading),
cooldown-wins-over-join precedence, warn_below defaults/parsing,
poll_minutes floor, is_available bool/null, fetched_at RFC 3339, unknown key
rejection, malformed bodies, refresh with extra keys, plain call during
in-flight retry. Response shape dump confirmed exact (status/error/
fetched_at/is_available/balance_infos/warn_below).
