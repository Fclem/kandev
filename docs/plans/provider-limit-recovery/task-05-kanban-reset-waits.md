---
id: "05-kanban-reset-waits"
title: "Kanban reset waits"
status: pending
wave: 4
depends_on:
  - "04-kanban-limit-fallback"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-002
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.2
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.3
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.4
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.6
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.7
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.8
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.9
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 05: Kanban Reset Waits

## Summary

Add independent, durable session waits to the task `deferred_launch` metadata
and a timer waker with probe gating. When no fallback applies, an opted-in
session with a trusted reset waits visibly, survives restart, can be cancelled,
and resumes once in the same session.

## In scope

- `task/models` collection of `ProviderLimitWait` entries keyed by session and
  failed-turn identity, plus the existing single automatic-launch slot.
- Merge, read, and clear helpers with compare-and-swap updates that preserve
  sibling waits, the session-ceiling launch record, and dependency intent.
- The wait branch of `handleProviderLimitFailure`, the
  `provider_limit_waits` counter (at most three), and the waiting status
  message metadata.
- `provider_limit_waker.go`: startup listing, an earliest-deadline timer,
  safety calls from the one-minute reconciliation sweep, grouping by mark,
  durable probe acquisition, and replay through the existing kind replay
  functions.
- Persist the exact acquired `ProbeLease` on the selected wait before replay.
  Restore that token after restart and use it on the matching terminal event;
  never reconstruct it from the circuit's current expiry.
- Cancellation hooks: `cancel_retry`, manual prompt, stop, step move, archive,
  and delete. Session actions clear only that session's matching wait;
  task-level invalidation clears all task-owned waits.
- Ceiling interaction for the single automatic-launch slot, preserving the
  launch identity and never replacing a session wait.
- The limit variant of the waiting card in `action-message.tsx`, with
  localized copy and a phone layout.
- Playwright wait, cancel, and auto-resume flows on desktop and phone.

## Out of scope

- Gating automatic launches before a failure (Task 06).
- Office (Task 07).

## Acceptance

1. Trusted-reset waits show the exact reset and never resume early. Distinct
   session waits on one task survive restart independently; cancelling or
   replaying one leaves the other intact.
2. With three sessions sharing one mark, exactly one probes. Persist its exact
   lease before replay; after restart, resume the same wait identity if its
   turn was not dispatched, and never duplicate an active turn. Matching
   completion releases that lease and wakes siblings; a stale token cannot
   release a later lease. A failed durable wait/owner write does not replay;
   a failed probe renews the wait.
3. An eight-day reset remains visible but creates no wait. The fourth
   consecutive wait, an unknown reset, or a reset beyond the trusted bound
   shows the existing card without automatic resume.

## ASCII UI preview

`UI-03: Chat waiting card` from the [plan](plan.md#ascii-ui-preview)
(AC 004.1, AC 004.5):

```text
+-------------------------------------------------------------+
| (clock) opus reached its usage limit. Resuming at 11:10      |
|         (in 1h 42m).                          [ Cancel ]     |
+-------------------------------------------------------------+
Phone: Cancel stays inline and content-width, >= 44px tall.
```

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/models -run 'ProviderLimitDeferral|Ceiling' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestProviderLimitWait|TestProviderLimitWaker|TestProviderLimitDeferral|TestProviderLimitProbeLease|Ceiling' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/action-message.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/provider-limit-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-provider-limit-recovery.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/models/{ceiling_record.go,provider_limit_record.go}`
- `apps/backend/internal/orchestrator/{provider_limit_failure.go,provider_limit_deferral.go,provider_limit_waker.go,ceiling_replay.go,ceiling_defer.go,event_handlers_transient.go}`
- `apps/web/components/task/chat/messages/action-message.tsx`, `apps/web/locales/*/task.json`
- `apps/web/e2e/tests/task/{provider-limit-recovery,mobile-provider-limit-recovery}.spec.ts`

## Dependencies

Task 04.

## Risks

- The shared `deferred_launch` compare-and-swap is easy to break. Run the full
  `Ceiling*` test set.
- Timer and clock tests must use an injected clock. Do not use wall-clock
  sleeps.

## Parallelism

`sequential`

## Inputs

- System design: Deferred work and waking.
- `ceiling_defer.go`, `ceiling_replay.go`, `dynamic_policy_recovery.go`
  timer pattern.

## Results

Pending.
