---
id: "02-retry-delivery-ownership"
title: "Close retry delivery and launch ownership gaps"
status: in-progress
wave: 2
depends_on:
  - "01-durable-retry-lifecycle"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RETRIES-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RETRIES-001.1
  - AC-OFFICE-AUTOMATION-RETRIES-001.3
  - AC-OFFICE-AUTOMATION-RETRIES-001.4
  - AC-OFFICE-AUTOMATION-RETRIES-001.9
  - AC-OFFICE-AUTOMATION-RETRIES-001.10
system_design:
  - ../../specs/office/system-design/automation-retries.md
---

# Task 02: Close Retry Delivery and Launch Ownership Gaps

## Outcome

Retry events reach the correct snapshot-owned destination, replay preserves exact identity and history, and run controls remain safe through uncertain provider acceptance.

## Scope and dependencies

- Resolve persisted run/snapshot routing before the destination-reader gate and exercise a capability-complete real event handler.
- Reject conflicting exact binding and preserve original action/reason on replay.
- Canonicalize disabled policy before active-only validation.
- Preserve timeline refresh/mode switching, surface Stop errors, serialize Stop submissions, and label the retry radio group without new copy.
- Trace session preparation, turn admission, durable delivery submissions, queued starts, and termination before implementing launch coordination. Persist one launch identity before external side effects; uncertainty must retain ownership and prevent automatic replay. A task external ID alone is insufficient.
- Reconcile with authoritative main, preserving current catalogue and selector identities, and address actual failed E2E leaves without weakening assertions or timeouts.

No provider idempotence assumption, live automation, real agent, operator workspace transfer, or credentials are permitted. No new service abstraction, rollout flag, or unrelated Settings refactor is included.

## Implementation acceptance

1. Run-only initial/recovery events and invalid snapshot/read failures are covered at the real event entrypoint; exact replay preserves history and conflicting bindings fail at persistence.
2. Two instances, lease expiry, queued-start replay, and crash after acceptance cannot produce a second launch. Stop/disable/delete retain unknown accepted ownership until resolved or stopped, on both SQLite and PostgreSQL.
3. Desktop and phone controls retain history mode and accessible naming; Stop failure is caught and duplicate taps do not duplicate requests. All hosted checks pass on the final batch.

## ASCII UI preview

UI-01 extends the [plan preview](plan.md#ui-01-retry-policy-target-eligibility-and-history-presentation). Composition is shared; phone controls retain their existing full-width 44px touch targets.

```text
Retry policy [named radio group]
  [Do not retry] [Finite] [Infinite]
Run history [Attempts / Timeline]
  [Stop retries] -> [busy, disabled] -> refreshed same history mode
  Failed stop -> localized error; run ownership remains visible
```

The preview specifies control state and accessible naming, not spacing or translated text. Existing selectors and localized keys remain authoritative.

## Files and verification

Primary boundaries: `internal/automation/{service,store,retry_operations,retry_recovery}.go`, `internal/orchestrator/{event_handlers_automation,automation_ceiling_start,task_operations}.go`, the automation run hook, retry policy section, and desktop/phone automation E2E coverage.

Hosted verification commands:

```sh
make -C apps/backend test
make -C apps/backend lint
cd apps/backend && go test -race ./internal/automation ./internal/orchestrator
cd apps/web && pnpm run typecheck && pnpm test --run
cd apps/web && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Commands are separate CI steps with the appropriate working directories. Broad checks run on hosted CI; only tiny synthetic proofs and explicit formatter paths run locally in inspected credential-free containment. PostgreSQL concurrency tests use the existing hosted fixture contract.

## Results and remaining gate

In progress. Independent corrections have permanent regressions for snapshot-owner routing, immutable exact binding and history, disabled-policy normalization, timeline mutation/mode refresh, Stop failure and duplicate requests, and accessible radio naming on desktop and phone. Tiny isolated proofs passed for seven hook cases, the existing policy tests, the routing entrypoint with boundary fakes, and the exact binding SQL with private SQLite fixtures. These are not full React, full backend, PostgreSQL, race, or E2E results. Explicit-path contained formatting includes new test files. Hosted verification remains outstanding.

### Reviewed launch ownership contract

The local implementation separates task creation from provider launch. Task-only operation commitments retain their creation identity but cannot authorize provider replay. The launch ledger reserves the immutable task/session/turn/submission tuple before effects, then fences admission and acceptance across service instances. Acceptance that cannot be resolved remains visibly pending; neither lease expiry nor absence from one executor releases ownership.

The following implementation boundaries require hosted verification:

1. Reservation and capacity admission serialize on the automation row in both dialects. Protocol provenance is recorded with admission and inherited by scheduled children; unresolved legacy provenance cannot reserve another launch.
2. Fresh and restored-workspace starts carry the same reserved identity into the existing initial-submission admission and acceptance receipts. Commandless workspace promotion decides its full launch path before consuming the single-use provider admission.
3. Queued release carries the same tuple and claims a fresh token only from definitively deferred work. Preparing, dispatching and uncertain work cannot be reclaimed after a lease expires. Accepted evidence can be reconciled only onto the original tuple.
4. Controls revoke group generations and future publication before attempting termination. Preparing, dispatching, accepted and unresolved legacy ownership remain until an exact terminal event or definitive synchronous stop. Retained owner tombstones fence delayed session/turn creation after settlement and history deletion; transaction-scoped exact manual permits preserve explicit manual creation without metadata authority.

Required semantic tests exercise the actual automation event, generic start, ceiling queue release, recovery and control endpoints: two service instances; lease expiry during blocked admission; acceptance followed by crash before binding; delayed queue release; stop/disable/delete racing admission; replay with changed binding or generation; and concurrent capacity admission. Provider boundaries use deterministic fakes. The source must not be pushed while these guarantees remain unimplemented.

### Integration and existing CI evidence

The contributor rebase `3ebe7b3072a5065d8c8be3ffda2ad1a7e367e707` already includes authoritative main `02ff0578357040b0546ea17cd9a00dff9ca9ee3b`. Its source/history and transitive inputs were statically audited before reconciliation. Forty saved owned files merged without conflicts; the previous integration/index is preserved in the private recovery ref and external byte/hash evidence. This retains catalogue, profile ordering, clarification, task transfers, agentctl authentication, journal shutdown/transactions, and the current CI project partitions and test-only filters. Corrective PR #4399 remains open at the last authenticated read; its PostgreSQL profile-order fixture correction must enter through a normal merged main rather than a duplicate patch. Final live-main reconciliation and hosted validation remain outstanding.

The old exact-head E2E failures remain real. Shard 5 failed visible diff line metadata; the reconciled helper addresses the observed positioning boundary but needs hosted validation. Shard 3's prompt label tap left its checkbox unchecked. The first failure has a screenshot and accessibility snapshot but no trace; its retry passed. Those artifacts do not establish a product root cause, and neither the assertion nor fail-on-flake policy is weakened.

### Launch implementation progress (unverified)

The current local batch adds versioned admissions and immutable launch records, non-expiring claim/acceptance fencing, identity propagation through session and turn creation, same-identity queue replay, cancellation/deletion retention, and actual provider acceptance callbacks. Database insertion guards cover both versioned and unresolved legacy retry rows. The exact SQLite legacy INSERT bypass has a semantic RED/GREEN proof with ordinary task behavior preserved (`../pr3898-evidence/ownership/guard-*.log`). Added permanent tests cover actual task turn and executor session persistence and the normal automation launch entrypoint with a fake provider boundary; these have not run in hosted CI yet.

The stop boundary audit found that the existing coordinator acknowledgment precedes detached teardown. A separate exact-execution synchronous stop now retains uncertainty on a failed acknowledgment and permits retrying that same execution without adopting a successor. Cold continuation recovery, all persisted mappings/SQL dialects, full entrypoint coverage, lint limits, and latest-main integration remain under review. No push or merge has been attempted; these partial results do not satisfy the delivery gate.

The contributor rebase to 3ebe7b307 was statically reconciled onto main 02ff0578; all 40 saved owned files merged without conflicts. The pre-binding task-creation interval now has metadata association guards in both SQL dialects and a regression through the actual StartTask and older PrepareSession boundaries. These Go tests remain unexecuted pending hosted CI.

Static legacy entrypoint tracing found that startTurnForSessionWithOwnershipChecked may reuse an existing turn, so INSERT guards alone do not fence that warm path. Canonical recovery-block quarantine/backfill now uses the existing checkSessionRecoveryBlock boundary and actual incarnation/generation, without fabricated acceptance evidence. Permanent tests cover the actual block lookup, older warm prompt admission, ordinary sessions, schema replay and both dialects. These remain unexecuted pending hosted CI.

### Retired admission compatibility

Run-history deletion and a known-execution stop do not revoke a separate delayed worker by themselves. Persist retry admission owner tombstones independently of history rows, including the task owner acquired during binding and the run owner carried in late task metadata. Session and turn INSERT guards must consult this retained evidence after cancellation, supersession, terminal settlement, and history or automation deletion. Only a matching live reserved tuple may admit an automatic creation. Preserve explicit manual creation through a server-owned request-origin context authorizing the exact creation ID inside the repository transaction; neither task metadata nor inherited session metadata grants that exception. The transaction inserts a temporary exact-kind/task/creation permit, performs the guarded INSERT, and removes the permit before commit; failure rolls both back. Older dispatchers cannot inherit a permit from product metadata. Tests must cover a delayed older PrepareSession, late task creation with automation_run_id after stop/delete, unrelated ordinary creation, explicit manual session/turn creation, and a new independently reserved retry on a reused task. Retained tombstones carry ownership and revocation only, never fabricated provider acceptance or termination.


The retained-admission contained SQLite proof reproduces cancellation's old INSERT bypass (semantic RED exit 1), then rejects delayed session/turn creation after stop, history deletion, and automation deletion (GREEN exit 0). It covers late task metadata ownership, copied manual metadata refusal, exact transaction manual permits, ordinary tasks, independent reserved retry reuse, disabled admission, and legacy owner tombstones. Source hashes and scope are recorded in `../pr3898-evidence/ownership/retired-proof/`; this is literal SQL/template validation, not full Go or PostgreSQL execution. Permanent tests exercise real executor preparation after exact StopRun and history deletion, automatic session opening, explicit manual preparation and turn creation, and transactional permit cleanup/rollback on both dialects. These permanent tests remain unexecuted pending hosted CI.


Control revocation and ownership settlement are separate transactions. Stop, disable, and deletion first revoke the affected live group's generation, future outbox publication, and any prepared/queued admission. Scheduled or proven unstarted work may settle immediately; preparing, dispatching, accepted, or unresolved legacy work retains its run and exact ownership evidence until definitive stop/terminal evidence. A failed control therefore cannot revive an earlier retry chain when enablement changes later. Final history/automation deletion still requires resolved ownership.


The current control proof rejects re-enabled future generations, revokes outbox work and retains preparing and legacy attempts (contained literal SQLite SQL, exit 0). The legacy stop proof checks exact task/session/turn/generation and retained committed, ambiguous or revoked operation tuples; a conflicting retained owner cannot certify termination (exit 0). The Go helper also checks the receipt write's affected-row count. Missing local execution and unbound legacy reconciliation have permanent regressions requiring visible uncertainty and refusal of deletion. A conversation fork checks pending retry ownership before its provider boundary and authorizes only its exact manual destination creation after existing access checks. The task repository's temporary permit schema replay, cleanup and rollback have both-dialect regressions.

Current-head CI evidence independently downloaded all additional failed frontend/E2E leaves and aggregates: frontend run 38041169276 failed 12 tests in five catalogue/MCP/mock files; E2E run 38041169290 shard 14 timed out in the saved profile-order selector test on all four attempts. Its retained trace reaches Quick Chat cancellation after Escape, then waits for a missing Cancel button. These shared-main failures must be mapped to normal merged #4399 before final integration; no duplicate selector patch, assertion weakening or timeout change is included here. Full Go, PostgreSQL, race, React and E2E validation remain outstanding.


The stop-entrypoint audit found that a task-only projection lookup omitted accepted-but-unprojected legacy operations. The local batch now includes observed operation ownership in the task lookup, validates partial binding consistency and hydrates only the returned view. Operation capture/backfill retains task ownership and applies canonical legacy quarantine before projection. A contained SQLite causal proof failed without those operation triggers (semantic RED) and passes with them, including history deletion and ordinary-task preservation (GREEN). Initial capsule-key and fixture-column setup failures are retained separately and are not semantic RED. A permanent actual StopRun regression covers unknown runtime acknowledgment, retained unprojected IDs, successful exact acknowledgment and cleanup-claim completion; PostgreSQL has an operation-owner/quarantine/schema-replay regression. Full package execution remains pending hosted CI.


The final main integration includes the normally merged profile-ordering fixture repairs and durable delivery reconstruction/retention. Retry initial launch persists the executor's final composed prompt under its pre-reserved canonical `prompt:retry:` submission identity before harness start. Commandless workspace promotion consumes admission only after command configuration. The shared resume entry point fences both ordinary and reserved-capacity recovery, while explicit Resume, Fresh Start and Continue from History reject unresolved retry ownership before settlement or continuation checkpoint mutation. State-only delivery reconciliation retains main's authenticated identity checks and does not authorize a new retry launch.

Verification remains bounded: the exact retry callback with deterministic dependency fakes failed when SQL admission was omitted (exit 1), then passed with final prompt, reserved identity, attachments and refusal cases (exit 0). This does not execute the complete Go/SQL/provider path. Permanent actual-entrypoint tests add canonical SQL-before-provider admission and unchanged turn identity across explicit recovery actions; full Go/PostgreSQL/race/E2E execution is pending hosted CI. The owned sandbox was re-inspected and restarted after machine shutdown; the initial stopped-container error was setup failure, not semantic RED.


Silent restart recovery also treats a retry continuation on an ordinary-origin task as automation-owned. Initial owner eligibility and each owner revalidation fence that session before source cleanup, queue pause, candidate allocation or checkpoint advancement. Candidate allocation revalidates ownership independently. The ordinary non-retry path remains eligible. Exact owner-method proofs with dependency fakes fail without these guards and pass with them; the permanent real-repository regression asserts source execution, queue and checkpoint preservation. Full package and race execution remain hosted-CI work.


Hosted CI on remediation head `13467a6c` exposed a legacy recovery visibility gap: terminalization rejected an unknown launch binding without the typed uncertainty error expected by reconciliation. Unbound legacy recovery now records uncertainty before considering outbox replay or terminalization, retaining its task/session/turn fields and open status. Modern replayable admissions remain untouched. An exact reconciliation-method proof with deterministic dependencies failed against the previous source and passed after the correction; the actual SQL update also passed an isolated in-memory SQLite proof with modern, stopped, ordinary and terminal controls. Full service and PostgreSQL verification remains pending the replacement hosted run.

The dispatch contract does not equate a failed provider response with termination. Updated fixtures reserve their immutable identities before dispatch and assert that ambiguous work never invokes a second callback or schedules a child. The retry-child fixture supplies exact terminal evidence before separate, idempotent failure finalization. Pre-binding history deletion succeeds only after a definitive stop of the complete retained operation tuple; task-only legacy operation fixtures require deletion to fail closed with no stop or task deletion. These permanent regression assertions are authored, awaiting hosted execution.
