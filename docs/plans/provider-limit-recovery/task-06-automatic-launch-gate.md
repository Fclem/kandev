---
id: "06-automatic-launch-gate"
title: "Automatic launch gate"
status: done
wave: 5
depends_on:
  - "05-kanban-reset-waits"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-005
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.2
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.3
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.4
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.5
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 06: Automatic Launch Gate

## Summary

Before ceiling admission, gate concrete-profile automatic launches on active
limit marks. Opted-in profiles proceed on an eligible fallback under a
launch-scoped exact policy, or defer durably until a trusted reset. Manual
work proceeds with a one-time notice.

## In scope

- `provider_limit_gate.go`, called in `admitOrDeferSeam1` and the seam 2, 3,
  and 4 gates. Seam 5 (dynamic) is excluded.
- A launch-scoped exact fallback policy and `errLimitFallbackUnavailable`,
  with gate re-evaluation that excludes the fallback.
- A deferral write with task `SCHEDULING` and a status note, reusing the
  Task 05 record and waker.
- A launch deferral probe persists the exact `ProbeLease` through replay and
  transfers it to the newly created session/turn owner before its prompt.
- A `provider_limit_notice` status message for manual prompts and launches,
  once per session and mark. Manual launch seams 1-4 call the notice helper
  after a session exists; `promptTask` calls it before dispatch.

## Out of scope

- Office dispatch (Task 07).

## Acceptance

1. Workflow auto-starts, queued drains, and deferred replays use an eligible
   fallback or wait only with Resume after reset on and a trusted reset. A
   launch conflict preserves the existing payload. Persist and transfer the
   exact launch probe lease to the started session/turn before its prompt;
   after restart resume the same launch identity if it has not started, and
   never duplicate an active turn. Matching completion releases the exact
   lease. A failed owner handoff sends no prompt, and a stale token cannot
   release a later lease.
2. Manual `StartTask`, `StartCreatedSession`, cold resume, resumed-session
   launch, and prompt paths use the requested model without deferral or model
   switching. Each shows one notice for an active mark; profiles that are not
   opted in keep their existing automatic behavior.
3. An unadvertised fallback fails before inference and is excluded from the
   next gate decision; no third model is used. Park only when Resume after
   reset is on and the reset is trusted. With resume off, an unknown reset, or
   a reset beyond seven days, proceed under existing admission rules without a
   wait.

## ASCII UI preview

`UI-05: Deferred task and manual notice` from the
[plan](plan.md#ascii-ui-preview) (AC 005.2, AC 005.4):

```text
Board card / session row:  Scheduling · waiting for opus limit reset at 11:10
Chat after a manual prompt: (i) opus is limited until 11:10. Sending anyway.
```

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestProviderLimitGate|ProviderLimitProbeLease|ManualProviderLimitNotice|CeilingSeam|Seam1|Seam2|Seam3|Seam4' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/status-message.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/provider-limit-recovery.spec.ts --grep "auto-start")
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/{provider_limit_gate.go,ceiling_seams.go,ceiling_seam2.go,ceiling_seam3.go,ceiling_seam4.go}`
- `apps/backend/internal/agent/runtime/lifecycle/{start_model.go,session.go}`
- `apps/web/components/task/chat/messages/status-message.tsx`, `apps/web/src/locales/*/task.json`

## Dependencies

Task 05.

## Risks

- Seam call shapes differ. Each one needs its payload so that replay stays
  identical to the ceiling replay contract.
- The launch-scoped exact policy must not change the saved profile's
  `RequireExactModel` or any other launch.

## Parallelism

`sequential`

## Inputs

- System design: Automatic launch gate.
- Ceiling seam integration tests.

## Results

Implemented the pre-admission provider-limit gate across automatic launch seams,
durable deferral and probe-lease transfer, and one-time manual notices. Created
and TODO tasks with provider-limit launch records now transition to SCHEDULING;
ordinary ceiling reconciliation remains unchanged. Launch queue summaries
accept the `provider_limit` reason, and retry timestamps use localized date
formatting.

- Focused Go regressions passed across orchestrator, task repository, launch
  summary, and task models, including provider gate/wait/waker/probe lease and
  created-task scheduling cases.
- `launch-queue-status.test.tsx`: 8 tests passed. `pnpm run typecheck` passed.
- `pnpm run build:e2e` passed.
- Desktop and mobile provider-limit recovery E2E suites each passed all three
  flows: automatic launch wait, manual notice, and reset wait/reload/cancel/
  resume. Desktop and phone screenshots were inspected; the phone layout has
  no horizontal overflow.
