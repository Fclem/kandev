---
id: "10-provider-limit-metrics"
title: "Provider limit metrics"
status: pending
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
   names, closed label values, and counting points.
2. Accepted mark creation and renewal increment once with the resolved scope
   and classified code. Lookups, ignored failures, duplicate no-ops, and failed
   persistence do not increment the mark counter.
3. Each final fallback decision increments once with its context and outcome,
   including early `not_advertised` and `marked` results and terminal `failed`
   paths. No fallback decision metric changes the selected route.
4. Each durable wait transition or probe outcome increments once with its
   context and outcome. Kanban and Office cancellation, exhaustion, successful
   resume, and probe failure are covered; no-op or stale events do not count.
5. Tests assert that labels never contain profile, binding, task, session, or
   run identifiers.

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

Pending.
