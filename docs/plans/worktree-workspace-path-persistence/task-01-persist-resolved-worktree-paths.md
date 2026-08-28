---
id: "01-persist-resolved-worktree-paths"
title: "Persist resolved worktree paths"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.6
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.7
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.8
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.9
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.10
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.11
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.12
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Persist resolved worktree paths

## Summary

Make the resolved host worktree durable across the lifecycle adapter,
task-environment inventory, session projection, and backend recovery. Add
backend Red-Green-Refactor coverage first, then prove Files and Changes continue
targeting the agent worktree after restart.

## In scope

- Source/materialization/reset use one relational admission-attempt owner plus
  worker token/incarnation/recovery epoch/DB deadline. Atomic claim/release and
  startup cross-check reject mismatched/orphan ownership; every transition CASes
  the exact unexpired identities.
- Workspace finalizer atomically includes conditional STARTING lifecycle outbox;
  no separate session/event transaction remains.
- Add eleven revision/projection/attempt columns, relational fenced journals,
  and canonical task/repository/folder digest.
- Owner cleanup barrier blocks admission, drives journals terminal, marks
  owner-deleted atomically, then deletes; missing-owner nonterminal is fatal.
- Persist session/environment revisions, projection generations, and complete
  before-images.
- Route lifecycle through exact attach/environment/materializer resolver modes;
  no newest-session, sibling, or creator fallback.
- Enforce exact Worktree response/member/slot/digest, containment, manager/Git
  registration, and source-path rejection before ready CAS; preserve explicit
  non-worktree inventory-only rules.
- Enforce admission-first lock order and canonical full-row revision/binding CAS
  for every repository writer/rollback; table-driven interleavings prove no
  deadlock, TOCTOU, metadata loss, or stale compensation.
- Preserve creating-failure/shared-ready/reset semantics, bounded recovery,
  owner-side physical generations and native DB generation/revision fences,
  deterministic probes, and exact compensation.
- Reset ready/stopped/failed through explicit `resetting` generation: reject live
  borrowers, snapshot non-running bindings, rollback before destruction, clear
  exact unchanged bindings and delete exact environment after destruction, and
  recover/retain journal by token/revision.
- Make ready additional/restore physically read-only; bounded projection claims,
  shared/exclusive leases, expiry reclaim, heartbeat, and post-I/O CAS fence
  repair and Files/Changes against purge/reset/manual mutation.
- Allocate independent executor identities; same-session resume requires exact
  persisted execution/executor/runtime-generation/launch-attempt tuple CAS.
- Coordinator lease keys are stable owner identities; mutable generations are
  fences. Acquire sorted leases before admission-first DB revalidation.
- CanonicalProjectionV1 validates every ingress/replay envelope via lane registry,
  strict I-JSON/NFC/explicit-null/RFC8785 and metadata-bound digest.
- Selection, pair-derived immutable conflicts/resync ACK CAS, and stale/
  incomparable schedules are arrival-order independent.
- Add relational lane-set parent/member migrations with task/environment scope
  anchors, exact fresh/upgrade/interrupted-replay DDL convergence, and
  orphan/mismatch/cascade constraints.
- One total UoW order governs claim, ACK, member replace, merge, conflict,
  tombstone, retention, and outbox; reverse schedules prove no deadlock.
- Ready source/projection use separate constrained resolvers; source sees only
  touched keys, projection physical state is read-only until derived CAS/bind.
- Consumer expired-claim startup/poller takeover rotates epoch and replays.
- Tombstone and generic payload vectors cover malformed/duplicate/Unicode/
  numeric/cross-domain input; add crash/failpoint/concurrency coverage.
- Add crash/failpoint/concurrency/compatibility tests plus Worktree primary/
  additional/restored Files, Changes, terminal CWD, runtime, and Git invariance
  E2E coverage.

## Out of scope

- Live database edits or task-directory scanning/backfill.
- No proactive worktree recreation or source-checkout mutation outside the
  existing manager missing-path recreate flow.
- Frontend production changes.
- GitHub title PR linking.

## Acceptance

- One DB admission serializes all paths. Source/materialization/reset attempts
  persist worker epochs; delete requests without takeover, recovery rotates
  worker token after lease expiry, stale callbacks fail, and terminal release
  precedes delete claim.
- Owner deletion persists integration acks; crash resumes before cascade.
- Canonical resolver and projection-generation fence make bound environment
  state authoritative for launch, resume, attachments, terminal, Files, and
  Changes; incomplete or concurrently changed bindings fail closed.
- Worktree ready publication requires complete validated physical tuples.
  Ready attach performs no physical mutation; other executors retain shapes.
- Before/after restart, primary/additional/restored sessions retain one
  canonical workspace, unchanged Git/files, independent runtime IDs, and
  terminal CWD; storage maintenance cannot race nonterminal recovery.
- Ready source claim proceeds for request-touched keys while concurrent reuse
  gets preparing; projection repair cannot mutate physical state and binds only
  after its derived generation CAS.

## Verification

```bash
(cd apps/backend && go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/task/repository/... ./internal/worktree ./internal/mcp/handlers)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?required for PostgreSQL verification}" && go test -race -run 'TestProjectionLaneSetMigrationFreshUpgradeReplay|TestProjectionBindingDigestAbsentNullNumericUnicode|TestPostgresProjectionSnapshotExactTieClears|TestPostgresProjectionSnapshotRelationTable|TestPostgresProjectionSnapshotCaptureCheckpointBothArrivalOrders|TestPostgresProjectionSnapshotCrossEpochBothDeliveryOrders|TestPostgresProjectionSnapshotStaleNoPendingClear|TestPostgresProjectionSnapshotIncomparableVectorAcrossEpochResync|TestPostgresProjectionIncomparableConflictBothOrdersIdentical|TestPostgresProjectionLaneSetSchemaConstraints|TestPostgresProjectionFullCoverageCanonicalLaneSet|TestPostgresProjectionFullCoverageLaneAddRemove|TestPostgresProjectionFullCoverageMalformedManifestReject|TestPostgresProjectionManifestMutationBetweenValidateApply|TestPostgresProjectionGlobalLockOrderReverseScheduleNoDeadlock|TestPostgresProjectionIncrementalCannotClearTaskPending|TestPostgresProjectionPayloadStrictIJSONVectors|TestPostgresProjectionConsumerExpiredClaimStartupTakeover|TestPostgresProjectionConsumerExpiredClaimPollerReplay|TestPostgresProjectionConsumerOldEpochCallbackNoop|TestPostgresProjectionTombstoneExactTupleAck|TestPostgresProjectionTombstoneEveryTupleFieldAlteredDiscard|TestPostgresProjectionTombstoneMalformedDigestReject|TestPostgresProjectionTombstoneDuplicateKeysReject|TestPostgresProjectionTombstoneNonNFCReject|TestPostgresProjectionTombstoneMalformedUTF8Reject|TestPostgresProjectionTombstoneCrossDomainReject|TestPostgresReadySourceMutationAdmissionLifecycle|TestPostgresReadySourceResolverTouchedKeysOnly|TestPostgresReadyProjectionRepairAdmissionLifecycle|TestPostgresProjectionResolverNoPhysicalCapability|TestPostgresSameSessionPreviousExecutionTupleCAS|TestPostgresJournalOrphanDuplicateRejected|TestPostgresFinalizerStartingOutboxAtomic|TestPostgresOwnerDeletionIntegrationAck' ./internal/task/repository/sqlite ./internal/orchestrator/executor ./internal/worktree)
(cd apps/web && pnpm e2e:run tests/session/worktree-workspace-recovery.spec.ts)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans
make fmt
make typecheck test lint
```

## Files likely touched

- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/backendapp/*_test.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/backendapp/workspace_source_materializer.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/persistence.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/worktree/worktree.go`
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/worktree/materialization_attempt.go`
- `apps/backend/internal/worktree/materialization_reconciler.go`
- `apps/backend/internal/worktree/materialization_reconciler_test.go`
- `apps/backend/internal/worktree/reset_attempt.go`
- `apps/backend/internal/worktree/reset_attempt_test.go`
- `apps/backend/internal/worktree/materialization_attempt_test.go`
- `apps/backend/internal/system/storage/workspaces/types.go`
- `apps/backend/internal/task/models/workspace_binding.go`
- `apps/backend/internal/mcp/handlers/spawn_session.go`
- `apps/backend/internal/mcp/handlers/*spawn_session*_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/*_test.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/task/service/canonical_session_workspace.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_reuse.go`
- `apps/backend/internal/orchestrator/executor/*_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/service/service_workspace_sources.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_admission.go`
- `apps/backend/internal/task/service/service_branch_recovery.go`
- `apps/backend/internal/task/repository/sqlite/task_lifecycle_event_outbox.go`
- `apps/backend/internal/task/repository/sqlite/workspace_source_mutation_attempt.go`
- `apps/backend/internal/task/service/service_branches.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/backendapp/storage_inventory.go`
- `apps/backend/internal/task/service/task_environment_maintenance.go`
- `apps/backend/internal/task/repository/sqlite/executor_running.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task_repository.go`
- `apps/backend/internal/task/repository/sqlite/repository_entity.go`
- `apps/backend/internal/task/repository/sqlite/workspace_folder.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/task/repository/sqlite/*task_environment*_test.go`
- `apps/backend/internal/task/repository/sqlite/worktree_materialization_attempt.go`
- `apps/backend/internal/task/repository/sqlite/*worktree_materialization_attempt*_test.go`
- `apps/backend/internal/task/repository/sqlite/worktree_materialization_member.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_reset_attempt.go`
- `apps/backend/internal/task/repository/sqlite/*task_environment_reset_attempt*_test.go`
- `apps/backend/internal/worktree/manager_state.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/*_test.go`
- `apps/web/e2e/tests/session/worktree-workspace-recovery.spec.ts`

## Dependencies

None.

## Risks

- Do not persist a source repository checkout as a physical worktree.
- Do not make `task_sessions.workspace_path` a second ownership source; it is a
  projection of the task environment for current APIs.
- Preserve SSH remote paths, container workspace paths, local repository paths,
  and repository-free scratch workspaces.
- Synchronize restart assertions with backend and session readiness; fixed
  sleeps or increased locator timeouts are not acceptable.
- Crash recovery must complete before scheduler/runtime launch; startup failure
  is preferable to running with an unreconciled materialization journal.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/additional-session-workspace-reuse.md`
- `docs/specs/tasks/system-design/additional-session-workspace-reuse.md`
- `docs/decisions/2026-08-08-task-owned-worktree-lifetime.md`
- Live-task evidence recorded in the plan's confirmed root cause.
- Existing path-ordering tests in
  `apps/backend/internal/orchestrator/executor/executor_execute_workspace_path_test.go`.
- Existing restart fixture patterns in
  `apps/web/e2e/tests/session/session-recovery.spec.ts`.

## Results

Pending.
