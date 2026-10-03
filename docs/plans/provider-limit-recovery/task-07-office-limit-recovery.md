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

- Add `limit_fallback_model`, `provider_limit_wait_key`, and
  `provider_limit_probe` to shared `internal/runs/models.Run`; keep the two
  internal ownership fields out of JSON and add Office SQLite persistence.
- Park limit waits with the existing `waiting_for_provider_capacity` status
  and `provider_limit_wait_key`, preserving the current due query and run-header
  badge. The opaque key distinguishes them from provider-health waits.
- Store the exact `ProbeLease` on its owner run. `LiftParkedRuns` and routed
  and unrouted dispatch block a retained same-mark successful owner even after
  mark closure. Health retry and workspace-routing disable preserve keyed waits;
  dispatch applies the active-mark gate when no owner remains.
- On successful `AgentCompleted`, resolve the run's execution binding and
  actual `effective_model`, atomically persist `providerlimit.ClearOnSuccess`,
  then durably release the attached probe lease. Reconcile finished successful
  runs that retain probe ownership on startup before dispatching their siblings.
  A clear, release, or owner-cleanup write failure retains the exact owner and
  parked siblings.

## Out of scope

- Workspace routing order, tiers, `office_provider_health` semantics, and
  dynamic profiles.

## Acceptance

1. An opted-in Office limit failure uses an eligible fallback once before
   provider advancement. An unadvertised fallback fails before inference and
   follows the no-fallback decision: park only with Resume after reset on and a
   trusted reset; otherwise keep existing routing/escalation. A failed durable
   mark write also keeps existing routing/escalation without limit fallback or
   parking. Test resume-off, unknown, and >7-day reset boundaries.
2. `AgentCompleted` atomically clears both resolved marks, then calls
   `ReleaseProbeDurable` for only the exact persisted `ProbeLease`. After
   release and owner/wait cleanup persist, it wakes siblings once. If clear,
   release, or cleanup fails, the owner and sibling waits remain durable and
   no sibling wakes. `LiftParkedRuns`, provider-health retry, and workspace
   routing disable preserve keyed limit waits while an owner remains. Routed
   and unrouted dispatch block on a retained same-mark successful owner even
   after mark closure, then enforce the active-mark gate.
3. Inject `ClearOnSuccess` persistence failure in the Office `AgentCompleted`
   path. Verify the lease is not released, owner/waits remain, and health retry,
   routing disable, ordinary ticks, and restart do not lift due siblings.
   Restore persistence and retry; verify one lift. Test release failure and
   owner/wait-cleanup failure separately after clear succeeds. For each case,
   keep the failed write failing during restart reconciliation. After restart,
   with the mark clear and owner retained, verify due lifts remain blocked and
   direct routed and unrouted dispatch each refuse to start a sibling. Restore
   persistence, then verify exact release and owner/wait cleanup persist before
   one lift and one dispatch.
4. `AgentFailed` and `AgentStopped` do not clear marks; limit failure renews
   the mark. Independently test non-limit failed and stopped probes staying
   parked until their 10-minute lease expires, then a different waiter probes.
   Stale or unrelated leases remain untouched.
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

- `apps/backend/internal/runs/models/run.go`
- `apps/backend/internal/office/service/{event_subscribers.go,scheduler_integration.go}`
- `apps/backend/internal/office/scheduler/{routing_lifecycle.go,dispatch_routing.go}`
- `apps/backend/internal/office/repository/sqlite/{run_routing.go,run_routing_test.go,base_migrations.go,route_attempts.go}`
- `apps/backend/internal/office/routing/{provider.go,provider_test.go}`
- `apps/backend/internal/office/service/provider_limit_test.go`
- `apps/backend/internal/office/scheduler/routing_lifecycle_test.go`
- `apps/web/e2e/tests/office/office-routing-recovery.spec.ts`

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
