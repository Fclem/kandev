---
id: "05-hover-line"
title: "Configurable hover line"
status: done
wave: 4
depends_on:
  - "02-message-metadata-setting"
  - "04-turn-metrics-window"
  - "06-deliver-late-timing"
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-005
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-005.2
  - AC-COSTS-CONVERSATION-USAGE-005.3
  - AC-COSTS-CONVERSATION-USAGE-005.4
  - AC-COSTS-CONVERSATION-USAGE-005.7
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 05: Configurable hover line

## Summary

Make the message hover line render the items the user selected, in a fixed order, and add TTFT, TPS, tokens, and cost on the last agent message of a completed turn. The default selection must leave the line unchanged. Add public documentation for the setting, metadata window, and displayed metric semantics.

## In scope

- Gate session configuration, timestamp, and duration by the setting in addition to the existing caller props, with the line-item rendering in a new `message-metadata-line.tsx` so `message-actions.tsx` stays within its lint limits (net-neutral).
- Render TTFT, TPS, tokens, and cost items using the task 04 projection and usage index, fetching usage only when a usage-backed item is selected. Evaluate the setting first, then turn completion; if no usage-backed item is selected, skip message-list scanning. Memoize the last qualifying message ID per `(messages array, turn)` so repeated message action renders reuse one scan. Provide a localized TTFT tooltip that defines the backend-observed `turn_started` receipt as its origin, excludes provider diagnostic/error text, distinguishes this interval from prompt-dispatch latency, and explains the single-line buffering caveat.
- Playwright desktop and mobile coverage. Display specs use route-fulfilled turn and usage-turn responses for deterministic `timing` and usage, because the mock agent replies instantly. One live spec uses the mock agent's slow-model delay tier to send a prompt and assert the TTFT item appears on the last agent message of an open page with no reload, which proves the real orchestrator write, `turn.updated`, gateway and store path (`AC-005.4`).

## Out of scope

- The settings editor and window section (tasks 03 and 04).

## ASCII UI preview

UI-01 line excerpt from the [full preview](plan.md#ascii-ui-preview), `AC-COSTS-CONVERSATION-USAGE-005.2` and `.4`.

```text
default:   [copy] [raw] [info]  gpt-5.6-sol  2m ago
selected:  [copy] [raw] [info]  gpt-5.6-sol  2m ago  TTFT 0.8s  42 tok/s  1.2k in / 380 out / 600 cached  $0.012 (est.)
```

## Acceptance

1. With the default setting, existing `message-actions` behavior and tests are unchanged, and no usage request is made.
2. Selected new items appear in fixed order (session configuration, timestamp, duration, TTFT, TPS, tokens, cost) only on the last agent entry with an action row of a completed turn, never while the turn runs, on earlier messages, or on tool-call entries, and an empty selection hides every selectable item.
3. A change in settings applies after reload and on another open client, and the line wraps on a phone viewport without horizontal overflow.
4. A live mock-agent spec (non-scenario prompt, mock-slow profile) shows the TTFT item on the last agent message of an open page with no reload. It asserts TTFT only, never TPS, because a one-line reply is flushed at completion and its generation window is about zero.
5. The TTFT tooltip appears on hover and keyboard focus, is available in all seven locales, and explains the observed turn-start origin, diagnostic filtering, and buffering caveat.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/messages/message-actions.test.tsx lib/turn-metrics.test.ts)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run e2e/tests/chat/message-metadata.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/chat/mobile-message-metadata.spec.ts)
```

## Files likely touched

- `apps/web/components/task/chat/messages/message-actions.tsx` (net-neutral), `message-metadata-line.tsx` (new) and tests
- `apps/web/e2e/tests/chat/message-metadata.spec.ts`, `mobile-message-metadata.spec.ts` (new), a per-test `message_metadata_fields` reset in both places of `e2e/fixtures/test-base.ts` (testPage fixture and global beforeEach) and its typing in `e2e/helpers/api-client.ts`
- `apps/web/src/locales/*/task.json`, `docs/public/configuration.md` (Task Behavior > Conversation setting and metric semantics), and `components/task/chat/messages/message-actions.test.tsx` (TTFT tooltip behavior)

## Dependencies

Tasks 02, 04 and 06. The settings PATCH can only set an explicit list, so the default-line E2E exercises the explicit default; the absent case is covered by store tests.

## Risks

- A new agent message can move the "last agent message" while the line is visible; derive it from store state.
- The E2E mock agent must emit usage and pricing to exercise cost; check `e2e/fixtures` before adding a seeding helper.

## Parallelism

`sequential`

## Inputs

- Design section "Presentation".
- `conversation-usage.spec.ts` and its helpers.

## Results

Implemented configurable hover metadata line, seven-locale TTFT help, selective turn usage items, mobile wrapping, and public configuration documentation. Verification: desktop metadata E2E (3/3, including live TTFT), mobile metadata E2E (1/1), focused Vitest (35/35), web typecheck, lint, i18n checks, and Prettier passed.
