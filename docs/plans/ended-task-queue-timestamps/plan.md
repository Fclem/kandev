---
created: 2026-11-05
status: draft
requirements:
  - REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001
system_design:
  - ../../specs/ui/system-design/queued-message-timestamps.md
legacy_specs: []
---

# Implementation Plan: Ended-task Queue Timestamps

## Overview

Extend the shipped queued-message timestamp capability for issue [5074](https://github.com/kdlbs/kandev/issues/5074). One vertical work order proves existing retained-queue access on ended sessions, adds all-age relative labels in queue context, and makes desktop details keyboard-accessible. The earlier [implemented package](../queued-message-timestamps/plan.md) remains the historical delivery record; do not reopen its completed task or overwrite its results.

UI owns this extension because the durable ingestion value already exists and this change concerns inspection and presentation, not admission or delivery.

## Confirmed decisions

The user selected **Keep saved preference** on 2026-11-05. Queue rows retain relative/absolute formatting; counterpart details provide the other form. This intentionally relaxes the issue's always-relative acceptance wording. Existing latest-contributor merge semantics remain unchanged.

## Scope

### In scope

- Retained queue inspection on ended tasks without agent startup.
- Queue-scoped relative labels at every age, including week-old leftovers, with exact ingestion date/time details.
- Desktop keyboard and phone touch details; reopen and reload preservation.
- Focused regression tests and queue-reference documentation.

### Out of scope

- Transcript formatting, new settings, periodic refresh, timestamp reconstruction.
- New backend fields, migrations, queue retention, merge policies, or recovery behavior.
- Automatically delivering queued messages or restoring deleted sessions.

## Technical approach

### Preserve the existing composition

`ChatInputArea` already wraps `ChatInputContainer` in `QueueAffordance` from `queued-ghost-list.tsx`; the wrapper renders its status-bar chip and expanded panel before the container child. Replacing that child with `SessionStoppedBanner` does not hide the queue. Do not relocate the owner or prescribe a stopped-banner production fix. Prove completed-session visibility through real task chat first and record a passing baseline honestly. Inspect the selected historical session, with a different active sibling as a regression case.

`queued_at` remains authoritative. No backend change is planned; manual/automatic merges still use the newest contributor. Archived queues are purged and passthrough terminal surfaces are outside this retained managed-session flow; neither calls for a new queue owner here.

### Queue formatting and disclosure changes

`QueuedGhostMessage` already renders `MessageTimestamp` from `entry.queued_at`. Add an optional `context` prop defaulting to `transcript`, and opt queue rows into `queue`. Extend `formatMessageTime` with an optional fourth context argument after `now`, preserving all existing transcript calls. Keep its one strict timestamp parse and cached locale formatters. Queue-relative mode retains compact labels below seven days and uses the existing locale-aware `formatRelativeTime` in `lib/i18n/formats.ts` at seven days and later, never a calendar-date substitute. Absolute modes and counterparts remain unchanged. Do not add a timer or change the saved preference.

For queue timestamps on fine-pointer desktop, make the existing metadata line a semantic button opening `@kandev/ui/popover` on click or Enter/Space. Keep the existing touch Drawer for phone layout or coarse pointers, including a narrow fine-pointer phone via `useResponsiveBreakpoint().isMobile`. Transcript defaults do not gain new controls. Both queue disclosure variants mark open content with `data-queue-escape-boundary` so Escape dismisses details only, then restores focus. Keep muted metadata styling and no extra icon. Native title/aria-label alone is not keyboard-access proof.

Before changing exported signatures, inspect references with the language server and update affected caller/behavior tests. Do not duplicate the formatter, panel, hooks, or domain state.

## ASCII UI preview

### UI-01: Ended task, expanded queue (desktop and phone)

Entry: existing queue chip on the selected ended session. Shared composition:

```text
[Queued messages (2) v]
  #1  [sender]  Older pending content        [actions]
               last week
  #2  [sender]  Recent pending content       [actions]
               5 minutes ago
[Session completed]                 [Resume] [New agent]
```

Phone retains this order with wrapping inside the content column and touch-sized timestamp targets. The queue panel owns scrolling; the completed banner stays outside its scroller. Actions shown are existing actions, not new permissions. Copy and times are illustrative and localized. In absolute modes the metadata line shows the saved absolute format instead.

### UI-02: Timestamp details (desktop popover / phone drawer)

```text
+----------------------------------+
| Message time                     |
| November 2, 2026 at 10:30:00 AM    |
+----------------------------------+
```

The metadata line is the disclosure trigger, without an extra icon. On desktop, Tab reaches it and Enter/Space opens this short content in an anchored Popover. On phone, tap opens the existing inset Drawer with a 44px minimum trigger. Escape dismisses details only and returns focus while the queue remains open. Relative labels always have an exact date/time counterpart; in absolute modes the label follows the saved format and details show relative age. Desktop retains compact controls; the short phone drawer uses shared safe-area behavior. No second navigation step, fixed time column, or new queue scroll owner.

## Tests

- AC .6, .7: establish existing visibility/preservation through real task-chat E2E, including a distinct active sibling. Do not mock away `QueueAffordance` or invent a stopped-banner-only failure.
- AC .6: extend `lib/i18n/message-time.test.ts` with queue-context cases just under seven days, exactly seven days, eight days, and a multi-month age. Use fixed `now`, locale-controlled expected phrases, both absolute modes, and invalid inputs. Retain the existing transcript seven-day calendar-date test unchanged.
- AC .1, .3-.5, .8: extend `queued-ghost-message-timestamp.test.tsx` for all-age labels, focusable fine-pointer trigger and visible Popover, touch details, invalid omission, and narrow fine-pointer phone. Avoid expected results computed by the formatter under test.
- AC .5, .8: extend `use-queue-panel-escape.test.tsx` for open Popover as well as Drawer boundaries; keep transcript `messages/message-actions.test.tsx` green.
- AC .2: retain existing manual/automatic merge cases and latest-contributor reload evidence.

## E2E tests

Extend `apps/web/e2e/tests/chat/queued-message-timestamps.spec.ts` (`chromium`) and `mobile-queued-message-timestamps.spec.ts` (`mobile-chrome`). Start with `seedTimestampQueue`: seed IDLE, disable auto-run and auto-merge, enqueue and capture IDs/content/queued_at. Update the SAME session with `seedTaskSession(task.id, {sessionId: identity.sessionId, state: "COMPLETED", completedAt: <RFC3339>, agentProfileId: seed.agentProfileId})`. Do not archive, delete, or invoke recovery. Include a separate active sibling with different queued content and explicitly select the completed session in the real task surface. Assert one chip/panel for the selected queue, not a sibling's messages.

Establish completed-session visibility before production changes; existing behavior may pass. Take behavioral RED on old relative labels and real keyboard disclosure, not missing selectors. For old entries, use `testPage.clock.setFixedTime` before navigation at least eight days after the newest captured server timestamp. It changes Date while leaving timers running; do not backdate production storage or sleep. Keep locale/time zone deterministic and restore saved settings.

Inspect both relative age and exact details in relative mode and both absolute modes. Desktop uses real Tab/Enter from the queue opener, never programmatic focus or title-only assertions. Escape closes only the details and returns focus. Phone uses tap/Drawer and measures 44px targets, containment, and no horizontal overflow; also exercise a 390px fine-pointer viewport in the desktop project. Navigate away, reopen the same session, then reload and compare persisted queue snapshot and COMPLETED state. Arm request observation before inspection and check persisted runtime state so startup/delivery mutations cannot hide behind unchanged labels. Run with `CAPTURE_PR_ASSETS=1` and inspect desktop/phone captures against UI-01/UI-02.

## Work orders

- [ ] [Task 01: Complete ended-session timestamp inspection](task-01-ended-queue-inspection.md)

Sequential implementation; user-authorized review children are read-only and do not implement this work order.

## Verification results

Implementation pending. Design-only checks on 2026-11-05 after review round 1:

- `python3 scripts/list-docs.py validate`: passed (431 decisions, 1920 specifications).
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/ended-task-queue-timestamps`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for these four documents plus the explicitly hypothetical planned `message-timestamp.tsx` runtime path; all requirement/AC/design references covered. This is design preflight, not implementation evidence.

Round 1 reported three actionable design findings, all addressed: retain the already-correct queue wrapper instead of prescribing relocation; specify queue-only all-age relative formatting; specify visible keyboard-accessible desktop details. Work-order execution, behavioral RED/GREEN, and runtime/visual checks remain pending. Earlier package results are not evidence for this extension.

Adversarial review loop completed on 2026-11-05 using four read-only Kandev
children configured with the OMP Sol 6.1 exact-model profile. Round 1's three
findings were addressed above; rounds 2, 3, and 4 each reported zero actionable
findings against the unchanged revised technical package. The loop stopped at
three consecutive clean rounds, not the 50-round cap. This result-only note
was added afterward; no implementation or runtime acceptance is implied.

## Risks

- An ended task may have multiple sessions: inspecting the active sibling's queue would misrepresent history.
- Completion or deletion can legitimately remove messages; scope is retained messages, not reconstruction.
- Queue inspection must not trigger recovery, auto-run, or delivery.
- Browser tests must use causal completion waits and isolated settings, not timing sleeps.
- Current source establishes queue visibility, not executed runtime proof. Do not make a production relocation merely to obtain a RED test.
- Changing the shared formatter's default would violate transcript behavior; the queue context is opt-in only.
- Native title does not meet keyboard disclosure acceptance; prove visible details with real keyboard input.

## Documentation impact

Update the queue section in `docs/public/operations.md` to explain retained ended-task inspection, ingestion rather than read time, saved formatting with all-age queue-relative labels, keyboard/touch details, and the unchanged merge rule. This is a reference-page update in the later implementation, not public documentation of an unshipped design. Reuse localized labels; any genuinely new copy requires all supported catalogs.
