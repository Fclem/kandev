---
status: draft
system: office
requirements:
  - REQ-OFFICE-COSTS-001
created: 2026-04-25
updated: 2026-10-14
owners:
  - cfl
---

# Office: Cost Tracking & Budget Management System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-COSTS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COSTS-001` | [Migrated source detail](#migrated-source-detail); [Costs Explorer request windows](#costs-explorer-request-windows) for AC-001.2/.8 |

## Migrated source detail

## Why

Office exposes agent spending and guardrails for unattended work through per-session estimates, agent/project/model aggregates, and budget alerts or hard stops.

**Costs are estimates, not billing.** Usage and public prices can differ from actual charges, particularly flat-fee subscriptions such as Claude Max or Copilot Enterprise. The explorer serves visibility and governance.

## What

- Every agent session generates cost events as it runs.
- Cost events are aggregated on read into multiple views (by agent, project, task, model, time window).
- Budget policies SHALL enforce spending limits per agent, project, or workspace with `notify_only`, `pause_agent`, or `block_new_tasks` actions.
- Cost is resolved per cost event via a two-layer lookup (provider-reported then `models.dev` cache); no static fallback table exists.
- Stale `models.dev` pricing remains readable while one coalesced refresh runs per
  client. Concurrent background lookups and explicit refresh calls do not start
  duplicate network fetches.
- For providers that emit token telemetry on the ACP wire (claude-acp, opencode-acp, gemini, codex-acp), cost events are populated from the `complete` event at end-of-turn.
- For providers without usable wire telemetry (codex per-turn split, amp), a disk-runner subsystem reads on-disk session files via pinned `@ccusage/*` packages and feeds normalized cost events into the same pipeline.
- The cost explorer UI surfaces aggregations and lets the user manage budget policies.

## Data model

### `office_cost_events`

| Field | Type |
| --- | --- |
| `id` | string |
| `session_id` | string |
| `task_id` | string |
| `agent_instance_id` | string |
| `project_id` | string |
| `model` | string |
| `provider` | string |
| `tokens_in` | int64 |
| `tokens_in_missing` | bool |
| `tokens_cached_in` | int64, nullable |
| `tokens_cached_read` | int64, nullable |
| `tokens_cached_write` | int64, nullable |
| `tokens_out` | int64, nullable |
| `cost_subcents` | int64 |
| `estimated` | bool |
| `usage_unavailable` | bool |
| `turn_id` | string, nullable |
| `usage_event_id` | string, nullable |
| `cost_source` | enum, nullable |
| `rate_input_per_million` | int64, nullable |
| `rate_cached_read_per_million` | int64, nullable |
| `rate_cached_write_per_million` | int64, nullable |
| `rate_output_per_million` | int64, nullable |
| `pricing_catalog_version` | string, nullable |
| `cost_contract_version` | int64, nullable |
| `provider_event_id` | string |
| `provider_credits` | int64 |
| `occurred_at` | timestamp |

- `id` is the unique primary key. `session_id` references `task_sessions.id`; `task_id` references `tasks.id`.
- `agent_instance_id` is null for non-office sessions. `project_id` is the write-time project snapshot, null if unassigned at event time.
- `model` identifies the model, for example `claude-sonnet-4-20250514`; `provider` identifies the provider, for example `anthropic` or `openai`.
- `tokens_in` is the input storage count. `tokens_in_missing` makes its placeholder JSON NULL and unavailable in aggregates, independently of output and spend; measured zero stays `0`. `tokens_cached_in` is the checked cache-read/write sum, NULL on overflow or unavailable usage, never wrapped or saturated.
- `tokens_cached_read` and `tokens_cached_write` split `tokens_cached_in`. They are NULL for legacy pre-contract-v2 rows and when no cache data was reported, never an inferred `0`: a missing split is not proof of zero cache usage. The split matters because cache read and write have different per-million rates.
- `tokens_out` is the output token count; NULL means never observed. New normalized prompt usage carries `output_tokens_present` separately from the numeric value. An observed zero is stored as `0`; the adapter_prompt.go context-occupancy fallback stores NULL because it cannot observe output tokens. For legacy events without the presence field, the subscriber retains the old rule: a non-estimated or nonzero value is observed. Contract v3 lets downstream per-output-token measures distinguish these states.
- `cost_subcents` stores hundredths of a cent, renamed from `cost_cents`. The UI divides by 10000.
- `estimated` is true when token counts are not authoritative for a complete turn: synthesized counts such as codex-acp cumulative-delta inference, or a provider frame covering only part of the turn such as Codex's last request. It is independent of `cost_source`.
- `usage_unavailable` marks native response coverage without amounts and defaults false for historical rows. True makes token/cost JSON fields null and keeps budgets degraded through `cost_source=unpriced`; scalar storage slots are placeholders, never measurements.
- `turn_id` is the turn resolved at completion. It is NULL when no turn could be resolved, such as an error/interrupt completion outside the normal ready path.
- `usage_event_id` is the publish-site idempotency key derived from `(session_id, execution_id, prompt_generation)`. A partial unique index on non-NULL values prevents redelivered prompt usage from double-inserting or double-counting rollups. It is NULL for legacy rows. Generation-less transports get a random per-publish key and intentionally are not deduplicated.
- `cost_source` is `provider_reported`, `models_dev_list` or `unpriced`, recording which lookup layer produced `cost_subcents`, independently of `estimated`. It is NULL for legacy rows.
- The four `rate_*_per_million` fields record the applied models.dev rates in subcents per million tokens. They are populated only for `cost_source=models_dev_list` and NULL otherwise, including provider-reported amounts without an applied rate table.
- `pricing_catalog_version` is the models.dev cache's RFC3339 load/fetch-time identifier; the dataset has no version field of its own. It is populated only for `cost_source=models_dev_list`.
- `cost_contract_version` is the in-band column-semantics marker for point-in-time Rill extracts. Current value `4` adds independent input presence; v3 added nullable output/presence and partial-frame estimation; v2 stopped forcing unpriced usage to be estimated. Legacy pre-contract markers remain NULL, never backfilled.
- `provider_event_id` is null for wire rows and non-null for disk-runner rows, formatted `ccusage:<provider>:<session_id>:<model>`. `provider_credits` holds Amp credits and is null elsewhere.
- `occurred_at` is when the cost was incurred.

Agent, run and subtree cached-token aggregates are nullable. If any included event has an unavailable merged count, omit that aggregate rather than present a partial sum or zero as exact. Measured zero remains zero. The existing nullable SQLite column needs no migration for merge overflow.

Agent, recent-run, run-detail and subtree input/output rollups are nullable. An included `usage_unavailable` row withholds both; `tokens_in_missing` withholds only input and NULL output only output. Never filter unknown rows into an apparently complete sum. JSON NULL reaches the existing desktop/phone unavailable marker; measured zero and independently priced spend survive.

Agent, recent-run, run-detail, subtree, workspace summary/dashboard and grouped monetary views exclude unavailable and explicitly unpriced placeholders. Without a recorded monetary measurement their amounts are NULL, not measured zero. Historical rows with NULL `cost_source` retain recorded amounts; independently recorded spend remains visible alongside coverage gaps. Desktop and phone views render unavailable amounts as `-` and explicitly measured zero as `$0.00`.

Workspace cost summary, composed breakdown, group rows and task-tree summaries expose `coverage_degraded`; the dashboard exposes `month_spend_coverage_degraded`. Unavailable, explicitly unpriced or NULL-cost events make coverage incomplete. Mixed groups retain known spend and show a translated incomplete-coverage notice rather than imply complete pricing. Empty workspaces and task trees have NULL monetary amounts without degraded coverage. Task-tree projections include active descendants and exclude archived descendants and unrelated tasks. These read projections do not change budget arithmetic, pricing, storage or spend-window degradation.

Task-tree counts use the shared safe-integer formatter: invalid or unsafe browser numbers are omitted rather than formatted as exact counts, while measured zero and the safe boundary remain visible.

Disk-runner rows are upserted by `(session_id, provider_event_id)`. For codex, after a successful aggregate row lands, the wire-side `estimated=true` rows for the same session are deleted to avoid double-counting.

Project cost attribution uses the current task assignment:

- `office_cost_events.project_id` is a write-time snapshot retained for cleanup and historical inspection.
- `tasks.project_id` is the current source of truth for project cost views and project budget totals.

**Contract v2 columns** (`tokens_cached_read`, `tokens_cached_write`, `turn_id`, `usage_event_id`, `cost_source`, the four `rate_*_per_million` columns, `pricing_catalog_version`, `cost_contract_version`) were added via an idempotent nullable-column migration (`migrateCostEventContract`); every legacy row reads NULL for all of them, never `0` - see the cross-cutting rule in `Persistence guarantees` below. Postgres parity follows the same migration shape per ADR 0027.

**Contract v3 semantics** keep the v2 column layout and pricing-source rules. They
define `estimated=true` for both synthesised totals and typed usage frames that
cover only part of a turn.

**Contract v4 semantics** add `tokens_in_missing INTEGER NOT NULL DEFAULT 0` through the idempotent shared migration. Absent input persists independently of measured output, is JSON NULL and withholds input aggregates. Legacy rows retain their scalar input meaning; erased historical presence cannot be reconstructed. Extracts must check this flag as well as full `usage_unavailable` coverage.

Taskless native gaps retain run-session/thread/turn/response identity and deduplicate separately from amounts, admitting later measurements. Notifications are null; extracts omit `usage_unavailable` placeholders without reclassifying historical rows.

### `office_budget_policies`

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | PK |
| `scope_type` | enum | `agent` \| `project` \| `workspace` |
| `scope_id` | string | scoped entity ID |
| `limit_cents` | int64 | hundredths of a cent |
| `period` | enum | `monthly` (calendar reset) \| `total` (lifetime) |
| `alert_threshold_pct` | int | default 80 |
| `action_on_exceed` | enum | `notify_only` \| `pause_agent` \| `block_new_tasks` |

Multiple policies can apply to the same scope (e.g. monthly + total).

### `TaskSession` additions

The [task cost ledger writer](../../task-cost-ledger/spec.md) (AC-10/AC-21), not Office, maintains `TaskSession.cost_subcents`, `tokens_in`, `tokens_out` and `tokens_cached_in` without ledger scans. Unknown output contributes zero, making that numeric rollup a lower bound; completeness requires the ledger read. The session/DTO merged cache column has no independent reader and reconciles with the ledger. Read/write cache splits exist only on ledger rows for contract-v2 per-type analysis.

### Per-agent budget

Each agent instance has `budget_monthly_cents` set at creation (or inherited from a workspace default). The CEO proposes budgets in hire requests; users adjust via agent instance settings.

### Pricing cache (`models.dev`)

The `models.dev` dataset is fetched once per day to a workspace-local disk cache at `<data-dir>/cache/models-dev.json`. The file lives on disk full-fat; only queried models load into the in-memory map. Refresh runs in a background goroutine; refresh failures fall back to the existing on-disk file. Pricing is recorded per-million-tokens for input, cached read, cached write, and output separately (Anthropic charges different rates for cached read vs. cached write).

Refresh is single-flight per client: stale reads do not wait; explicit callers
share the result. Failure/cancellation releases the guard for retry. Replace
the cache by unique same-directory temporary file and atomic rename; on failure
remove the temporary and retain prior disk/memory data. Mark fresh only after
both disk commit and in-memory index replacement succeed.

### Per-CLI usage shapes (ACP wire)

- **claude-acp**: `result.usage` (camelCase, `cachedRead`/`cachedWriteTokens`), plus cumulative `usage_update.cost.amount` in USD; the adapter emits its nonnegative turn delta when present. claude-acp uses logical aliases (`sonnet`, `haiku`); provider-reported cost is the only accurate signal.
- **opencode-acp**: `result.usage` with `inputTokens`/`outputTokens`/`thoughtTokens` (no cached tokens). Optional `usage_update.cost.amount` (often `0` on BYOK).
- **gemini**: `result._meta.quota.token_count.{input_tokens, output_tokens}` (snake_case, no cached, no cost).
- **codex-acp**: 1.4.0+ typed `result.usage` reports only the last model request on normal end_turn, cancellation and terminal failure. Accept its input/output/cache split but mark `estimated=true` until upstream supplies whole-turn totals. Older/absent frames use nonnegative `usage_update.used` occupancy growth as an unsplit per-turn estimate; compaction or other decreases reset that baseline.
- **auggie**, **copilot-acp**: not tracked. `_meta.copilotUsage` is a billing multiplier; Copilot `/usage` would require scraping.

### Disk-runner provider coverage

- **codex** - disk runner is the preferred source. Cost rows promoted from `estimated=true` to `estimated=false` with full token split.
- **amp** - disk runner is the only source. Cost events emitted with `estimated=false`; `credits` captured in `provider_credits`.
- **claude, opencode, gemini** - disk runner NOT used; wire data is authoritative.
- **auggie, copilot** - no `@ccusage/*` package; out of scope.

## API surface

The raw cost-list response always includes `tokens_out`. It contains a number
for an observed count and JSON `null` for an unknown count. It does not omit
the field when the count is unknown.

### Cost resolution (two-layer lookup)

1. **Provider-reported cost** (`cost_source = provider_reported`). If the CLI emits cumulative USD session cost on `usage_update`, subtract the previously consumed session baseline and store the nonnegative turn delta as `int64(amount * 10000)` hundredths-of-a-cent. Ignore non-USD cost values. This is the only accurate path for claude-acp. No rate columns are recorded on this path.
2. **`models.dev` lookup** (`cost_source = models_dev_list`). For CLIs reporting tokens but no cost (gemini, opencode BYOK, codex fallback), resolve pricing against the cached dataset. The four applied per-million rates and the catalogue's `pricing_catalog_version` are recorded alongside `cost_subcents`.

When both miss (first-boot, no network, model unknown, proprietary id), the row records `cost_subcents=0` and `cost_source=unpriced`. `estimated` is NOT forced true on this path - it independently tracks whether the token counts themselves were synthesised (see the `office_cost_events` table above). Before contract v2, an unpriced row was indistinguishable from a token-synthesis row; downstream consumers that read `estimated` as a stand-in for pricing confidence should switch to `cost_source`. UI shows "pricing unavailable". Users can override pricing per model in workspace settings.

### Disk-runner binary: `cmd/usage-runner`

Standalone Go binary built into the backend Docker image alongside `kandev` and `agentctl`. Invoked after `session/complete` events for relevant providers, and periodically (every 60s) to catch sessions completed during backend downtime.

Spawns `npx -y @ccusage/<provider>@latest session --json`, reads stdout, emits normalized `CostEvent` records.

Join key: `TaskSession.ACPSessionID == ccusage.sessionId`. Verified empirically:

- codex: `session_meta.id` in rollout JSONL equals ACP session id equals rollout filename suffix.
- amp: thread JSON top-level `id` equals agent's session id.

ccusage's `session --json` rolls per-turn events into one summary per `(session_id, model)`. One cost row per session-model pair, not per turn. `provider_event_id` is deterministic from the aggregate row's identity, so re-runs replace rows.

ccusage version policy: track `@latest`. Mitigations: schema-validate ccusage's JSON output at decode time; nightly CI smoke job runs the runner against committed fixture inputs (fake `HOME` with synthetic rollout/threads files) and asserts the JSON contract.

### Cost aggregation queries

Aggregated on read (not pre-computed):

- **By agent instance**: total spend per agent over a time window.
- **By project**: total across all tasks.
- **By task**: total across all sessions.
- **By model**: spend broken down by model.
- **By time**: daily/weekly/monthly trends.

Token aggregates are unavailable when an included row lacks the corresponding
measurement; a known subtotal is not presented as a complete token count.
Monetary projections retain independently recorded spend, including measured
zero, but return null when no priced amount exists and flag degraded coverage
for unpriced or unavailable observations. These rules apply within the selected
Explorer window as well as lifetime reads; budget arithmetic remains separate.

### Costs Explorer request windows

This section governs AC-OFFICE-COSTS-001.2/.8 across the composed HTTP read,
SQL aggregation and shared desktop/phone controls. Office owns this vertical
contract because it owns `office_cost_events` and the workspace breakdown.
The [task cost ledger](../../task-cost-ledger/spec.md) owns separate immutable
usage recording and session rollups; the
[budget measurement contract](../requirements/budget-measurement-integrity.md)
owns enforcement windows. Neither is changed by an Explorer selection.

**Request contract.** `GET /api/v1/office/workspaces/:wsId/costs/breakdown`
accepts one optional `range` query parameter:

| Query | Lower bound | Upper bound |
| --- | --- | --- |
| omitted | none (existing lifetime behavior) | none |
| `range=mtd` | first day of request instant's UTC month, 00:00 UTC | request instant |
| `range=30d` | request instant minus `30 * 24h` | request instant |

Selected windows are half-open: `start <= occurred_at < before`. Last 30 days
is an elapsed duration, not 30 civil dates or a local-midnight truncation.
At UTC month rollover MTD starts in the new month; at its first instant the
window is empty. Future events are excluded from selected windows. Omitted
range retains the historical unbounded query, including its existing treatment
of future timestamps. The handler rejects a supplied empty value, repeated
values (including duplicates), or anything outside `mtd`/`30d` with HTTP 400
and the existing `{error: ...}` envelope before querying storage. Validation
errors have a stable diagnostic; database errors remain HTTP 500. The window
selection adds no request field beyond this query parameter. Response totals
and groups retain the nullable monetary and coverage projection above.

**Service boundary.** Preserve `CostService.GetCostsBreakdown(ctx, wsID)` for
unfiltered callers. Add `GetCostsBreakdownForRange(ctx, wsID, range)` for the
handler. It validates the range and captures `time.Now().UTC()` once before
launching the existing five errgroup queries. A small deterministic window
resolver accepts that instant explicitly, permitting fixed calendar/offset
boundary tests. Pass the same start/before pair to every query; no query reads
the clock. The lifetime path uses the current repository methods. Keep empty
arrays normalized and errors joined before returning. Sharing temporal bounds
does not promise a transaction-wide database snapshot: the existing concurrent
read architecture is retained, so a write between queries has its existing
visibility semantics.

**Repository boundary.** Use `GetCostSummaryInWindow` for nullable spend and
coverage, alongside `GetCostsByAgentInWindow` / `GetCostsByProjectInWindow` /
`GetCostsByModelInWindow` / `GetCostsByProviderInWindow`, each accepting
`start, before time.Time`. Add these to the cost service's `Repository`
interface. Share the existing query bodies through private per-query helpers
so lifetime methods retain their signatures and no budget-facing `*Since`
method changes. Window helpers add only bound parameters
`AND e.occurred_at >= ? AND e.occurred_at < ?` to the existing WHERE clause,
before GROUP BY; convert both arguments to UTC and use `r.ro.Rebind`.
Ordinary timestamp comparison, SUM/COUNT/COALESCE and existing concatenation
are portable across SQLite and PostgreSQL. Do not introduce dialect-specific
date arithmetic, string-formatted timestamps, migrations or browser filtering.

Keep workspace scoping through the existing task/run-session joins and
`COALESCE(t.workspace_id, rs.workspace_id)`. Preserve event-time agent profile
identity, deleted-profile empty labels, live task project assignment (including
unassigned/run-only groups), model grouping by `(provider, model)`, provider
fallback labels, all event counts, integer subcent arithmetic and zero-cost
rows. Each dimension includes every selected in-workspace event exactly once;
its known priced sum equals the selected known spend and its counts equal the
selected event cardinality. Unknown-only groups remain unavailable rather than
zero. Group count cards continue to count groups in their selected
breakdown. No new event-count field is needed in the composed response.

**Frontend boundary.** In `lib/api/domains/office-api.ts`, keep
`getCostsBreakdown(workspaceId, options?)` compatible by extending the optional
request options with a typed `range?: "mtd" | "30d"`. Destructure range from
transport options; append the encoded query only when present and preserve
signals/other request options. `CostOverview` passes its selected range on
initial load, selection and Office live refresh. Include range in the existing
fetch callback dependencies so live refresh cannot retain an earlier range.
Retain the request-generation guard and existing error handling. Track the
loaded range with the loaded workspace: results are display-eligible only when
both match the current selection. Pending or failed switches use the existing
empty view rather than another window's spending; same-range live refresh
retains its current behavior. Do not fetch
or filter raw lifetime rows in the browser.

**Mobile and validation.** Shared `CostOverview` controls, API and tables retain
their layout, copy, targets, navigation, breakpoints, drawer and scroll owner.
Under the bounded mobile-parity exemption, actual component actions through the
real API wrapper/intercepted fetch plus HTTP/SQL tests prove this data-only
change; no new phone capture/browser-build gate is required. Existing
`e2e/tests/office/costs.spec.ts` matches `URL.pathname` through `waitForHttp`;
its query-string unit test protects compatibility.

Regressions cover month/year/leap and offset/DST boundaries, exact endpoints,
event-time versus ingestion-time ordering, empty windows, every group/count,
workspace isolation, task/run-session rows, project reassignment and deleted
profiles. Use real SQLite repository/service/Gin tests and env-gated PostgreSQL
parity through `testutil.OpenIsolatedPostgres`/`PostgresDSNFromEnv`, initializing
task then Office stores. Never use SQLite fixture DDL on PostgreSQL; no DSN
means skipped, not executed.

Implementation is tracked in the
[cost range repair plan](../../../plans/office-cost-date-range/plan.md).

### Cost explorer UI routes

- `/office/company/costs` - summary bar, by-agent/by-project/by-model/by-time views, budget policies CRUD.
- Drill-in: agent rows -> `/office/agents/[id]` (Runs tab shows per-session cost; Overview shows cumulative + gauge); project rows -> project detail.

### Dashboard integration

`/office` shows per-agent budget utilization gauges on agent status cards and a "Spend this month" stat.

## State machine

Budget enforcement transitions (per agent instance):

| From | Trigger | To | Effect |
| --- | --- | --- | --- |
| `idle`/`running` | spend crosses `alert_threshold_pct` | unchanged | inbox item and CEO `budget_alert` wakeup |
| `idle`/`running` | spend exceeds `limit_cents`, action `pause_agent` | `paused`, reason `budget_exceeded` | pending wakeups finished; current turn completes; no more prompts |
| `paused` (budget) | user increases budget | `idle` | resume wakeups |
| `paused` (budget) | first-of-month UTC reset | `idle` if within new limits | reset counters |

Monthly reset is idempotent: backend restart mid-month does not refire.

## Permissions

Cost and budget access is workspace-scoped under the [Office auth gate](../requirements/agents.md#permissions): user-session UI requests bypass as admin; agent JWTs use Office permission middleware.

- **Read** (`GET /workspaces/:wsId/costs*`): every workspace member and agent role sees workspace-wide totals, without agent/project/field restrictions.
- **Budgets** (`POST /workspaces/:wsId/budgets`, `PATCH/DELETE /budgets/:id`): UI/admin only. The CEO uses `kandev-team-admin` for checks and inbox proposals, not direct budget HTTP mutations.
- **Pricing overrides** in workspace settings: user only; agents cannot mutate them.
- **Pause** (`CheckBudget`, action `pause_agent`): the subscriber's `system` actor performs the same pause as a user. Only the user can resume by raising the budget or awaiting monthly reset.

Conformance tests cover authenticated workspace reads and budget mutations under the existing admin/UI and JWT-bypass rules.

## Failure modes

- **models.dev miss**: row recorded with `cost_subcents=0` and `cost_source=unpriced`; `estimated` is unaffected (tracks token-synthesis independently, see the `office_cost_events` table). UI shows "pricing unavailable".
- **Prompt-usage event redelivered** (e.g. a reconnecting WS client replaying a buffered stream event): the second `CreateCostEvent` call collides on the unique `usage_event_id` partial index and is dropped as a no-op rather than double-inserting or double-counting the session rollup.
- **Writer silently stops**: every path a prompt-usage event can take (decode failure, missing task/session ids, task-fields lookup failure, insert failure or duplicate, or a successful write) increments an `expvar` counter (`cost_events_written_total` / `cost_events_dropped_total`, dev-mode `/debug/vars`) and logs, so a stopped writer is observable instead of silently producing zero rows.
- **`recordCostEventAndRollup` only inserts `office_cost_events`** ([task-cost-ledger](../../task-cost-ledger/spec.md) AC-10/AC-21): it no longer increments the `task_sessions` rollup or shares a transaction with it. The ledger writer's own insert (`task_usage_events`) and its `task_sessions` rollup increment are wrapped in one transaction on its side, independent of this call, so a redelivery is idempotent on each ledger's own unique index without the two ledgers coordinating a shared transaction.
- **models.dev fetch fails or is canceled**: background refresh falls back to
  existing on-disk and in-memory data; no crash. The in-flight guard is released
  and a later refresh can retry.
- **models.dev refresh is requested concurrently**: one network request and one
  atomic cache replacement run per client. All explicit callers observe the
  shared result, and temporary files do not collide or remain after failure.
- **Disk-runner `npx` unavailable**: log one warning per provider per process lifetime, mark run skipped, backend continues. Codex sessions retain wire-side `estimated=true` rows; amp sessions remain untracked.
- **ccusage JSON schema drift**: schema validator returns decode error; office subscriber treats run as no-op (no rows touched). Codex falls back to wire-side estimated path; amp absent. Nightly fixture-smoke CI alerts maintainers.
- **`@ccusage/<provider>@latest` yanked**: next runner invocation fails; coverage degrades the same as parse failure.
- **Coalescing**: if `session/complete` fires while a runner is already executing for that provider, the second invocation is dropped. The 60s sweep catches sessions in the gap.

## Persistence guarantees

**Survives restart:**

- `office_cost_events` rows (full history, never trimmed). Disk-runner rows keyed by `(session_id, provider_event_id)` survive re-ingestion without duplicating; wire-side rows are additionally deduplicated by the unique `usage_event_id` partial index (contract v2). The wire-side `estimated=true` rows for codex are deleted once the matching aggregate row lands.
- **Column semantics never change value mid-series across a contract bump.** Legacy rows (written before a column's semantics changed) read NULL, never `0`, for a newly nullable value - the downstream point-in-time extract has no schema versioning, so a column whose meaning flips from "not recorded" to "recorded as zero" (or vice versa) partway through the series would be silently discontinuous. This applies both to columns introduced by contract v2 and to `tokens_out`, an existing column whose contract became nullable in v3: pre-v3 rows read `0`, not NULL, for `tokens_out`. Contract v3 changes the meaning of `estimated` for new rows only; legacy rows keep their recorded value. `cost_contract_version` on each row is the in-band marker a consumer checks instead of inferring from a date.
- `office_budget_policies` rows.
- `TaskSession.cost_subcents` / `tokens_in` / `tokens_out` / `tokens_cached_in` running totals, maintained by the task cost ledger writer, not this Office subscriber ([task-cost-ledger](../../task-cost-ledger/spec.md) AC-10/AC-21); its ledger insert and rollup increment are atomic, so they cannot diverge on a partial write. The numeric `tokens_out` rollup is a lower bound when a source row has an unknown output count.
- Per-agent `budget_monthly_cents` stored on the agent instance row.
- Per-model pricing overrides stored in workspace settings.
- The on-disk `models.dev` cache at `<data-dir>/cache/models-dev.json`. Recovery on next boot: the in-memory pricing map is empty until first query; queries fall back to the on-disk file when the background refresh has not yet completed.
- A failed refresh never replaces or invalidates the last valid cache. Refresh
  coordination and temporary files are transient and do not survive restart.
- Activity log entries `budget.alert` and `budget.exceeded` (workspace-scoped, included in the standard office backup as part of normal SQLite persistence - see `persistence.Provide` for the snapshot policy).

**Does NOT survive restart:**

- In-memory pricing map inside the models.dev client. Rebuilt lazily on first lookup from the on-disk cache file.
- The disk-runner coalescing set (the in-process record of "already running for provider X"). After restart the 60s periodic sweep is the catch-up path - no in-flight session state is replayed.
- Wire-side `estimated=true` rows for codex sessions that completed during downtime: those are reconciled when the next disk-runner sweep promotes them to the aggregate row.
- The "monthly reset already ran for month M" guard is durable (idempotent by month boundary) so a restart on the 1st UTC does not refire the reset for agents that already returned to `idle`.
- Cached aggregation results in the frontend store (`office.costSummary`, `office.budgetPolicies`) - rehydrated from the API on next page load.

No TTL or retention is applied to `office_cost_events`; rows accumulate for the lifetime of the workspace. Cleanup happens only through workspace deletion (cascade via task workspace foreign key) and agent / project deletion garbage collection (see `office/repository/sqlite/runtime.go`).
