---
created: 2026-10-04
status: implemented
requirements:
  - REQ-TASKS-SESSION-STOP-OWNERSHIP-001
system_design:
  - ../../specs/tasks/system-design/session-stop-teardown-ownership.md
legacy_specs: []
---

# Implementation Plan: Session Stop Teardown Ownership

## Overview

Prevent a direct session stop and concurrent terminal cleanup from issuing
multiple stops for the same execution. The task system requirement and design
own exact-execution identity and ownership. The provider-limit recovery work is
unrelated to this lifecycle repair.

## Work order

1. [Exact-session stop ownership](task-01-exact-session-stop-ownership.md)

The work order also corrects the coordinator-stop test so it verifies attempted
candidate coverage without pinning the number of execution lookups for a
candidate. The second lookup is required to revalidate the current execution
before terminal tombstoning.

## Scope

- Claim teardown ownership for the exact execution targeted by a session stop
  while the per-session cancellation guard is held.
- Ensure concurrent cleanup does not issue a second stop for that execution.
- Preserve the existing teardown behavior for other execution IDs and existing
  terminal-state handling.
- Keep the partial-failure coordinator test focused on candidate coverage and
  workflow outcome, not incidental internal lookup counts.

## Out of scope

- Provider-limit classification, fallback, or routing behavior.
- Changes to task-wide stop target selection or its partial-failure policy.
- New metrics, persistence, or public documentation.
