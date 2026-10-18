---
status: draft
system: costs
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-001
  - REQ-COSTS-CONVERSATION-USAGE-002
  - REQ-COSTS-CONVERSATION-USAGE-003
  - REQ-COSTS-CONVERSATION-USAGE-004
  - REQ-COSTS-CONVERSATION-USAGE-005
---

# Built-in conversation usage system design

## Boundary and existing implementation

The costs system owns accounting semantics and chat usage projections.
The [agent design](../../agents/system-design/codex-app-server.md) supplies normalized native observations.
Keep one task ledger writer in `internal/task/usage` and the existing shared pricing logic in `internal/common/costs`.
Do not depend on Office or a plugin for collection, pricing, or display.

Existing `task_usage_events` rows contain a Kandev turn ID, token components, pricing provenance, and a deduplication key.
Existing HTTP routes expose task and session totals, but not response-level detail.
`lib/ws/handlers/prompt-usage.ts` currently stores the latest prompt counts only.
`TokenUsageDisplay` shows context occupancy, not monetary spend.

This design extends the [legacy ledger contract](../../task-cost-ledger/spec.md).
Its original AC-23 arithmetic remains authoritative for existing token columns.
Native observations add a separate event admission path and optional attribution/breakdown columns.
No historical rows change meaning. The implementation must amend the legacy scope wording with a link to this extension.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-COSTS-CONVERSATION-USAGE-001 | Measurements, attribution, persistence |
| REQ-COSTS-CONVERSATION-USAGE-002 | Prices and estimates |
| REQ-COSTS-CONVERSATION-USAGE-003 | Read surface and presentation |
| REQ-COSTS-CONVERSATION-USAGE-004 | Persistence, compatibility, validation |
| REQ-COSTS-CONVERSATION-USAGE-005 | Message and turn metadata |

## Verified protocol surface

Local schema generation on Codex 0.154.0 established these shapes without a model turn:

| Surface | Scope and fields | Availability |
| --- | --- | --- |
| `thread/tokenUsage/updated` | `threadId`, `turnId`, `tokenUsage.total`, `tokenUsage.last`, `modelContextWindow` | Standard notification, including resume replay |
| `rawResponse/completed` | `threadId`, `turnId`, `responseId`, nullable `usage` and `usageMetadata` | Marked internal-only upstream; not a universal compatibility promise |
| `TokenUsageBreakdown` | `inputTokens`, `cachedInputTokens`, `cacheWriteInputTokens`, `outputTokens`, `reasoningOutputTokens`, `totalTokens` | Counts, not currency |
| `ResponseUsageMetadata` | Nullable string `amount`, nullable arbitrary `metadata` | Schema does not establish monetary units |
| `account/usage/read` with `threadId` | Optional `threadUsage`, including `estimatedUsageUsdMicros` and credit micros | Billing-route dependent, nullable thread USD estimate |

References: [native thread schema](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/app-server-protocol/src/protocol/v2/thread.rs),
[response metadata](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/protocol/src/response_usage.rs),
and [resume replay](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/app-server/src/request_processors/token_usage_replay.rs).
The local generated schemas are investigation evidence in temporary storage, not checked-in product contracts yet.

## Measurements and normalized observations

Add a normalized `usage_observation` event rather than synthesizing `complete` events for accounting.
Proposed payload fields:

- Stable observation ID, source (`response` or `turn_fallback`), provider thread/turn/response IDs, and optional provider item ID.
- Kandev session and originating turn binding, parent native thread, and direct-versus-child scope.
- Observation schema version, token presence, measurement provenance, and completeness.
- Observed model/provider/service tier when available. Never infer a child's model from an unrelated parent profile.

Normalize native counts into the existing disjoint ledger components before publication.
Set `InputTokens` to native input minus cached input, and `CachedReadTokens` to native cached input.
Set `OutputTokens` to native output, which already includes reasoning, and leave native `ThoughtTokens` zero.
Add nullable `ReasoningOutputTokens` as an informational subset, excluded from the existing total and price sums.
Persist that subset in a new nullable ledger column. Existing `TokensThought` retains its existing meaning for other adapters.
The ledger total formula and contract version stay unchanged because existing columns keep their meaning.
Keep ACP input/output/cache/thought presence across JSON-key unescaping, SDK decode and wire. Preserve zero and valid siblings; absent-only usage stays absent, and negative typed categories become absent. Office rejects negative payloads. Metadata floats must be integers in [0, 2^53); typed int64 retains range. Legacy `tokens_in_missing` defaults false. Unknown required counts forbid catalogue pricing. Negative provider cost is absent; measured zero stays authoritative.
Grok reasoning follows these bounds; valid reasoning-only usage, including zero, remains measured.
Persisted debug counts omit unmeasured categories and retain presence-known zero.
Decode native response usage with `json.Number`; int64 lexemes retain range/precision. Response vectors require non-null input/cache-read/output/reasoning/total; only cache-write defaults zero. Invalid responses retain no-amount coverage.
Preserve a differing native reported total separately for diagnosis, not as a second authoritative aggregate.
Reject invalid negative values, overflow, and impossible subsets as unusable measurements.

Example: input 1,000, cached input 600, output 200, reasoning output 80 means 1,200 total tokens.
The price uses 400 uncached input, 600 cached input, and 200 output tokens.
Reasoning 80 is visible detail, not an additional price term.
For a nonzero native cache-write category, preserve it as optional `ReportedCacheWriteInputTokens` detail.
Do not populate the existing independently additive `CachedWriteTokens` until a reviewed source-specific rule establishes its meaning.
Until that rule exists, keep the price unavailable and mark the breakdown incomplete.
Do not silently discard that field or invent an additional input charge.

## Attribution and aggregation

Persist each attributable `rawResponse/completed` once on arrival, keyed by Kandev session/native thread/response, never execution. Replay is idempotent; distinct responses remain distinct.
Validate native thread binding before accounting; provider strings never become Kandev task/session IDs.

Root `codex_native_turn_id` is private. Guard generation/captured ownership on starts and lazy first output; output cannot replace a binding.
Direct observations require unique task/session/provider ownership. One scalar query inspects at most two IDs, no history/map. Omit missing, ambiguous or conflicting ownership before amounts/gaps.
Children retain their originating Kandev ID and native turn IDs. Diagnose/omit unbound events, never inherit active context.
Root-turn cost includes direct and attributable child observations, with separate subtotals.
Session cost sums each observation once. Descendant rollups are display projections, not additional ledger rows.

Capture model/Kandev generation once per provider turn; duplicates neither replace nor renew it.
One locked map/FIFO retains 128 root/child captures; fresh sessions and different-thread resume clear it, same-thread resume preserves it.
Delayed responses keep captured ownership; evicted/unbound responses diagnose/omit without amounts or active-turn mutation.

`tokenUsage.total` is cumulative, not an increment; `last` is response/context detail, not turn work. Resume/fork replay cannot establish new amounts. Exact responses exclude cumulative totals/terminal prompt summaries.

Turn estimates require proven final cumulative boundaries. Pinned native v0.154.0 supplies no final-count marker; completion cannot prove its last snapshot final. Do not derive new native amounts from cumulative notifications.
Keep exact responses and amount-free gaps; never add a synthetic zero or remainder. Invalid, replayed or late cumulative snapshots cannot alter amounts or suppress exact responses.
Previously recorded estimated rows remain readable with estimated provenance.
The inspection fixture must establish normal event ordering before exact completeness is advertised.

Persist in-flight response observations so interrupted or crashed turns retain known work.
A completion changes attribution/completeness state, not previously recorded token amounts.
A later child completion updates its originating turn projection without generating another root completion. A restored child with unproven origin never falls back to the active root turn.

## Persistence and compatibility

Extend the immutable ledger with nullable native thread/turn/response/item IDs, scope and measurement source. Reuse the writer, mirrored decoder, bounded queue, checked arithmetic, ownership validation and transactional session rollups; preserve plugin fields.

Atomically mark owner-validated `task_usage_coverage` for monotonic scoped gaps, without amounts. Task FK survives session/turn cascades alongside retained amounts. Backfill live gaps only; never reconstruct purged evidence. Commit before invalidation; reads stay partial. Unknown ordering withholds `last_response`, not known rows.

`task_usage_turn_index` indexes amount-free turns. Backfill measured `MIN(event id)` cursors and owner-validated gaps; advance PostgreSQL's sequence. Atomically index writes, preserving numeric cursor identities. Order by owning turn creation, not usage arrival; late historical gaps never become latest. Cursors resume that chronology. Zero-row partial turns remain unavailable, never synthetic amounts.
Batch cursor, amount and owner/coverage reads per bounded page, never per-turn queries.

Use one orchestrator native publisher with versioned fields on the existing usage bus; retain ACP `publishPromptUsage`. Suppress native terminal summaries. Task and Office accept additive fields before native publication. Share arithmetic in `internal/common/costs`; no task-to-Office import, second task writer or plugin-side cost calculator.

Office IDs use run-session/thread/turn/response (source fallback). After task coverage commits, publish task-owned gaps on the existing event: task rejects nil usage; Office stores `usage_unavailable`. Separate gap/amount deduplication preserves later amounts. Null/unpriced gaps degrade budgets without spend; legacy defaults false. Captured models, direct-only fallback, OR suppression; children never create runs.

Use current SQLite/PostgreSQL migrations. Existing rows stay immutable, new fields default absent, and unknown observation schemas fail visibly.

## Prices and estimates

Keep existing provenance: `provider_reported`, `models_dev_list`, and `unpriced`.
Separate token completeness from cost provenance. Exact tokens can still have an estimated price.
Use the recorded actual model and service tier when a reviewed rate exists.
Missing model identity or an unpriced tier produces an unavailable cost, not a guessed default-model price.

Do not map `usageMetadata.amount` to `ProviderReportedCostSubcents` until currency and units have authoritative evidence.
The debugger can expose the raw field. Production only retains allow-listed typed metadata.
The first version can show per-response and per-turn calculated USD estimates using existing catalogue rates.
Subscription wording states that this is an API-equivalent estimate, not an additional subscription charge.

When available, `threadUsage.estimatedUsageUsdMicros` produces a separate provider-estimate snapshot.
Store amount, currency, thread scope, observed time, and stale/unavailable state without adding it to ledger totals.
Use int64 decimal parsing and decimal strings across browser boundaries when values exceed safe JSON integer precision.
Credit micros stay credits and never pass through a USD formatter.
Refresh on explicit detail requests and completed root turns with bounded coalescing, not per text delta.
Failure does not block chat or erase the last known snapshot.
Forked thread estimates can include inherited history, so label their native-thread scope explicitly and keep session recorded totals separate.

## Read surface and presentation

Keep existing task/session totals routes; add input/total completeness for missing required counts and native coverage gaps. Omit incomplete session totals without changing stored lower-bound arithmetic.
Add authorized `GET /tasks/:id/sessions/:sessionId/usage/turns` with cursor and limit.
Add `GET /tasks/:id/sessions/:sessionId/usage/turns/:turnId` for one Kandev turn and its bounded response detail.
Include counts, per-category presence, completeness, direct/child subtotals, price sources, and last-response detail.
Buckets retain numeric counts and cache-presence flags (any measured row); `input_complete` requires input on every row. Response `input_tokens_present` preserves zero. Omit incomplete aggregate input. Optional `reasoning_tokens` sums native reasoning subsets or ACP thoughts, without changing token, cost, or TPS arithmetic.
Native thread IDs never replace Kandev route ownership.

After committed ledger writes publish `session.usage_updated`; read committed projections, not streamed-token USD. Coalesce invalidations; refresh on reconnect, activation and detail opening. Guard task/session identity. Delayed pricing is pending, not zero.
Store accepted invalidation generations per turn and summary. Withhold older-generation totals/latest turn until current reads succeed, including pending retries; same-generation failures retain accepted cache. Refresh newest then demanded stale history without cursor/exhaustion changes. Reconnect refreshes history. Reject older responses; catch pending reads up to invalidations. Skip undemanded history.
Keep the latest response summary without a one-to-one rendered-message mapping. Show latest-turn usage in the unified disclosure, not every text fragment.

The task/session-scoped `ConversationUsageDisplay` lives in `ChatStatusBar`'s right-hand controls above the composer, beside Threads, transcript navigation, and Share. `MessageListFooter` no longer mounts it. An archived transcript has no composer status row, so `ChatFooter` keeps the same control beside Jump to latest in the read-only archived banner. Both paths use the selected task/session identity, existing usage hook, committed projections, `conversation-usage-trigger` identity, and latest-turn/session breakdown. Render the active right-hand cluster when a task and session are present so pending and failed reads can surface, but leave no blank control or gap when the usage component returns nothing. Keep a proceed action right-aligned in the same action group even when Usage returns nothing. A session switch must replace the displayed usage with the newly selected session's data.

Desktop: a stats icon without visible text opens the existing usage popover. Give the icon the localized Usage accessible name and a hover/focus tooltip; keep its compact size aligned with adjacent status-row icons.
Phone and coarse pointers: the same icon has a touch-sized target and opens the inset drawer when the mobile breakpoint is active or the shared `useTouchDrawer()` policy is true. This keeps narrow fine-pointer windows on the phone path as well as coarse-pointer devices. Reuse the `MobilePickerSheet` geometry pattern. The status row may wrap at narrow widths, but the controls remain reachable without horizontal page scrolling.
Order: this turn, last response, direct/child split, token breakdown, price provenance, then session totals.
A long response list uses one scrollable body with a fixed header and safe-area padding.
Use a 44px touch target and the adjacent status-row icon size on desktop, with localized labels, focus return, and no horizontal page overflow. Hide the desktop tooltip while its popover is open, including after keyboard activation.
Context occupancy stays in its existing control with corrected native provenance.

## Message and turn metadata

This section owns the metrics shown from a message. The ledger, its pricing, and the usage read routes stay the only source of tokens and cost.

### Turn timing capture

A mutex-guarded per-session timing record belongs to orchestrator `Service`; `handleAgentStreamEvent` serializes each session. `EventTypeTurnStarted` records backend receipt and active turn ID: TTFT excludes pre-start delivery but includes output delivery. Self-resume may start without a turn; bind on first output after streaming creates it, using `nonCreatingActiveTurnID`.
The first nonempty, non-diagnostic `message_streaming`/`thinking_streaming` frame stamps first output and binds a missing turn ID; reuse `observedOutput` filtering.
Generation-bearing frames must pass the existing prompt-generation ownership guard before start, first-output, or error cleanup mutates timing. An event carrying a Kandev turn identity must also match that turn and execution, so a predecessor cannot stamp or discard its successor's record.
Lifecycle holds ID-less chunks until the first newline, so TTFT can also be biased late for a single-line answer. The metadata window's localized TTFT tooltip states both this buffering caveat and that the interval starts at backend receipt of `turn_started`, not prompt dispatch.
`EventTypeResponseAttemptReset` does not clear either value.
The record is consumed exactly once by the complete-stream write path: atomically match the session, completing execution, captured Kandev turn ID, and prompt generation when both sides carry one before reading and deleting it inside `handleAgentStreamEvent`. A mismatched completion leaves the replacement record untouched, even when executions share a Kandev turn ID. It is also removed there on error and cancel, and replaced on the next `turn_started`. It is never keyed on the turn being active and is never cleared from `completeTurnForSession` or `agent.ready`, which run in a different consumer. Cancellation is detected in the completion path from `stop_reason == cancelled` only, because the terminal marker carries no cancel state.
The close stamp lives on the in-memory `turnTimingRecord.completedAt`, set from the immutable backend receipt timestamp captured immediately before `handleAgentReady` calls `completeTurnForSession`. `readyTurnMark` carries only `turnID` and `expiresAt` for turn identity and is not part of timing. The `agent.completed` handler must not timestamp terminal-marker creation as turn completion: it may run after a ready close or before the actual terminal close. If terminal completion finds the turn still open and closes it in `finishAgentCompleted`, it stamps `completedAt` immediately before that call; if `completedAt` is already set, the first close wins and terminal handling preserves it. The record captures prompt generation from `turn_started` when available, otherwise from the first eligible output or first matching close; generation-zero frames retain session/execution/turn compatibility. Late terminal consumption compares this captured identity rather than requiring the execution to remain the lifecycle manager's current prompt. Both `persistCompleteStreamOutput` and `flushTerminalCompleteStream` read the same record, even when the payload already contains `TurnID`. A missing close stamp keeps `ttft_ms` and omits `generation_ms`; never substitute complete-stream receipt or terminal-marker creation time. Expiry of a `readyTurnMark` never affects `generation_ms`. `Service.timingClock` is the injectable clock for all timing stamps.
Start, first-output, and completion stamps are backend receipt times; agentctl may run on another host, so no agentctl clock is compared with the backend clock. If the matching close mark is unavailable, the already in-memory timing record cannot safely establish a complete interval and TPS is omitted.

Timing is written by its own call from `persistCompleteStreamOutput` and `flushTerminalCompleteStream`, outside the `Usage == nil` guard of `persistTurnPromptMetadata`, because native Codex and some ACP completions carry no usage.
It writes `timing` with integer milliseconds `ttft_ms` (start to first output) and `generation_ms` (first output to Kandev turn completion), only when both stamps exist, the turn was not cancelled, the record matches the completion's execution and captured turn ID plus any mutually available prompt generation, and the record was not already consumed. Calculate `generation_ms` from the immutable close timestamp `turnTimingRecord.completedAt`, never from the terminal marker's creation time or complete-stream receipt/flush time. A delayed stream flush therefore cannot inflate TPS. A late flush of a predecessor after a successor `turn_started` writes nothing, and a second completion never overwrites or aggregates timing. A restart loses the in-memory start/output record, so that turn has no `timing` and the line and window omit TTFT and TPS.
Lifecycle publishes `agent.ready` and closes the turn before the complete-stream frame of the same completion on every transport, so the timing patch normally lands after `turn.completed`. `turn.updated` is therefore the normal delivery path, not a fallback. The task `Service.PatchTurnMetadata` publishes a new `events.TurnUpdated` (`turn.updated`) whenever the patched keys include `timing`, whether or not the turn is still active, using `publishTurnEvent` after re-reading the turn with `GetTurn` so the event carries the new `updated_at` (the web `shouldApplyTurnUpdate` rejects an equal or older one) and the same sanitized metadata shape as `turn.completed`. The action is `ws.ActionSessionTurnUpdated` in `pkg/websocket/actions.go`. The gateway handles it in its own case that calls only `BroadcastToSession`, never `BroadcastConversationMutation`: a metadata patch already advances the conversation revision without a receipt, so v2 recovery/hydration can encounter the timing metadata and depends on the projection allowlist below. No conversation mutation broadcast is needed to trigger recovery, and there is no receipt to carry this event. The web `session.turn.updated` handler applies `addTurn` only, with none of the completed-turn side effects. The action is added to neither `CORE_CONVERSATION_ACTIONS` nor `ORDERED_EVENT_ACTIONS`. Publishing from the service keeps the orchestrator `TurnService` interface and its fakes unchanged. Re-publishing `TurnCompleted` is not allowed.
`timing` reaches the web through `turn.updated` and the REST DTO, which passes metadata through. Add `timing` to the keys allowed by `models.ProjectTurnMetadata` so both v2 recovery/hydration and a later receipt-driven turn upsert preserve it. Wall-clock time needs no new field because `Turn.StartedAt` and `CompletedAt` already exist.
Timing is captured for every turn regardless of the display setting.

### Tokens, cost, and TPS projection

Line, window and Usage popover use `UsageTurn.total` (direct plus child). Cost uses `cost_subcents` and `cost_sources`; omit it when absent or `unpriced_count` is positive.
Require nonnegative safe counts/TPS operands; omit empty/overflowed buckets and incomplete totals (even positive subtotals). Total overflow preserves direct reasoning, timing and cost; direct overflow hides reasoning/TPS. Reported cache-write is non-additive, non-priceable last-response detail. The window labels partial/estimated usage alongside independent metrics; zero-row partial coverage is unavailable.
Index usage newest-first (cap 100) with bounded history. Empty pages exhaust it. Refresh newest data and stale demanded history on invalidation, preserving cursors/exhaustion. Withhold stale metrics during refresh; report pending, or unavailable on failure. Only windows fetch missing non-exhausted turns. Coalesce/retry invalidation-raced 404s and revisit active absent demand. Scope response detail to invalidation too; refresh demanded same-turn detail and reject earlier responses.
Join bounded usage pages to the actual latest hydrated conversation turn by strict creation nanoseconds. Unknown/tied chronology or no match omits current metrics, never borrows older usage. Join live state so late replies cannot restore predecessor attribution. Session recorded totals remain independent.
TPS is `(direct.output_tokens + direct.thought_tokens) / (generation_ms / 1000)`, excluding concurrent child output. ACP thought tokens remain additive. Require a positive safe numerator, positive generation window, complete direct output and non-estimated/non-partial completeness. ACP `unknown` remains eligible with complete output; native rows are `exact`. Never infer missing output solely from `unknown`.
The generation window includes tool execution time, so TPS is an average turn output rate, not decode speed, and its label says so.
The reasoning row uses nullable `direct.reasoning_tokens`, summed across measured direct responses including native output subsets. List pages carry it without detail reads. Show measured zero; omit absent and child reasoning.
Parse both duration endpoints with the strict RFC3339/Nano helper; omit malformed values. Preserve prompt whole-second flooring and clock-skew clamp.
No new backend read route is added, and no plugin or SDK surface changes.

### Setting contract

Store `message_metadata_fields` beside existing JSON lists in `user/models`, `user/store/sqlite.go` (`marshalUserSettingsPayload`, `scanUserSettings`), `user/dto`, `user/controller`, `user/service` (`applyBasicSettings`, `publishUserSettingsEvent`), `backendapp.mapUserSettingsState` and `settingscatalog`. No SQL migration. Regenerate `contract.generated.json` and `profile-contract.generated.json` with `go run ./cmd/settings-catalog`; CI checks `scripts/settings-contract-ci.test.mjs`.
The closed value set is `session_config`, `timestamp`, `duration`, `ttft`, `tps`, `tokens`, `cost`. The model default is `session_config`, `timestamp`, `duration`.
The store distinguishes an absent value (use the default) from an explicit empty list (hide everything). The update request carries `*[]string`, so a nil pointer leaves the value unchanged and an empty pointer clears it.
An update with an unknown or duplicate value is rejected with a field error that names the setting and leaves the stored value unchanged. A stored value with unknown entries, for example after a downgrade, drops them on read.
The boot payload exposes `messageMetadataFields`, and the existing settings event carries the snake_case key so other open clients update without reload.
The editor admits PATCH responses through the shared revision check and maps accepted responses into the store. Newer WebSocket state wins; local changes remain a dirty draft. Rejecting a stale response throws `SettingsSaveCancelledError`, preventing the coordinator from releasing pending navigation.

### Presentation

`apps/web/lib/message-metadata-fields.ts` owns the value set, defaults, and a validator shared by the settings mapper and the components.
`MessageActions` renders each item only when the caller allows it (`showTimestamp`, `showModel`) or, for new items, when the user selected it and the message qualifies. The line order is fixed to the current order, then the new items: session configuration, timestamp, duration, TTFT, TPS, tokens, cost. It never follows selection order.
When timestamp is selected, its label uses the existing per-user `messageTimeDisplay` preference and shared `formatMessageTime` formatter. The alternate absolute or relative form remains available in the hover/accessible description and touch drawer; selecting a time format never enables an otherwise hidden timestamp.
The line reuses `settings:messageMetadataTTFT` for the abbreviated label. The window retains its full label and puts TTFT help in `SettingsInfo`, reusing its circled icon, desktop hover/focus tooltip and touch drawer with 44px targets. Do not render TTFT help inline or change TPS help.
The renderer's adapter matchers live in a leaf module, `apps/web/components/task/chat/message-adapter-matchers.ts`, as an ordered table of predicates and `rendersActions` flags. `message-renderer.tsx` joins the table to its render functions in the same order; `lib/turn-metrics.ts` imports only the leaf matcher, avoiding a library-to-component dependency cycle. A message qualifies for TTFT, TPS, tokens, and cost when it is the last agent entry of a completed turn for which the adapter matcher says an action row renders. It is true when an agent-authored entry falls through to the catch-all adapter that renders `ChatMessage`, and false for a message with `metadata.actions`, a payload-retention marker, a task description, and the tool call, thinking, and script types. The existing `messageTurnDurationSeconds` rule keeps duration on user prompt messages. The line items render from a new `message-metadata-line.tsx`, not `message-actions.tsx`.
The qualifying-message check evaluates the user's metadata-field selection first, then checks that the turn is completed. If no usage-backed field is selected, it does no message-list scan. Otherwise it computes the last qualifying message ID once per `(messages array, turn)` with a memoized pass, so repeated `MessageActions` renders do not each scan the session history.
`MessageTurnMetrics` lives in `message-turn-metrics.tsx` and sits at the top of `MessageDebugDialog`, above the raw entries. The dialog gate also opens when the message's turn resolves, even if its metadata is empty and no usage exists, so derivable wall time remains accessible. A message-actions test covers an empty-metadata message with a completed turn. Its rows are wall time, TTFT, TPS, input, output, cached read, cached write, reasoning, and cost with its provenance label; all available values are shown regardless of the hover-line setting, and a pending usage read shows a pending row. The localized TTFT tooltip defines its interval as backend receipt of `turn_started` to first non-empty, non-diagnostic message or reasoning output received by the backend, explains that this is not prompt-dispatch latency and provider diagnostic/error text is ignored, and notes single-line buffering can delay the observation. The TPS label says it is an average turn rate including tool time. Cost is omitted while pending or when unpriced. The existing raw entries, including `turn_metadata.timing`, remain below it.

### Failure behavior

A failed usage read leaves the usage-backed items out of the line and shows an unavailable row in the window. It never blocks the transcript and never shows zero.
A failed settings save leaves the stored value unchanged, keeps the draft dirty, and shows an error.

### Metadata validation

Timing cases: normal/reasoning-first output, single-line buffering, usage-free completion, no output, cancellation, restart and repeated starts; open clients receive timing without reload. Tool delay counts toward generation; post-close stream delay does not. Correlate session/execution/turn ownership, preserve ready close across terminal markers and consume once on either complete path. Missing/mismatched close omits generation/TPS but retains valid TTFT. Use controlled ready/terminal/stream timestamps with explicit and compatibility turn IDs, including terminal-only close.
Settings cases: absent, empty, invalid value and boot-payload round trip. TPS cases: partial/estimated completeness, incomplete direct output, zero numerator/window, unknown completeness with complete/incomplete output, ACP reasoning sums and child exclusion. Omit unpriced cost.
Message eligibility: multiple agent entries, tool-ending and running turns. Paging: multiple pages, empty exhaustion without repeat reads, bounded demand, newest/history invalidation, stale admission, reconnect and fallback 404.

## Validation

Prove two responses within one turn, duplicate replay, resume replay, interrupted turns, late child usage, and fork history exclusion.
Test 1,000/600/200/80 breakdown arithmetic and mixed ACP/native aggregates.
Test exact zero, missing counts, unavailable prices, unknown currency, unknown tiers, and integer overflow.
Test both Office and task consumers against the same observation, with the plugin absent and Office disabled in the UI scenario.
Test authorization and stale response guards for every read route.
Desktop and mobile E2E cover detail access, pending/error states, reload, background activity, and provenance labels.
