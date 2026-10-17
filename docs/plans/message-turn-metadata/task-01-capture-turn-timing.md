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
- Write `timing.ttft_ms` and `timing.generation_ms` from its own call in `persistCompleteStreamOutput` and `flushTerminalCompleteStream`, outside the `Usage == nil` guard, only when both stamps exist, the turn was not cancelled (`stop_reason == cancelled`), the record matches the completing execution and captured turn ID plus mutually available prompt generation, and the record was not already consumed. The close stamp lives on `turnTimingRecord.completedAt`, set from the backend receipt timestamp captured immediately before the ready handler calls `completeTurnForSession`; `readyTurnMark` carries only `turnID` and `expiresAt`. `agent.completed` must not timestamp terminal-marker creation as turn completion: if it closes a still-open turn, stamp `completedAt` immediately before `completeTurnForSession`; if already set, preserve it. The record captures generation from the start event, first eligible output, or first matching close when available; generation-zero transports still require matching session, execution, and Kandev turn ID. Both complete-stream paths read it, even when the payload supplies `TurnID`, and calculate `generation_ms` from `completedAt`. A missing close stamp keeps `ttft_ms` and omits `generation_ms`; never substitute complete-stream or terminal-marker creation time. `readyTurnMark` expiry never affects `generation_ms`.
- Allow `timing` through `models.ProjectTurnMetadata` so v2 recovery/hydration and a later receipt-driven turn upsert preserve it (the turn DTO already passes metadata through, and `turn.updated` in task 06 delivers it).
- Consume the record exactly once in that write path: atomically match session, execution, captured Kandev turn ID, and mutually available prompt generation before reading and deleting it inside `handleAgentStreamEvent`. Mismatches preserve the successor's record. Remove the matching record there on error and cancel, and replace it on a repeated `turn_started`. Never clear it from `completeTurnForSession` or `agent.ready`.

## Out of scope

- Delivery of a patch that lands after `turn.completed` (task 06).
- Frontend display, TPS computation, and the user setting.
- Provider-reported timing and agentctl changes.

## Acceptance

1. A normal turn stores `ttft_ms` and `generation_ms` that match the injected clock, with `ttft_ms` measured from backend receipt of `turn_started` rather than prompt dispatch (`TestTurnTimingUsesTurnStartedReceipt`); a non-empty diagnostic candidate before real output does not stamp TTFT and the subsequent non-diagnostic message or reasoning does (`TestTurnTimingIgnoresDiagnosticBeforeOutput`). A reasoning-first turn stamps first output on the thinking event, a single-line response with no newline and a completion with no `Usage` still store timing, and a synthetic self-resume turn whose Kandev turn is created by its first message binds the timing record to that turn and stores timing (`TestTurnTimingLazilyCreatedTurn`).
2. A controlled-clock tool-use delay between first output and Kandev turn completion is included in `generation_ms` (`TestTurnTimingIncludesToolDelay`); delayed complete-stream processing does not extend it (`TestTurnTimingUsesTurnCompletionForGenerationEnd`). With ready close at t1, terminal event at t2, and stream processing at t3, the timestamp remains t1 for both TurnID-bearing and compatibility payloads (`TestTurnTimingKeepsReadyCloseAcrossTerminalMarker`). If no ready close occurred and the terminal path closes the active turn, its close timestamp is captured immediately before that close and used (`TestTurnTimingUsesTerminalCloseBoundary`). Mismatched session, execution, or turn ID records, and a missing close stamp, omit `generation_ms` and TPS (a missing close stamp keeps `ttft_ms`; `TestTurnTimingOmitsGenerationWithoutLiveCloseMark`). Both write paths read the matching record once and terminal handling preserves an existing close stamp. A turn with no output, a restart between start and completion, a cancelled turn, or a predecessor flush after a successor `turn_started` stores no `timing`, a second completion does not overwrite it, and a repeated `turn_started` does not reuse old stamps.
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

Correction (review rounds 24-26): timing records are correlated by session, execution and Kandev turn ID (turn_started carries no prompt generation on ACP transports). A missing close mark keeps `ttft_ms` and omits `generation_ms`. `Service.timingClock` is the injectable clock; handler-level `timingFlow` tests in `turn_timing_flow_test.go` cover receipt-to-ready measurement, ready close preserved across the terminal marker for TurnID and compatibility payloads, omitted generation without a close mark, and no-output turns. Not written under the work-order names: `TestTurnTimingUsesTurnStartedReceipt`, `TestTurnTimingIgnoresDiagnosticBeforeOutput`, `TestTurnTimingLazilyCreatedTurn`, `TestTurnTimingCorrelatesRegularAndTerminalCompletionPaths`, and the cancel, restart and second-completion cases; there is no close-mark expiry: `readyTurnMark` expiry never affects `generation_ms`, which depends only on `turnTimingRecord.completedAt` being set. Handler-level tests present in `turn_timing_flow_test.go`: `TestTurnTimingFlowMeasuresReceiptToReadyClose`, `TestTurnTimingKeepsReadyCloseAcrossTerminalMarker`, `TestTurnTimingOmitsGenerationWithoutLiveCloseMark`, `TestTurnTimingFlowOmitsTurnWithoutOutput`.

Round 3 adversarial-review correction (2026-10-13): timing mutations now reject stale prompt generations before first-output capture, turn-start capture, or error discard, while retaining compatibility for transports without generation or turn identifiers. Discard additionally matches the timing record's execution and optional event turn. `TestTurnTimingStalePromptFramesPreserveSuccessor` reproduced all four stale-frame cases before the change and verifies the successor's own TTFT and generation interval afterward. The full orchestrator package passed with the race detector; scoped orchestrator/DTO golangci-lint reported zero issues.

Round 4 adversarial-review correction (2026-10-13): both complete-stream paths pass immutable event ownership into timing consumption. A single mutex-protected match checks session, execution, captured turn ID, and mutually available prompt generation before deletion, preserving a successor record when a late predecessor shares its turn ID. Generation is captured at start, first eligible output, or first matching close, retaining generation-zero compatibility. Five regression cases reproduced premature persistence before the corresponding corrections and passed afterward: execution replacement before/after successor close, and generation first available at start/output/ready close. The final timing/terminal regressions and full orchestrator package passed with the race detector; owned model-projection and gateway tests, vet, scoped zero-issue golangci-lint, and specification checks passed. A disposable smoke used real turn/message services, SQLite, handlers, and serialized `turn.updated` publication: no predecessor timing update, then one successor event carrying TTFT 350 ms and generation 1500 ms, with duplicates emitting no second timing update. The smoke used controlled lifecycle/clock fixtures, not an external provider process or browser; its source and owned temp directory were removed.
