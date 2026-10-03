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

Persist each attributable `rawResponse/completed` measurement once when it arrives.
Derive observation identity from Kandev session, native thread, and native response ID, excluding process execution ID.
This makes reconnect replay idempotent without suppressing a distinct later response.
Validate the native thread binding before the orchestrator publishes the accounting event.
Never trust a provider string as a Kandev task or session ID.

Root native turns bind to the actual Kandev turn at dispatch.
Children inherit the original root-turn association at spawn and keep their own native turn IDs.
Unbound events remain pending in a bounded buffer. On expiry, record a diagnostic gap instead of guessing attribution.
Root-turn cost includes direct and attributable child observations, with separate subtotals.
Session cost sums each observation once. Descendant rollups are display projections, not additional ledger rows.

Capture the model and Kandev prompt generation when a provider turn is first bound.
Delayed response observations and fallback snapshots use that captured attribution even if a later prompt changes the selected model or active generation.
Child observations use the child binding's model and generation, while retaining the originating root Kandev turn.
Duplicate start notifications must not replace an established attribution.

`tokenUsage.total` is a cumulative snapshot, not an increment.
`tokenUsage.last` is last-response/context detail, not whole-turn work.
On resume or fork, use restored snapshots as baselines only.
When exact response observations exist, do not also record their cumulative totals or terminal prompt summary.

If no response observations arrive during a turn, a complete monotonic baseline-to-final delta can produce one `turn_fallback` row.
Use a deterministic session/native-thread/native-turn fallback key and label it estimated.
If boundaries are missing, counters reset, or history replays, leave the turn incomplete and do not invent a delta.
If response observations cover only part of a turn, retain them as partial. Do not add an unproven remainder.
Serialize fallback selection at terminal processing and persist the selected mode.
Late response frames cannot add a second accounting mode for the same finalized turn.
The inspection fixture must establish normal event ordering before exact completeness is advertised.

Persist in-flight response observations so interrupted or crashed turns retain known work.
A completion changes attribution/completeness state, not previously recorded token amounts.
A later child completion updates its originating turn projection without generating another root completion.

## Persistence and compatibility

Extend the ledger with nullable native thread/turn/response/item identities, scope, and measurement-source columns.
Add optional subset and attribution handling to the task writer and its mirrored wire decoder.
Keep the existing bounded queue, integer arithmetic, append-only rows, ownership validation, and transactional session rollup.
Preserve old columns and API fields for plugin compatibility.
Add a small attribution/completeness record keyed by session and native turn for baseline, mode, terminal status, and recovery gaps.
This record can change state; usage amounts remain immutable.

Introduce one usage-observation publisher in the orchestrator that reuses the existing usage bus subject with versioned payloads.
The existing `publishPromptUsage` path remains for ACP.
Suppress native terminal-summary publication when native observation accounting owns the turn.
Both task and Office consumers must accept the additive observation fields before native publication is enabled.
Keep shared token/price arithmetic in `internal/common/costs`; task code must not import Office.
Office deduplicates the same observation IDs and must not treat a child observation as a new agent run.
No second task-ledger writer or plugin-side cost calculator is introduced.

Schema changes support SQLite and PostgreSQL through the current repository migration patterns.
Existing rows remain immutable. New fields default absent, and unknown observation schemas fail visibly rather than guess their meaning.

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

Keep existing task/session totals routes.
Add authorized `GET /tasks/:id/sessions/:sessionId/usage/turns` with cursor and limit.
Add `GET /tasks/:id/sessions/:sessionId/usage/turns/:turnId` for one Kandev turn and its bounded response detail.
Include counts, per-category presence, completeness, direct/child subtotals, price sources, and last-response detail.
Native thread IDs never replace Kandev route ownership.

Publish a small `session.usage_updated` invalidation after a committed ledger write.
Frontend reads the committed projection; it does not calculate USD from streamed tokens.
Coalesce invalidations and refresh on reconnect, session activation, and explicit detail opening.
Guard late fetches by task/session identity. A pricing delay shows pending state rather than zero.
Store a latest response summary without implying a one-to-one match to the last rendered message.
Show the latest turn's usage inside the unified disclosure, not on every text fragment.

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

Add one in-memory per-session timing record to the orchestrator `Service`, written and consumed from `handleAgentStreamEvent`, which is serialized per session but concurrent across sessions, so the record map is guarded by a mutex or `sync.Map` like `readyTurnMarks`. `EventTypeTurnStarted` stores the backend receipt time and the active Kandev turn ID when one exists. This is the TTFT origin: the metric is backend-observed turn-start to first-output time, not prompt-dispatch-to-output latency. It excludes delay before `turn_started` reaches the backend and includes delivery latency for the first-output event. Some self-resume prompts emit `turn_started` before a Kandev turn exists; in that case the record stores an empty ID and binds it on the first-output stamp, after `handleMessageStreamingEvent` or `handleThinkingStreamingEvent` has created the turn, by reading `nonCreatingActiveTurnID`.
The first `message_streaming` or `thinking_streaming` event with non-empty, non-diagnostic text after `turn_started` stores the first-output time and, if needed, binds the record to the turn ID that now exists. These are the lifecycle-converted names the orchestrator receives, and the same filter as `observedOutput` applies.
Lifecycle holds ID-less chunks until the first newline, so TTFT can also be biased late for a single-line answer. The metadata window's localized TTFT tooltip states both this buffering caveat and that the interval starts at backend receipt of `turn_started`, not prompt dispatch.
`EventTypeResponseAttemptReset` does not clear either value.
The record is consumed exactly once by the complete-stream write path: read and delete by Kandev turn ID inside `handleAgentStreamEvent`. It is also removed there on error and cancel, and replaced on the next `turn_started`. It is never keyed on the turn being active and is never cleared from `completeTurnForSession` or `agent.ready`, which run in a different consumer. Cancellation is detected in the completion path from `stop_reason == cancelled` only, because the terminal marker carries no cancel state.
`readyTurnMark` carries both turn identity and the immutable backend receipt timestamp captured immediately before `handleAgentReady` calls `completeTurnForSession`. The `agent.completed` handler must not timestamp terminal-marker creation as turn completion: it may run after a ready close or before the actual terminal close. If terminal completion finds the turn still open and closes it in `finishAgentCompleted`, it records the same close mark immediately before that call; if a matching ready mark already exists, terminal handling preserves it. The mark is keyed by session, execution, prompt generation, and turn ID (generation-less completions use the existing FIFO), and is consumed by both `persistCompleteStreamOutput` and `flushTerminalCompleteStream`, even when the payload already contains `TurnID`. Missing, expired, or mismatched close marks omit `generation_ms`; never substitute complete-stream receipt or terminal-marker creation time.
Start, first-output, and completion stamps are backend receipt times; agentctl may run on another host, so no agentctl clock is compared with the backend clock. If the matching close mark is unavailable, the already in-memory timing record cannot safely establish a complete interval and TPS is omitted.

Timing is written by its own call from `persistCompleteStreamOutput` and `flushTerminalCompleteStream`, outside the `Usage == nil` guard of `persistTurnPromptMetadata`, because native Codex and some ACP completions carry no usage.
It writes `timing` with integer milliseconds `ttft_ms` (start to first output) and `generation_ms` (first output to Kandev turn completion), only when both stamps exist, the turn was not cancelled, the record's turn ID equals the completion's captured turn ID, and the record was not already consumed. Calculate `generation_ms` from the immutable close timestamp carried by the matching `readyTurnMark`, never from the terminal marker's creation time or complete-stream receipt/flush time. A delayed stream flush therefore cannot inflate TPS. A late flush of a predecessor after a successor `turn_started` writes nothing, and a second completion never overwrites or aggregates timing. A restart loses the in-memory start/output record, so that turn has no `timing` and the line and window omit TTFT and TPS.
Lifecycle publishes `agent.ready` and closes the turn before the complete-stream frame of the same completion on every transport, so the timing patch normally lands after `turn.completed`. `turn.updated` is therefore the normal delivery path, not a fallback. The task `Service.PatchTurnMetadata` publishes a new `events.TurnUpdated` (`turn.updated`) whenever the patched keys include `timing`, whether or not the turn is still active, using `publishTurnEvent` after re-reading the turn with `GetTurn` so the event carries the new `updated_at` (the web `shouldApplyTurnUpdate` rejects an equal or older one) and the same sanitized metadata shape as `turn.completed`. The action is `ws.ActionSessionTurnUpdated` in `pkg/websocket/actions.go`. The gateway handles it in its own case that calls only `BroadcastToSession`, never `BroadcastConversationMutation`: a metadata patch already advances the conversation revision without a receipt, so v2 recovery/hydration can encounter the timing metadata and depends on the projection allowlist below. No conversation mutation broadcast is needed to trigger recovery, and there is no receipt to carry this event. The web `session.turn.updated` handler applies `addTurn` only, with none of the completed-turn side effects. The action is added to neither `CORE_CONVERSATION_ACTIONS` nor `ORDERED_EVENT_ACTIONS`. Publishing from the service keeps the orchestrator `TurnService` interface and its fakes unchanged. Re-publishing `TurnCompleted` is not allowed.
`timing` reaches the web through `turn.updated` and the REST DTO, which passes metadata through. Add `timing` to the keys allowed by `models.ProjectTurnMetadata` so both v2 recovery/hydration and a later receipt-driven turn upsert preserve it. Wall-clock time needs no new field because `Turn.StartedAt` and `CompletedAt` already exist.
Timing is captured for every turn regardless of the display setting.

### Tokens, cost, and TPS projection

Tokens and cost come from `UsageTurn.total` (direct plus child) of the usage turns read surface, so the line, the window, and the Usage popover agree. Cost uses `cost_subcents` with `cost_sources` for provenance. A turn with `unpriced_count` greater than zero, or no cost, shows no cost.
The browser keeps an index of `UsageTurn` by turn ID per session in the app store. `listSessionUsageTurns` gains a cursor and limit; the index reads newest-first pages (server cap 100) until it covers the turns that have a rendered qualifying message or a page is empty, which sets a per-session `exhausted` flag. After exhaustion a turn absent from the index has no measurement and triggers no further fetch. Each demand reads a bounded number of pages. The existing `usageInvalidation` counter refreshes only the newest page. The window fetches the single-turn route for a turn absent from the index and not yet exhausted, and a 404 there means no measurement, not a failure.
TPS is `(direct.output_tokens + direct.thought_tokens) / (generation_ms / 1000)`. Child output runs concurrently on other threads, so it is excluded. The ledger adds thought tokens as their own term for ACP rows, so both are summed. It requires `UsageTurn.completeness` other than `estimated` or `partial` (ACP rows report `unknown`, which remains eligible only when `direct.output_complete` is true; native rows report `exact`), `direct.output_complete`, a nonzero numerator, and `generation_ms > 0`. Incomplete output means `direct.output_complete == false`; do not infer output incompleteness from the `unknown` turn-completeness label alone.
The generation window includes tool execution time, so TPS is an average turn output rate, not decode speed, and its label says so.
The window reasoning row uses `direct.thought_tokens`, or the sum of `reasoning_output_tokens` from the single-turn route when present.
No new backend read route is added, and no plugin or SDK surface changes.

### Setting contract

Add `message_metadata_fields` to the JSON user settings blob. Keep it beside `quick_chat_tab_order_by_workspace` and the other list settings in `user/models`, `user/store/sqlite.go` (`marshalUserSettingsPayload`, `scanUserSettings`), `user/dto`, `user/controller`, `user/service` (`applyBasicSettings`, `publishUserSettingsEvent`), `backendapp.mapUserSettingsState`, and the `settingscatalog` descriptors. No SQL column or migration is needed. Regenerate the settings discovery contracts (`contract.generated.json`, `profile-contract.generated.json`) with `go run ./cmd/settings-catalog`, which CI checks through `scripts/settings-contract-ci.test.mjs`.
The closed value set is `session_config`, `timestamp`, `duration`, `ttft`, `tps`, `tokens`, `cost`. The model default is `session_config`, `timestamp`, `duration`.
The store distinguishes an absent value (use the default) from an explicit empty list (hide everything). The update request carries `*[]string`, so a nil pointer leaves the value unchanged and an empty pointer clears it.
An update with an unknown or duplicate value is rejected with a field error that names the setting and leaves the stored value unchanged. A stored value with unknown entries, for example after a downgrade, drops them on read.
The boot payload exposes `messageMetadataFields`, and the existing settings event carries the snake_case key so other open clients update without reload.

### Presentation

`apps/web/lib/message-metadata-fields.ts` owns the value set, defaults, and a validator shared by the settings mapper and the components.
`MessageActions` renders each item only when the caller allows it (`showTimestamp`, `showModel`) or, for new items, when the user selected it and the message qualifies. The line order is fixed to the current order, then the new items: session configuration, timestamp, duration, TTFT, TPS, tokens, cost. It never follows selection order.
The renderer's adapter matchers live in a leaf module, `apps/web/components/task/chat/message-adapter-matchers.ts`, as an ordered table of predicates and `rendersActions` flags. `message-renderer.tsx` joins the table to its render functions in the same order; `lib/turn-metrics.ts` imports only the leaf matcher, avoiding a library-to-component dependency cycle. A message qualifies for TTFT, TPS, tokens, and cost when it is the last agent entry of a completed turn for which the adapter matcher says an action row renders. It is true when an agent-authored entry falls through to the catch-all adapter that renders `ChatMessage`, and false for a message with `metadata.actions`, a payload-retention marker, a task description, and the tool call, thinking, and script types. The existing `messageTurnDurationSeconds` rule keeps duration on user prompt messages. The line items render from a new `message-metadata-line.tsx`, not `message-actions.tsx`.
The qualifying-message check evaluates the user's metadata-field selection first, then checks that the turn is completed. If no usage-backed field is selected, it does no message-list scan. Otherwise it computes the last qualifying message ID once per `(messages array, turn)` with a memoized pass, so repeated `MessageActions` renders do not each scan the session history.
`MessageTurnMetrics` lives in `message-turn-metrics.tsx` and sits at the top of `MessageDebugDialog`, above the raw entries. The dialog gate also opens when the message's turn resolves, even if its metadata is empty and no usage exists, so derivable wall time remains accessible. A message-actions test covers an empty-metadata message with a completed turn. Its rows are wall time, TTFT, TPS, input, output, cached read, cached write, reasoning, and cost with its provenance label; all available values are shown regardless of the hover-line setting, and a pending usage read shows a pending row. The localized TTFT tooltip defines its interval as backend receipt of `turn_started` to first non-empty, non-diagnostic message or reasoning output received by the backend, explains that this is not prompt-dispatch latency and provider diagnostic/error text is ignored, and notes single-line buffering can delay the observation. The TPS label says it is an average turn rate including tool time. Cost is omitted while pending or when unpriced. The existing raw entries, including `turn_metadata.timing`, remain below it.

### Failure behavior

A failed usage read leaves the usage-backed items out of the line and shows an unavailable row in the window. It never blocks the transcript and never shows zero.
A failed settings save leaves the stored value unchanged, keeps the draft dirty, and shows an error.

### Metadata validation

Prove timing for a normal turn, a reasoning-first turn, a single-line response with no newline, a completion with no usage, a turn with no output, a cancelled turn, a restart between start and completion, and a repeated `turn_started`, and prove an open client receives `timing` without reload. Verify generation timing includes tool-use delay but excludes delay between turn close and complete-stream processing. Verify close marks correlate by session, execution, prompt generation, and turn ID; a ready mark survives a later terminal marker; regular and terminal complete-stream paths consume the matching mark once; and missing, expired, or mismatched marks omit `generation_ms` and TPS. Use controlled timestamps for ready close t1, terminal event t2, and stream processing t3 for both TurnID-bearing and compatibility payloads, and for the terminal-only close boundary. Test settings absent, explicit empty, unknown value rejected, and round trip with a boot-payload projection. Test TPS omission for estimated or partial completeness, incomplete direct output, zero-numerator, and zero-window turns; test `unknown` completeness with both complete and incomplete direct output, ACP thought-token summing, child output exclusion, and cost omission for an unpriced turn. Test the qualifying-message rule with several agent messages in one turn, a turn ending in a tool call, and a running turn. Test usage paging for a session longer than one page, empty-page exhaustion without repeat fetches, the per-demand page bound, newest-page-only invalidation refresh, and the single-turn 404 fallback.

## Validation

Prove two responses within one turn, duplicate replay, resume baselines, interrupted turns, late child usage, and fork history exclusion.
Test 1,000/600/200/80 breakdown arithmetic and mixed ACP/native aggregates.
Test exact zero, missing counts, unavailable prices, unknown currency, unknown tiers, and integer overflow.
Test both Office and task consumers against the same observation, with the plugin absent and Office disabled in the UI scenario.
Test authorization and stale response guards for every read route.
Desktop and mobile E2E cover detail access, pending/error states, reload, background activity, and provenance labels.
