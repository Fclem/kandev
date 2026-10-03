---
created: 2026-10-03
status: draft
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-005
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
legacy_specs: []
---

# Implementation plan: Message and turn metadata

## Overview

Show timing, tokens, and cost for a turn from its messages. The metadata window shows every available value by default. A new per-user setting selects which items the hover metadata line shows. The costs system owns this because it extends the built-in conversation usage display.

Order: capture TTFT timing on the backend and add the setting end to end in parallel (disjoint files), then deliver late timing to open clients, add the settings editor, the metrics projection with the window section, and finally the hover line and E2E. Tokens and cost reuse the ledger read routes, so no accounting change is needed.

Settled choices: the setting gates display only (data is always collected); TTFT and TPS are backend-measured; cost is the ledger cost only with no plugin contract; new line items default off and the existing items default on.

## Scope

### In scope

- Per-session turn timing capture and `timing` turn metadata.
- User setting `message_metadata_fields` across store, service, DTO, controller, boot payload, web state, and a settings editor.
- Turn metrics section in the message metadata window and the configurable hover line.
- Locale strings in all seven catalogs, and public docs for the setting and metric semantics (task 05) plus the session-scoped WebSocket event (task 06).

### Out of scope

- Plugin-supplied cost, provider-reported TTFT or TPS, and an instance runtime flag.
- Changes to ledger arithmetic, pricing, or usage read routes.
- Backfilling timing for turns completed before this change.

## Technical approach

Backend timing: a per-session record (keyed by Kandev turn ID) in `internal/orchestrator`, set in `handleAgentStreamEvent` on `EventTypeTurnStarted` and the first non-empty, non-diagnostic `message_streaming`/`thinking_streaming` event, and consumed once by its own write from `persistCompleteStreamOutput`/`flushTerminalCompleteStream` outside the usage guard and without requiring an active turn. `ttft_ms` uses backend receipt time for those events; `generation_ms` ends at the close timestamp on the matching `readyTurnMark`, not terminal-marker creation or delayed complete-stream processing time. The ready mark is captured immediately before the actual `completeTurnForSession` close; a terminal close captures the mark at its own close boundary only when no ready mark already exists. Payload `TurnID` never skips close-mark validation; unmatched marks result in no `generation_ms`/TPS. `models.ProjectTurnMetadata` allows `timing`. Because lifecycle closes the turn before the complete-stream frame, the patch normally lands after `turn.completed`, so task 06 is the normal delivery path: `events.TurnUpdated` from `task.Service.PatchTurnMetadata` whenever `timing` is patched (re-read for a newer `updated_at`, sanitized like `turn.completed`), a gateway case using `BroadcastToSession` only, and a `session.turn.updated` web handler.

Setting: `message_metadata_fields` follows the existing list settings. Absent means default, an explicit empty list means none, unknown values are rejected on write and dropped on read. Boot key `messageMetadataFields`.

Web: `lib/message-metadata-fields.ts` (values, defaults, validator), a `lib/turn-metrics.ts` projection (TPS, formatting, qualifying-message rule via the ordered matcher table in `components/task/chat/message-adapter-matchers.ts`), a store-backed usage turn index with an `exhausted` flag fed by a cursor-paged `listSessionUsageTurns`, `MessageTurnMetrics` in `message-turn-metrics.tsx`, hover-line items in a new `message-metadata-line.tsx`, and a checkbox group on the Conversation tab of `task-behavior-settings.tsx`.

| Item | Source | Shown on | Omitted when |
| --- | --- | --- | --- |
| Session config, timestamp | message and turn metadata | every message (existing) | unchanged |
| Duration | `Turn.StartedAt`/`CompletedAt` | user prompt (existing) | unchanged |
| TTFT | `turn.metadata.timing.ttft_ms` | last agent message with an action row, completed turn | no timing recorded or cancelled |
| TPS | direct output + thought tokens, `timing.generation_ms` | same | estimated, partial, incomplete, zero output, or zero window |
| Tokens | `UsageTurn.total` | same | usage pending or failed |
| Cost | `UsageTurn.total.cost_subcents` | same | unpriced or pending |

## ASCII UI preview

UI-01: Hover metadata line and metadata window. Bracketed text is an annotation.

```text
Desktop, agent message hover line, with ttft, tps, tokens, cost selected
  [copy] [raw] [info]  gpt-5.6-sol  2m ago  TTFT 0.8s  42 tok/s  1.2k in / 380 out / 600 cached  $0.012 (est.)

Desktop, same message, default selection
  [copy] [raw] [info]  gpt-5.6-sol  2m ago

Metadata window (always full, no setting)
  Message metadata
  Turn metrics
    Wall time        12s
    TTFT             0.8s
    TPS              42 tok/s (average turn rate)
    Input            1,000
    Output           380   (reasoning 80)
    Cached read      600
    Cost             $0.012  calculated estimate
  MESSAGE_ID ...
  (raw entries continue)

Settings, Task behavior, Message metadata
  [x] Model and session options   [x] Timestamp   [x] Duration
  [ ] TTFT        [ ] Tokens per second          [ ] Tokens   [ ] Cost

Phone: the line wraps within the message column; the window opens as the existing dialog;
settings checkboxes stack with 44px targets.
```

Maps to `AC-COSTS-CONVERSATION-USAGE-005.1`, `.2`, `.4`, `.7`.

## Tests

- `AC-005.5`: `internal/orchestrator/turn_timing_test.go` (`TestTurnTiming*`, including `TestTurnTimingIncludesToolDelay`, `TestTurnTimingUsesTurnCompletionForGenerationEnd`, `TestTurnTimingKeepsReadyCloseAcrossTerminalMarker`, `TestTurnTimingUsesTerminalCloseBoundary`, `TestTurnTimingRejectsMismatchedCloseMark` (session, execution, prompt generation, turn ID), `TestTurnTimingOmitsGenerationWithoutLiveCloseMark` (missing/expired), and `TestTurnTimingCorrelatesRegularAndTerminalCompletionPaths`), `lib/turn-metrics.test.ts` (child output exclusion and `unknown` completeness with complete/incomplete direct output), and `hooks/domains/session/use-session-usage-turns.test.ts` (paging coverage, exhaustion, demand bounds, invalidation refresh, and single-turn 404 fallback).
- `AC-005.2`, `.3`: `internal/user/store/message_metadata_fields_test.go`, `internal/user/service/service_test.go`, `internal/backendapp/boot_state_user_settings_test.go`, `lib/ssr/user-settings.test.ts`, `lib/message-metadata-fields.test.ts`.
- `AC-005.1`, `.4`, `.6`: `lib/turn-metrics.test.ts`, `components/task/chat/messages/message-actions.test.tsx`, a new `message-turn-metrics.test.tsx`, `internal/task/service/service_turns_test.go` (a patched `timing` publishes `turn.updated`, other keys do not), a gateway test that `turn.updated` emits no `session.conversation.changed`, and `lib/ws/handlers/turns.test.ts`.
- `AC-005.7`: `components/settings/message-metadata-settings.test.tsx`.

## E2E tests

- `apps/web/e2e/tests/chat/message-metadata.spec.ts` (Chromium): default line unchanged, window shows metrics, enabling items in settings adds them to the last agent message after reload and on an open client without reload, empty selection hides optional items, and a failed save leaves the stored selection unchanged (`AC-005.1` to `.4`, `.7`). A live spec using the mock agent's slow-model delay asserts TTFT appears on an open page with no reload, as the non-stubbed proof of `AC-005.4`. Per-test reset of `message_metadata_fields` is added to the e2e fixtures.
- `apps/web/e2e/tests/chat/mobile-message-metadata.spec.ts` (mobile Chrome project): settings group reachable with 44px targets and line wraps without horizontal overflow (`AC-005.7`).

## Work orders

- [ ] [Task 01: Capture turn timing](task-01-capture-turn-timing.md)
- [ ] [Task 06: Deliver late timing updates](task-06-deliver-late-timing.md)
- [ ] [Task 02: Message metadata fields setting](task-02-message-metadata-setting.md)
- [ ] [Task 03: Settings editor](task-03-settings-editor.md)
- [ ] [Task 04: Turn metrics and metadata window](task-04-turn-metrics-window.md)
- [ ] [Task 05: Configurable hover line](task-05-hover-line.md)

Dependency order: 01 and 02 are parallel-safe, then 03 (needs 02) and 06 (needs 01), 04 (needs 01; also sequenced after 03 because both regenerate the pseudo locale and `_verbatim.json`), 05 (needs 02, 04 and 06).

## Verification results

Pending.

## Risks

- TTFT is a backend-observed interval from receipt of `turn_started` to receipt of the first non-empty, non-diagnostic message or reasoning output; provider diagnostic/error text is ignored. The start excludes delay before the lifecycle event reaches the backend; the output receipt includes event delivery latency, so this is not uniformly an upper or lower bound on provider TTFT. Single-line replies may be observed late because lifecycle buffers ID-less chunks until a newline.
- A last-agent-message rule needs the session's message list in the store; a message arriving after render can move the line. The selector must be derived from store state, not render order.
- ACP usage completeness is `unknown`; it remains eligible for TPS only when `direct.output_complete` is true. Estimated and partial usage are excluded.
- Seven locale catalogs per copy change; the zh-Hant pair is generated.
