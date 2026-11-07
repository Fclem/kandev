---
status: active
system: ui
created: 2026-10-28
owners:
  - kandev
---

# Queued Message Timestamps Requirements

## Overview

Users need to see when pending messages entered the queue. A merged message
must show the newest time among its contributing messages.

UI owns this presentation contract alongside the existing queue controls.
Queue persistence, admission, and delivery retain their existing owners.
This capability extends the [message-time preference](message-time-display.md)
to pending queue rows.

## Requirements

### REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001: Pending message time

**Intent:** Make the age of pending content visible before delivery.

#### Acceptance criteria

- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.1:** Each pending message with a valid queued time shall display that time in its queue row. This includes attachment-only messages and every sender kind, in task chat and Quick Chat.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.2:** After manual or automatic merging, the surviving row shall display the latest queued time among all contributing messages. Repeated merges, reordered rows, and reloads shall preserve this result. The displayed time shall not be the time of the merge action.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.3:** The timestamp shall follow the saved message-time preference, interface locale, regional formatting, and browser time zone. It shall expose the same counterpart form as transcript timestamps.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.4:** Missing or invalid queued times shall produce no timestamp or details control. The message and its actions shall remain usable.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.5:** Desktop and phone rows shall show the timestamp below the preview. Phone users shall access its counterpart through a details drawer. Touch targets shall measure at least 44 pixels in both dimensions. Long localized dates shall remain contained without overlapping row actions or causing horizontal page overflow. Accessible names shall include the label and counterpart. Escape shall close the details drawer while keeping the queue panel open.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.6:** Opening an ended task with retained queued messages shall expose those messages and their timestamps without starting or resuming an agent. Relative mode shall show ingestion age at every age, including messages at least seven days old, with the exact ingestion date and time available in details. Absolute modes shall retain the saved preference and expose relative age as the counterpart.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.7:** Navigating away, reloading, or reopening the same task shall preserve each retained message's ingestion timestamp. Viewing timestamp details shall not dequeue, deliver, or modify a message.
- **AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.8:** Desktop keyboard users shall be able to reach a queued-message timestamp and reveal its visible counterpart using standard keyboard controls. Escape shall dismiss only the timestamp details, leave the queue panel open, and return focus to the timestamp trigger. The timestamp line itself shall provide the disclosure without requiring an additional icon.

Criteria .6-.8 are draft extensions for issue #5074. Existing criteria retain
their active contract; this extension preserves the saved display preference
and does not change transcript formatting.

## Out of scope

New time settings, periodic time refresh, merge eligibility changes, historical
timestamp reconstruction, and timestamps in the collapsed queue-count chip.
The inline queue editor retains its current layout.

## Implementation plans

- [Queue timestamps](../../../plans/queued-message-timestamps/plan.md)
- [Ended-task queue inspection](../../../plans/ended-task-queue-timestamps/plan.md)
