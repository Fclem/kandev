---
created: 2026-08-25
status: draft
requirements:
  - REQ-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001
system_design:
  - ../../specs/integrations/system-design/github-title-pr-linking.md
legacy_specs: []
---

# Implementation Plan: GitHub Title PR Linking

## Overview

Add one backend-owned title-reference pipeline. Task lifecycle publications
identify title modifications and unarchives, while the existing session state
event identifies starts and restores. The GitHub service resolves every title
number across server-owned task repositories in cancellable background work and
persists only unambiguous matches through the existing task-PR association path.
Backend contract tests precede implementation; one Playwright scenario proves
the resulting associations appear through the existing multi-PR task UI.

## Scope

### In scope

- Parse every distinct valid `#<positive number>` reference from task titles.
- Trigger after title modification, task unarchive, and session start/restore.
- Resolve each number against all linked GitHub repositories with workspace
  automation credentials.
- Link exactly one definitive repository match; fail closed on zero, multiple,
  or indeterminate results.
- Reuse existing multi-PR persistence, events, status UI, unlinking, and
  automation behavior.
- Coalesce per-task background work without losing a trigger that arrives while
  resolution is in flight.

### Out of scope

- Task-creation-only scans or startup/periodic backfills.
- Full URLs or repository-qualified title syntax.
- Changes to the manual PR link dialog.
- GitHub issues, GitLab merge requests, Azure DevOps pull requests, or plugins.
- New visible UI, copy, controls, settings, or mobile-specific layouts.

## Technical approach

### Reasoned task lifecycle events

Lifecycle UoW atomically writes task/session/source state and outbox; workspace
finalizer includes STARTING, standalone uses full identity CAS, unarchive uses
archived-at/sequence, replacement claims source attempt.

Only unforgeable nonzero-sequence DurableLifecycleEvent on internal trigger
subject reaches GitHub; UI subjects cannot schedule. Inventory/runtime tests
reject bypass. Lifecycle claims use DB clock/epoch; conflict owner-fences
quarantine, delete force-discards, admission prevents post-GC recreation.

Title dispatcher startup requeues expired/admit pending, then wake+5s poll;
gate-owned Stop prevents late Add. Purge persists permanent task tombstone before
job invalidation; schedule requires task/admission/no tombstone, so post-GC late
events discard.

Worker buffers results; CommitTitleScanGeneration atomically revalidates
lifecycle/job/admission owner+event, requested=claimed, no invalidation, writes
all TaskPR/outboxes, and completes. Newer/conflict/takeover/purge discards.

### Reference resolution

Worker loads latest task, every link/entity through `TaskIssueStore`; missing or
invalid identity is indeterminate. Require exact task workspace and normalized
first-party host; dedupe branch links by repository ID.

Raw provider input validates scope. Fingerprint includes every identity plus
fresh secret version/memory-only HMAC; unverifiable freshness fails. Upserts
bump generation and each cache hit reloads identity/material.

Add fatal/replay-safe SQLite/PostgreSQL purge timestamp, dual counters,
event/projection outboxes/consumer offsets, and exact TaskPR rebuild list.

`MutateTaskPR` locks shared task admission; non-purge rejects delete request.
Owner coordinator passes exact TaskAdmissionLease to intent-idempotent
PreparePurge; its durable ack covers every prior writer before cascade.
Mutation still owns no-op/counters/row/outboxes and terminal GC ordering.

Backend constructs one neutral ProjectionFence for GitHub Store/event/projection
dispatchers, task projector, and owner-deletion coordinator. Each effect/ack
CASes token/revisions under it. Purge supersedes claims/inserts barrier; GC
requires exact event/barrier/effects/lower rows terminal. Branch/generic writers
preserve reserved metadata.

Preserve top-level updated/deleted payloads with additive counters/kind;
typed/map/NATS/WS normalization and boot/backend guards reject stale delivery.

Provide constructs; inject all required dependencies and Validate before side
effects. Start subscriber readiness, dispatchers, then producers with reverse
cleanup stack. Partial failure unsubscribes/stops/waits and owner-CAS releases
claims; retry uses new instance and durable recovery, no duplicates.

## Tests

- Parser/service links all unique references.
- Lifecycle/scan tests cover dispatcher lifecycle, conflict order, newer/
  takeover/purge during I/O with zero TaskPR side effects, tombstone after GC,
  owner discard, direct rejection.
- DB-clock lease tests advance without sleeps.
- Raw/cache rotates identity/secret; unverifiable fails.
- Replacement/materialization uses one source attempt.
- Store/projection cross-handle delete/purge fence tests.
- Startup tests fail each dependency/step, assert reverse cleanup, then clean retry.

## Coverage map

| Acceptance criterion | Owning boundary | Planned evidence |
| --- | --- | --- |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.1` | transactional reason result, task/session triggers, parser | concurrent-writer reason tests and title service tests |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.2` | strict access proof and identity-bound association | raw-principal and Store mutation tests |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.3` | indeterminate/ambiguity taxonomy | access-proof, `(nil,nil)`, host/workspace, conflict tests |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.4` | multi-number/repository iteration | parser/service tests and `pr-detection.spec.ts` |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.5` | asynchronous worker and retry lifecycle | blocked-provider, later-trigger, admission/Stop barriers |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.6` | dual revisions, durable event/projection outboxes | publisher matrix, projector races, wire/restart dedupe |

## E2E tests

Extend `apps/web/e2e/tests/pr/pr-detection.spec.ts` using
`{testPage, apiClient, seedData, backend}`:

1. Use existing `seedData.workflowId`, `startStepId`, and `agentProfileId`; call
   `mockGitHubReset()` and `mockGitHubSetUser("test-user")`.
2. Create and initialize Git repositories with `main` at
   `backend.tmpDir/repos/title-web` and `.../title-api`; persist both in
   `seedData.workspaceId` with provider `github`, host `https://github.com`,
   owner `acme`, and names `web`/`api`.
3. Before task/session start call `mockGitHubAddPRs` with complete objects
   (`title`, `state=open`, `head_branch`, `base_branch=main`,
   `author_login=test-user`, `repo_owner=acme`, `repo_name`): web 101, api 202,
   and 303 once in each repo. The helper seeds matching access-proof identity.
4. Create/start `Review #101 #202 #303` in the existing workflow/start step,
   using the seed agent and both repository IDs.
5. `expect.poll(() => apiClient.listTaskPRs(task.id))` until exact identities
   web#101/api#202 exist and #303 does not, then open the task and assert topbar
   count 2, both multi-PR identities, and no #303 UI entry.

No separate mobile test is required because this change adds no interaction or
responsive presentation; it populates the existing provider association that
already has desktop/mobile coverage.

On a fresh worktree run `cd apps && pnpm install --frozen-lockfile` once. The
managed `pnpm e2e:run` command rebuilds backend, web, mock-agent, and packaged
fixtures. PostgreSQL store/CAS coverage requires `KANDEV_TEST_POSTGRES_DSN` and
the required **Backend Postgres** (`postgres-boot`) CI job.

## Work orders

- [ ] [Task 01: Link title PR references](task-01-link-title-pr-references.md)

## Verification results

Pending.

## Risks

- GitHub can hide inaccessible private resources behind 404. Only a PR 404
  after positive same-client repository access proof is definitive; inaccessible
  or failed proof remains indeterminate and prevents linking.
- A title edit can race an in-flight scan. The per-task dirty replay is required;
  singleflight alone would lose the later trigger.
- User-required root verification is broad and may surface unrelated existing
  failures. The focused backend and Playwright commands remain the behavioral
  evidence for this work order.
