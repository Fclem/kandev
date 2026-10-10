---
id: "03-shared-limit-marks"
title: "Shared limit marks"
status: done
wave: 2
depends_on:
  - "01-classify-limit-timing-and-scope"
  - "02-profile-limit-settings"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.1
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.2
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.3
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.4
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.5
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.6
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.7
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.8
  - AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.9
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
---

# Task 03: Shared Limit Marks

## Summary

Add `providerlimit`, a durable store of binding-scoped limit marks on the
existing circuit registry. Attempt to persist a mark for every concrete
Kanban limit failure; use it for recovery only after the write succeeds.
Atomically clear it on success. Expose active marks through an API and
WebSocket notification that feed a `limited until` pill on profile rows.

## In scope

- Move and export `profileCredentialBindingDescriptor` into package `dynamic`.
- Circuit registry `Close`, `List(prefix)`, and `CircuitSnapshot.ResetKnown`,
  with an additive `dynamic_resource_circuits.reset_known` column.
- Add provider-limit-only `OpenDurable`, `CloseManyDurable`,
  `AcquireProbeDurable`, and `ReleaseProbeDurable` operations. They persist
  proposed state before publishing it in memory and return save errors;
  existing dynamic-routing mutations keep their current behavior.
- Extend `CircuitPersistence` with an all-or-nothing `SaveCircuits` batch.
  Implement the SQLite adapter with one transaction covering every snapshot
  upsert; a row or commit error rolls back the full batch. Update every
  persistence implementation and test fake. `CloseManyDurable` writes both
  closed snapshots in one batch and changes neither in-memory mark on failure.
- `providerlimit.Service`: `Record`, `Lookup`, atomic `ClearOnSuccess`,
  durable probe acquire/release (10-minute lease), and `List`. Expiry follows
  AC 002.3. Provider-qualified account keys follow AC 002.9, through a
  provider-qualified-model capability on `omp-acp` and `opencode-acp` in
  `internal/agent/agents`.
- Wire the service in `backendapp/services.go`, independent of
  `features.dynamicAgentRouting`.
- Kanban recording in the failure path for all concrete profiles, and
  clearing on successful turn completion. No other behavior change.
- `GET /api/v1/agent-profiles/limits`, the `agent.profile.limits_updated`
  broadcast, a web store slice, and the profile-row pill.

## Out of scope

- Fallback, waits, gates, and Office recording (Tasks 04-07).

## Acceptance

1. An account-scope mark limits every model on its binding. For OMP, it
   limits only the failed provider's models: an `anthropic/...` spend mark
   leaves `openai-codex/...` eligible. A model-scope mark limits only its
   model. Unprovable bindings isolate to the profile, and accepted marks
   restore after restart.
2. A later expiry extends a mark and an earlier one never shortens it. A
   known eight-day reset sets expiry to that instant; a reset beyond seven
   days does not permit automatic resumption. Success atomically clears both
   keys through one `SaveCircuits` transaction. If the second SQLite snapshot
   write fails after the first row operation, the transaction rolls back and
   both marks remain active in memory and after repository reopen/restore.
   Probe leases persist and restore with the exact expiry token; a stale token
   cannot release a later lease. Failed writes or a missing persistence
   adapter publish no new state: current recovery stays on its existing
   surface until a later durable write succeeds. Tests cover failed mark
   writes, missing adapter, retry and restart restore, atomic-clear registry
   failure, SQLite rollback after the first row operation plus reopen/restore,
   and failed lease writes.
3. A profile row shows a localized `limited until <time>` pill while a mark is
   active, on desktop and phone. The pill disappears after expiry or clear.

## ASCII UI preview

`UI-02: Profile row limit indicator` from the [plan](plan.md#ascii-ui-preview)
(AC 002.8):

```text
| Claude  Opus profile  [model: opus] [fallback: sonnet] [limited until 11:10] |
```

## Verification

Run this complete block from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/providerlimit ./internal/agent/runtime/dynamic -run 'TestDurable|TestProviderLimitPersistence' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/providerlimit ./internal/agent/runtime/dynamic -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run 'Circuit' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestProviderLimitMark' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/agents/agent-profiles-section.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/providerlimit/` (new)
- `apps/backend/internal/agent/runtime/dynamic/{circuit.go,fingerprint.go}`
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`
- `apps/backend/internal/task/repository/sqlite/{base_schema.go,dynamic_route.go}`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/web/components/settings/agents/agent-profiles-section.tsx`
- `apps/web/lib/api/domains/` and the agents store slice

## Dependencies

Task 01 (`LimitScope`, `RetryAfter`) and Task 02 (profile fields for the
endpoint).

## Risks

- Dynamic routing must keep ignoring `limit|` keys. Add a regression test in
  `dynamic/engine_test.go`.
- The existing circuit pending flush is not a durable commit for providerlimit
  state. A failed provider-limit mutation leaves prior durable state unchanged
  and cannot enable fallback, waiting, or probe recovery.

## Parallelism

`sequential`

## Inputs

- System design: Limit marks.
- `dynamic/circuit.go` and `fingerprint.go` tests.

## Results

Implemented the shared provider-limit service, opaque binding descriptors,
provider-qualified account domains, durable reset knowledge, atomic batch
clears, exact-token ten-minute probes, SQL migration/restore, concrete Kanban
recording/clearing, privacy-safe REST/WS projection, and localized profile pills.
Dynamic routing remains independent of `limit|` marks; Office recording and
fallback/wait behavior remain assigned to the later work orders.

Observed RED: SQL lost reset knowledge and omitted closed probe ownership;
the global API leaked an Office-private profile; the profile indicator was
absent; root StrictMode cancelled the initial offline read. These regressions
and atomic-clear, commit-rollback and lease failure cases now pass. Existing
wording/mirror tests were removed rather than repinned.

Verification:

- Complete backend block passed: targeted durable/provider persistence,
  full providerlimit/dynamic packages, all `Circuit` SQL tests, and
  `TestProviderLimitMark` orchestrator tests.
- Focused real API/broadcast tests passed. Native backend compiled.
- Final profile/hook/settings-store run: 39 tests passed. Earlier root-store
  and WS handler runs passed. Typecheck, i18n check, i18n ratchet and scoped
  ESLint passed; scoped lint has no remaining warnings.
- Native runtime smoke with dynamic routing explicitly disabled restored a
  real SQLite mark. The API returned only the five public fields. Desktop
  and 393px coarse-pointer phone rendered the pill without horizontal
  overflow. Real expiry removed the phone pill and returned HTTP 200 with
  an empty projection. Screenshots were visually inspected.
- `git diff --check` passed. Disposable smoke source and owned UI/browser
  services were removed/stopped; the owned PostgreSQL service remains for
  later package verification.
