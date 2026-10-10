---
id: "04-kanban-limit-fallback"
title: "Kanban limit fallback"
status: done
wave: 3
depends_on:
  - "03-shared-limit-marks"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-003
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.2
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.3
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.4
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.5
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-003.6
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 04: Kanban Limit Fallback

## Summary

When a concrete Kanban session on an opted-in profile ends with a limit
failure, `handleProviderLimitFailure` switches it to its eligible, advertised,
unmarked fallback model. It continues the same session through the existing
model-switch prompt path and records one durable warning per decision.

## In scope

- `orchestrator/provider_limit_failure.go`: the decision tree up to the
  fallback branch. A non-fallback outcome returns `nil`, so the existing card
  stays in place until Task 05. A failed durable mark write exits to that card
  without switching.
- Accepted recovery returns a callback to the existing bounded failure worker,
  after the session guard is released. Failed-execution stream guards remain
  scoped to the failed prompt generation, allowing output from a newly admitted
  prompt on the same ACP process without accepting late failed-prompt frames.
- The advertised check through the lifecycle `CachedModelState`.
- Choosing between the original input and the continuation instruction.
- The per-turn `provider_limit_fallback_turn` marker.
- A `model_selection_warning` status message with reason `provider_limit` and
  a deterministic `decision_id`. The web reason string goes in all locales.
- Mock-agent `/provider-limit <model> <seconds>` scenario, and the Playwright
  fallback flow.

## Out of scope

- Waits, the waiting card, and launch gating (Tasks 05 and 06).

## Acceptance

1. A limit on the primary model of an opted-in compatible profile continues the
   same session on the fallback. The UI shows exactly one warning row, and a
   reload does not duplicate it.
2. Strict, automatic-fallback, unadvertised, or marked fallbacks, a second
   limit in the same turn, and a failed durable mark write never switch. They
   show the existing card.
3. A never-started turn resends its input once. A turn with prior turn events
   receives only the continuation instruction.

## ASCII UI preview

`UI-04: Fallback warning row` from the [plan](plan.md#ascii-ui-preview)
(AC 003.3):

```text
(!) Switched from opus to sonnet because opus reached a provider limit.
```

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestProviderLimitFallback' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestHandleTransientFailure|TestRecoverable|TestModelSwitch' -count=1)
(cd apps/backend && go test -tags fts5 ./cmd/mock-agent -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/status-message.test.tsx)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/provider-limit-recovery.spec.ts -- --grep "fallback")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-provider-limit-recovery.spec.ts -- --grep "fallback")
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/{provider_limit_failure.go,event_handlers_agent.go,task_operations.go}`
- `apps/backend/cmd/mock-agent/` (scenario)
- `apps/web/components/task/chat/messages/status-message.tsx`, `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/task/provider-limit-recovery.spec.ts`

## Dependencies

Task 03.

## Risks

- The switch runs inside the failure handler's locked state. Dispatch must
  happen after the lock is released, following the pattern of the transient
  retry timer.
- A passthrough session restarts its PTY to switch. Its conversation
  continuity follows the existing override behavior.

## Parallelism

`sequential`

## Inputs

- System design: Kanban failure flow.
- `event_handlers_transient.go` `retryTransientPrompt`.

## Results

- Implemented advertised, unmarked, opted-in fallback with durable per-turn
  ownership and deterministic warning identity. Unknown or observed turn
  evidence uses only the continuation instruction; never-started input replays
  once. Cancellation, archive, stop, deletion, persistence failure, and repeated
  failures retain the guarded existing recovery behavior.
- Native E2E exposed two production integration defects: the lifecycle adapter
  omitted the live model-state reader, and synchronous reported-error cleanup
  settled the failed turn before the recovery actor. Fixed both with regression
  coverage. Generation-scoped terminal guards now preserve actual fallback
  output while rejecting late predecessor frames and failures, including after
  successful completion.
- Exact backend verification passed: fallback tests, transient/recoverable/model
  switch tests, and the complete mock-agent package. Status-message tests: 14
  passed. Typecheck, scoped eslint, i18n check, and new-code ratchet passed.
- Exact Chromium and mobile-chrome fallback flows passed. Both exercised actual
  ACP inference, persisted fallback output, one warning after reload, unchanged
  conversation identity and prior input, and a second user prompt using the
  sticky fallback without another warning. Phone uses its existing explicit
  Send-button interaction; no document overflow. Screenshots inspected.
- Specification catalog validation, all-file specification lint, and
  `git diff --check` passed.
