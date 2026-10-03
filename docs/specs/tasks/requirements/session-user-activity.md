---
status: draft
system: tasks
created: 2026-10-03
owners:
  - kandev
---

# Session User Activity Requirements

## Overview

Kandev records how long agents run but not how long a person spends working in
a session. Existing duration statistics are agent turn time. Effort analysis,
including the story-point estimator planned as a plugin, needs a measured
human-engagement signal per session that does not depend on message timestamps
alone.

The task system owns this signal because it owns session lifecycle and runtime
state. Plugins consume it through the
[effort-signals Host contract](../../plugins/requirements/effort-signals-host.md);
that contract does not own capture or storage.

## Terminology

- **Active interval:** A contiguous span in which a person was foregrounded on a
  session and interacting with the page.
- **Active time:** The sum of a session's active-interval durations. When
  several people work in one session, the total is aggregate person-time.
- **Activity measured:** True when capture was enabled from the session's start
  and was not disabled while the session was open, so an absence of intervals
  means zero measured engagement rather than unknown engagement.
- **Interaction:** A pointer, keyboard, scroll, or touch event inside the task
  page. Only the fact of interaction is observed, never its content.

## Requirements

### REQ-TASKS-USER-ACTIVITY-001: Foreground interaction capture

**Intent:** Count only time in which a person is demonstrably engaged with a
session, not time in which a tab is merely open.

**User story:** As a user, I want engagement time to exclude idle and
backgrounded tabs, so that effort analysis reflects my actual work.

#### Acceptance criteria

- **AC-TASKS-USER-ACTIVITY-001.1:** When the page is visible, the window has
  focus, and an interaction occurred within the last 60 seconds, the browser
  shall report activity for the session shown in the focused session surface.
- **AC-TASKS-USER-ACTIVITY-001.2:** When the page is hidden, the window loses
  focus, or no interaction occurred within 60 seconds, the browser shall stop
  reporting for that session.
- **AC-TASKS-USER-ACTIVITY-001.3:** When one person has several tabs or windows
  showing the same session, the system shall count each wall-clock instant once
  for that person.
- **AC-TASKS-USER-ACTIVITY-001.4:** When a task page shows several sessions,
  the system shall attribute activity only to the session whose surface holds
  the user's focus.
- **AC-TASKS-USER-ACTIVITY-001.5:** The reported signal shall carry no message
  text, key identity, pointer position, file path, or page content.
- **AC-TASKS-USER-ACTIVITY-001.6:** The server shall use its own clock for
  interval boundaries and ignore any client-supplied timestamp.

### REQ-TASKS-USER-ACTIVITY-002: Interval storage and totals

**Intent:** Keep a compact, durable, queryable record of active time per
session.

#### Acceptance criteria

- **AC-TASKS-USER-ACTIVITY-002.1:** When a report for one session and actor
  arrives less than 45 seconds after that actor's last recorded report, the
  system shall extend the open active interval instead of creating a new one.
- **AC-TASKS-USER-ACTIVITY-002.2:** When a report arrives 45 seconds or more
  after the last recorded report, the system shall close the previous interval at
  its last recorded report and open a new interval.
- **AC-TASKS-USER-ACTIVITY-002.3:** The system shall credit no more than 45
  seconds between consecutive recorded reports, so a suspended laptop or lost
  connection cannot inflate active time.
- **AC-TASKS-USER-ACTIVITY-002.4:** When a report arrives less than 5 seconds
  after the last recorded report for the same session and actor, the system
  shall accept it without recording it or changing the interval.
- **AC-TASKS-USER-ACTIVITY-002.5:** When a session or its task is deleted, the
  system shall delete its intervals.
- **AC-TASKS-USER-ACTIVITY-002.6:** The total for a session shall equal the sum
  of its interval durations and shall be reproducible after a restart.
- **AC-TASKS-USER-ACTIVITY-002.7:** A report for a session the actor cannot
  access, or that does not exist, shall be rejected without revealing which.

### REQ-TASKS-USER-ACTIVITY-003: Control, history, and measured state

**Intent:** Let an operator disable capture and keep unmeasured history honest.

#### Acceptance criteria

- **AC-TASKS-USER-ACTIVITY-003.1:** When capture is disabled, the browser shall
  send no reports and the server shall store none for reports that arrive. A page
  loaded while capture was enabled shall stop reporting once the server answers a
  report as disabled or the page is told to reload for a new backend generation.
- **AC-TASKS-USER-ACTIVITY-003.2:** The system shall not backfill intervals for
  sessions that ended before capture was first enabled.
- **AC-TASKS-USER-ACTIVITY-003.3:** A session that started before capture was
  first enabled shall report activity as not measured, distinguishable from a
  measured session with zero active time.
- **AC-TASKS-USER-ACTIVITY-003.4:** Disabling capture shall retain existing
  intervals and shall not mark newly started sessions as measured.
- **AC-TASKS-USER-ACTIVITY-003.5:** The feature shall ship disabled in every
  shipped profile and be enabled through the runtime-flag mechanism without a
  schema change.
- **AC-TASKS-USER-ACTIVITY-003.6:** When capture was disabled at any point
  during a session's life, including while the session was open or after it
  resumed, the session shall report activity as not measured; a session that
  ended (`completed_at`) before the disable shall keep its measured state.

## Out of scope

- Plugin-facing reads and their authorization, which the
  [effort-signals Host contract](../../plugins/requirements/effort-signals-host.md) owns.
- Keystroke, screen, or typing-speed capture.
- Per-user dashboards of one's own activity in core UI.
- Backfilling history from message timestamps.
- Story-point or effort estimation.
