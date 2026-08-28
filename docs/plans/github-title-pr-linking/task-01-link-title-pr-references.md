---
id: "01-link-title-pr-references"
title: "Link title PR references"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.1
  - AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.2
  - AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.3
  - AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.4
  - AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.5
  - AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.6
system_design:
  - ../../specs/integrations/system-design/github-title-pr-linking.md
---

# Task 01: Link title PR references

## Summary

Implement the reasoned task/session triggers and GitHub-owned background
resolver that turns every independently unambiguous title PR number into an
existing `TaskPR` association. Prove the backend behavior with race-safe tests
and the user-visible result with one focused Playwright scenario.

## In scope

- Lifecycle UoW: workspace finalizer atomically includes STARTING outbox;
  standalone STARTING and all unarchive paths use ABA-safe CAS. Repository
  replacement claims source attempt and commits links/binding/journal/outbox.
- Scan dispatcher recovers/wakes/polls/stops; worker buffers results, uses
  bounded claim-fenced failure backoff, and one transaction writes every
  TaskPR/outbox plus completion.
- Permanent tombstone blocks scheduling after purged job GC.
- All outbox/scan leases use normalized DB UTC clock. Lifecycle terminal writes
  CAS owner/epoch; conflict quarantines, delete force-discards.
- Internal unforgeable durable event/nonzero sequence blocks direct publishers
  and post-GC recreation.
- Parse every distinct valid positive PR number from latest stored title.
- Common raw provider/factory input/result; legacy gains scoped input. Validate
  raw connection/user/app fields, fingerprint every identity field, bump
  generation on upsert, and revalidate fresh connection on every cache hit.
- Add purge timestamp, dual counters, event/projection/consumer migrations.
- Sole `MutateTaskPR` makes purge terminal, gates delayed watch/sync/post-GC
  create, and covers task/orphan tombstones plus delivery-gated GC.
- Projection owns v1/v2 generation/effects. Task delete/worker share fence;
  each effect/ack CASes token/revisions. Purge supersedes claims, inserts exact
  barrier, and GC requires all exact event/effect/lower states terminal.
- Preserve current subjects/top-level shapes with normalized stale guards.
- Provide constructs; fail-fast Validate precedes side effects. Partial Start
  reverse-cleans subscribers/workers/claims; retry uses new recovered instance.
- One admission/fence/lifecycle/scan/association/attachment/outbox lock order
  covers schedule, commit, projection, manual mutation, expiry, and purge across
  separate DB handles.

## Out of scope

- No new user-facing API, component, copy, or translation; internal dual
  revisions, tombstone/checkpoint state, and event/projection outboxes are in
  scope.
- Manual link-dialog behavior.
- Other providers or issue references.
- Startup and periodic scans.

## Acceptance

- Lifecycle ack requires durable scan generation; duplicate/stale safe, conflict
  quarantined terminal, claim epoch fences newer/purge, job retained until GC.
  Failure/cancellation/lease expiry release only by owner CAS; retries are
  bounded and a newer eligible event alone reopens terminal failed work.
- Eligible transitions atomically enqueue; STARTING identity and unarchive
  archived-at/lifecycle-sequence CAS reject ABA across every caller.
- Link only exact unique authorized candidates; ambiguity/partial evidence no-op.
- Cache hit validates fresh full identity plus secret version/HMAC; unverifiable
  freshness fails closed.
- Owner-delete admission rejects late TaskPR writers; purge ack covers earlier.
- One injected DB fence guards every projection effect/delete/purge and GC
  requires exact event/barrier/effects terminal.

## Verification

```bash
(cd apps/backend && go test -race ./internal/github ./internal/task/service ./internal/backendapp ./internal/automation ./internal/orchestrator ./internal/task/statussummary)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?required for PostgreSQL verification}" && go test -race -run 'TestPostgresLifecycleSequenceConflictQuarantine|TestPostgresLifecycleOwnerDeleteDiscard|TestPostgresTitleScanDelayedWorkerEpoch|TestPostgresTitleScanPurgeRetention|TestPostgresTitleScanLockInterleavingNoDeadlock|TestPostgresProjectionPurgeManualLockInterleavingNoDeadlock|TestPostgresTitleScanFailureBackoffAndSupersession|TestPostgresCredentialSecretFreshness|TestPostgresComparisonProjectionCrossProcessFence' ./internal/task/repository/sqlite ./internal/github ./internal/providerchange)
(cd apps/web && pnpm e2e:run tests/pr/pr-detection.spec.ts -- --grep "links every unambiguous PR number from the task title")
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans
make fmt
make typecheck test lint
```

## Files likely touched

- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/models/comparison_target.go`
- `apps/backend/internal/task/models/comparison_target_test.go`
- `apps/backend/internal/task/service/handoff_service.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/*_test.go`
- `apps/backend/internal/github/service.go`
- `apps/backend/internal/github/service_task_events.go`
- `apps/backend/internal/github/provider.go`
- `apps/backend/internal/backendapp/services_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/task/repository/sqlite/task_lifecycle_event_outbox.go`
- `apps/backend/internal/task/service/task_lifecycle_event_outbox.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_admission.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/task/service/lifecycle_event_architecture_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/github/client.go`
- `apps/backend/internal/github/auth_principal.go`
- `apps/backend/internal/github/auth_resolver.go`
- `apps/backend/internal/github/service_routing.go`
- `apps/backend/internal/github/app_credential_provider.go`
- `apps/backend/internal/github/mock_auth.go`
- `apps/backend/internal/github/service_app_auth.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/service_task_title_pr.go`
- `apps/backend/internal/github/credential_freshness.go`
- `apps/backend/internal/github/service_task_issue.go`
- `apps/backend/internal/github/personal_connection_repository.go`
- `apps/backend/internal/github/auth_resolver_test.go`
- `apps/backend/internal/github/store_connections.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/noop_client.go`
- `apps/backend/internal/github/noop_client_test.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_task_cleanup.go`
- `apps/backend/internal/github/store_task_cleanup_test.go`
- `apps/backend/internal/github/mock_client.go`
- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/mock_client_test.go`
- `apps/backend/internal/github/service_auth_test.go`
- `apps/backend/internal/github/task_pr_outbox.go`
- `apps/backend/internal/github/task_pr_outbox_test.go`
- `apps/backend/internal/github/comparison_target_outbox.go`
- `apps/backend/internal/github/title_scan_jobs.go`
- `apps/backend/internal/github/title_scan_jobs_test.go`
- `apps/backend/internal/github/comparison_target_outbox_test.go`
- `apps/backend/internal/github/task_pr_event_decode.go`
- `apps/backend/internal/github/task_pr_event_decode_test.go`
- `apps/backend/internal/github/service_task_title_pr_test.go`
- `apps/backend/internal/github/service_task_events_test.go`
- `apps/backend/internal/github/task_owned_purger.go`
- `apps/backend/internal/github/service_pr_watch_test.go`
- `apps/backend/internal/github/store_multi_repo_test.go`
- `apps/backend/internal/github/service_task_pr_detach.go`
- `apps/backend/internal/github/service_task_pr_detach_test.go`
- `apps/backend/internal/task/service/service_comparison_target_projection.go`
- `apps/backend/internal/task/service/service_comparison_target.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_comparison_targets.go`
- `apps/backend/internal/automation/github_pr_merged_subscriber.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/task/service/service_branch_recovery.go`
- `apps/backend/internal/task/service/service_branch_recovery_test.go`
- `apps/backend/internal/task/repository/sqlite/task_repository_test.go`
- `apps/backend/internal/providerchange/projection_fence.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/service/comparison_projection_fence.go`
- `apps/backend/internal/automation/github_pr_merged_subscriber_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- `apps/backend/internal/task/statussummary/projector_events_test.go`
- `apps/backend/internal/events/types.go`
- `apps/web/lib/ws/handlers/github.ts`
- `apps/web/lib/ws/handlers/github.test.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/github-slice.test.ts`
- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/types/github.ts`
- `apps/backend/internal/task/repository/sqlite/task_repository.go`
- `apps/web/e2e/tests/pr/pr-detection.spec.ts`

## Dependencies

None.

## Risks

- Treat only typed GitHub 404 responses as definitive absence; all other errors
  and empty/unknown/conflicting provider hosts fail closed for that number.
- Deduplicate repeated branch attachments by persisted repository ID without
  collapsing conflicting repository descriptors.
- Do not hold `Service.mu` during repository or provider calls.
- Register workers under the same stopped-gate lock used by `Stop`, unsubscribe
  event sources first, and prove a trigger/Stop race cannot call `bgWG.Add`
  after waiting starts.
- A task may receive a second trigger while provider reads are in flight. A
  one-shot singleflight implementation would lose that update.

## Parallelism

`sequential`

## Inputs

- `docs/specs/integrations/requirements/github-title-pr-linking.md`
- `docs/specs/integrations/system-design/github-title-pr-linking.md`
- `docs/decisions/0047-github-authentication-ownership.md`
- Existing manual association in
  `apps/backend/internal/github/service_pr_watch.go`.
- Existing task event subscriptions in
  `apps/backend/internal/github/service_task_events.go`.
- Existing multi-PR Playwright patterns in
  `apps/web/e2e/tests/pr/pr-detection.spec.ts` and
  `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`.

## Results

Pending.
