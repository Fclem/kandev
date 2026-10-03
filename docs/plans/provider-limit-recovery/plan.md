---
created: 2026-10-03
status: draft
requirements:
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-001
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-002
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-003
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-004
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-005
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-006
  - REQ-AGENTS-PROVIDER-LIMIT-RECOVERY-007
system_design:
  - ../../specs/agents/system-design/provider-limit-recovery.md
legacy_specs: []
---

# Implementation Plan: Provider Limit Recovery

## Overview

This plan delivers [issue #3811](https://github.com/kdlbs/kandev/issues/3811)
for Kanban sessions and Office runs on concrete profiles. The
[requirements](../../specs/agents/requirements/provider-limit-recovery.md),
[system design](../../specs/agents/system-design/provider-limit-recovery.md),
and [ADR](../../decisions/2026-10-03-opt-in-provider-limit-recovery.md) own the
contracts.

The work orders run in this order:

1. Classification evidence and the profile settings. These are independent.
   OMP prompt-end failure conversion (Task 09) follows classification.
2. Shared limit marks.
3. Kanban fallback.
4. Kanban reset waits.
5. The automatic launch gate.
6. Office.
7. Provider-limit metrics after all instrumented paths are delivered.
8. Documentation and lifecycle promotion.

Every step leaves a working product, and nothing changes until a profile opts
in. Marks come before any consumer because both Kanban and Office read them.
Fallback comes before waits because a wait is the branch taken after a
fallback is not possible. The launch gate introduces the launch-scoped exact
fallback policy that Office reuses.

### Settled decisions

These were confirmed by the user on 2026-10-03:

- Scope is Kanban plus Office, on concrete profiles only.
- The settings are two per-profile switches, both off by default, with no
  release toggle.
- Kandev honors any trusted reset up to the fixed seven-day bound.
- Opted-in automatic launches use an eligible fallback when available, wait
  only when Resume after reset is on and a reset is trusted, and otherwise
  follow existing admission rules with the mark retained. Manual work keeps the
  requested model and shows a notice without deferral.

### Investigation summary

- No open PR addresses #3811. Prerequisites are merged: #4135 (Claude session
  limit), #4019 (Codex usage limit), and #4078 (OpenCode credits).
- Fixed Kanban quota failures go to `handleRecoverableFailureLockedState`.
  Rate limits are retried only when the reset is no more than 60 seconds away
  (`routingerr.isShortRetryable`, through a ladder of at most five attempts).
- No `retry-after-ms` value crosses ACP today. `ProviderErrorFromError`
  projects only a sanitized message, the RPC code, and `errorKind`.
- `profileCredentialBindingDescriptor` and the circuit registry are built in
  every installation (`backendapp/services.go`). Only dynamic routing
  consumes them.
- `office_provider_health` records quota at provider scope for each workspace.
  Office already parks runs and lifts them at `earliest_retry_at`, with no
  seven-day cap.

## Scope

### In scope

- Delay and scope evidence for Claude, Codex, OpenCode, Anthropic API, and
  Gemini reports that have fixtures.
- Per-profile settings, with persistence, API, and UI on desktop and phone.
- Durable shared marks keyed by credential binding, with probes and
  indicators.
- Kanban fallback with warnings, durable waits, the launch gate, and notices.
- Office fallback, parking, the lift probe, and the dispatch gate.
- Metrics, public documentation, and an `AGENTS.md` observability entry.

### Out of scope

- Dynamic profiles, fallback chains, switching back after a reset, and a
  configurable maximum wait.
- Classifying `codex-app-server` rate-limit windows, and Cursor, Droid,
  Auggie, Antigravity, or Qwen limits, until a failure fixture is captured.
- Usage meters and predictive throttling.

## Technical approach

The [system design](../../specs/agents/system-design/provider-limit-recovery.md)
names the exact components. Implementation boundaries:

| Boundary | Main files |
| --- | --- |
| Classification | `routingerr/{routingerr.go,rules.go,resethint.go}`, `streams/provider_error.go`, `transport/acp/opencode_stderr.go` |
| Settings | `agent/settings/{models,dto,store,handlers}`, `lifecycle/profile_resolver.go`, `components/agent/cli-profile-*.tsx`, `components/settings/profile-model-fields.tsx`, `model-fallback-settings-shell.tsx` |
| Marks | new `agent/runtime/providerlimit`, `dynamic/{circuit.go,fingerprint.go}`, `task/repository/sqlite/{base_schema.go,dynamic_route.go}`, `backendapp/services.go` |
| Kanban | new `orchestrator/provider_limit_{failure,deferral,waker,gate}.go`, `event_handlers_agent.go`, `ceiling_seam*.go`, `task/models` deferral half |
| Office | `office/service/event_subscribers.go`, `office/scheduler/{routing_lifecycle.go,dispatch_routing.go}`, `office/service/scheduler_integration.go`, run and route-attempt repositories |
| Web chat | `components/task/chat/messages/action-message.tsx`, `status-message.tsx` |

Compatibility matrix:

| Provider / transport | Evidence shape | Behavior | Verification | Unsupported fallback |
| --- | --- | --- | --- | --- |
| Claude ACP subscription | session-limit text and zoned clock | account mark, wait/resume | existing classifier fixtures and orchestrator tests | no hint: card |
| Claude ACP API key | `rate_limit_error` with optional delay | model-scope rate mark, exact-ms wait | ACP projection fixtures (`Data` and text) | no delay: card |
| Claude ACP spend ceiling | `spend limit` / `enforced_spend_limit_reached` | account quota | classifier fixture | as left |
| OMP ACP (Anthropic models) | final assistant chunk `429 {rate_limit_error}` with optional `retry-after-ms=`; prompt then ends `end_turn` | terminal failure; spend gives account quota for the `anthropic` prefix only; exact-ms wait; fallback to another provider prefix stays eligible | adapter frame test plus classifier fixture | envelope followed by output, or format drift: normal completion |
| Codex ACP | usage-limit notice | account mark | existing fixture | unzoned date: no hint |
| OpenCode ACP stderr | structured reset | account mark | existing fixtures | minute precision only |
| Gemini | `RESOURCE_EXHAUSTED` with `retryDelay` | model quota with delay | classifier fixture | card |
| Others | none | unchanged | negative tests | existing behavior |

## ASCII UI preview

Structure is required. Spacing is illustrative. All copy is localized.

`UI-01: Fallback settings` (Settings > Agents > profile, Fallback settings
expanded; AC 001.2, AC 001.3, AC 001.5)

```text
Desktop
+-------------------------------------------------------------+
| Fallback settings          fallback: gpt-5 · limit recovery v|
|-------------------------------------------------------------|
| Require exact model (i)                              [ off ] |
| Automatic fallback (i)                               [ off ] |
| Fallback model   [ gpt-5                           v ]      |
| ----------------------------------------------------------- |
| Use fallback model when limited (i)                  [ on  ] |
|   Continue on the fallback model when a provider limit stops |
|   the primary model.                                         |
| Resume after reset (i)                               [ on  ] |
|   Resume stopped work automatically after a known reset      |
|   (up to 7 days).                                            |
+-------------------------------------------------------------+
Disabled state: when Require exact model or Automatic fallback is on, or the
fallback model is empty, the limit-fallback switch is dimmed and keeps its
value. Helper: "Needs an explicit fallback model".
```

```text
Phone (same order, single column; switches stay right-aligned, help (i)
opens the existing drawer; touch targets >= 44px)
+---------------------------------+
| Fallback settings             v |
| limit recovery on               |
|---------------------------------|
| Use fallback model when    [on] |
| limited (i)                     |
| Continue on the fallback ...    |
| Resume after reset (i)     [on] |
| Resume stopped work ...         |
+---------------------------------+
```

`UI-02: Profile row limit indicator` (Settings > Agents list; AC 002.8)

```text
| Claude  Opus profile  [model: opus] [fallback: sonnet] [limited until 11:10] |
Phone: pills wrap onto the next line; no horizontal overflow.
```

`UI-03: Chat waiting card` (task chat after a limit failure; AC 004.1, AC 004.5)

```text
+-------------------------------------------------------------+
| (clock) opus reached its usage limit. Resuming at 11:10      |
|         (in 1h 42m).                          [ Cancel ]     |
+-------------------------------------------------------------+
Phone: Cancel stays inline and content-width, >= 44px tall.
```

`UI-04: Fallback warning row` (task chat; AC 003.3; existing
model-selection warning row with a new reason string)

```text
(!) Switched from opus to sonnet because opus reached a provider limit.
```

`UI-05: Deferred task and manual notice` (AC 005.2, AC 005.4)

```text
Board card / session row:  Scheduling · waiting for opus limit reset at 11:10
Chat after a manual prompt: (i) opus is limited until 11:10. Sending anyway.
```

## Tests

| AC | Evidence |
| --- | --- |
| 007.1-007.4, 007.6-007.7 | `routingerr/classify_test.go` `TestClassifyLimitScope`, `TestClassifyRetryAfterPrecedence` (including an eight-day delay and unrepresentable values), `TestClassifyAnthropicSpendLimit`, `TestClassifyGeminiResourceExhausted`, `TestClassifyOMPAnthropicEnvelope`, `TestSanitizeRedactsRequestID`; `transport/acp/opencode_stderr_test.go` `TestProviderErrorFromErrorRetryAfterMs` |
| 007.5 | `transport/acp/adapter_prompt_test.go` `TestOMPPromptEnd*`; `orchestrator/event_handlers_transient_omp_test.go` `TestClassifyKanbanFailureOMP` |
| 001.1, 001.3, 001.4, 001.6 | `agent/settings/store/sqlite_limit_recovery_test.go`, `handlers/profile_handlers_limit_test.go`, `lifecycle/limit_policy_test.go` `TestLimitPolicyFor` |
| 001.2, 001.3, 001.5 | `cli-profile-fallback-fields.test.tsx`, `model-fallback-settings-shell.test.tsx`, `agent-profile-dirty.test.ts` |
| 002.1-002.7, 002.9 | `providerlimit/service_test.go` (including OMP provider-prefix scope and eight-day mark expiry); `dynamic/circuit_test.go` `TestCircuitCloseAndList`, failed write/missing adapter publish no state, successful retry survives registry restore, failed atomic dual-key clear preserves both marks, exact probe lease restore/stale release; `task/repository/sqlite/dynamic_route_test.go` reset_known round trip and `TestSaveCircuitsRollbackAfterSecondSnapshotFailure` (inject failure after first row operation, then reopen repository and restore both active marks) |
| 002.8 | `agent-profiles-section.test.tsx` limited pill |
| 003.1-003.6 | `orchestrator/provider_limit_failure_test.go`, including no fallback on durable mark-write failure |
| 004.1-004.9 | `orchestrator/provider_limit_waker_test.go` (same-task independent waits across restart/cancel/replay; exact lease A acquire, restart, matching completion release, sibling lift, and stale A cannot release B; failed owner persistence does not replay), `provider_limit_deferral_test.go`, `action-message.test.tsx` limit card |
| 005.1-005.5 | `orchestrator/provider_limit_gate_test.go` (including distinct-launch conflict and unknown/>7-day proceed cases; launch probe lease persists and transfers to the session/turn owner before prompt, then survives restart and releases only on matching completion), manual notice integration tests for `StartTask`, `StartCreatedSession`, `ensureSessionRunning`, `ResumeTaskSessionWithOptions`, and `promptTask`, plus `status-message.test.tsx` |
| 006.1-006.5, 002.5-002.6 | `office/service/provider_limit_test.go` eligible fallback; unadvertised fallback fails before inference without trying another model, then parks only for resume-on + trusted reset (including tests for resume-off, unknown, and >7-day reset); failed mark write keeps existing routing and does not trigger limit fallback/parking; success clears both resolved marks only after durable write and wakes a sibling; persist/recover exact lease A and prove matching completion releases A while unrelated/stale lease B is untouched; Office success uses `ReleaseProbeDurable`; injected release/cleanup failure keeps the exact owner and due siblings blocked across ordinary scheduler ticks and restart, despite cleared marks; reconciliation retries before a single sibling lift; `AgentFailed` and `AgentStopped` preserve marks/sibling wait, with independent non-limit failed/stopped probe tests proving siblings stay parked before expiry and a different waiter may probe after the exact 10-minute lease expiry; limit failure renews the mark. |
| Observability | Task 10 metric tests for mark recording, each fallback outcome, and Kanban/Office wait transitions; closed labels, structured logs, and no-op/duplicate boundaries. |

## E2E tests

The mock agent gains a `/provider-limit <model> <seconds>` scenario. Every
prompt on that model then returns a Claude-style session-limit error whose
reset is the given number of seconds after the error. Other models succeed.

| Flow | AC | Spec / project |
| --- | --- | --- |
| Toggle, disable, persist switches | 001.2-001.5 | `tests/settings/agent-limit-recovery-settings.spec.ts` (chromium), `tests/settings/mobile-agent-limit-recovery-settings.spec.ts` (mobile-chrome) |
| Limit leads to a fallback switch and a warning | 003.1, 003.3 | `tests/task/provider-limit-recovery.spec.ts` (chromium) |
| Limit without fallback: wait card, cancel, auto resume | 004.1, 004.5 | same file; mobile cancel in `tests/task/mobile-provider-limit-recovery.spec.ts` |
| Profile-row limited pill | 002.8 | `tests/settings/agent-limit-recovery-settings.spec.ts` |

## Work orders

- [ ] [Task 01: Classify limit timing and scope](task-01-classify-limit-timing-and-scope.md)
- [ ] [Task 02: Per-profile limit recovery settings](task-02-profile-limit-settings.md)
- [ ] [Task 03: Shared limit marks](task-03-shared-limit-marks.md)
- [ ] [Task 04: Kanban limit fallback](task-04-kanban-limit-fallback.md)
- [ ] [Task 05: Kanban reset waits](task-05-kanban-reset-waits.md)
- [ ] [Task 06: Automatic launch gate](task-06-automatic-launch-gate.md)
- [ ] [Task 07: Office limit recovery](task-07-office-limit-recovery.md)
- [ ] [Task 09: OMP prompt-end limit failure](task-09-omp-prompt-end-limit-failure.md)
- [ ] [Task 10: Provider limit metrics](task-10-provider-limit-metrics.md)
- [ ] [Task 08: Documentation and promotion](task-08-docs-and-promotion.md)

Dependency order: tasks 01 and 02 can run in parallel. Task 09 follows 01 and
can run in parallel with 02 and 03. Then 03, 04, 05, 06, and 07 run in
sequence. Task 10 depends on 07 and 09; Task 08 runs last.

Delivery note: Tasks 01 and 09 can ship as their own PR. That PR fixes OMP
turns that are recorded as "completed" when the provider rejected the
request; such a turn shows the recovery card instead. It does not deliver
failover. Failover for OMP needs Tasks 03-06 (Kanban) or 07 (Office).

## Verification results

Implementation: pending.

Design checks on 2026-10-03:

- `python3 scripts/list-docs.py validate`: passed (344 decisions, 1323 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation `validateCoverage` preflight: `covered`, no errors.
- `git diff --check`: passed.

## Risks

- **Probe fan-out.** Many waiting sessions share one account mark. If the
  probe never settles, siblings wait up to the 10-minute lease each round.
- **Deferral record coupling.** The provider-limit half shares
  `deferred_launch` with the ceiling half. Merge and compare-and-swap rules
  must be tested against the existing ceiling replay tests.
- **Circuit-store availability.** A failed mark write uses the existing failure
  surface; a failed clear keeps marks and siblings blocked; a failed lease
  acquire/release or owner write does not replay or wake siblings. A retained
  Office owner blocks due-run lifts even after marks close; every tick checks
  the barrier and startup reconciliation retries before lift. Recovery resumes
  only after the required durable operation succeeds.
- **Continuation semantics.** For a harness that does not keep the user turn
  after a provider rejection, a continuation instruction loses the request.
  The no-turn-event rule resends the input in that case.
- **Long resets.** A trusted reset up to seven days holds opted-in work for
  days. A longer known reset remains visible and sets mark expiry, but creates
  no automatic wait; new launches follow normal admission. Users can still
  cancel a trusted-reset wait or send a manual prompt.
- **Office scope.** The Office routing specs are deprecated in favor of
  dynamic routing. This package adds only profile-local behavior and leaves
  workspace routing semantics unchanged.
- **Unproven bindings.** Profiles without a billing type or secret-backed env
  get profile-isolated marks, so cross-profile sharing applies only to
  provable bindings.
- **OMP visible change.** Task 09 turns OMP limit replies into failed turns
  for every OMP profile, opted in or not. The match is pinned to the observed
  OMP 18.5.0 envelope; any drift falls back to today's completion.
