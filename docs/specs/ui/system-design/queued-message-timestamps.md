---
status: current
system: ui
requirements:
  - REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001
---

# Queued Message Timestamps System Design

## Purpose and boundaries

Expose the existing queue timestamp through the reusable chat timestamp UI.
This is a presentation extension. It needs no backend change, migration,
new preference, or new API field.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001 | Data source, Shared presentation, Responsive behavior, Ended-task inspection extension (draft), Verification |

## Data source

`QueuedMessage.queued_at` in `apps/web/lib/state/slices/session/types.ts`
already arrives through queue status responses and store reconciliation.
Render this server value without deriving a timestamp from browser receipt time.

In `internal/orchestrator/messagequeue/auto_merge.go`, `latestQueuedAt` selects
the maximum time. `buildAutoMergedEntry` uses it for automatic merges.
Manual merging in `repository_sqlite.go` persists and returns the same maximum.
`useEntryMutations` in `use-queue.ts` refetches after manual merging.
Repeated merges therefore retain the maximum across all source messages,
independent of queue position. Existing persistence tests cover this behavior.

The manual-merge requirement records the same maximum-time behavior.
The active automatic-merge contract already specifies the maximum in
AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.9.

## Shared presentation

`MessageTimestamp` lives in `messages/message-timestamp.tsx` and serves both
`messages/message-actions.tsx` and the queue. Transcript behavior and selectors
remain stable. `QueuedGhostMessage` renders it in `DisplayView`, after
the message preview and attachments inside the existing flexible content column.
Render it independently of text presence so attachment-only rows receive it.

Keep `formatMessageTime` as the formatting and strict timestamp-validation
boundary. Read `userSettings.messageTimeDisplay` and the active locale through
the existing hooks. Reuse `task:messageTime` and
`task:messageTimestampAriaLabel`. No new translation keys are expected.

The component accepts an optional `wrap` property for queue containment.
Use existing muted text, small monospace type, and tabular numbers.
Allow long absolute labels to wrap within the content column. Do not introduce
a fixed time column, new icon, animation, or additional scroll region.

## Responsive behavior

The entry point remains the queue chip and expanded queue panel.
The nearest mobile exemplar is the existing transcript `MessageTimestamp`:
`useTouchDrawer` selects a details drawer for phone and coarse-pointer input.
Reuse that interaction, focus return, and 44-pixel trigger minimum.
The timestamp drawer marks its Escape boundary with `data-queue-escape-boundary`.
`useQueuePanelEscape` defers to that boundary while the drawer has
`data-state="open"`, including the interval before focus moves into it.
Closing the drawer therefore leaves the queue panel open.
The queue panel remains the scroll owner. The timestamp stays outside the
drag handle and action controls. Both desktop and phone use the same content
order, with more wrapping permitted on phones.

## Failure and recovery

Invalid times render nothing through the formatter's existing null result.
Queue refetch and event reconciliation remain authoritative after merges.
No client timestamp fallback may replace missing server evidence.
Existing rows merged by an older backend can only show their retained value.
Deleted source rows cannot supply historical timestamps.

## Ended-task inspection extension (draft)

Issue [5074](https://github.com/kdlbs/kandev/issues/5074) extends the existing
presentation contract through AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.6-.8.
The user explicitly chose to retain the saved message-time preference on
2026-11-05; always-relative labels in absolute modes are excluded. The changes
below override only the queue-specific formatting/disclosure described above.
Transcript defaults and its seven-day calendar-date rule remain unchanged.

### Existing composition and retained state

`ChatInputArea` already wraps `ChatInputContainer` in `QueueAffordance`
from `queued-ghost-list.tsx`. `QueueAffordance` owns the session-scoped
`useQueue` read, passes the chip through `ChatStatusBar`, and renders its
expanded panel before its children. `ChatInputContainer` returning
`SessionStoppedBanner` replaces only the child, not the queue owner.
There is no source-backed completed-session visibility defect at this boundary.
Do not relocate or duplicate the wrapper, subscriptions, or recovery UI.

Prove this existing composition through the actual managed task-chat surface
and selected historical session, including a sibling with a different queue.
A passing pre-change visibility scenario is valid baseline evidence, not a
reason to invent a composition change or force a failing stopped-banner test.
The scope is retained queues on nonarchived managed sessions. Archived/deleted
queues and passthrough terminal surfaces do not establish a defect in this path
and are not new support targets.

No new API, timestamp field, storage migration, retention policy, timer, or
telemetry is needed. `queued_at` stays authoritative. Inspecting the queue or
details must not invoke recovery, delivery, queue mutations, or agent startup;
retain existing action authorization rather than broadening permissions.

### Queue-scoped relative formatting

`formatMessageTime` currently replaces relative labels with calendar dates
at seven days, while retaining an absolute counterpart. This leaves old rows
without an age. Add an optional fourth context argument (`transcript` by
default, `queue` for pending rows) after the existing `now` argument.
Keep one strict timestamp parse and existing cached locale formatters.
For queue-relative labels below seven days retain the current compact style;
at seven days and later use the existing locale-aware `formatRelativeTime`
from `lib/i18n/formats.ts`, which supports weeks, months, and years.
Both absolute modes and their relative counterpart retain current formatting.
The relative label's counterpart stays the existing absolute date/time.
No preference value, locale key, clock-refresh policy, or transcript default
changes. Calendar month/year approximations remain those of the shared helper.

### Queue-scoped keyboard disclosure

Add an optional `context` prop to `MessageTimestamp`, defaulting to
`transcript`; `QueuedGhostMessage` opts into `queue`. The queue branch passes
that context to the formatter and uses the timestamp line as a semantic
button. Keep the muted metadata style, visible focus indication, `<time
dateTime>` value, and accessible label containing both forms. Invalid values
still omit the entire control. No extra icon or fixed time column.

For fine-pointer desktop, use `@kandev/ui/popover`: click or Enter/Space
opens visible counterpart details; Escape dismisses and returns focus to the
trigger. A native title may remain a hover convenience, never keyboard proof.
For phone layout or coarse-pointer input, retain the existing Drawer pattern.
Use `useResponsiveBreakpoint().isMobile` as well as `useTouchDrawer()` so a
narrow fine-pointer phone also receives the phone interaction. Keep fine-pointer
desktop controls compact and phone/coarse-pointer hit areas at least 44px.

Both open Popover and Drawer content must expose
`data-queue-escape-boundary` with `data-state="open"` so `useQueuePanelEscape`
defers to the inner disclosure even before focus enters it. Closing details
leaves the queue open. Reuse primitive focus return, short-drawer safe-area
behavior, and localized `task:messageTime`/timestamp accessible labels.
Queue panel scrolling remains unchanged. Transcript callers retain their
current presentation and interactions.

### Verification boundary

First prove retained ended-session visibility using the real wrapper, with no
production change. Seed IDLE, disable auto-run/auto-merge, enqueue and capture
IDs/timestamps, then update that same session row to COMPLETED using
`seedTaskSession` with explicit `sessionId` and `completedAt`. Do not archive,
delete, or resume to construct the scenario. Select this historical session
explicitly when a different active sibling exists.

Take behavioral RED on the actual gaps: queue-relative labels just below,
at, and beyond seven days, and real desktop Tab/Enter details access.
Keep transcript seven-day regression coverage intact. For browser old-age
cases, use `page.clock.setFixedTime` before navigation with a reference at least
eight days after the newest captured `queued_at`; this advances Date without
freezing timers or modifying server timestamps. Cover both absolute preferences,
phone drawer, narrow fine-pointer layout, dismissal and focus return.
After inspecting, navigating away/reopening, and reloading, compare queue IDs,
contents and `queued_at` to their pre-completion snapshot, verify the selected
session remains COMPLETED and no runtime was started. Existing merge tests
remain authoritative for latest-contributor semantics.

## Verification

Component tests cover queue timestamps, all display modes, invalid input,
attachment-only rows, sender kinds, updated entry props, and the touch drawer.
Keep transcript timestamp tests as extraction regression coverage.
Desktop and mobile browser tests exercise manual and automatic merging,
reload, newest-time selection, and contained long dates.
Use server status values as expected timestamps and causal event waits.

## Implementation plans

- [Queue timestamps](../../../plans/queued-message-timestamps/plan.md)
- [Ended-task queue inspection](../../../plans/ended-task-queue-timestamps/plan.md)
