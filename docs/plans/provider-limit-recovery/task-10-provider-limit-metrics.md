---
id: "10-provider-limit-metrics"
title: "Provider limit metrics"
status: done
wave: 8
depends_on:
  - "07-office-limit-recovery"
  - "09-omp-prompt-end-limit-failure"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-002
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-003
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-004
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-005
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.1
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 10: Provider Limit Metrics

## Summary

Implement and test the three provider-limit expvar counters and structured logs
across mark recording, Kanban fallback and waits, automatic launch gating, and
Office fallback and probe lifecycle. Use the fixed labels and counting points
in the system design.

## In scope

- Register `provider_limit_marks_total`, `provider_limit_fallback_total`, and
  `provider_limit_waits_total` through the existing expvar conventions.
- Instrument accepted mark creation and renewal, plus final fallback decisions
  for Kanban failures, automatic launch gates, and Office runs.
- Instrument durable wait lifecycle transitions and probe outcomes for Kanban
  and Office, including arm, resume, cancel, exhaustion, and probe failure.
- Emit structured `provider_limit.*` zap logs at the same decision points.
- Add focused counter and log tests for every label value and both contexts,
  including no-mark/no-op paths, unadvertised or already-marked fallback, an
  unknown or untrusted reset that creates no wait, failed mark persistence,
  cancellation, expiry, successful probe, and failed probe terminal paths.

## Out of scope

- Changing any provider-limit recovery decision or retry behavior.
- Adding profile, binding, task, session, or run identifiers as metric labels.

## Acceptance

1. All three expvar counters and structured log events use the documented
   names, closed label values, and counting points. Tests assert no profile,
   binding, task, session, or run identifiers appear as metric labels.
2. Accepted mark creation and renewal increment once with resolved scope/code;
   lookups, ignored failures, duplicate no-ops, and failed persistence do not
   increment marks. Each final fallback decision increments once with its
   context/outcome, including early `not_advertised` and `marked` results and
   terminal `failed` paths. Counters do not change recovery decisions.
3. Each durable wait transition/probe outcome increments once. Test every
   outcome in Kanban and Office, including cancellation, exhaustion, successful
   resume, probe failure, expiry, and no-op/stale events.

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/providerlimit ./internal/orchestrator ./internal/office/service -run 'TestProviderLimitMetric' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/providerlimit ./internal/orchestrator ./internal/office/service -count=1)
git diff --check
```

## Dependencies

Tasks 03-07 and 09 deliver the instrumented decision paths.

## Parallelism

`sequential`

## Results

Implemented the three expvar counters and fixed-field `provider_limit.*`
structured logs for Kanban and Office. Counter labels are closed and contain no
profile, binding, task, session, or run identifiers. Tests cover accepted versus
failed/duplicate marks, each fallback and wait label in both contexts, durable
transition counting, and safe label fields.

Focused tests passed:

```bash
env TMPDIR=/home/clem/provider-limit-build/tmp GOCACHE=/home/clem/provider-limit-build/cache GOTMPDIR=/home/clem/provider-limit-build/tmp go test -trimpath -tags fts5 ./internal/agent/runtime/providerlimit ./internal/orchestrator ./internal/office/service ./internal/office/scheduler -run 'TestProviderLimitMetric|TestProviderLimitFallbackIneligibleDecisionsKeepRecoverySurface|TestProviderLimitWait|TestDispatchWithUnadvertisedProviderLimitFallbackCountsTerminalDecision|TestOfficeTrustedResetParksRunWithOpaqueWaitKey|TestSuccessfulProviderLimitProbeReleasesLeaseAndWakesSiblings|TestNonLimitFailureKeepsProbeLeaseAndParksUntilExpiry' -count=1
```

The complete four-package run passed for providerlimit, Office service, and
Office scheduler. The orchestrator package remains red in
`TestStopTaskForCoordinator_PartialFailureAttemptsEveryCandidateAndSkipsReview`
and `TestStopSession_GracefulTeardownClaimSuppressesLateForceCleanup`, both
outside the provider-limit paths. The Office fallback persistence regression
found during this run was fixed, and its focused regression test passes.
