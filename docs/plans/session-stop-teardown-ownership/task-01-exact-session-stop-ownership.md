---
id: "01-exact-session-stop-ownership"
title: "Exact-session stop ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-STOP-OWNERSHIP-001
acceptance_criteria:
  - AC-TASKS-SESSION-STOP-OWNERSHIP-001.1
  - AC-TASKS-SESSION-STOP-OWNERSHIP-001.2
  - AC-TASKS-SESSION-STOP-OWNERSHIP-001.3
system_design:
  - ../../specs/tasks/system-design/session-stop-teardown-ownership.md
---

# Task 01: Exact-Session Stop Ownership

## Objective

Make an accepted `StopSession` own teardown of its exact current execution before
releasing the session cancellation guard, so concurrent terminal cleanup cannot
issue a duplicate stop.

## Context

`RegisterExecutionStopOwner` uses a non-blocking session-guard acquisition and
may skip registration when that guard is already held. `StopSession` must
therefore claim the requested teardown intent for the exact execution itself
before releasing the guard. Terminal cleanup must observe that claim and avoid
issuing a second stop.

`TestStopTaskForCoordinator_PartialFailureAttemptsEveryCandidateAndSkipsReview`
also expects a fixed count of execution lookups for `session-a`. The additional
lookup revalidates the current execution before marking it terminal; test
candidate coverage and final state instead of that internal call count.

## Scope

- Add a regression test proving a later cleanup does not force-stop the exact
  execution already accepted by `StopSession`.
- Claim the requested teardown intent for the exact execution in the guarded
  session-stop path, and schedule that explicit stop after releasing the guard.
- Preserve cancellation, provider-wait/streak cleanup, and terminal-state
  semantics on all stop result paths.
- Update the coordinator partial-failure test to assert that every candidate is
  attempted and review is skipped without requiring an exact lookup count.

## Acceptance criteria

- The exact-session stop ownership acceptance criteria pass.
- The coordinator partial-failure test proves all candidates are attempted,
  session-a is stopped, the failed candidate remains correctly represented, and
  the review transition is skipped.
- A different execution ID associated with the same session is not suppressed by
  the prior execution's teardown claim.


## Likely files and risks

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/execution_teardown_ownership_test.go`
- `apps/backend/internal/orchestrator/coordinator_stop_test.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`

The teardown intent must be published under the cancellation guard, but executor
process-control work must remain outside it. Preserve existing stop-result,
terminal-state, and provider-wait cleanup semantics.

## Implementation results

The focused orchestrator regressions, focused executor stop-result tests,
race-focused lifecycle regressions, and full orchestrator package all passed.

## Validation

Run from `apps/backend`:

Focused lifecycle regressions:

    go test -trimpath -tags fts5 ./internal/orchestrator -run 'TestStopTaskForCoordinator_PartialFailureAttemptsEveryCandidateAndSkipsReview|TestStopSession_GracefulTeardownClaimSuppressesLateForceCleanup' -count=1

Executor stop-result tests:

    go test -trimpath -tags fts5 ./internal/orchestrator/executor -run 'TestStopSessionDetailed' -count=1

Race-focused lifecycle regressions:

    go test -race -trimpath -tags fts5 ./internal/orchestrator -run 'TestStopTaskForCoordinator_PartialFailureAttemptsEveryCandidateAndSkipsReview|TestStopSession_GracefulTeardownClaimSuppressesLateForceCleanup' -count=1

Full orchestrator package:

    go test -trimpath -tags fts5 ./internal/orchestrator -count=1
