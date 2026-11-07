---
id: "01-ended-queue-inspection"
title: "Complete ended-session timestamp inspection"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001
acceptance_criteria:
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.1
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.2
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.3
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.4
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.5
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.6
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.7
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.8
system_design:
  - ../../specs/ui/system-design/queued-message-timestamps.md
---

# Task 01: Complete ended-session timestamp inspection

## Summary

Prove existing retained-queue access after a managed session ends, then fix the actual presentation gaps: all-age relative labels and keyboard-accessible desktop details. Preserve saved formatting, transcript defaults, and original server ingestion time.

## In scope

- Establish the current `ChatInputArea` -> `QueueAffordance` -> `ChatInputContainer` composition as baseline. Do not relocate or duplicate its queue owner or require a failing stopped-banner-only test.
- Add behavioral RED for queue-relative labels at/after seven days and visible keyboard details. Implement the queue-scoped formatter/component context described in the design; keep transcript callers unchanged. Inspect exported-symbol references before signature changes.
- Extend existing desktop and mobile browser flows for the real completed-session surface, historical identity, timestamp preservation, and details focus/dismissal.
- Update the public queue reference and record actual verification results here and in the plan.

## Out of scope

Backend schema, retention, new settings, transcript changes, automatic delivery, merge-policy changes, and missing-history reconstruction.

## Acceptance

1. The selected ended session exposes one retained queue without recovery/startup; a distinct active sibling's queue does not replace it. Existing visibility may already pass before implementation.
2. Queue-relative mode shows age below, at, and beyond seven days; both absolute modes retain saved formatting and relative details. Transcript seven-day formatting is unchanged. Reopening/reload preserve original IDs/content/queued_at and COMPLETED state without runtime startup.
3. Real desktop Tab/Enter and phone tap reveal visible counterpart details from the timestamp line. Escape dismisses details only and returns focus. Phone and narrow fine-pointer layouts retain 44px targets and contained rows; existing merge behavior stays green.

## ASCII UI preview

### UI-01: Ended task, expanded queue (desktop and phone)

```text
[Queued messages (2) v]
  #1 [sender] Older pending content [actions]
              last week
  #2 [sender] Recent content        [actions]
              5 minutes ago
[Session completed]       [Resume] [New agent]
```

### UI-02: Timestamp details (desktop popover / phone drawer)

```text
+----------------------------------+
| Message time                     |
| November 2, 2026 at 10:30:00 AM    |
+----------------------------------+
```

See [full preview and annotations](plan.md#ascii-ui-preview). Applies to AC .1, .3, .5-.8. The timestamp line opens an anchored desktop Popover via click or Enter/Space and a phone/coarse-pointer Drawer via tap; both return focus on dismissal without closing the queue. Phone layout also applies at narrow fine-pointer widths. Shared content order, wrapping rows, existing queue scroller, no extra icon, and contained touch targets are structural. Text is illustrative, localized, and preference-dependent. No new action permissions.

## Verification

Run from repository root. Install dependencies once in a fresh worktree. Follow `/tdd`, `/e2e`, and `/mobile-parity`. First record the completed-session visibility baseline without production changes. For the new formatting/disclosure behavior capture expected-assertion RED and GREEN; do not manufacture RED by mocking out the real queue wrapper or checking a missing test ID.

```bash
(cd apps && pnpm install --frozen-lockfile)
make fmt
(cd apps/web && pnpm exec vitest run lib/i18n/message-time.test.ts components/task/chat/queued-ghost-message-timestamp.test.tsx components/task/chat/use-queue-panel-escape.test.tsx components/task/chat/messages/message-actions.test.tsx)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --project=chromium e2e/tests/chat/queued-message-timestamps.spec.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --project=mobile-chrome e2e/tests/chat/mobile-queued-message-timestamps.spec.ts)
make typecheck test lint
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner rebuilds web/backend; do not test stale assets or override worker limits. Assert discovery of both intended projects. Inspect actual captures against UI-01/UI-02; merely calling `prCapture` without `CAPTURE_PR_ASSETS=1` is not capture evidence. Run a 390px fine-pointer case in the desktop file as well as the configured phone project.

Seed with existing `seedTimestampQueue`, capture the queue, and update the SAME session to COMPLETED with explicit `sessionId` and `completedAt`. Disable auto-run and auto-merge. Add a distinct active sibling queue and select the historical session through real task UI; `openTimestampQueue` currently opens a task's default session and may need a selected-session helper. Do not archive/delete or start a runtime to seed this retained state.

Use fixed `now` for formatter boundary tests and `testPage.clock.setFixedTime` before browser navigation to age captured server timestamps by at least eight days. Do not freeze timers or backdate storage. Exercise relative mode plus both absolute modes and restore settings. Compare persisted IDs/content/queued_at and session/runtime state after details, navigation and reload. Observe outgoing mutations before inspection. Test desktop with real Tab/Enter, not programmatic focus or title-only checks, then Escape/focus return. Preserve existing merge tests. Expected consumer labels must not come from the formatter under test.

## Files likely touched

- `apps/web/lib/i18n/message-time.ts`
- `apps/web/lib/i18n/message-time.test.ts`
- `apps/web/components/task/chat/messages/message-timestamp.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message-timestamp.test.tsx`
- `apps/web/components/task/chat/use-queue-panel-escape.test.tsx`
- `apps/web/e2e/tests/chat/queued-message-timestamps.spec.ts`
- `apps/web/e2e/tests/chat/mobile-queued-message-timestamps.spec.ts`
- `apps/web/e2e/tests/chat/queued-message-timestamps.helpers.ts`
- `docs/public/operations.md`

## Dependencies

None. Existing queue timestamp implementation is already present.

## Risks

Queue versus transcript context drift; selected historical versus active sibling identity; duplicated queue subscriptions; recovery side effects; Escape closing the queue before details; tests deleting a queue to seed completion. Read `chat-input-area.tsx`, `queued-ghost-list.tsx`, and `use-queue.ts` as composition inputs, not mandatory production changes. Existing visibility can pass before changes, while old-age and keyboard failures provide the real RED gates.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/queued-message-timestamps.md).
- [Design extension](../../specs/ui/system-design/queued-message-timestamps.md#ended-task-inspection-extension-draft).
- Existing queue wrapper, timestamp formatter/component, shared Popover/Drawer primitives, queue Escape boundary, and desktop/mobile timestamp E2E files.
- Earlier completed package is historical context, not authorization to mark this task done.

## Results

Pending. After implementation, record exact commands, results, failing-before evidence, and visual captures. Keep existing active/current spec lifecycle intact; the appended extension is draft until implemented and accepted.
