---
status: active
system: tasks
created: 2026-10-04
owners:
  - kandev
---

# Session Stop Teardown Ownership Requirements

## Overview

The task system owns session execution lifecycle and teardown. A session stop
must establish who owns stopping the exact execution before releasing the
session cancellation guard, so concurrent terminal cleanup cannot issue a
second stop for the same execution.

## Terminology

- **Exact execution:** The execution identifier observed for a session when the
  stop is accepted; a later execution for that session is a different owner
  scope.
- **Teardown owner:** The lifecycle path authorized to issue the stop for an
  exact execution.

## Requirements

### REQ-TASKS-SESSION-STOP-OWNERSHIP-001: Exact-session stop ownership

**Intent:** Prevent concurrent session-stop and terminal-cleanup paths from
stopping the same execution more than once.

**User story:** As a task operator, I want a session stop to own teardown of its
current execution, so that concurrent cleanup does not issue a duplicate stop.

#### Acceptance criteria

- **AC-TASKS-SESSION-STOP-OWNERSHIP-001.1:** When `StopSession` accepts a stop
  for a current execution, the session lifecycle shall claim teardown ownership
  for that exact execution before releasing the session cancellation guard.
- **AC-TASKS-SESSION-STOP-OWNERSHIP-001.2:** When terminal cleanup races with
  an accepted session stop for the same execution, cleanup shall not issue a
  second stop for that execution.
- **AC-TASKS-SESSION-STOP-OWNERSHIP-001.3:** Teardown ownership for one
  execution shall not suppress stopping a different execution later associated
  with the same session.

## Out of scope

- Task-wide stop candidate selection, partial-failure policy, and workflow
  transitions.
- Provider-limit recovery and provider routing.
- Teardown retry policy after an owner has issued its stop.

## Implementation plans

- [Session stop teardown ownership](../../plans/session-stop-teardown-ownership/plan.md)
