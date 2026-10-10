---
status: current
system: tasks
requirements:
  - REQ-TASKS-SESSION-STOP-OWNERSHIP-001
---

# Session Stop Teardown Ownership System Design

## Purpose and boundaries

The task system owns session execution identity, stop acceptance, and teardown
ownership. Executor process control performs the requested stop; this design
ensures that session cancellation and terminal cleanup agree on which path owns
that stop. Provider-specific stop behavior is outside this boundary.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-SESSION-STOP-OWNERSHIP-001` | [Control flow](#control-flow) |

## Components and responsibilities

- `orchestrator.Service.StopSession` accepts a session-scoped stop while holding
  the per-session cancellation guard and obtains a structured result from
  `Executor.StopSessionDetailed`.
- `RegisterExecutionStopOwner` coordinates terminal cleanup with the same
  per-session guard. It is non-blocking and may decline ownership registration
  while another path holds that guard.
- Terminal cleanup consults the execution teardown intent before issuing a
  stop, then marks the execution terminal.

## Data and contracts

The ownership key is the exact execution ID returned for the session stop. A
session ID alone is insufficient because a session may later own another
execution. The accepted stop path must publish the requested teardown intent
while it still owns the session cancellation guard; a contending registration
callback cannot substitute for this claim.

## Control flow

1. `StopSession` acquires the session cancellation guard and requests a stop.
2. When the result identifies a current execution, `StopSession` claims graceful
   or forced ownership for that exact ID before releasing the guard, whether or
   not the session-state transition changed the row.
3. The explicit stop's teardown is scheduled after releasing the guard. The
   caller preserves legacy process-stop behavior when the state write fails.
4. If terminal cleanup arrives for that execution, its ownership check observes
   the claim and does not issue a second stop.
5. A stop for a different execution ID is evaluated independently.

The implementation must preserve existing terminal-state handling and
cancellation-guard ordering. The stop result, not a later session lookup, is
the source of the execution identity used for the claim.

## Failure and recovery

A stop result without a current execution cannot claim ownership. An explicit
stop for an existing execution remains scheduled even if its state transition
did not change the row or failed to persist. A denied terminal-cleanup claim
must not launch another process stop or alter terminal classification.

## Persistence

Teardown intent remains in the existing in-memory execution lifecycle state;
this capability adds no durable records or migrations.

## Security

The exact execution ID comes from the backend executor stop result. No
user-controlled execution identity is accepted.

## Observability

No new metrics or logs are required. Existing stop and cleanup diagnostics
remain the evidence for teardown decisions.

## Related decisions

- [Runtime cleanup system design](runtime-cleanup.md)

## Related plans

- [Session stop teardown ownership](../../plans/session-stop-teardown-ownership/plan.md)
