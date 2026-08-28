---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001
---

# GitHub Title PR Linking System Design

## Purpose and boundaries

The GitHub integration owns title-reference resolution because it alone can
apply workspace GitHub credentials, distinguish provider outcomes, validate
repository identity, and persist `github_task_prs`. The task system remains the
source of task titles and lifecycle events; it emits trigger reasons but does
not parse GitHub references or call GitHub.

The design preserves existing GitHub PR subjects and top-level payload shapes.
It adds no user-facing API or UI. Internal TaskPR DTOs, boot hydration, Zustand
state, and backend consumers gain event-sequence/tombstone fields. Persistence
adds separate association revision/event sequence plus durable outboxes.

## Requirement mapping

| Acceptance criterion | Owning design section |
| --- | --- |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.1` | [Trigger flow](#trigger-flow) |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.2` | [Resolution and association](#resolution-and-association) |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.3` | [Data and contracts](#data-and-contracts) |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.4` | [Data and contracts](#data-and-contracts) |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.5` | [Owned background work](#owned-background-work) |
| `AC-INTEGRATIONS-GITHUB-TITLE-PR-LINKING-001.6` | [Resolution and association](#resolution-and-association) |

## Components and responsibilities

### Task lifecycle trigger reasons

Task lifecycle UoW atomically writes eligible mutation, lifecycle sequence, and
outbox. Title-only needs no source lease. Optional repository replacement first
claims `WorkspaceSourceMutationCoordinator` operation `replace_repositories`;
UoW validates token and atomically writes task/repositories, binding generation/
digest, source journal effects, and title outbox. Physical environment changes
continue under that attempt or it commits terminal when none exist.

Repository failure rolls all back. Returned persisted row/title result drives
event; pending-title/Handoff use required UoW with no nil/legacy skip.

Workspace launch/resume/recovery passes lifecycle-event hook into
`FinalizeTaskEnvironmentLaunch`, which CASes STARTING and outbox in its existing
environment/slot/session transaction. Only non-workspace transitions use
`TransitionSessionToStartingAndEnqueue`; both require state/session revision/
attempt/environment/route/materializer identity. ABA/stale writes enqueue none.

`UnarchiveTaskReturningMutation` requires expected archived timestamp plus task
lifecycle sequence, CASes active state, increments sequence, and inserts
`unarchived` in one transaction. Direct, Handoff root/descendant, and all legacy
unarchive paths use it; concurrent archive/unarchive cannot emit stale reason.


Eligible events are unforgeable `DurableLifecycleEvent` with UoW token/nonzero
sequence, published only on `task.lifecycle.integration_trigger`. Update/pending
title, every unarchive, orchestrator/streaming/MCP STARTING use UoW. Existing UI
task/session publishers stay on original subjects and cannot schedule scans;
architecture inventory/runtime rejection tests pin closure.

### GitHub task-event subscriber

GitHub subscribes only `task.lifecycle.integration_trigger`, validates durable
capability/sequence, and enqueues scan. Existing archive/delete cleanup remains
on its separate subjects.

The service uses its existing `TaskIssueStore` dependency to load the latest
task row, task-repository links, and repository entities. Loading current state
inside the worker means a queued trigger never resolves a stale title or stale
repository set.

### Owned background work

Event CAS: duplicate no-op, stale ignore, conflicting sequence invokes quarantine.
Strictly newer increments requested once. If job running, same transaction marks
claimed generation invalidated/completed, increments epoch, clears owner/deadline,
and sets pending; stale worker never requeues. Otherwise state becomes pending.

All affected transactions share one lock-class order: task admission;
`providerchange_projection_fences`; lifecycle row; scan tombstone; scan job;
TaskPR association; task-repository attachment; then event/projection outboxes,
purge barriers, and effect rows in primary-key order. A transaction may skip a
class but never acquire an earlier one later. Candidate rows are selected
without retained locks; the always-present admission row serializes absent-row
creation. Schedule, commit, manual mutation, projection, invalidation, expiry,
purge, and deletion use this order in both dialects.

`GitHubTitleScanDispatcher.Start` synchronously recovers eligible expired running
jobs and admits due pending work before returning; post-commit nonblocking wake
plus 5-second fallback DB poll handles dropped wake/crash. Admission gate
performs WaitGroup Add under lock; Stop closes gate, cancels/waits, leaving jobs
replayable. This is job recovery, never an all-task backfill.

Claim requires `requested > max(completed,invalidated)`, due `retry_at` (NULL or
at/before DB now), snapshots generation/event, sets owner, and increments epoch.
Worker buffers provider results. `CommitTitleScanGeneration` follows the
canonical lock order and requires exact running owner/epoch/event,
`requested=claimed`, claimed not invalidated, no purge/delete; it atomically
writes all TaskPR/outboxes, clears
failure state, and completes.

Every non-commit exit invokes an owner/epoch/generation/event CAS. Shutdown
cancellation releases running to pending without charging a failure. Provider,
auth, transient-store, panic, and expired-lease recovery invoke
`FailTitleScanGeneration`, increment `attempts`, clear the claim, and either set
pending with `retry_at = DB now + min(5s * 2^(attempts-1), 5m)` or set terminal
`failed` after five failures. Only a strictly newer eligible event reopens
`failed`, resets attempts/error/retry, and schedules a new generation.
Superseded, invalidated, purged, or stale-owner failure callbacks are no-ops and
cannot requeue old work.

Conflict before completion atomically invalidates/completes claimed generation,
increments epoch, clears owner/deadline, then sets pending only if a newer valid
generation exists, otherwise idle. Conflict after completed commit is
audit-only; purge sets purged. Claim never selects invalidated generation,
preventing retry storms.

## Data and contracts

A title PR reference is a `#` followed by a positive, safely representable
decimal integer at identifier boundaries. Parsing returns every distinct number
in title order. Zero, signed values, decimal suffixes, and unsafe integers are
ignored.

Repository candidates come only from task repository links whose persisted
repository entity has:

- `provider = "github"`;
- a normalized `provider_host` equal to the supported first-party GitHub origin
  (`https://github.com`); empty, unknown, conflicting, or enterprise origins
  fail closed;
- a non-empty provider owner and provider repository name;
- a non-empty repository workspace ID equal to the task's persisted workspace
  ID; cross-workspace and legacy workspaceless rows fail closed; and
- an actual task-repository link loaded from the server-owned task store.

Repository-row host and comparison-target host have distinct canonical forms.
`Repository.ProviderHost` is normalized as an HTTPS origin
(`https://github.com`). Mapping to `ComparisonTargetRepository` parses that
origin and stores lowercase host-only `github.com`, path `owner/repo`, canonical
credential-free HTTPS `remote_url`, and persisted provider repository ID when
present. Equality compares normalized semantic fields, not the origin string to
the host-only string. Non-HTTPS, credentialed, queried, fragmented, or
mismatched URLs fail closed.

Migration `tasks.add_lifecycle_event_sequence_v1` adds
`BIGINT NOT NULL DEFAULT 0`; `task_lifecycle_event_outbox.create_v1` creates:

```sql
CREATE TABLE task_lifecycle_event_outbox (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  sequence BIGINT NOT NULL,
  subject TEXT NOT NULL CHECK (
    subject = 'task.lifecycle.integration_trigger'
  ),
  origin_subject TEXT NOT NULL CHECK (
    origin_subject IN ('task.updated','session.state_changed')
  ),
  reason TEXT NOT NULL CHECK (
    reason IN ('title_modified','unarchived','session_starting')
  ),
  payload_json TEXT NOT NULL,
  payload_digest TEXT NOT NULL,
  state TEXT NOT NULL CHECK (
    state IN ('pending','claimed','delivered','quarantined','discarded')
  ),
  claim_owner TEXT NOT NULL DEFAULT '',
  claim_epoch BIGINT NOT NULL DEFAULT 0,
  claim_until TIMESTAMP,
  quarantine_requested INTEGER NOT NULL DEFAULT 0,
  conflict_event_id TEXT NOT NULL DEFAULT '',
  conflict_payload_digest TEXT NOT NULL DEFAULT '',
  attempts INTEGER NOT NULL DEFAULT 0,
  sanitized_error TEXT NOT NULL DEFAULT '',
  terminal_reason TEXT NOT NULL DEFAULT '' CHECK (
    terminal_reason IN ('','sequence_conflict','owner_deleted')
  ),
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  delivered_at TIMESTAMP,
  quarantined_at TIMESTAMP,
  discarded_at TIMESTAMP,
  UNIQUE(task_id, sequence)
);
CREATE INDEX idx_task_lifecycle_outbox_claim
  ON task_lifecycle_event_outbox(state, task_id, sequence, created_at);
```

Unique conflict locks lifecycle row and scan job. Same ID/digest no-op. If
matching scan generation is not completed, it increments both claim epochs,
clears owners, marks lifecycle quarantined, and invalidates scan generation in
one transaction; Schedule/Commit CAS then fail. After completed scan it records
audit/alert only. Owner-delete is forced discard override. All worker terminal
writes require claimed owner+epoch; stale no-op and later sequence proceeds.

Dispatcher/subscriber lock task admission; missing/delete/purged job discards.
Only unforgeable `DurableLifecycleEvent` on internal subject is accepted.
Ordinary UI `task.updated`/`session.state_changed` cannot schedule title scans.

All lifecycle/TaskPR/projection/scan claim, renew, expiry, and retention
timestamps come from `DBLeaseClock.NowTx`, never process time. PostgreSQL uses
`date_trunc('milliseconds', CURRENT_TIMESTAMP AT TIME ZONE 'UTC')`; SQLite uses
`strftime('%Y-%m-%dT%H:%M:%fZ','now')`. Stored/compared precision is UTC
milliseconds in the same transaction. Tests advance an injected DB-clock SQL
source, never sleep.

Migration `github_title_scan_jobs.create_v1` creates:

```sql
CREATE TABLE github_title_scan_jobs (
  task_id TEXT PRIMARY KEY,
  last_event_id TEXT NOT NULL DEFAULT '',
  last_event_sequence BIGINT NOT NULL DEFAULT 0,
  requested_generation BIGINT NOT NULL DEFAULT 0,
  completed_generation BIGINT NOT NULL DEFAULT 0,
  claimed_generation BIGINT NOT NULL DEFAULT 0,
  claim_epoch BIGINT NOT NULL DEFAULT 0,
  claimed_event_id TEXT NOT NULL DEFAULT '',
  claimed_event_sequence BIGINT NOT NULL DEFAULT 0,
  invalidated_generation BIGINT NOT NULL DEFAULT 0,
  invalidated_event_sequence BIGINT NOT NULL DEFAULT 0,
  state TEXT NOT NULL CHECK (
    state IN ('idle','pending','running','failed','purged')
  ),
  claim_owner TEXT NOT NULL DEFAULT '',
  claim_until TIMESTAMP,
  retry_at TIMESTAMP,
  attempts INTEGER NOT NULL DEFAULT 0,
  sanitized_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  completed_at TIMESTAMP,
  purged_at TIMESTAMP
);
CREATE INDEX idx_github_title_scan_jobs_claim
  ON github_title_scan_jobs(state, retry_at, claim_until, updated_at);

CREATE TABLE github_title_scan_tombstones (
  task_id TEXT PRIMARY KEY,
  purge_event_sequence BIGINT NOT NULL,
  purged_at TIMESTAMP NOT NULL
);
```

Schedule transaction requires existing task, admission `delete_requested=0`, no
scan tombstone, and nonpurged job. Purge/missing-task transaction upserts
tombstone before invalidating job claim; integration ack includes it. Purged job
GC requires tombstone and seven-day cutoff. Tombstone is retained because task
IDs are never reused, so late events after job GC are discarded.

Migration `github_task_prs.add_revisions_and_purge_v1` adds/backfills:

- association revision and event sequence as `BIGINT NOT NULL DEFAULT 0`; and
- nullable `purged_at` for hard-deleted task/orphan tombstone retention.

Created rows start both at 1. Detach, restore, target, and purge increment both;
sync/status/review/check/merge changes increment event only. Exact unchanged
writes are event-free. Three `create_v1` migrations add event/projection
outboxes and consumer offsets.

Every `github_task_prs` create/rebuild/copy uses this exact final list:

```text
id, workspace_id, task_id, repository_id, owner, repo, pr_number, pr_url,
pr_title, head_branch, base_branch, author_login, state, review_state,
checks_state, mergeable_state, merge_queue_state, merge_queue_position,
merge_queue_estimated_time_to_merge_seconds, review_count, pending_review_count,
required_reviews, comment_count, unresolved_review_threads, checks_total,
checks_passing, additions, deletions, created_at, merged_at, closed_at,
last_synced_at, detached_at, purged_at, updated_at, is_draft, changed_files,
merged_by_login, closed_by_login, auto_merge_observed_at,
association_revision, event_sequence
```

All migrations are fatal/replay-safe on SQLite/PostgreSQL and verify exact
columns, copy lists, defaults, NULL counts, checks, foreign keys, indexes, and
catalogs.

Candidate identity is the persisted repository ID plus normalized
host/owner/name. Repeated task-repository links for the same repository ID
(including different branch attachments) collapse to one provider query and
one ambiguity vote. Conflicting descriptors for one ID are indeterminate
rather than guessed.

A task-repository link whose repository entity is missing, or whose GitHub
identity is incomplete/invalid, is an indeterminate candidate rather than a
skipped or definitive no-match candidate. One such candidate prevents automatic
association for every referenced number in that pass.

Title worker never uses `Service.client`; each candidate binds exact
`{WorkspaceID: task.WorkspaceID, Purpose: Automation, RepoOwner, RepoName}`.
All installation/automation/user/PAT/GHCLI/legacy-adapter/Mock providers return
common `RawResolvedCredential` and populate principal workspace from their
loaded connection. One validator rejects empty/mismatch against request and
connection before normalization/cache; no post-provider scope assignment.

All paths adapt to:

```text
ResolveRaw(ctx, RawCredentialInput{
  request, workspace_connection?, user_connection?
}) -> RawResolvedCredential
```

Legacy factories now receive workspace connection plus request. Validator
requires client, request/connection/principal workspace equality, requested user
for personal credentials, and matching source/app registration/credential
generations before converting to cached `ResolvedCredential`.

`RawScopeFingerprint` covers all identity fields above plus credential
freshness. Each adapter exposes `CredentialFreshnessReader`: authoritative
secret-store version when available; otherwise HMAC-SHA256 of freshly revealed
PAT/user/app/GHCLI/legacy material using process-random memory-only key.
Installation includes app-secret version and installation-token generation/
expiry. Raw secret/HMAC never persists or logs; unverifiable freshness forbids
cache reuse and fails closed.

Connection/user upserts transactionally compare identity/secret bindings and
bump generation. Before every hit resolver reloads connections and freshness;
key/entry fingerprint mismatch evicts, including same-identity secret rotation.

No separate title state is persisted; full PR maps to existing TaskPR.

## Trigger flow

1. Eligible mutation commits lifecycle outbox; UI event remains separate.
2. Lifecycle dispatcher selects a candidate without retaining a row lock, then
   calls required internal `DurableLifecycleConsumer.AdmitTx` with capability/
   owner/epoch.
3. GitHub `ScheduleTitleScanTx` acquires task admission, lifecycle row,
   tombstone, and scan job in canonical order; it validates all, upserts the
   generation, and marks the lifecycle row delivered in the same shared-DB
   transaction.
4. Dedicated scan dispatcher claims due job; worker reloads latest task/sources.
5. Worker parses all numbers and resolves each across every eligible repository.
6. It commits unambiguous TaskPRs and terminal outcomes, then CAS-completes only
   the claimed scan generation and loops if newer exists. Every failure or
   cancellation uses the claim-fenced release contract.

Task creation alone is not a trigger. Ordinary UI subjects, invalid capability,
sequence zero, missing/deleting task, or retained purge tombstone schedule none.

## Resolution and association

`BindAutomationRepositoryReader` uses that validated raw seam. The resolver then
normalizes/caches, while reader captures exact Client, raw principal, request,
credential/app generations, workspace epoch, and ref behind opaque token.
Strict title path forbids legacy/process fallback; broad `Client` is unchanged.
Probe/GetPR validate token, scope, ref, and live epoch before/after I/O;
rotation/reassignment is indeterminate.

Only a PR-endpoint 404 from the proved authorized reader is `ErrPRNotFound`.
Repository proof 404/false/error, generic 404, `(nil,nil)`, invalid identity,
cancellation, or any other failure is indeterminate. Mock/test clients exercise
the same adapter.

The resolver associates only when exactly one repository returns a pull request
and every other repository returns a definitive no-match. Zero matches,
multiple matches, or any indeterminate result produce no association for that
number. Resolution then continues with the remaining title numbers.

The resolver binds a `TitlePRCandidate` containing task ID, task workspace ID,
repository ID, normalized GitHub host/owner/name, and requested PR number.
Before any write it verifies the provider response has that exact
owner/name/number and a canonical-host PR URL.

`Store.MutateTaskPR(ctx, TaskPRMutation)` is the sole writer, replacing direct
Create/Replace/Restore/Update/Detach/delete-cleanup methods. Kind is
`create|restore|sync|target|detach|purge`; input carries identity, complete row,
expected revision, event kind, and optional projection. Result returns persisted
row, changed/outcome, counters, event kind, and projection action.
Title scans cannot call it directly; `CommitTitleScanGeneration` invokes its
transaction-local primitive only after scan claim predicates lock.

One serialized transaction (`BEGIN IMMEDIATE` SQLite; row
`SELECT ... FOR UPDATE` PostgreSQL) loads the current row, compares all
observable fields, and returns event-free no-op when unchanged. Otherwise it
applies the row, allocates event sequence `current+1`, conditionally allocates
association revision, and inserts event plus projection outboxes before commit.
Create starts both at 1. Restore/target/detach/purge increment both; sync
increments event only. Concurrent writers cannot share sequence or publish an
uncommitted row.
It preserves existing populatedness, merge-queue, and
`auto_merge_observed_at` first-writer latch semantics while holding that lock.

Title create additionally guards task/workspace/link/repository identity in the
same transaction and returns created/existing_active/detached/conflict.

GitHub `TaskOwnedIntegrationPurger.PreparePurge(intentID, taskID,
TaskAdmissionLease)` validates shared owner-delete token/generation,
batch-purges, terminalizes projection barriers, deletes other GitHub state, and
returns durable ack stored on cleanup job. Retry is intent-idempotent; orphan
heal uses same path and `task.deleted` is notification only.
GC requires exact purge event sequence delivered, exact purge association
revision's `purge_barrier` terminal/delivered with every effect applied or
superseded, all lower rows terminal, and seven-day cutoff. Thus no event/effect
can cross hard deletion.

Every non-purge mutation first locks task admission row and requires task,
`delete_requested=0`, no owner-delete token, and `purged_at IS NULL`; otherwise
typed `owner_deleting|purged` no-op. Purge requires exact delete
token/generation. A prior writer commits before delete request and is covered by
purge; later watch/poller/create cannot enqueue. Post-GC create still requires
task; only idempotent purge touches deleting/purged identity.

`ComparisonTarget` v2 losslessly extends v1 with association revision and full
repository identities. The existing canonical
`internal/task/models.ComparisonTarget` owns decode/validation/encode/equality;
GitLab v1 remains valid, unknown versions fail closed, and v1 without
association identity is never cleared by GitHub detach.

```text
version, provider, kind, number, head_branch, target_branch,
head_repository{host,path,provider_id?,remote_url},
target_repository{host,path,provider_id?,remote_url},
association_id, association_revision, task_repository_id
```

Backendapp constructs one `*providerchange.DBProjectionFence` and injects that
exact instance into GitHub Store/event/projection dispatchers, task projector,
and owner-deletion coordinator. No package constructor/private mutex exists.
The fence uses durable row token/generation/delete-request semantics in both
dialects.
Provider IDs and canonical credential-free clone URLs remain lossless. A valid
v1 upgrades only after exact attachment/head validation.

The ownership boundary is asynchronous and durable, not a cross-package SQL
transaction. `github.Store` may read task/repository identity in its guarded
`INSERT ... SELECT`, but mutates only `github_task_prs` and GitHub-owned
outboxes. It never writes `task_repositories.metadata`.

`TaskComparisonTargetProjector`, implemented by task service, owns all target
writes and side effects. GitHub projection payload adds the task repository's
transactionally observed `expected_projection_generation` to full provider/
association/repository/head identity.

Reserved metadata `comparison_target_projection_v1` is:

```text
generation, owner(manual|provider), association_id,
applied_association_revision, effects_acked_generation
```

Repository operation `ApplyTaskRepositoryComparisonProjection` locks the exact
attachment. A same-association lower/equal revision returns its stored replay/
effects state. Otherwise it requires expected local generation and full
identity, writes/clears v2 target, increments generation, and records applied
tombstone after clear. Provider set/detach, restore, reconciliation, manual
base-branch clear, and non-GitHub v1 all use it.

Branch recovery uses field-only `SetTaskRepositoryCheckoutBranchIfEmpty`; it
never writes metadata. Remaining generic `UpdateTaskRepository` transactionally
reloads/merges reserved projection keys and cannot replace or delete them.
Production callsites are audited off whole-row stale metadata writes.

GitHub Store reads/locks projection generation while enqueueing but never writes
task metadata. A concurrent manual change increments generation, so delayed set
is terminal stale; exact clear never removes an unrelated target.

The injected fence serializes projection effects and task/attachment deletion.
A worker selects candidates without retaining locks, then acquires admission,
fence, exact attachment, and outbox/effect rows in the global order. It validates
owner token plus association/projection generation and pending effect, executes
one complete-state effect, then marks applied. Manual projection takes
admission/fence/attachment in that order. Deletion sets delete-request and waits
for owner release; purge invalidates token and supersedes rows. Startup takeover
requires an expired claim plus effect-state reconciliation.
Purge transaction inserts exact-revision barrier with every effect superseded;
missing-owner worker terminalizes its row. Complete-state effects may replay
before deletion request, never after cascade.

An existing association is acceptable only when workspace and normalized
owner/repo match. Empty/foreign/conflicting legacy rows return `conflict`.
Different numbers/repositories create independent rows.

### Durable TaskPR and comparison-projection outboxes

```sql
CREATE TABLE github_task_pr_outbox (
  id TEXT PRIMARY KEY,
  association_id TEXT NOT NULL,
  association_revision BIGINT NOT NULL,
  event_sequence BIGINT NOT NULL,
  workspace_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  event_kind TEXT NOT NULL CHECK (event_kind IN (
    'linked','restored','synced','target_updated','detached','purged'
  )),
  subject TEXT NOT NULL CHECK (subject IN (
    'github.task_pr.updated','github.task_pr.deleted'
  )),
  payload_json TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN
    ('pending','claimed','delivered','superseded')),
  terminal_reason TEXT NOT NULL DEFAULT '',
  terminal_epoch BIGINT NOT NULL DEFAULT 0,
  claim_owner TEXT NOT NULL DEFAULT '',
  claim_until TIMESTAMP,
  attempts INTEGER NOT NULL DEFAULT 0,
  sanitized_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  delivered_at TIMESTAMP,
  UNIQUE(association_id, event_sequence)
);
CREATE INDEX idx_github_task_pr_outbox_claim
  ON github_task_pr_outbox(state, association_id, event_sequence, created_at);

-- Owner-delete CASes pending/claimed lower rows to superseded with purge
-- epoch/reason; stale claimant no-ops. GC accepts delivered/superseded.
-- Blocked publisher, delete/takeover, and both delivery orders are tested.
CREATE INDEX idx_github_task_pr_outbox_delivered
  ON github_task_pr_outbox(state, delivered_at);

CREATE TABLE providerchange_projection_fences (
  association_id TEXT PRIMARY KEY,
  owner_token TEXT NOT NULL DEFAULT '',
  fencing_generation BIGINT NOT NULL DEFAULT 0,
  delete_requested INTEGER NOT NULL DEFAULT 0,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE github_comparison_target_outbox (
  id TEXT PRIMARY KEY,
  association_id TEXT NOT NULL,
  association_revision BIGINT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('set','clear','purge_barrier')),
  payload_json TEXT NOT NULL,
  state TEXT NOT NULL CHECK (
    state IN ('pending','claimed','delivered','superseded')
  ),
  claim_owner TEXT NOT NULL DEFAULT '',
  claim_until TIMESTAMP,
  projection_generation BIGINT NOT NULL DEFAULT 0,
  effect_owner_token TEXT NOT NULL DEFAULT '',
  effect_state_json TEXT NOT NULL DEFAULT
    '{"session_base":"pending","live_push":"pending","task_updated":"pending"}',
  attempts INTEGER NOT NULL DEFAULT 0,
  sanitized_error TEXT NOT NULL DEFAULT '',
  terminal_reason TEXT NOT NULL DEFAULT '' CHECK (
    terminal_reason IN ('','owner_missing','stale','ineligible','purged')
  ),
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  delivered_at TIMESTAMP,
  UNIQUE(association_id, association_revision)
);
CREATE INDEX idx_github_target_outbox_claim
  ON github_comparison_target_outbox(
    state, association_id, association_revision, created_at
  );
CREATE INDEX idx_github_target_outbox_delivered
  ON github_comparison_target_outbox(state, delivered_at);

CREATE TABLE github_task_pr_consumer_offsets (
  consumer_id TEXT NOT NULL,
  association_id TEXT NOT NULL,
  event_sequence BIGINT NOT NULL,
  event_id TEXT NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  PRIMARY KEY(consumer_id, association_id)
);
```

The event dispatcher is the sole publisher of both existing TaskPR subjects.
Manual/watch/branch, poller/sync, mock, restore, title, target, detach, task
purge, and orphan-heal paths mutate/enqueue transactionally; direct publication
or TaskPR deletion is removed.

Wire compatibility is a clean additive cutover:

- updated subject payload remains a top-level complete `TaskPR`;
- deleted subject payload remains a top-level `TaskPRDeletedEvent`;
- both add `event_version=1`, stable `event_id`, `association_revision`,
  `event_sequence`, and `event_kind`; and
- typed pointer/value, JSON/NATS map, and websocket forms normalize through one
  decoder per subject. Missing version is accepted as legacy sequence zero only
  before sequenced state exists; unknown versions fail closed.

Existing clients ignore additive fields. New consumers retain highest event
sequence per association; deleted retains a tombstone. Boot includes active
TaskPRs plus hidden tombstones. Frontend seeds its in-memory guard from boot.
State projectors CAS sequence with projection state. Automation uses `event_id`
as its durable trigger idempotency key and advances
`github_task_pr_consumer_offsets` in the same transaction as trigger creation;
a duplicate key is successful replay.

Both dispatchers use startup expired-claim recovery, post-commit nonblocking
wake, and 5-second fallback poll. Claims select the lowest undelivered event
sequence or projection association revision respectively, block on a lower
pending/claimed row, lease for 30 seconds with UUID owner, and require owner for
ack. PostgreSQL uses `FOR UPDATE SKIP LOCKED`; SQLite uses `BEGIN IMMEDIATE`.
Delivery is at-least-once; projection calls the injected task projector and
retries transient errors.

`Provide` only constructs. Composition injects every required store,
`TaskIssueStore`, projector, authorizer, fences, DB clock, and app factories,
then `ValidateDependencies` fails before subscription/goroutine/claim if nil.

Start uses reverse-unwind stack: register subscriber readiness; start event,
projection, and title-scan dispatchers; then app/stale producers. Each successful
step immediately registers Stop. Later failure unsubscribes/stops/waits in
reverse and owner-CAS returns claims pending before returning fatal boot error.
Failed instance is not restartable; retry constructs new service, which recovers
durable jobs without duplicate consumers/claims. Missing credentials alone stays
nonfatal. Main retains final cleanup only after complete Start.

## Failure and recovery

The event handler only schedules work, so GitHub latency and errors cannot fail
or delay the originating task mutation or session transition. Worker errors are
logged with task ID, repository identity, and PR number without exposing
credentials. No candidate is linked from partial evidence.

Unresolved references are not persisted as failures. A later title
modification, unarchive, or session start/restore triggers a fresh scan, which
allows recovery after credentials, permissions, provider availability, or
repository contents change.

Service shutdown first unsubscribes event triggers, then closes the
mutex-protected scheduling gate, cancels in-flight resolution, and waits for
registered workers. A future eligible trigger after restart starts from
persisted task and repository state; there is no startup backfill.

## Security

- Browser input cannot nominate a repository for automatic linking.
- Candidate repositories come from server-owned task attachments and persisted
  provider identity.
- Every provider read uses the task workspace's automation connection and
  repository scope.
- Ambiguous and partially observed candidate sets fail closed.
- Association retains the existing task/workspace validation and event-routing
  boundaries.

## Observability

Successful links retain structured logs/current event. Errors warn without
credentials; ambiguous/no-match debug. Sequence quarantine emits alert.

## Tests

Resolver tests cover identity and secret rotation for every adapter: fresh
version/HMAC evicts, unverifiable freshness fails, and nothing persists/logs.

DB-clock tests advance every lifecycle/TaskPR/projection/scan lease without
sleeps and prove skewed process clocks irrelevant.

Trigger tests cover dispatcher replay/wake/Stop; conflict before/after admission,
newer/takeover/purge during provider I/O produce zero TaskPR side effects; owner
discard, post-GC tombstone, direct rejection, finalizer/ABA/unarchive/restart.

Store tests interleave owner-delete request with prior/late TaskPR writers;
admission epoch ensures purge ack covers all committed rows.

Projection tests use separate DB handles and the one injected fence across
delete/purge/every effect; token/revision/barrier/GC fencing holds cross-process.

Provider tests inject each Start failure and nil dependency; reverse cleanup
leaves no subscriber/worker/claim and new-instance retry has no duplicates.
Wire tests cover normalization, versions, reverse delivery, boot/checkpoints.

Playwright asserts only deterministic unique title references link.

## Related decisions

- [ADR 0047: Separate GitHub deployment, workspace automation, and personal identities](../../../decisions/0047-github-authentication-ownership.md)
