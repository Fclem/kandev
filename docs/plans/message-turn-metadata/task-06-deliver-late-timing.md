---
id: "06-deliver-late-timing"
title: "Deliver late timing updates"
status: done
wave: 2
depends_on:
  - "01-capture-turn-timing"
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-005
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-005.4
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 06: Deliver late timing updates

## Summary

Lifecycle closes the turn on `agent.ready` before the complete-stream frame, so the `timing` patch normally lands after `turn.completed`. Publish a turn update whenever a `timing` patch lands, so open clients receive it without a reload; this is the normal delivery path.

## In scope

- `events.TurnUpdated` (`turn.updated`) and `ws.ActionSessionTurnUpdated` in `apps/backend/pkg/websocket/actions.go`.
- `task.Service.PatchTurnMetadata` publishes it only when the patched keys include `timing`, whether or not the turn is still active: re-read with `GetTurn` after the patch so the event carries the new `updated_at`, and publish through `publishTurnEvent(events.TurnUpdated, turn, nil)` so metadata is sanitized exactly as for `turn.completed`.
- Gateway case in `task_notifications.go` that calls only `BroadcastToSession`, never `BroadcastConversationMutation`.
- `session.turn.updated` handler in `apps/web/lib/ws/handlers/turns.ts` applying `addTurn` only, with a `BackendMessage<'session.turn.updated', TurnEventPayload>` entry in `lib/types/session-events.ts`. List `session.turn.updated` under the session lifecycle event names in `docs/public/websocket-api.md` and document that lifecycle updates are session-subscriber scoped, not globally broadcast.
- The action is not added to `CORE_CONVERSATION_ACTIONS` or `ORDERED_EVENT_ACTIONS`.

## Out of scope

- Capturing or writing `timing` (task 01), changes to the orchestrator `TurnService` interface, and re-publishing `TurnCompleted`.

## Acceptance

1. A `timing` patch publishes one `turn.updated` carrying a strictly newer `updated_at` and metadata sanitized exactly as `turn.completed`, for both a completed and a still-active turn; patches of other keys (prompt usage, native turn id, error terminated) publish nothing.
2. The gateway delivers it to the session's subscribers and emits no `session.conversation.changed`.
3. The web store applies it to the existing completed turn without active-turn or empty-notice side effects.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service/ ./internal/gateway/websocket/ ./pkg/websocket/ -count=1)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/turns.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/backend/internal/events/types.go`, `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/task/service/service_turns.go` and `service_turns_test.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go` and its test
- `apps/web/lib/ws/handlers/turns.ts`, `lib/types/session-events.ts`, `turns.test.ts`
- `docs/public/websocket-api.md`

## Dependencies

Task 01, which supplies the timing patch this event delivers.

## Risks

- A metadata patch advances the session conversation revision without a receipt; v2 recovery/hydration therefore can encounter `timing` even without a conversation broadcast. The gateway test must assert the update itself is session-scoped and emits no `session.conversation.changed`.
- `shouldApplyTurnUpdate` ignores an equal or older `updated_at`, so a stale snapshot would be silently dropped.

## Parallelism

`sequential`

## Inputs

- Design section "Turn timing capture".
- `publishTurnEvent` and the `session.turn.completed` path in gateway and web.

## Results

Results: Timing metadata patches now publish sanitized `turn.updated` snapshots after re-reading the persisted turn; the gateway routes them only to session subscribers, and the web handler updates the turn without completion side effects. The three backend packages, all 24 turn-handler tests, and web typecheck passed. `docs/public/websocket-api.md` already lists this lifecycle event and its session-only delivery scope, so it required no edit.
