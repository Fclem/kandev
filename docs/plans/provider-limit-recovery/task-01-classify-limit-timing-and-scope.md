---
id: "01-classify-limit-timing-and-scope"
title: "Classify limit timing and scope"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.2
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.3
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.4
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.6
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-007.7
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 01: Classify Limit Timing and Scope

## Summary

Give every limit classification a scope and the most precise reset the harness
exposes. Carry structured retry delays across ACP with exact millisecond
precision. Add rules only for evidenced Anthropic spend and Gemini quota
messages.

## In scope

- `routingerr.Error.LimitScope` with rule-declared `account` scope, and a
  `model` default for every `quota_limited` or `rate_limited` result.
- `routingerr.Input.RetryAfter` and the reset precedence in `Classify`.
- `streams.ProviderError.RetryAfterMs`, filled in `ProviderErrorFromError`
  from allowlisted `acp.RequestError.Data` keys and bounded text patterns.
  Values must be positive and fit `time.Duration`; valid delays beyond seven
  days remain known reset evidence but are not trusted for automatic resumption.
- Pass-through in `classifyKanbanFailure` and Office `HandlePostStartFailure`.
- Propagate `ProviderError.Source` as `routingerr.Input.DiagnosticSource`;
  OMP rules require `omp_acp`, never an inferred text marker.
- Rules `claude.stderr.spend_limit.v1` (ordered before rate),
  `gemini.stderr.quota.v1`, `omp.chunk.anthropic_spend.v1` (ordered first), and
  `omp.chunk.anthropic_rate.v1`.
- `req_` provider request-ID redaction in `routingerr/sanitize.go` and
  `streams/provider_error.go`.

## Out of scope

- Any consumer behavior change. Marks, waits, and fallback are later tasks.
- `codex-app-server` rate-limit windows, and harnesses without fixtures.

## Acceptance

1. The issue example (`rate_limit_error`, monthly spend limit,
   `retry-after-ms=274579000`) classifies as account-scope `quota_limited`
   with reset = `OccurredAt + 274579000ms`, exact to the millisecond. It does
   so from `claude-acp` raw evidence and `omp-acp` adapter-converted evidence
   carrying `DiagnosticSource=omp_acp`; other agent IDs and unmarked OMP prose
   keep their current results. Both sanitizer paths drop `req_011CfL9vJs9DYV45eqL6jbh9`
   (AC 007.6).
2. Precedence is absolute reset, then milliseconds, then seconds, then text.
   Malformed, negative, zero, or unrepresentable delays are dropped. An
   eight-day delay remains an exact known reset. Existing Claude, Codex, and
   OpenCode fixtures keep their codes and gain the scopes listed in AC 007.3.
3. A Gemini `RESOURCE_EXHAUSTED` with `retryDelay "34s"` classifies as
   model-scope quota with a 34-second reset. Unrelated text stays unchanged.

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/routingerr -run 'TestClassifyLimitScope|TestClassifyRetryAfterPrecedence|TestClassifyAnthropicSpendLimit|TestClassifyGeminiResourceExhausted|TestClassifyOMPAnthropicEnvelope|TestSanitizeRedactsRequestID' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/types/streams -run 'RequestID' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/routingerr ./internal/agent/runtime/dynamic -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'TestProviderErrorFromError' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestClassifyKanbanFailure' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/office/scheduler -run 'TestHandlePostStartFailure' -count=1)
git diff --check
```

Run the first command before implementation to get red evidence.

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/{routingerr.go,rules.go,resethint.go,classify_test.go}`
- `apps/backend/internal/agentctl/types/streams/provider_error.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/{opencode_stderr.go,opencode_stderr_test.go}`
- `apps/backend/internal/orchestrator/event_handlers_transient.go`
- `apps/backend/internal/office/scheduler/routing_lifecycle.go`

## Dependencies

None.

## Risks

- The spend rule must win over `claude.stderr.rate.v1`. Rule order is
  significant.
- Fractional seconds (`41.5s`) must convert without float rounding. Parse the
  value as a decimal string.

## Parallelism

`parallel-safe` with Task 02. The files are disjoint.

## Inputs

- System design: Harness evidence, Classification evidence.
- `docs/plans/claude-session-limit-classification/plan.md` test patterns.

## Results

Completed on 2026-10-03.

- RED: spend quota, Gemini quota, limit scope, reset precision/precedence,
  OMP source gating, short request-ID redaction, and ACP delay projection
  failed on expected behavior assertions. Kanban and Office timing consumers
  also failed before propagation. Missing Gemini duration units failed before
  strict unit validation.
- GREEN: every command in the Verification block passed, with `-trimpath`
  added for shared build-cache compatibility. The same package paths, test
  selectors, tags, and `-count=1` were used.
- Added common duration-range boundary coverage; classifier, stream,
  projection, Kanban, and Office regressions pass.
- Smoke: a real ACP RequestError projection classified the issue fixture as
  `quota_limited`, account scope, delay `274579000ms`, exact reset
  `2026-10-06T16:16:19.123Z`, with the request ID redacted.
- The existing `ProviderError.Source` is propagated into internal
  `DiagnosticSource`, requiring `omp_acp` for OMP rules. Design and work
  order were synchronized after the user requested continuation.
- The shared cache volume and `/tmp` were unavailable; checks used isolated
  `TMPDIR`, `GOTMPDIR`, and `GOCACHE` under `/home/clem/provider-limit-build`.
