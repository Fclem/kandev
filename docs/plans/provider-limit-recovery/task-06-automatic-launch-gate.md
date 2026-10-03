---
id: "06-automatic-launch-gate"
title: "Automatic launch gate"
status: pending
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
- A `provider_limit_notice` status message for manual prompts and launches,
  once per session and mark.
- Localized copy for the scheduling note and the manual notice.

## Out of scope

- Office dispatch (Task 07).

## Acceptance

1. A workflow auto-start, queued drain, or deferred replay on an opted-in
   profile with a limited model starts on the fallback with one warning.
   Without a fallback, it defers to the reset and replays exactly once.
2. An unadvertised fallback fails before inference, and the launch then
   defers or proceeds under the remaining rules. No third model is used.
3. Manual launches and profiles that are not opted in behave as before. A
   manual prompt shows one notice.
4. When one automatic launch already owns the task's deferred-launch slot, a
   distinct limit-deferred launch receives an explicit conflict. The existing
   launch payload remains intact, and the later caller retains ownership.

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
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestProviderLimitGate|CeilingSeam|Seam1|Seam2|Seam3|Seam4' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'StartModel|LimitFallbackLaunch' -count=1)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/provider-limit-recovery.spec.ts -- --grep "auto-start")
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/{provider_limit_gate.go,ceiling_seams.go,ceiling_seam2.go,ceiling_seam3.go,ceiling_seam4.go}`
- `apps/backend/internal/agent/runtime/lifecycle/{start_model.go,session.go}`
- `apps/web/components/chat/messages/status-message.tsx`, `apps/web/locales/*/task.json`

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

Pending.
