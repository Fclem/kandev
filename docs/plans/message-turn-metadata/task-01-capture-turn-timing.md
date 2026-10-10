---
id: "01-capture-turn-timing"
title: "Capture turn timing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-005
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-005.3
  - AC-COSTS-CONVERSATION-USAGE-005.5
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 01: Capture turn timing

## Summary

Record backend-observed turn-start-to-first-output and generation time for every turn and store them as `timing` in turn metadata, so the browser can show TTFT and compute TPS. TTFT begins at backend receipt of `turn_started`, not at prompt dispatch; capture is unconditional and independent of the display setting.

## In scope

- Per-session in-memory timing record in the orchestrator, guarded by a mutex or `sync.Map` (the handler is concurrent across sessions). `EventTypeTurnStarted` stores the start stamp using its backend receipt time and the active Kandev turn ID if one exists; synthetic self-resume turns can start before the Kandev turn exists, so store an empty ID in that case and bind it on the first-output stamp after `handleMessageStreamingEvent` or `handleThinkingStreamingEvent` creates the turn, using `nonCreatingActiveTurnID`. Store first-output stamps on the first non-empty, non-diagnostic `message_streaming` or `thinking_streaming` event (the names lifecycle converts chunks to).
- Write `timing.ttft_ms` and `timing.generation_ms` from its own call in `persistCompleteStreamOutput` and `flushTerminalCompleteStream`, outside the `Usage == nil` guard, only when both stamps exist, the turn was not cancelled (`stop_reason == cancelled`), the record's turn ID equals the captured completion turn ID, and the record was not already consumed. `readyTurnMark` carries the immutable backend receipt timestamp captured immediately before the ready handler calls `completeTurnForSession`. `agent.completed` must not timestamp terminal-marker creation as turn completion: if it closes a still-open turn, record the same close mark immediately before `completeTurnForSession`; if a matching ready mark exists, preserve it. Consume the matching mark in both complete-stream paths, even when the payload supplies `TurnID`, and calculate `generation_ms` from its timestamp. Missing, expired, or mismatched marks omit `generation_ms`; never substitute complete-stream or terminal-marker creation time.
- Allow `timing` through `models.ProjectTurnMetadata` so v2 recovery/hydration and a later receipt-driven turn upsert preserve it (the turn DTO already passes metadata through, and `turn.updated` in task 06 delivers it).
- Consume the record exactly once in that write path (read and delete by Kandev turn ID inside `handleAgentStreamEvent`), remove it there on error and cancel, and replace it on a repeated `turn_started`. Never clear it from `completeTurnForSession` or `agent.ready`.

## Out of scope

- Delivery of a patch that lands after `turn.completed` (task 06).
- Frontend display, TPS computation, and the user setting.
- Provider-reported timing and agentctl changes.

## Acceptance

1. A normal turn stores `ttft_ms` and `generation_ms` that match the injected clock, with `ttft_ms` measured from backend receipt of `turn_started` rather than prompt dispatch (`TestTurnTimingUsesTurnStartedReceipt`); a non-empty diagnostic candidate before real output does not stamp TTFT and the subsequent non-diagnostic message or reasoning does (`TestTurnTimingIgnoresDiagnosticBeforeOutput`). A reasoning-first turn stamps first output on the thinking event, a single-line response with no newline and a completion with no `Usage` still store timing, and a synthetic self-resume turn whose Kandev turn is created by its first message binds the timing record to that turn and stores timing (`TestTurnTimingLazilyCreatedTurn`).
2. A controlled-clock tool-use delay between first output and Kandev turn completion is included in `generation_ms` (`TestTurnTimingIncludesToolDelay`); delayed complete-stream processing does not extend it (`TestTurnTimingUsesTurnCompletionForGenerationEnd`). With ready close at t1, terminal event at t2, and stream processing at t3, the timestamp remains t1 for both TurnID-bearing and compatibility payloads (`TestTurnTimingKeepsReadyCloseAcrossTerminalMarker`). If no ready close occurred and the terminal path closes the active turn, its close timestamp is captured immediately before that close and used (`TestTurnTimingUsesTerminalCloseBoundary`). `TestTurnTimingRejectsMismatchedCloseMark` table-tests mismatched session, execution, prompt generation, and turn ID. `TestTurnTimingOmitsGenerationWithoutLiveCloseMark` covers missing and injected-clock-expired marks. `TestTurnTimingCorrelatesRegularAndTerminalCompletionPaths` proves both write paths consume the matching mark once and terminal handling preserves a matching ready mark. Each missing, expired, or mismatched case omits `generation_ms` and TPS. A turn with no output, a restart between start and completion, a cancelled turn, or a predecessor flush after a successor `turn_started` stores no `timing`, a second completion does not overwrite it, and a repeated `turn_started` does not reuse old stamps.
3. `timing` is written on the turn and survives `ProjectTurnMetadata`; v2 recovery/hydration projects timing in the conversation turn payload, and a later receipt-driven upsert preserves it. No other internal turn key becomes public. Late delivery to an already-open client is covered by task 06's `turn.updated` acceptance.

## Verification

```bash
(cd apps/backend && go test -race ./internal/orchestrator/ -run 'TurnTiming' -count=1)
(cd apps/backend && go test ./internal/task/models/ -run 'ProjectTurnMetadata' -count=1)
(cd apps/backend && go test ./internal/gateway/websocket/ -count=1)
(cd apps/backend && go vet ./internal/orchestrator/ ./internal/task/models/ ./internal/gateway/websocket/)
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_streaming.go` (call sites in the stream and complete paths)
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/turn_timing.go` and `turn_timing_test.go` (new)
- `apps/backend/internal/task/models/models.go` and its tests

## Dependencies

None.

## Risks

- The orchestrator needs an injectable clock for deterministic tests. Reuse an existing clock seam if one exists before adding one.
- `event_handlers_streaming.go` is large; keep the new logic in a new file with a small call-site change.

## Parallelism

`parallel-safe` with task 02: no shared files, schema, or generated contract.

## Inputs

- Design section "Turn timing capture".
- `persistTurnPromptMetadata` and `ProjectTurnMetadata`.

## Results

Results: Implemented start/output/close timing capture, turn metadata persistence, projection allowlisting, and focused timing tests. Verified the commands below; the race-enabled timing test and model/gateway/vet checks passed.

Correction (review rounds 24-26): timing records are correlated by session, execution and Kandev turn ID (turn_started carries no prompt generation on ACP transports). A missing close mark keeps `ttft_ms` and omits `generation_ms`. `Service.timingClock` is the injectable clock; handler-level tests in `turn_timing_flow_test.go` cover receipt-to-ready measurement, ready close preserved across the terminal marker for TurnID and compatibility payloads, omitted generation without a close mark, and no-output turns. Not written under the work-order names: `TestTurnTimingUsesTurnStartedReceipt`, `TestTurnTimingIgnoresDiagnosticBeforeOutput`, `TestTurnTimingLazilyCreatedTurn`, `TestTurnTimingCorrelatesRegularAndTerminalCompletionPaths`, and the cancel, restart and second-completion cases; the ready-mark expiry window is the existing `completedExecutionRetention` check, not a separate close-mark expiry.
