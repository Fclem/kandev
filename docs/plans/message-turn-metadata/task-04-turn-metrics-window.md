---
id: "04-turn-metrics-window"
title: "Turn metrics and metadata window"
status: pending
wave: 3
depends_on:
  - "01-capture-turn-timing"
  - "03-settings-editor"
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-005
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-005.1
  - AC-COSTS-CONVERSATION-USAGE-005.5
  - AC-COSTS-CONVERSATION-USAGE-005.6
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 04: Turn metrics and metadata window

## Summary

Build the turn metrics projection and show it at the top of the message metadata window with no setting. Provide the store-backed usage turn index that the hover line reuses.

## In scope

- `lib/turn-metrics.ts`: TPS rule (direct output plus thought tokens; reject estimated/partial completeness, incomplete direct output, zero output, or zero generation window; allow completeness `unknown` only when direct output is complete), formatting, and the qualifying-message rule using the ordered matcher table in `components/task/chat/message-adapter-matchers.ts` (shared by `message-renderer.tsx`, imported directly by this library; no dependency on the renderer component module).
- Cursor-paged `listSessionUsageTurns` and a store-backed per-session `UsageTurn` index that covers the rendered range, sets an `exhausted` flag on an empty page (a turn with no ledger rows then has no measurement and triggers no further fetch), bounds pages per demand, and refreshes only the newest page on `usageInvalidation`, with a single-turn fallback for the window where 404 means no measurement.
- `MessageTurnMetrics` in `message-turn-metrics.tsx`, rendered inside the existing scrolling entries container of `MessageDebugDialog` (not as a second direct grid child, which would break `message-metadata-overflow.spec.ts`), with pending and unavailable rows, localized in all seven catalogs. Its dialog gate passes when the turn resolves even if message/turn metadata and usage are empty, so wall time remains accessible.

## Out of scope

- The hover line and the setting-driven selection (task 05).
- Usage routes, ledger, or pricing changes.

## ASCII UI preview

UI-01 window excerpt from the [full preview](plan.md#ascii-ui-preview), `AC-COSTS-CONVERSATION-USAGE-005.1`.

```text
Message metadata
  Turn metrics: Wall time, TTFT, TPS, Input, Output (reasoning), Cached read/write, Cost + provenance
  raw entries below
```

## Acceptance

1. The window shows every available turn value with no setting, even when message and turn metadata are empty, and omits values with no measurement; pending or unavailable usage appears as a status row instead of zero. `message-actions.test.tsx` covers opening the dialog for an empty-metadata message with a resolved completed turn.
2. TPS uses only `direct.output_tokens + direct.thought_tokens`, excludes nonzero child output, and divides by the generation interval that includes tool execution time. It is omitted for estimated or partial completeness, incomplete direct output (`output_complete == false`), zero-numerator, and zero-window turns; `unknown` completeness remains eligible when direct output is complete. Tests cover child output exclusion and both `unknown` plus complete output and `unknown` plus incomplete output. ACP thought tokens are summed into the numerator, and cost is omitted for unpriced turns.
3. Opening the window never blocks the transcript when the usage read fails.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/turn-metrics.test.ts hooks/domains/session/use-session-usage-turns.test.ts components/task/chat/messages/message-turn-metrics.test.tsx components/task/chat/messages/message-actions.test.tsx)
(cd apps/web && pnpm e2e:run e2e/tests/chat/message-metadata-overflow.spec.ts e2e/tests/chat/message-metadata-turn-resolution.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/chat/mobile-message-metadata-overflow.spec.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check)
(cd apps/web && pnpm run typecheck && pnpm run lint)
```

## Files likely touched

- `apps/web/lib/turn-metrics.ts` and test (new)
- `apps/web/hooks/domains/session/use-session-usage-turns.ts` and `use-session-usage-turns.test.ts` (new; pagination to the rendered-turn range, empty-page exhaustion/no repeat fetch, per-demand page bound, invalidation refreshing only the newest page, and single-turn 404 fallback)
- `apps/web/lib/api/domains/conversation-usage-api.ts` (cursor and limit), usage turn index state
- `apps/web/components/task/chat/message-adapter-matchers.ts` (new leaf matcher table) and `message-renderer.tsx` (join render functions to matcher order), `messages/message-turn-metrics.tsx` (new), and the `MessageDebugDialog` mount in `messages/message-actions.tsx` (smallest possible change)
- `apps/web/src/locales/*/task.json`

## Dependencies

Tasks 01 and 03. Task 03 is a dependency only to sequence the shared pseudo locale and `_verbatim.json` edits.

## Risks

- `message-actions.tsx` is near the TypeScript size limits; put new code in new files.
- Paging a long session must stay bounded; fetch only when needed.

## Parallelism

`sequential`

## Inputs

- Design section "Tokens, cost, and TPS projection".
- `useConversationUsage` and `lib/utils/conversation-usage.ts`.

## Results

Pending.
