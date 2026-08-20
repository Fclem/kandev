---
id: "03-backend-balance-client"
title: "DeepSeek balance client"
status: pending
wave: 1
depends_on: ["02-plugin-repository-bootstrap"]
plan: "plan.md"
spec: "../../specs/deepseek-credits-plugin/spec.md"
---

# Task 03: DeepSeek balance client

## Intent

Implement `server/balance.go`: a small DeepSeek API client for
`GET /user/balance` with a strict error taxonomy, fully covered by
`httptest`-based unit tests. No plugin wiring yet.

## Owned paths

- `server/balance.go`
- `server/balance_test.go`

## Dependencies

Task 02 (repository skeleton).

## Acceptance

1. `fetchBalance(ctx, apiKey)` calls `GET {base}/user/balance` with header
   `Authorization: Bearer <apiKey>`; `base` is injectable for tests and
   defaults to `https://api.deepseek.com`; the request has a 10 s timeout and a
   1 MiB response-size cap.
2. A 200 response parses into `Balance{ IsAvailable bool; BalanceInfos
   []BalanceInfo }` with `BalanceInfo{ Currency, TotalBalance,
   GrantedBalance, ToppedUpBalance string }`, matching the documented payload
   (`total_balance`, `granted_balance`, `topped_up_balance` are strings). The
   slice preserves DeepSeek's response order: the spec's primary currency is
   defined as the first entry, and DeepSeek does not document ordering (the
   spec records this as an accepted assumption).
3. Failures classify into typed codes the plugin will surface verbatim:
   `invalid_key` (401), `insufficient_balance` (402 — DeepSeek's documented
   "Insufficient Balance" error), `rate_limited` (429), `timeout`, `network`
   (DNS / refused / reset), `http` (other non-2xx), `bad_response` (malformed JSON,
   wrong shape — including a missing `is_available`, a `currency` outside
   the documented `{CNY, USD}` enum, or a `total_balance` /
   `granted_balance` / `topped_up_balance` that does not parse as a finite
   decimal — or oversized body). The three balance fields are documented
   only as strings, so the client must validate the numbers itself: a value
   like `"abc"` passes the string check but would render as `¥NaN` and
   silently disable the amber threshold (NaN comparisons are false), so it
   is rejected. An EMPTY `balance_infos` array is NOT a failure: it is a
   valid snapshot (`is_available` honored; the UI renders the unavailable
   state with no currency, per the spec). No error string ever contains the
   API key or the `Authorization` header.

## Verification

```sh
make test-backend
make vet
```

`server/balance_test.go` cases: golden payload; 401; 402 (classified as
`insufficient_balance`, not generic `http`); 429; 500; malformed body;
wrong shape (missing `is_available`; `currency` outside `{CNY, USD}`, e.g.
`EUR`, rejected as `bad_response`; non-numeric `total_balance` e.g. `"abc"`
rejected as `bad_response`); EMPTY `balance_infos` array accepted as a
valid snapshot with `is_available` honored (both true and false); oversized
body (over the cap); timeout; Bearer header assertion; error-message
redaction assertion.

## Risks

- DeepSeek API drift: the golden fixture mirrors the official docs; if a live
  response differs, update the fixture with captured evidence and note it.
- Keep the client free of plugin/global state so task 04 can call it from the
  poller without refactoring.
