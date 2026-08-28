---
status: current
system: tasks
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
created: 2026-07-22
owners:
  - kandev
---
# Attach Workspace Sources System Design

## Purpose and boundaries

Technical source for `REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001`.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001 | Migrated legacy design detail below |

## Migrated design source

## Why

Tasks may grow by attached repositories/folders without recreating or losing context.

## What

- Files exposes desktop/mobile **Workspace actions** with **Add Repositories**
  and **Open workspace folder**. Its shared task-create selector supports saved,
  local, and remote repositories; supported executors also offer a folder. One
  submission may mix them.
- Adding another repository or folder appends a row without hiding or discarding configured rows,
  so one submission can mix workspace, local, remote, and folder sources.
- Repository rows choose a base branch with the shared task-creation controls. The add-sources UI
  does not expose a second checkout-branch field.
- A successful submission makes every added source visible as a named top-level entry in the Files
  panel. Repository sources also appear in repository-aware Changes, branch, editor, and pull
  request surfaces; folder sources remain file-only.
- Contradictory repository/branch pairs, contradictory canonical folder paths, cross-workspace repository
  IDs, invalid remote URLs, and inaccessible local paths are rejected before the task changes.
- A source file rename may stay within its canonical workspace/source root; a cross-root move or
  rename is rejected before either source is mutated.
- A multi-source submission is atomic from the user's perspective: either every source is attached
  and materialized in the current task environment, or none of the new attachments remain. When an
  attachment repointed a pre-existing Kandev-owned entry, a failed submission restores that entry to
  the target it had before the attachment rather than deleting it.
`task_sources(id PK,task_id,source_key_v1,kind,UNIQUE(task_id,source_key_v1),
UNIQUE(id,task_id))` is canonical; repository/folder `(source_id,task_id)` composite
FK prevents cross-task rows. Migration maps/backfills deterministically, collision
fails, then writers/readers/rollback cut over; ADR/plan boundary is amended.
- `ValidateOwnedDirectoryLink` is side-effect-free and exclusive to ready reuse.
  Mutating `EnsureOwnedDirectoryLink` requires an internal capability containing
  marker task/workspace/task-dir/layout identity, directory resource fence,
  environment/session identity and revision/projection, attempt/admission/
  worker epochs, binding generation, intended key, and row revision. Under the
  resource lease authorization reloads/CASes all fields before every effect,
  rescan, publication, and compensation.
- Capabilities are single-use. Successful after-probe/journal CAS returns the
  next capability with current environment/projection/binding/row revisions;
  multi-source effects run sequentially and never weaken expected values.
  `AuthorizeWorkspaceCompensation` capability for current owner/epoch/key/fence;
  CAS loss leaves recovery to winner.
- Delivery holds exclusive consumer claim through idempotent apply, checkpoint and
ACK in one transaction. Generation/reset tuple CASes before and after apply; GC
cannot install snapshot concurrently. Crash replays inbox event safely; stale/
out-of-order/duplicate delivery no-ops. Reset-vs-delivery/crash tests pin it.
- Reservation references immutable `lease_claim_id,lease_epoch`; deadline/
  heartbeat are mutable lease data. Renewal changes deadline only; takeover
  rotates claim/epoch and atomically transfers/replays reservation. Tests pin it.
- Repository attachment works for every executor that can run the task. Arbitrary folders are
  available only to Local and Worktree tasks, where the selected host paths remain live. Container
  and remote pickers do not offer the folder source kind, and the backend rejects a forged folder
  request without changing the task.
- Kandev may re-root or restart an idle task environment when its executor cannot safely change the
  agent working directory in place. The action is unavailable while a turn or tool call is active,
  and the backend independently rejects that race with a conflict response.
- Before submission, the desktop dialog and phone drawer explain the executor-specific effect on
  the agent working directory, provider session context, terminal and workspace processes, existing
  files and Git changes, and atomic rollback. They state that **Cancel** leaves the task unchanged.
- Providers that honor a changed `session/load` working directory keep their native ACP session
  after the re-root. Providers that do not are started in a fresh ACP session at the promoted task
  root, and Kandev rehydrates the recorded conversation context with the next prompt.
- Worktree and Local/Local PC rebinds stop terminal shells, the task editor server, dev servers, and
  other agentctl-managed workspace processes; users must reopen or restart them. Docker, SSH, and
  Sprites attach repository siblings through the live workspace and rescan without restarting the
  agent or those processes.
- When **Add Repositories to workspace** is unavailable, the combined Files action remains
  reachable so **Open workspace folder** still works. The repository action is disabled and shows
  the reason in touch-visible text rather than relying on a tooltip.
- Existing conversations, task state, plan, sessions, and repository attachments remain intact.
- Agents receive a batch `add_workspace_sources_kandev` MCP tool that uses the same validation and
  materialization path.
- The worktree-only `add_branch_to_task_kandev` compatibility lane is also an
  explicit source mutation. During the invoking turn it atomically claims a
  source attempt for exactly
  `repository:<repository_id>:<base_branch>:<checkout_branch>`, journals the
  full before row/binding and physical effect, and uses the same capability CAS.
  It may create/repoint only that key, rolls it back exactly on failure, promotes
  the persisted root, refreshes trackers, and never changes agent/terminal CWD.
- Repository resolution performs no durable catalog insert before the source
  transaction. That transaction writes any new workspace Repository entity,
  exact task link, and `repository_entity`/link journal members atomically.
  Recovery/rollback deletes an attempt-created entity only at exact identity/
  revision with no other references; reused entities are never deleted.

Decisions: [ADR-2026-07-22-runtime-mutable-task-workspace-sources](../../../decisions/2026-07-22-runtime-mutable-task-workspace-sources.md)
[ADR-2026-07-23-workspace-source-root-move-boundary](../../../decisions/2026-07-23-workspace-source-root-move-boundary.md),
and [ADR-2026-07-27-legacy-add-branch-live-rescan](../../../decisions/2026-07-27-legacy-add-branch-live-rescan.md).

## Data model

Repository attachments continue to use `task_repositories`; their current uniqueness contract on
`(task_id, repository_id, base_branch, checkout_branch)` is unchanged.

Arbitrary folder attachments use `task_workspace_folders`:

| Field                      | Contract                                             |
| -------------------------- | ---------------------------------------------------- |
| `id`                       | Stable attachment identity.                          |
| `task_id`                  | Owning task; cascade-deleted with the task.          |
| `local_path`               | Canonical absolute path selected on the Kandev host. |
| `display_name`             | Sanitized, non-empty top-level workspace entry name. |
| `position`                 | Stable order among folder attachments.               |
| `created_at`, `updated_at` | Audit timestamps.                                    |

`(task_id, local_path)` and `(task_id, display_name)` are unique. The effective source projection
combines ordered `task_repositories` and `task_workspace_folders`; it does not replace repository
identity or make folders participate in Git operations.

## API surface

`POST /api/v1/tasks/:id/workspace-sources`

```json
{
  "sources": [
    {
      "kind": "repository",
      "repository_id": "optional-workspace-repository-id",
      "local_path": "optional-local-git-path",
      "remote_url": "optional-provider-or-pasted-url",
      "provider": "optional-provider",
      "provider_repo_id": "optional-provider-id",
      "provider_owner": "optional-provider-owner",
      "provider_name": "optional-provider-name",
      "base_branch": "main",
      "checkout_branch": "optional-existing-branch"
    },
    {
      "kind": "folder",
      "local_path": "/absolute/path/to/folder",
      "display_name": "optional-name"
    }
  ]
}
```

All lanes use `CanonicalProjectionV1`:

```text
WorkspaceProjectionEnvelope<T> { order {stream_id,stream_epoch,stream_sequence,
task_id,environment_id,binding_generation,projection_generation,snapshot_epoch,
snapshot_id,delete_epoch,source_attempt_id,projection_event_id,lane,
schema_version,lane_version,lane_key,event_kind,authoritative,resync_of,coverage},
payload T }
```

CanonicalProjectionV1 frames domain tags/components as big-endian uint64
length/value UTF-8 bytes; length overflow rejects. Parser rejects duplicate JSON
members before model decode and numbers outside IEEE-754 safe I-JSON range.
Recursive strings NFC-normalize; objects use RFC8785. A set sorts
`(RFC8785(element),length)` and rejects duplicates; ordered arrays preserve
order. Declared maps serialize as RFC8785 array pairs `[key,value]`, recursively
normalized and sorted by RFC8785 pair bytes; post-NFC key collisions reject.
Payload/envelope digests frame version/header/canonical bytes. Go/TS share golden
vectors for nested maps, non-BMP/NFC collisions, numeric bounds, duplicate keys.

`Select` is commutative: vector dominance, then equal-vector epoch, capture over
checkpoint, then exact event/digest tie; stale ACKs without clearing, remaining
differences conflict/resync. Null `(binding,0)` precedes live; state keys
`(lane,lane_version,lane_key)`.

`TaskRepository` owns:

```text
workspace_source_projection_lane_sets(
 id,stream_id NOT NULL,scope_key NOT NULL,scope_kind NOT NULL CHECK(task|environment),
 task_id NOT NULL,environment_id nullable,current_epoch,encoding_version,revision,
 PRIMARY KEY(id),UNIQUE(stream_id,scope_key),FK(stream_id,scope_key) scope,
 CHECK((scope_kind=task AND environment_id IS NULL) OR
 (scope_kind=environment AND environment_id IS NOT NULL)),FK task/environment owner)
workspace_source_projection_lane_set_epochs(
 lane_set_id NOT NULL,epoch NOT NULL,expected_count,manifest_digest,
 PRIMARY KEY(lane_set_id,epoch),FK(lane_set_id) RESTRICT)
workspace_source_projection_lane_set_members(
 lane_set_id NOT NULL,epoch NOT NULL,lane NOT NULL,lane_version NOT NULL,
 lane_key NOT NULL,PRIMARY KEY(lane_set_id,epoch,lane,lane_version,lane_key),
 FK(lane_set_id,epoch) lane-set-epoch CASCADE)
```

Stream/task/environment consistency and immutable epoch parent reject cross-scope/
stale members. Current pointer advances without touching pending epochs; writer
snapshots epoch before outbox. Teardown holds global locks then deletes
reservation->outbox->members->epochs->lane-set before stream. Migration/replay
reject ownership/epoch mismatch; rotate-with-pending crash/replay is tested.
Manifest v1 length-prefixes unsigned-UTF8 sorted lane/version/key tuples.
Coverage/apply follows the canonical admission->stream->lane-set->lane-member->
publication-claim->outbox->assignment->consumer-claim->snapshot->conflict->
tombstone->no-reuse order in Additional Session Workspace Reuse. Writers,
snapshots, GC and delete take its ordered subsets; coordinator leases remain
outside the UoW. Interleavings test the same graph.

### Projection delivery

Tombstone `(stream_id,delete_epoch,terminal_sequence,stream_generation,digest,
retired_at)` is irreversible. Consumer/Compare checks it before vector selection;
retired-generation frames drop. Terminal invalidates reservations/assignments,
including delayed higher sequence frames. Schedules cover terminal/reconnect.
Full/summary task/session payloads expose effective `workspace_path`; for
multi-source tasks it is the parent containing all entries. `worktree_path`
remains primary-repository compatibility output and is never promoted over a
newer ordered workspace projection.

Chat file links resolve against `workspace_path`, so an absolute path under any attached source is
converted to its task-root-relative Files path before it is opened. Clients may fall back to
`worktree_path` for legacy session payloads that do not yet include `workspace_path`. Absolute paths
outside the effective workspace remain non-actionable and are never rewritten into a workspace
file request.

`add_workspace_sources_kandev` accepts the same union/provenance/idempotency
rules as POST and returns `WorkspaceProjectionEnvelope<WorkspaceSourcesResult>`,
the shared payload plus environment/vector/attempt/event/authoritative metadata.
Kandev MCP/session/UI consumers run `Compare`; incomparable or legacy missing
metadata requests canonical resync before applying. Agent-readable output alone
is informational and never mutates a client projection.

`add_branch_to_task_kandev` returns `CanonicalProjectionV1`; absent environment/
snapshot/coverage values are null, never empty/zero. Deferred results retain
their real binding/attempt/event metadata and null unavailable fields. Every
result passes registry validation and `Compare`; textual agent output is
informational only.

## Permissions

The action follows Kandev's trusted-local-user model and is scoped to the task's workspace. Saved
repository IDs must belong to that workspace. Explicit local repository and folder selections grant
access only to their canonical paths, not to parent directories, sibling paths, or filesystem
volumes. Remote credentials follow the existing provider-neutral repository contract and are never
persisted in source URLs or copied into agent-visible metadata.

## Failure modes

| Condition                                                      | Observable behavior                                                                                                                                 |
| -------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| A turn or tool call is active for batch attachment             | The UI disables the action when known; a racing batch request returns `409` without mutation.                                                       |
| The invoking agent calls legacy add-branch during its turn     | The worktree is created and trackers refresh without stopping the active agent, terminals, or workspace processes.                                  |
| Any source is invalid or contradicts an existing source identity | The full batch is rejected before persistence or materialization. Exact normalized retries are no-ops.                                             |
| A host materializer fails                                      | New filesystem entries and source records are rolled back; existing task contents remain.                                                           |
| A container/remote repository clone fails                      | Newly created remote entries are removed best-effort, durable attachments are rolled back, and the response identifies the failed source.           |
| A container/remote task submits a folder source                | The request returns `422` without persistence or filesystem changes.                                                                                |
| Agentctl cannot rescan the new root                            | The attachment fails rather than reporting success with a stale Files tree.                                                                         |
| A chat message links to an absolute path outside the task root | The link is not sent to the workspace file API; normal external/unsafe-path handling remains in effect.                                              |
| An idle agent must restart to adopt the promoted root          | The intentional stop is not shown as a prior agent failure; the replacement agent uses the promoted task root.                                      |
| A requested file move or rename crosses canonical source roots | The request is rejected before either source is mutated.                                                                                            |
| A persisted local folder later disappears                      | The current live environment keeps its existing materialization; a new/reset environment surfaces the missing source and does not silently omit it. |
| A Kandev-owned task-root link differs from durable spec | Initial materialization or explicit source mutation may repoint under journal/fences; ready reuse fails `workspace_reuse_unsafe`. |
| Ownership marker names another task | Fail closed marker-conflict; leave its target unchanged. |
| Repoint batch fails | Journal restores prior targets and deletes only created entries; no pre-existing entry is deleted. |
| Safe replacement fails | Windows member persists typed prior target/identity/proof, backup path, generation fence and phase `replace_planned->removed->restored|replaced|unsafe`. Recovery probes intended replacement first, restores exact prior only if needed, retains admission on missing/ambiguous, and deletes backup only after verified terminal outcome. Crash/remove/recreate/restore-failure/takeover junction schedules pin it. |

## Persistence guarantees

Repository and folder attachments survive backend restarts. Local/worktree environments continue to
resolve the exact canonical host path. New container or remote environments recreate repository
checkouts from durable repository attachments; they never persist folder attachments. Existing task
conversations and source records survive an environment restart even when runtime materialization
must be retried.

Each task that materializes a Kandev-owned task root under the tasks base directory derives its
task-root directory name from task identity. The name is collision-resistant, not injective: two
distinct tasks — including two tasks whose titles sanitize to the same slug, or a local task and a
worktree task sharing a title — are overwhelmingly unlikely to resolve to the same task root, and any
residual collision is caught by the fail-closed ownership marker, which verifies task identity before
any Kandev-owned entry under a shared root is repointed. The task-root name is computed once and
persisted; every relaunch and resume of that task reuses the persisted name.

## Scenarios

- **GIVEN** a repository-backed task with the Files tree loaded, **WHEN** the user opens the
  workspace-actions control, **THEN** one menu exposes **Add Repositories to workspace** and **Open
  workspace folder**.
- **GIVEN** a running worktree task with one repository and no active turn, **WHEN** the user opens
  **Add sources**, chooses **Workspace repository**, and adds a saved or discovered repository and
  branch with the shared task-create selector, **THEN** the new worktree appears as a top-level
  Files entry and in repository-aware Changes surfaces without recreating the task.
- **GIVEN** an idle task whose runtime resumes its agent or emits a late session-resumed status while
  adopting an updated workspace root, **WHEN** the user attaches another source, **THEN** lifecycle
  boot/status messages do not create a phantom turn and the subsequent attachment succeeds.
- **GIVEN** an idle single-repository task whose agent started in the repository directory, **WHEN**
  another source promotes the workspace to the task root, **THEN** the next agent prompt runs from
  the task root without a previous-agent-error banner; compatible providers retain their native
  session and incompatible providers receive a fresh session with recorded conversation context.
- **GIVEN** an idle Worktree or Local task, **WHEN** the user opens **Add sources**, **THEN** a
  visible consequence summary explains that the CWD moves to the task root, the agent and
  agentctl-managed workspace processes restart or stop, recorded task context remains, and
  provider-private context that Kandev did not record may not carry over.
- **GIVEN** an idle Docker, SSH, or Sprites task, **WHEN** the user opens **Add sources**, **THEN**
  the consequence summary says repositories are attached and rescanned under the current remote
  workspace without restarting the agent or changing its CWD.
- **GIVEN** configured source rows in either add-sources surface, **WHEN** the user chooses
  **Cancel** or closes the surface before submission, **THEN** no attachment request is sent and
  the task workspace remains unchanged.
- **GIVEN** an Add sources batch with a local row already configured, **WHEN** the user chooses
  **Remote repository** from **Add repository**, **THEN** both rows remain visible in the batch and
  submit atomically.
- **GIVEN** a repository-backed local task, **WHEN** the user adds a local Git repository and an
  arbitrary folder in one submission, **THEN** both live sources appear under one task workspace and
  the folder does not appear in Git-only controls.
- **GIVEN** a Docker, SSH, or Sprites task, **WHEN** the user opens **Add sources**, **THEN** the
  workspace, local-Git, and remote repository choices are available from **Add repository**, and
  the local-folder affordance is not offered.
- **GIVEN** a Docker, SSH, or Sprites task, **WHEN** a client submits a forged folder source,
  **THEN** the backend returns `422` and leaves the task and executor filesystem unchanged.
- **GIVEN** a mixed three-source submission whose second source cannot be cloned, **WHEN**
  materialization fails, **THEN** none of the three new attachments remain in the database, Files
  tree, or executor workspace.
- **GIVEN** an active agent turn, **WHEN** the user attempts to add sources, **THEN** no source is
  attached, the **Add Repositories to workspace** menu item explains that the task must be idle
  first, and **Open workspace folder** remains available.
- **GIVEN** an active worktree-executor agent whose CWD is the initial repository, **WHEN** it calls
  `add_branch_to_task_kandev`, **THEN** Kandev creates the new repository/branch worktree as a
  sibling under the task root, promotes the persisted workspace path, refreshes Files and
  repository trackers, and does not restart the agent or change its CWD.
- **GIVEN** a live legacy add-branch materialization, **WHEN** the MCP result returns, **THEN** it
  includes the absolute new `worktree_path`, the promoted `task_workspace_path`, and
  `agent_cwd_changed: false`.
- **GIVEN** the original repository has no pending changes, **WHEN** a legacy add-branch call creates
  a sibling worktree, **THEN** Git status in the original repository does not report the sibling as
  an untracked or changed path.
- **GIVEN** a task whose workspace contains multiple repositories, **WHEN** an agent message links
  to an absolute file path in either the primary or an attached repository, **THEN** clicking the
  link opens the exact file through the task-root-relative Files API path without a file-not-found
  notification.
- **GIVEN** that multi-repository task is reloaded after workspace promotion, **WHEN** the user
  clicks the same chat file link, **THEN** the session-restored `workspace_path` resolves the same
  file and `worktree_path` still identifies the primary repository.
- **GIVEN** a legacy single-repository session payload without `workspace_path`, **WHEN** a chat
  file link is inside `worktree_path`, **THEN** the link continues to open as a repository-relative
  path.
- **GIVEN** a live legacy add-branch materialization fails, **WHEN** the MCP call returns an error,
  **THEN** the new `task_repositories` row and any newly created repository entity are rolled back
  while the agent and existing processes continue running.
- **GIVEN** the marker changes after the cheap preflight, **WHEN** ready source
  mutation reaches its lease-held check, **THEN** it returns unsafe/reset with no
  admission, journal, catalog/link, outbox, physical, or response-projection row.
- **GIVEN** a source identity conflicts with an already attached repository/branch or canonical folder,
  **WHEN** it is submitted, **THEN** the request returns a conflict and leaves the task unchanged.
- **GIVEN** the same normalized repository/branch or canonical folder is already attached, **WHEN** it is
  submitted again, **THEN** the request succeeds without changing the task.
- **GIVEN** a phone viewport on the Files tab, **WHEN** the user opens the 44px workspace-actions
  control, **THEN** an inset bottom-sheet menu exposes both actions with touch-sized rows.
- **GIVEN** that phone action menu, **WHEN** the user selects **Add Repositories to workspace**,
  chooses repository kinds from the touch-sized **Add repository** menu, adds two sources, and
  submits, **THEN** a touch-usable full-height picker completes the same operation without
  horizontal document overflow and returns focus to the workspace-actions control.
- **GIVEN** an agent calls `add_workspace_sources_kandev` for its current idle task or an idle
  same-workspace direct child, **WHEN** all
  inputs materialize, **THEN** the UI receives the same task and session updates as the human flow.
- **GIVEN** two distinct tasks whose titles sanitize to the same task-root slug, **WHEN** each task
  materializes a Kandev-owned task root, **THEN** their collision-resistant suffixes normally produce
  different task-root directory names; if a residual suffix collision occurs, the ownership marker
  rejects cross-task repointing with a marker-conflict error rather than redirecting the other task's
  entries.
- **GIVEN** a ready environment and POST/MCP add-sources request whose
  request-touched owned link differs from its intended target, **WHEN** the
  source attempt owns admission and validates marker/generation, **THEN** it
  journals and atomically repoints that link.
- **GIVEN** the same mismatch during session attach, ready launch/resume, or
  restore, **WHEN** reuse validation runs, **THEN** no physical repair occurs and
  it returns `workspace_reuse_unsafe` with reset action.
- **GIVEN** legacy `add_branch_to_task_kandev` creates a catalog Repository,
  **WHEN** crash/cancellation occurs before link completion, **THEN** the durable
  repository-entity/link members resume or delete only the exact unreferenced
  entity and retry is idempotent.
- **GIVEN** a Kandev-owned task-root entry that is not a directory link (a real file or directory a
  reconcile did not create), **WHEN** the task launches or resumes, **THEN** reconciliation does not
  delete or overwrite it and the launch surfaces an error identifying the conflicting entry.
- **GIVEN** two persisted legacy tasks whose environments share one Kandev-owned task root, **WHEN**
  the second task launches or resumes and its ownership marker does not match the root's marker,
  **THEN** reconciliation fails closed with a marker-conflict error and does not redirect the first
  task's live entry.
- **GIVEN** a multi-source attachment that repointed a pre-existing Kandev-owned entry, **WHEN** a
  later source in the same submission fails to materialize, **THEN** rollback restores the repointed
  entry to its prior target and no pre-existing entry is left deleted.
- **GIVEN** initial materialization or an explicit source attempt replaces an
  owned entry, **WHEN** recreate can fail after old-link removal, **THEN**
  containment rejects before removal and the durable replacement protocol
  restores or enters reset-required unsafe state, never ready reuse repair.

## Out of scope

- Removing or detaching sources after they have been attached.
- Promoting a repository-less task into a repository-backed task.
- Copying, mounting, or synchronizing arbitrary host folders into container or remote executors.
- Running batch workspace-source attachment while an agent turn or tool call is active; the legacy
  worktree-only `add_branch_to_task_kandev` compatibility path is the explicit exception.
- Changing the running agent or terminal CWD during a legacy add-branch call.
- Nesting a new Git repository or worktree inside the current repository.
- Expanding or bypassing a provider-owned filesystem sandbox when it excludes the returned sibling
  path.
- Reordering sources after attachment.
- Sharing task-creation state, its mode switch, or its **None**/scratch semantics with Add sources;
  only the repository picker leaves are shared.
- Making the unimplemented remote Docker executor runnable; its source-materializer capability is
  required when that executor becomes available.

## Implementation plan

See [Attach Workspace Sources plan](../../../plans/attach-workspace-sources/plan.md) and the
[live add-branch compatibility repair plan](../../../plans/restore-live-add-branch/plan.md), plus the
[multi-repository chat file-link repair plan](../../../plans/multi-repo-chat-file-links/plan.md) and the
[owned link target mismatch repair plan](../../../plans/owned-link-target-mismatch-repair/plan.md).