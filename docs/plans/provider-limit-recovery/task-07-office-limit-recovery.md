---
id: "07-office-limit-recovery"
title: "Office limit recovery"
status: pending
wave: 6
depends_on:
  - "06-automatic-launch-gate"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-002
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.2
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.3
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.4
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-006.5
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 07: Office Limit Recovery

## Summary

Office records limit marks for runs on concrete profiles and clears them after
successful turns. For opted-in profiles, it requeues once on the profile's
fallback model, or parks the run until a trusted reset with probe-gated
lifting. Office provider health, routing, and inbox behavior stay unchanged
for every other case.

## In scope

- The `limit_fallback_model` and `provider_limit_probe` run fields (additive
  migration), dispatch of the same execution profile under the launch-scoped
  exact policy, and route attempt `requested_model`/`effective_model`/
  `override_reason`.
- Blocked status `waiting_for_limit_reset` through
  `ParkRunForProviderCapacity`, a probe-gated lift in
  `SchedulerIntegration.liftParkedRoutingRuns`, and its localized Office label.
- Store the acquired `ProbeLease` identity with the lifted run and release it
  from the matching terminal handler.
- On successful `AgentCompleted`, resolve the run's execution binding and
  actual `effective_model`, call `providerlimit.ClearOnSuccess`, and release an
  attached probe lease after clearing. Stops and failures do not clear marks.
- The dispatch gate in `DispatchWithRouting` and unrouted dispatch.

## Out of scope

- Workspace routing order, tiers, `office_provider_health` semantics, and
  dynamic profiles.

## Acceptance

1. An opted-in Office run that hits a limit relaunches once on the fallback
   before provider advancement. The route attempt records both models and the
   reason. An unadvertised fallback parks instead of drifting to another
   model.
2. Without a fallback, a trusted reset parks the run until the reset. Exactly
   one parked run per mark lifts first, and the rest follow after success.
3. A successful Office turn clears the mark for the resolved execution
   binding and effective model. A successful probe releases a parked sibling.
   Persist and recover the exact acquired `ProbeLease` identity on the lifted
   run; its matching `AgentCompleted` releases that lease and wakes the sibling,
   while an unrelated or stale lease remains untouched.
4. `AgentFailed` and `AgentStopped` do not clear marks. A limit failure renews
   the mark and leaves siblings parked. A non-limit failed or stopped probe
   leaves siblings parked until the 10-minute lease expires and another waiter
   may probe.
5. Runs on profiles that are not opted in keep their current Office test
   outcomes. Health retry and mark clearing do not affect each other.

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/office/service -run 'TestProviderLimit|TestPostStartFallback|SchedulerIntegration' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/office/scheduler ./internal/office/repository/sqlite ./internal/office/routing -count=1)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/office/office-routing-recovery.spec.ts)
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/service/{event_subscribers.go,scheduler_integration.go}`
- `apps/backend/internal/office/scheduler/{routing_lifecycle.go,dispatch_routing.go}`
- `apps/backend/internal/office/repository/sqlite/{run_routing.go,base_migrations.go,route_attempts.go}`
- `apps/backend/internal/office/models/models.go`
- `apps/web/locales/*/office.json`

## Dependencies

Task 06 (launch-scoped exact fallback policy).

## Risks

- The Office routing specs are deprecated. Keep the change profile-local and
  record it only in the agents design.
- Short-retry, health, and limit parking can overlap. The order must be: short
  retry, then limit recovery, then existing fallback and escalation.

## Parallelism

`sequential`

## Inputs

- System design: Office runs.
- `routing_lifecycle_test.go` and `office-routing-recovery.spec.ts`.

## Results

Pending.
