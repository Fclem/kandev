---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
---

# Additional Session Workspace Reuse System Design

## Purpose and boundaries

Task environments durably own workspaces for primary/additional sessions and
project their resolved paths into sessions, runtime recovery, Files, and
Changes.

Workspace owns repository catalog/Git worktree mechanics. Task owns
`task_environments`, repository slots, session binding, recovery admission, and
cleanup. The environment references workspace-created physical resources; no
second owner exists.

## Requirement mapping

| Acceptance criterion | Owning design section |
| --- | --- |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1` | [Restart and session recovery](#restart-and-session-recovery) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2` | [Restart and session recovery](#restart-and-session-recovery) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3` | [Independent execution identity](#independent-execution-identity) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4` | [Executor validation matrix](#executor-validation-matrix) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5` | [Files and Changes projections](#files-and-changes-projections) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.6` | [Launch persistence](#launch-persistence) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.7` | [Restart and session recovery](#restart-and-session-recovery) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.8` | [Failure and recovery](#failure-and-recovery) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.9` | [Failure and recovery](#failure-and-recovery) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.10` | [Failure and recovery](#failure-and-recovery) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.11` | [Executor validation matrix](#executor-validation-matrix) |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.12` | [Restart and session recovery](#restart-and-session-recovery) |

## Confirmed regression
Observed regression: ACP used the task worktree while persisted session/
environment paths pointed at the source checkout and physical repository fields
were empty. `persistLaunchState` omitted resolved projection, while
`computeWorkspacePath` preferred request `RepositoryPath` when compatibility
`WorktreePath` was empty.

## Canonical workspace state

[ADR-2026-08-08-task-owned-worktree-lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
owns the boundary: environment owns workspace; each environment-repository row
owns one physical worktree; sessions bind to environment. Repository
`worktree_id/path/branch` is physical truth. Session `workspace_path` is only an
effective projection (or repository-free legacy value), must equal its bound
environment, and never supplies ownership/recovery.

Task service resolver modes are non-interchangeable:

- `ResolveForAttach(sessionID)` requires that exact binding's ready, complete
  environment and overwrites every session-derived path/slot/ID.
- `ResolveForEnvironment(environmentID)` projects that environment/slots; a
  session caller must use attach, never newest-sibling selection.
- `ResolveForMaterialization(sessionID,attemptID)` requires creating status and
  the exact launch owner/lease; only initial preparation/finalization uses it.
- `ResolveForSourceMutation(environmentID,attemptID,capability)` requires the
  ready before-image plus exact live request-owned `source` admission, journal,
  worker epoch, and resource fences. It exposes only request-touched keys, may
  enter preparing/mutate them, and never binds a session.
- `ResolveForProjectionRepair(sessionID,ownerEpoch)` requires ready canonical
  inventory and exact request-owned `projection` admission. It exposes physical
  state read-only and permits only the derived session CAS before bind.

Workspace getters delegate to the named mode. No generic/by-task/creator
fallback exists; only unbound legacy sessions use snapshots. Environment
execution uses environment mode; callers select attach, materialization,
source-mutation, or projection-repair explicitly. Session workspace is output.

## Launch response normalization

Lifecycle workspace choice is: SSH canonical remote path; else concrete worktree;
else resolved nonempty `WorkspacePath`; request repository path only for modes
without distinct preparation. Worktree never prefers source checkout over a
different resolved response path.

Orchestrator journals before mutation and recovers exact generation. Members store
parent/resource identity, before/after digests, owner token/epoch, generation,
phase/timestamps. Windows additionally stores typed before/after junction target,
target identity/proof, contained backup path/type, effect token and verified state.
Phase `replace_planned|removed|restore_pending|restored|replaced|unsafe` CASes
exact member/effect; recovery uses journal alone, retains backup until verified
terminal, and rejects invalid containment/proof.
Windows effect token is durable `(member,generation,owner,process,phase,ack)`:
lease expiry marks takeover pending but cannot release path ownership until helper
exits/acks terminal; supervisor joins or kills helper before successor effect.
SQLite/Postgres schedules pause syscall, expire/take over, install successor,
then release stale helper; stale helper cannot mutate.
`mutation_capabilities` includes member/effect/intended_fence, boot-safe process
incarnation/helper handle, phase/ack and owner generation. Claim/ack CAS exact
token; lease remains non-stealable until joined/terminated/probed for every
adapter. Stream terminal UoW locks stream, rotates generation, invalidates
reservations, rejects append/claim old epoch; tombstone is irreversible independent
of sequence/vector. Crash syscall and reservation/delayed-append schedules pin it.
Server persists reset capability hash/state `issued|consumed|expired|invalidated` with
task/workspace/environment/member/generation/nonce/expiry/revision. One UoW
CASes issued->consumed into reset attempt; duplicate/restart/expiry/cross-task/
concurrent use rejects. Trusted-local authorization issues it.

Version-1 physical identity payloads carry `resource_fence_generation`; logical
DB resources use their native generation/revision as specified below:

- refs `{exists,ref,oid,resource_fence_generation}`; remote
  `{exists,name,url,fetch_specs[],push_specs[],resource_fence_generation}`;
- worktree `{exists,source_common_dir,path,branch_ref,head_oid,
  resource_fence_generation}`; directory
  `{exists,path,file_type,device,inode,marker:{task_id,workspace_id,
  task_dir_name,layout_version,resource_fence_generation,
  workspace_projection_generation}}`;
- manager `{exists,id,task_id,task_environment_id,repository_id,
  repository_path,path,branch,branch_slug,status,revision,
  resource_fence_generation}`;
- runtime `{exists,executor_type,runtime_id,generation,status,
  resource_fence_generation}`; registration
  `{exists,row_id,session_id,runtime_id,generation,launch_attempt_id}`;
- repository entity `{exists,canonical_full_row,revision}`; link
  `{exists,id,task_id,repository_id,base_branch,checkout_branch,position,
  metadata,workspace_source_hash,revision,created_at,updated_at}`; folder
  `{exists,id,task_id,canonical_path,display_name,position,revision}`; and remote
  directory `{exists,host_fingerprint,user,path,marker_hash,
  resource_fence_generation}`.

Arrays/maps/paths normalize and equality includes the effective fence. Local Git
uses an atomic common-dir fence file under its keyed lock; directories carry it
in their marker; manager/runtime adapters CAS their record; SSH uses remote lock
plus atomic marker. Executor `registration.generation` is its fence.
`task_repository_link` is the canonical full `task_repositories` row keyed by
`(task_id,repository_id,base_branch,checkout_branch)`. Equality includes every
persisted column and canonical metadata JSON, including `workspace_source`,
comparison target, and remote-contribution keys. Its revision fences both
attachment and branch fields.

Generic update, base/checkout selection and recovery, comparison-target set/
clear, repository create/replace, source-batch and legacy add-branch insert/
update/delete, cleanup, and rollback all CAS full before ID/key/revision. A
change increments row revision and task binding generation atomically and
returns the intended revision. Compensation restores only that same full row at
the intended revision; concurrent metadata/branch/delete wins and rollback
fails closed.

Source Claim stores admission identity, worker token, process incarnation,
recovery epoch, DB-clock claim deadline, and heartbeat. RequestDelete only flags.
Live worker checkpoints/renews between bounded effects and compensates on flag.
Lease time uses shared normalized UTC-millisecond `DBLeaseClock`, never process
wall clock; effect deadline is shorter than claim window.

`RecoverSourceAttempt` may take over only after DB deadline: lock admission+
attempt, verify identity/epoch, increment epoch, replace worker/incarnation/
deadline, then compensate. Every heartbeat/effect/transition CASes all identities
and unexpired claim. Delayed owner fails. Terminal release alone permits
owner-delete's next fencing generation.

## Launch persistence

After lifecycle preparation succeeds, one repository transaction:

1. normalizes the effective workspace and authoritative repository slots;
2. CASes and persists `task_environments.workspace_path`, revision, status, and
   every canonical `task_environment_repos` tuple;
3. binds/synchronizes session and prepare metadata under expected identities;
4. when starting agent, CASes STARTING, increments task lifecycle sequence, and
   inserts `session_starting` outbox in this same transaction; and
5. commits before ready/agent-process publication.

`FinalizeTaskEnvironmentLaunch` is the required workspace lifecycle UoW; it has
no post-finalization session/event write. LaunchPreparedSession and ResumeSession
use it. Standalone non-workspace transitions alone use
`TransitionSessionToStartingAndEnqueue`. Optional finalizer/per-row fallback is
removed.

Both environment and session carry monotonic revisions. Finalization accepts
expected revisions/status/materializer plus current launch attempt. A fresh
materialization requires `creating`; recovery requires `ready`. Stale CAS loses
without clobbering the winner.

Before ready CAS, executor-specific validation is mandatory:

- Worktree with repositories requires exactly one response/member/slot per
  expected repository and branch identity, with non-empty worktree ID/path/
  branch, task-root containment without symlink escape, exact manager record and
  Git registration, and canonical workspace distinct from every source checkout.
- Repository-free Worktree requires one validated workspace member and zero
  repository slots.
- Local, Docker, SSH, and Sprites may persist inventory-only repository slots
  only under their existing executor-specific runtime/inventory validators.

Claim digest covers task workspace/parent/ephemeral/policy; every repository
link plus referenced workspace/source/path/branch/scripts/copy configuration;
and every folder path/name/position. Encoding v1 schema-validates strict I-JSON,
NFC-normalizes strings, represents every optional field as JSON null (never
omitted or zero-substituted), rejects unknown/unsafe values, then applies RFC
8785. Paths must already be canonical absolute. `workspace_source` is recursive
I-JSON; policy allows only `{mode,group_id}`. Combined sources carry `kind` and
sort `(position, repository-before-folder, id)`.

Every task-repository writer named above, source/folder mutation, task reparent/
detach, workspace move, and policy change increments affected task binding
generation in its transaction. The digest includes only workspace-owning
metadata, but comparison-only metadata still advances row/binding revisions to
fence full-row rollback.
Finalization locks/reloads generation, digest, full rows, and journal tuples;
mismatch compensates instead of publishing stale ownership.

The versioned before-image contains session state/error/full metadata,
environment/workspace binding, execution profile, route fields, and timestamps.

Every later session/routing/credential/prepare write CASes launch ID and expected
session revision, increments revision, and advances the journal's expected
revision. Final commit does the same. Rollback/recovery restores the entire
before-image only at exact launch ID/revision, clears launch ID, and increments
revision. A newer generation is preserved and makes the old attempt
`superseded`; field-by-field best-effort restoration is forbidden.

On failure, a creating materializer journals `failure_state_pending`, CASes the
unchanged environment to failed, rolls back the session before-image, stops the
exact runtime generation, compensates members, and closes the attempt. A ready
shared environment remains ready; only the failed attempt's session/runtime/
resources roll back. Errors leave the journal nonterminal for bounded retry and
startup recovery; admission remains `workspace_preparing`.

A failed creating environment requires Reset Environment before a later launch
materializes a replacement. No ready environment, starting session, routing or
credential snapshot, running agent, or nonterminal journal may survive
unreconciled into normal scheduling.

## Persistence schema and migration
- `task_sources.create_v1`;

The error-returning migration identities are:

- `tasks.add_workspace_binding_revision_v1`;
- `task_repositories.add_revision_v1`;
- `task_workspace_folders.add_revision_v1`;
- `task_environments.add_revision_and_projection_generation_v1`;
- `task_environments.add_reset_attempt_id_v1`;
- `task_sessions.add_launch_attempt_revision_projection_v1`;
- `executors_running.add_launch_attempt_generation_v1`;
- `task_environment_admissions.create_v1`;
- `workspace_admission_history.create_v1`;
- `workspace_attempt_owners.create_v1`;
- `worktree_materialization_attempts.create_v1`;
- `worktree_materialization_members.create_v1`;
- `workspace_source_mutation_attempts.create_v1`;
- `workspace_source_mutation_members.create_v1`;
- `task_environment_reset_attempts.create_v1`;
- `mutation_capabilities.create_v1`;
- `task_environment_reset_members.create_v1`;
- `stream_assignment_markers.create_v1`;
- `workspace_source_projection_outbox.create_v1`;
- `task_reset_capabilities.create_v1`;
- `workspace_source_projection_consumers.create_v1`;
- `workspace_source_projection_conflicts.create_v1`;
- `workspace_source_projection_snapshots.create_v1`;
- `workspace_source_projection_lane_sets.create_v1`;
- `workspace_source_projection_lane_set_members.create_v1`;
- `workspace_source_projection_lane_set_epochs.create_v1`;
- `task_projection_no_reuse.create_v1`;
- `task_projection_publication_streams.create_v1`;
- `task_projection_publication_leases.create_v1`;
- `task_projection_reservations.create_v1`;
- `task_resource_cleanup_jobs.create_v1`;
- `task_resource_cleanup_members.create_v1`;
- `task_cleanup_cancellations.create_v1`; and
- `workspace_source_projection_tombstones.create_v1`.
They run before runtime/workers/schedulers after ownership normalization and
never use `MigrateLogger.Apply`.
Canonical admission uses `admission_id PK,scope_kind NOT NULL,task_id NOT NULL,
environment_id nullable,mode,owner,epoch,revision`, CHECK task scope has NULL
environment and environment scope non-NULL, partial UNIQUE(task_id) task scope
and UNIQUE(task_id,environment_id) environment scope. Task UoW locks task row
then environment row when present; source/projection/launch/reset revalidate
both. Fresh/upgrade/replay reject NULL/partial/wrong parent identities.
Outbox `(stream_id,stream_epoch,sequence,event_id,lane_set_id,lane_set_epoch,
lane,member_key,manifest_digest,binding_revision,projection_generation,
source_attempt_id,envelope_digest,body,state,PK(stream_id,sequence),UNIQUE
event/lane,FK(lane_set_id,lane_set_epoch),INSERT-only identity columns)`;
reservation repeats exact outbox identity. Consumers `(stream_id,consumer_id,
generation,owner,claim_epoch,deadline,checkpoint,PK(stream_id,consumer_id))`;
snapshots `(stream_id,epoch,high_watermark,generation,body,PK(stream_id,epoch))`;
conflicts `(stream_id,lane,sequence,state,PK(stream_id,lane,sequence))`; markers
`(stream_id,sequence,event_id,PK(stream_id,sequence),INSERT-only)`; no-reuse
`(stream_id,retired_epoch,digest,PK(stream_id),no task FK)`. Dependencies:
admission history/current->stream->lane epoch->outbox->consumer->lease->
tombstone; locked checkpoint resumes committed migration only.
Admission history is append-only and callback CAS includes scope/history/worker/
cancellation; task deletion retires child->history. Coordinator UoW order:
admission->job->member->cancellation->stream->lane-set->member->claim->outbox->
assignment->consumer->snapshot->conflict->tombstone->no-reuse. Snapshot/ACK
generation CAS fences GC/reset; gap resyncs snapshot. Portable scalar identity
checks plus FKs reject partial/wrong parent. Revisions/generations are monotonic;
reset status/token/epoch claim CASes.


One `WorkspaceResourceCoordinator` owns stable global cross-process keys;
generations are fence values, never key components. Keys cover environment,
runtime, worktree, directory, container, sandbox, and integration owner identity.
Readers take shared outer environment then sorted resource leases; mutations/
cleanup take exclusive. No DB transaction waits for a resource lease: select
identities, acquire leases, then open the canonical admission-first transaction
and revalidate. Under lease that transaction allocates intended generation and
journals pending effect; lease remains through effect/probe/CAS. Takeover uses
the same stable key and reconciles before replacement/release.

Takeover/recovery selects IDs without locks, acquires sorted stable leases, then
opens admission-first UoW and locks admission/owner/journal. It verifies,
rotates epoch/token/deadline, journals recovery, releases DB locks for physical
probe/effect, then repeats lease/UoW/CAS. `ClaimExpiredAdmissionRecovery` and
every direct owner use this sequence; requests remain preparing/task-deleting
until terminal release, stale callbacks fail CAS. Reverse source/cleanup/
projection/reset/retention schedules prove no DB/lease ABBA.
`task.deleted` only notifies.

After probe reconciliation, `Manager.RecoverExecutions` calls `RecoverAll` once.
Any runtime or identity/registration failure closes clients, rolls back new
registrations, publishes/persists nothing, and aborts boot. Complete success
returns the recovery-generation token consumed once by `Manager.Start`; invalid
reuse fails before schedulers. Reconciler Start/Stop owns daily retention.

Retention deletes terminal launch/source/reset rows after seven days in batches
of 100; members cascade. Projection delete tombstones retain at least that period
and no task ID is reusable after GC. PostgreSQL uses `SKIP LOCKED`, SQLite
`BEGIN IMMEDIATE`; nonterminal rows never prune.

The final rebuild/copy column lists are centralized and exact:

```text
task_sessions:
id, task_id, agent_profile_id, execution_profile_id, executor_id,
executor_profile_id, environment_id, repository_id, base_branch,
agent_profile_snapshot, executor_snapshot, environment_snapshot,
repository_snapshot, state, error_message, metadata, started_at, completed_at,
updated_at, is_primary, is_passthrough, review_status, base_commit_sha,
task_environment_id, cost_subcents, tokens_in, tokens_out, workspace_path,
name, last_read_message_id, tokens_cached_in, route_generation, route_state,
route_reason, downstream_acp_session_id, launch_attempt_id, revision,
workspace_projection_generation

task_environments:
id, task_id, executor_type, executor_id, executor_profile_id, control_port,
status, materialization_session_id, reset_attempt_id, workspace_path,
container_id, container_bootstrap_nonce_secret_id,
container_control_auth_token_secret_id, sandbox_id, task_dir_name, created_at,
updated_at, revision, workspace_projection_generation

task_environment_repos:
id, task_environment_id, repository_id, branch_slug, worktree_id,
worktree_path, worktree_branch, position, error_message, status, created_at,
updated_at, merged_at, deleted_at
```

SQLite adds/backfills all named columns/relations, normalizes optional empty IDs
to NULL where their schema permits it, copies lane sets/members with constraints,
then verifies DDL, FKs/uniques/indexes/checks, orphan/mismatch/duplicate
rejection, and migration identity replay. PostgreSQL catalog-verifies the same.
DDL/copy/replay/verification failure aborts; fresh, upgraded, and replayed
schemas converge exactly.

Migrations never scan/mutate task directories. Ready validation requires marker
task/workspace/task-dir/layout identity, positive resource fence equal to the
directory/manager record, and workspace projection generation equal to the
environment. Missing/stale marker is unsafe/reset. Initial creating journals a
directory member keyed `workspace-marker:<task_dir_name>` with absent before
state and full intended task/workspace/task-dir/layout/resource-fence/projection
tuple. Under launch capability and both leases it writes then probes the marker
before any link. Crash recovery uses the same member. Compensation obtains a
fresh current-epoch capability and deletes only the exact intended tuple under
both leases; CAS/equality loss is no-op for the successor. Source/ready paths
never bootstrap ownership.

## Restart and session recovery

Recovery loads the workspace from the bound task environment and its repository
rows. Process-local execution metadata, a sibling execution ID, the source
repository path, and the session's compatibility workspace projection cannot
override a complete canonical environment.

Ready attach/restore never physically mutates. It validates inventory, then
binds or repairs only derived session projection under `projection` admission
and generation CAS. The request continues; concurrent reuse gets
`ErrWorkspacePreparing`. It never derives inventory from lifecycle response or
calls manager create/recreate. Missing directory/tuple/registration or changed
generation fails closed `workspace_reuse_unsafe`; only initial materialization
may create/recreate/finalize a worktree.

Owner precedence applies before environment status:

| Admission/owner state | Environment | Outcome |
| --- | --- | --- |
| malformed, terminal-linked, wrong identity/kind, or missing journal | any | startup-fatal; post-start sanitized internal/unhealthy |
| protected Windows `restore_pending|unsafe` | any | Reset/takeover resumes; delete stores pending intent only; no cascade |
| `delete_requested` or live/expired owner-delete | any | claim/resume cleanup; task-deleting rejection |
| exact expired-unclaimed owner | any | atomically claim recovery; `ErrWorkspacePreparing` |
| another live/expired-recovering owner | any | `ErrWorkspacePreparing` |
| request-owned source | ready | prepare, mutate journaled keys, publish ready or compensate/unsafe |
| request-owned projection | ready | attach/restore repairs derived projection only, then binds or fails closed |
| idle | ready | attach after inventory validation |
| idle | stopped, failed, unknown | `ErrWorkspaceReuseUnsafe` |
| idle | creating or resetting | corruption as first row |

Classification precedes session creation. Source mutation may claim idle/ready
and proceed; concurrent reuse sees preparing until publication/compensation.
Projection repair is physical read-only and binds only after CAS.

`Executor.validateReuseEnvironmentInventory` dispatches by executor. Worktree's
side-effect-free `CanonicalWorktreeValidator` receives server task/workspace/
environment IDs, marker layout/task directory, repository/source path, branch
slot, and persisted physical tuple. It rejects outside-root/symlink paths,
marker mismatch, missing/mismatched manager record, or mismatched read-only
`git worktree list --porcelain` registration.

Exactly one active complete row must match each expected repository/branch;
duplicates, inactive/incomplete rows, or any identity/path/branch mismatch are
unsafe. Success returns environment revision plus tuple digest.
`BindSessionToReadyEnvironment` transaction rechecks exact identity, ready/
unowned status, revision, and digest.

The environment `workspace_projection_generation` fences its path, repository
tuples, and session projections. Launch/source/reset/manual attachment/purge
increment it with their canonical mutation; projection repair locks and CASes
environment generation plus session revision/projected generation. Conflict
retries once from a fresh canonical snapshot, then fails closed.

`TaskEnvironmentStatusResetting` and immutable reset UUID share coordinator
admission with launch/attach. Begin accepts ready/stopped/failed only without
STARTING/RUNNING borrower, live runtime, or nonterminal attempt, and snapshots
all non-running bound sessions.

Claim CASes source status/revision to resetting, stores token, and increments
revision; attach reports preparing. Every effect CASes environment/token/
revision. Before destruction rollback restores source and clears token. After
destruction one transaction clears only unchanged snapshotted bindings, commits
journal, and deletes exact environment. A changed/running binding loses CAS and
recovery resumes; replacement generations remain untouched.

`TaskAdmissionStore` uses the canonical relation order above. Coordinator leases
are outer physical-effect fences; publication and consumer claims are transactional
DB rows. An apply/checkpoint/ACK holds consumer claim for its one UoW only, then
releases before transport/effect and exact-CASes on re-entry. Reverse schedules
prove no DB/lease ABBA and stale owner no-op.

### Independent execution identity
Each attached session receives unique execution/runtime/generation/registration/
process/control endpoint/ACP channel; executors may share workspace, never them.


Resume accepts `PreviousExecutionID` only when it equals the same session's
persisted execution/executor/runtime-generation/launch-attempt tuple and CASes
that tuple plus session revision. New/additional launch strips it; sibling
identity fails. Unique execution/runtime constraints enforce non-sharing.
Incapable adapters return reuse-unsupported before effects.

### Executor validation matrix

The new physical-worktree validator applies only to `worktree` executors:

| Executor | Canonical validation in this repair |
| --- | --- |
| Worktree | Ready-only status, complete environment-repository tuples, and `CanonicalWorktreeValidator`. |
| Local source-folder execution | Existing server-owned local-path/repository checks; no worktree tuple is invented. |
| Docker/remote Docker | Existing container/runtime-handle and repository inventory validation; inventory-only rows remain valid. |
| SSH | Existing remote task-directory/runtime validation; host worktree fields remain optional. |
| Sprites | Existing sandbox/runtime-handle and repository inventory validation; host worktree fields remain optional. |

The dispatcher must not send inventory-only rows from Local, Docker, SSH, or
Sprites through the host Git validator. Compatibility tests pin that behavior.
An executor unable to allocate an independent runtime returns
`ErrWorkspaceReuseUnsupported` before side effects. `recoverable` means the same
executor/request may succeed after the prescribed retry/reset; unsupported is
user-remediable only by selecting another executor and is therefore false.

| State | `reason` | `recoverable` | `retry_after_ms` | `action` |
| --- | --- | --- | --- | --- |
| Preparing | `workspace_preparing` | `true` | `1000` | `retry` |
| Unsafe | `workspace_reuse_unsafe` | `true` | `0` | `reset_environment` |
| Unsupported | `workspace_reuse_unsupported` | `false` | `0` | `choose_executor` |

Typed sentinels `ErrWorkspacePreparing`, `ErrWorkspaceReuseUnsafe`, and
`ErrWorkspaceReuseUnsupported` map to `MessageTypeError`, code `CONFLICT`,
exact four table detail keys, and fixed messages respectively: `"the task
workspace is still being prepared"`, `"the task workspace cannot be safely
reused"`, `"the selected executor cannot attach another session to this
workspace"`. MCP preserves payload. Internal errors use `INTERNAL_ERROR`,
`"failed to spawn session"`, and no details. Paths, branches, credentials,
tokens, executor secrets, and raw errors never appear. Future HTTP maps the same
sentinels to 409.

## Files and Changes projections

Task APIs use a 30-second DB-clock `projection` admission claim with process
incarnation and 5-second heartbeat. Files/Changes also hold a crash-releasing
shared `WorkspaceResourceLease`; every path/resource writer takes its exclusive
side before nested per-resource mutation leases. Heartbeat failure cancels I/O,
but writers still wait for shared-lease release. After I/O, exact admission
owner/generation and environment/session projection generations are CAS-checked;
failure discards output and fails closed. Frontend path is output only.

## Failure and recovery

- Concrete worktree tuples are never downgraded by inventory-only input.
- Ready attach/restore is read-only and fails closed without changing borrowers.
- Every materialization mutation is covered by a durable attempt journal.
- Startup reconciles nonterminal journals before scheduler/runtime launch and
  fails startup if safe terminal recovery cannot complete.
- Dedicated schema migrations are error-returning; no unexpected migration
  failure is swallowed.
- No startup recovery scans arbitrary task directories or Git registrations.

## Security

All paths originate from server-owned records, worktree manager, or successful
executor lifecycle response. Containment, ownership-marker, symlink, and
authorization checks remain. Browser payloads cannot nominate or repair paths.

## Tests

Backend suites cover:

- resolver/digest/projection and complete status classification;
- full repository-row CAS across every writer/rollback interleaving;
- total lock order plus projection/purge/reset/manual schedules;
- projection expiry/stale callbacks and DB-clock takeover;
- capability-gated mutation versus side-effect-free ready validation;
- batch/legacy-branch exact-key journal and rollback;
- independent execution/exact resume tuple; and
- replay corruption, cleanup, unchanged Git/files, and redaction.

Playwright proves agent-only untracked files in Files/Changes/terminal across an
additional session and backend restart.

## Related decisions

- [Keep Worktree Ownership at the Task Lifecycle](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
- [Preserve Live Rescan for Legacy Add Branch](../../../decisions/2026-07-27-legacy-add-branch-live-rescan.md)
