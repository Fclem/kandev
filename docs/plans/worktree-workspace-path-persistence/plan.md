---
created: 2026-08-25
status: draft
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
legacy_specs: []
---

# Implementation Plan: Worktree Workspace Path Persistence

## Overview

Repair the launch and recovery persistence boundary so the worktree path used by
the agent is also the canonical task-environment path consumed by Files and
Changes. First add a failing backend regression spanning lifecycle response
mapping, environment repository persistence, and session projection. Then make
the smallest path-ordering and persistence changes, followed by one Playwright
restart scenario proving the user-facing surfaces remain attached to the task
worktree.

## Confirmed root cause

The current task launched ACP in its generated task worktree, but durable
`task_sessions` and `task_environments` retained the source repository path and
the environment repository row retained no physical worktree tuple.
`persistLaunchState` does not synchronize `TaskSession.WorkspacePath` from the
successful launch response. When a compatibility worktree path is absent,
`computeWorkspacePath` chooses `LaunchAgentRequest.RepositoryPath` before the
resolved `LaunchAgentResponse.WorkspacePath`, allowing recovery to select the
clean source checkout.

## Scope

### In scope

- Preserve resolved worktree identity and manager-originated
  created/recreated/reused outcomes plus compensation tokens across lifecycle
  and orchestrator boundaries.
- Prefer a successful resolved workspace over the source checkout for worktree
  executors.
- Atomically persist canonical environment/repository/session state under
  environment/session revisions and immutable launch-attempt guards.
- Claim/journal before lifecycle runtime creation or registration; propagate
  attempt identity through manager execution and recover `executors_running`.
- Persist exact session/route/credential before-image and restore only by
  attempt/revision CAS.
- Require complete executor-specific physical tuples before ready finalization;
  cover zero-repository workspaces with a workspace journal member.
- Resolve every environment-backed workspace consumer from the canonical
  environment, never session/source input.
- Protect resetting/nonterminal-journal paths from storage maintenance.
- Preserve concrete tuples, ready-only attachment, Local/Docker/SSH/Sprites
  compatibility, typed errors, and restart Files/Changes/terminal behavior.

### Out of scope

- Moving or copying files between the task worktree and source checkout.
- Mutating existing live database rows by hand or adding a filesystem-scanning
  startup backfill.
- Changing task worktree ownership or cleanup policy.
- Frontend fallback logic, new UI, copy, or settings.
- The separate GitHub title PR linking implementation.


## Technical approach

### Lifecycle response contract

Implement journals plus required `LaunchAttemptRuntimeStore`; migrate/model
`executors_running` attempt ID/runtime generation and CAS every registration/
repair/delete. Claim/create/row-member commit/process Add preserve exact owner.

Construct/wire first. Reconciler probes, then RecoverExecutions calls RecoverAll
once. Any partial/runtime/register error yields no token/Add/reconnect and aborts;
all-success token-bound Start reconnects/loops once.

### Canonical path selection

Task resolver exposes attach, exact-environment, and claimed-materializer modes.
Session provider requires ready exact binding; environment provider projects
that exact row without newest-session selection; materializer alone accepts its
creating attempt. No generic/sibling fallback. All consumers choose a mode.

### Environment and session persistence

Source attempt stores admission identity plus worker token/incarnation, recovery
epoch, DB deadline and heartbeat. Delete flags only. Live owner compensates;
expired claim alone permits atomic recovery token/epoch rotation. Every
heartbeat/effect CASes unexpired identities; terminal release precedes delete.

Binding digest schema-validates strict I-JSON, NFC strings, canonical paths, and
explicit null for every optional field, then RFC8785-canonicalizes. It includes
task policy, all repository materialization fields, and folders sorted
`(position,kind,id)`. Source/reparent/policy writers increment generation;
finalization locks/recompares before ready.

`FinalizeTaskEnvironmentLaunch` validates binding/tuples, then atomically commits
environment, slots, session projection/state, launch journal, and conditional
STARTING lifecycle outbox. No post-finalization session/event write. Ready attach
remains read-only revision/digest bind.

Reset atomically claims a durable journal with reset UUID,
ready/stopped/failed source, `resetting` revision, and exact non-running
bound-session snapshot only when no
STARTING/RUNNING borrower, live runtime, or nonterminal attempt exists. Before
destruction rollback restores source/clears token; afterward completion clears
only unchanged non-running bindings, commits journal, and deletes the exact
environment. Startup recovers every resetting generation; late callbacks cannot
mutate a replacement.

One backend `TaskAdmissionStore` row per task is backfilled/created atomically.
Task/GitHub/cleanup/storage share Claim/Validate/RequestDelete/Release and exact
lease token/generation. Launch/source/attach/reset/delete/storage serialize
cross-process; attach holds through bind, delete through cascade, crash recovery
through terminal journal.

CanonicalProjectionV1 makes every optional envelope field explicit null and
strictly validates/normalizes envelope, arrays, payload, and tombstone once at
all ingress/replay paths through a versioned lane registry. Its metadata-bound
digest prevents cross-lane reinterpretation. Incomparable conflicts retain a
pair-derived canonical resync object/idempotency key and clear only by CAS on
matching authoritative proof. Lane-set migration uses task/environment anchored
scope FKs, conditional checks, partial uniques, cascade members, catalog/row
verification, and transactional interrupted-upgrade replay. Every claim, ACK,
merge, conflict, tombstone, retention, and outbox operation selects candidates
unlocked then takes the published total UoW order.

Idle/ready source mutation uses `ResolveForSourceMutation`, exact `source`
owner/journal/capability, and request-touched keys only. Ready projection repair
uses `ResolveForProjectionRepair`; physical inventory is read-only and only its
derived generation CAS may precede bind. Concurrent reuse sees preparing.

Define one `IsTaskEnvironmentAttachable` policy used by preflight, waiter,
shared-group binding, and session-binding repository queries: only `ready` is
attachable; stopped and unknown values fail before binding.

For `worktree`, `Executor.validateReuseEnvironmentInventory` calls a required
side-effect-free `CanonicalWorktreeValidator`. Input includes task/workspace/
environment IDs, semantic marker layout, task directory, repository ID,
server-loaded source repository path, slot, and persisted worktree tuple. The
manager checks task-root containment without symlink escape; actual marker
fields; manager record environment/task/repository/source/path/branch; and
read-only Git registration.

Dispatch is executor-specific: Worktree uses the new host validator; Local,
Docker, SSH, and Sprites retain their existing local/runtime-handle/inventory
validation and valid inventory-only shapes. The host validator is never applied
to them.

Add `ErrWorkspaceReuseUnsupported` and map all reuse errors from
`spawn_session_kandev` to exact `CONFLICT` details:

- preparing: `{reason: workspace_preparing, recoverable: true,
  retry_after_ms: 1000, action: retry}`;
- unsafe: `{reason: workspace_reuse_unsafe, recoverable: true,
  retry_after_ms: 0, action: reset_environment}`;
- unsupported: `{reason: workspace_reuse_unsupported, recoverable: false,
  retry_after_ms: 0, action: choose_executor}`.

Sanitize messages/details against path, branch, credential, token, and executor
secret disclosure.

### Existing frontend projections

No frontend fallback or UI change is planned. The task-session API and all
backend workspace consumers use the canonical resolver, then expose the
synchronized `workspace_path` projection consumed by Files. Changes uses the
same validated environment/repository inventory.

## Tests

- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1` and `.6`: orchestrator
  integration tests start with distinct source/worktree paths and assert a
  ready environment has one complete, active canonical repository tuple per
  expected slot.
- `.3` and `.5`: primary, additional, and restored sessions reference the same
  canonical environment/workspace while retaining independent runtime IDs; the
  additional terminal opens in that workspace without changing Git state,
  tracked files, or untracked files.
- `.1` and `.6`: every preflight, waiter, shared-group binder, and
  session-binding query rejects stopped/unknown states. The worktree validator
  rejects missing physical fields, duplicate/inactive slots, outside-root and
  symlinked paths, wrong marker task/workspace/layout, wrong manager environment
  identity, absent/mismatched manager records, unregistered worktrees, and
  repository/source/worktree path/branch mismatches without writes.
- `.6/.8/.9`: nine revisions plus launch/source/reset/admission journals,
  deterministic digest and guarded compensation.
- Separate handles race all claims. Source delete live-wait/expired takeover
  rotates worker incarnation/epoch and rejects delayed callback before handoff.
- DB-clock tests advance expiry without sleeps; live owner cannot be stolen.
- Partial runtime recovery yields no token; successful Start recovers once.
- Repository replacement versus materialization/crash is source-journal fenced;
  finalizer commit/crash proves STARTING outbox cannot split from ready/session.
- `.1`, `.2`, `.5`, `.7`: attach/reset barriers cover revision/digest TOCTOU,
  attach-first/reset-first/late-reset orders, no session on CAS loss, and no
  manager/lifecycle/Git/file/borrower mutation on invalid ready attach.
- Compatibility tests prove Worktree-only host validation and valid
  Local/Docker/SSH/Sprites paths. `.4`/`.11` tests pin exact error envelope,
  messages, four-key details, values, unknown/internal omission, mapping, and
  redaction.
- Conflicting-path tests cover launch request, resume/restart, passthrough
  attachments, terminal/runtime prep, API Files root, and Git Changes root,
  proving bound environment authority and incomplete-state failure.
- Worktree finalization rejects source paths and incomplete/extra/inventory-only
  response/member/slot sets; repo-less Worktree accepts only its workspace
  member.
- Projection matrix runs outbox write/replay, boot hydration, WS, REST, and MCP
  through registry/version/CanonicalProjectionV1; each rejects unknown version,
  cross-lane payload, digest mismatch, and legacy metadata except registered v1
  normalization. Migration tests cover interrupted replay, partial-null
  ownership, scope mismatch, quarantine, pair-resync CAS/late response, and
  per-class reverse lock schedules.
- Version/framing tests pin legacy-v1-only normalization, schema/lane version
  identity, set/order/map collision rules, direct-ingress dead letters,
  response-first and request-first resync staging/CAS, immutable environment
  deletion ordering, and Windows interrupted replacement recovery.
- Corpus `apps/packages/projection-canonical-v1-vectors.json` is loaded by Go/TS
  CI. Valid JSON numeric spellings (including `-0`, exponent, decimal) are
  accepted then canonicalized; only duplicate members, non-I-JSON unsafe values,
  NFC collisions, overflow and schema failures reject. Each vector stores raw
  input, canonical bytes/digest or exact error code.
- SQLite/Postgres and transport schedule blocks publication, commits delete/no-
  reuse tombstone, enqueues stale update/ACK, then releases publication and
  proves no upsert plus terminal rejection when parent row is absent.
- SourceKeyV1 vectors pin null/empty branch, UUID/branch aliases, canonical URL/
  path, Unicode, duplicate key, and changed attachment revision. Delete test
  atomically commits epoch/tombstone/delete outbox before releasing publisher.
  Windows reset tests cover protected unsafe recovery, archive/delete/GC block,
  stale CAS, restore-or-owned-marker destroy, and ordered release.
- Publication schedule blocks before send, commits delete, sends stale bytes, and
  proves receiver tombstone/fence rejects durable/client apply and checkpoint.
  It does not claim to retract an already-emitted network frame. SourceKeyV1
  vectors include full branch tuple, local locator, URL aliases/query rejection,
  null/empty/NFC. Windows schedules cover reset versus archive/delete,
  crash/retry/takeover, and permanent-missing retirement.
- Barrier test blocks archive worker after precheck/member lease, unarchives and
  creates successor resource, then releases stale worker; SQLite/Postgres plus
  restart/takeover prove cancellation CAS cannot delete successor.
- Transport/DB schedules cover two consumers isolated ACK, reset-only stale
delivery, reservation abort/claim/crash/delete replay, and cancellation
eighth-attempt barrier mapping.
- Deterministic tests cover cancellation claim-crash/failed outcome, delivery
  apply-vs-reset/crash/duplicate ACK, reservation lease publish takeover/delete,
  and multi-consumer stream ordering.

## Coverage map

| Acceptance criterion | Owning boundary | Planned evidence |
| --- | --- | --- |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1` | `Executor.validateReuseEnvironmentInventory`, `BindSessionToReadyEnvironment` | `executor_workspace_reuse_test.go` ready/invalid inventory and attach CAS |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2` | Ready attach-only dispatch | `executor_workspace_reuse_test.go` no manager/lifecycle mutation |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3` | Executor-specific runtime creation | `executor_multi_repo_test.go`, restart E2E runtime IDs |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4` | Typed reuse sentinels and MCP mapper | `workspace_binding_test.go`, `spawn_session_test.go` |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5` | `CanonicalSessionWorkspaceResolver` and API projection | conflicting-path unit tests plus Files/Changes/terminal E2E |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.6` | binding-generation plus physical tuple finalization gate | attachment-race and response/member/slot/digest tests |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.7` | Attach lease and read-only validator | attach/reset barriers plus restart E2E Git invariants |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.8` | Durable attempt journal and startup recovery | `materialization_attempt_test.go`, migration crash failpoints |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.9` | Resume/prepare-only rollback | `executor_resume_test.go` attempt-scoped state/credential cases |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.10` | Manager journal compensation | worktree created/recreated/reused/mixed crash tests |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.11` | `spawn_session_kandev` error envelope | exact schema/value/redaction tests |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.12` | Source/projection admission state machine | ready source lifecycle, concurrent preparing, and read-only projection repair tests |

## E2E tests

Add `apps/web/e2e/tests/session/worktree-workspace-recovery.spec.ts` for
`AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3`, `.5`, `.6`, and `.7`:

1. Use `test-base` fixtures `{testPage, apiClient, seedData, backend}` and
   `apiClient.createTaskWithAgent` with
   `executor_profile_id: seedData.worktreeExecutorProfileId`,
   `repository_ids: [seedData.repositoryId]`, and
   `/e2e:untracked-file-setup`.
2. Wait for `"untracked-file-setup complete"`; assert Changes and Files expose
   `untracked_test.txt`, terminal `pwd` is the task worktree, and
   `path.join(seedData.repositoryPath, "untracked_test.txt")` does not exist.
3. Spawn an additional session, assert a different runtime/session ID, same
   terminal CWD, and unchanged marker/Git state.
4. Call `backend.restart()`, `testPage.reload()`,
   `SessionPage.waitForLoad()`, and `waitForChatIdle()`; poll
   `apiClient.listTaskSessions(taskId)` for restored readiness.
5. Attach another session and reassert Files, Changes, terminal CWD, independent
   IDs, unchanged Git state, and source-checkout absence.

On a fresh worktree run `cd apps && pnpm install --frozen-lockfile` once. The
required `pnpm e2e:run` command is the managed runner and rebuilds backend, web,
mock-agent, and packaged fixtures before the Chromium scenario. PostgreSQL
coverage requires `KANDEV_TEST_POSTGRES_DSN` and is also exercised by the
required **Backend Postgres** (`postgres-boot`) CI job.

## Work orders

- [ ] [Task 01: Persist resolved worktree paths](task-01-persist-resolved-worktree-paths.md)

## Verification results

Pending.

## Risks

- Path selection differs for host worktrees, SSH, Docker, Sprites, local
  repository execution, and repository-free sessions. Tests must preserve every
  existing executor-specific rule while changing only worktree fallback order.
- `task_environment_repos` is the physical-worktree source of truth under ADR
  2026-08-08. Updating only `task_sessions.workspace_path` would mask the defect
  and leave recovery unsafe.
- Backend restart tests can race session recovery. The Playwright test must poll
  backend/session readiness rather than extending UI timeouts.
