---
status: draft
system: tasks
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-004
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-005
---

# Audited cross-workspace task transfers System Design

## Purpose and boundaries

The task system owns the transfer because it owns task identity, placement,
lifecycle, and movement authorization. The existing transfer is one domain
command shared by the MCP adapter and the proposed human Web adapter, a service
that owns boundary authorization, and a repository preservation transaction.

The domain/MCP sections describe the existing implementation. The Web surface,
typed Web diagnostics, task-write checks, and client reconciliation below are
draft additions for [issue #5075](https://github.com/kdlbs/kandev/issues/5075).
No new preservation policy, transfer ledger, feature flag, or relation-mapping
mechanism is introduced.

Adjacent contracts this design uses but does not own:

- Workspace ownership and repository/worktree identity belong to the
  [workspace system](../../workspaces/README.md).
- Session lifecycle and turn ownership belong to the session lifecycle design.
- The MCP tool surface and profile gating belong to the MCP server surface.
- Notification and cache rehydration belong to the gateway contract.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001` | Transfer command, Atomic transaction, Destination lane resolution, Preservation and rebinding |
| `REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002` | Actor attestation, Boundary authorization |
| `REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003` | Idempotency and replay, Audit, Post-commit reconciliation |
| `REQ-TASKS-CROSS-WORKSPACE-TRANSFER-004` | Web movement navigation, Workspace transfer form, Responsive composition |
| `REQ-TASKS-CROSS-WORKSPACE-TRANSFER-005` | Human HTTP boundary, Safe conflict diagnostics, Submission recovery, Client reconciliation |

## Components and responsibilities

- **MCP tool (`transfer_task_kandev`)** — decodes the request, validates the
  schema, attests the actor from the server session (never the payload), and
  forwards to the service. Registered only on configuration, external, and
  Office surfaces with a server-attested CEO or coordinator path; ordinary task
  agents never receive it.
- **Task service** — resolves workspace and workflow reachability, delegates
  actor attestation to the coordinator authorizer, and constructs the typed
  transfer command with authorized owner identity.
- **Repository transfer transaction** — locks the task row, verifies every
  placement predicate, resolves the destination lane, validates equivalence and
  workspace boundary, inventories relations, persists the transfer and audit
  record, and commits atomically.
- **Broadcast and reconciliation path** — after commit, publishes the
  destination-scoped task update through the source boundary and reconciles
  dependent caches and events exactly once.

## Data and contracts

The typed command binds: exact task ID; expected source workspace, workflow,
and lane; expected task `updated_at` generation; destination workspace,
workflow, and lane (stable lane ID or unique exact name); idempotency key; and
the `preserve-task-identity-v1` policy.

The durable receipt contains: operation ID, source and destination placement,
committed task generation, step-transition ID, session census, preservation
counts and digest, idempotency key, and policy. It intentionally contains
identifiers and counts, never prompts, message bodies, secrets, or credentials.

Actor identity is a server-attested structure (human or coordinator) resolved
from the authenticated context; request payloads never populate it.

Configuration authority requires the persisted calling session's boolean
configuration-purpose marker. Task metadata and request fields cannot grant
it. Automation, coordinator, and managed conversation restrictions remain in
force. Office CEO attestation and the transaction both require an enabled,
undeleted source profile with an active status and the exact running session
and runner-seat binding.

Lane equivalence includes session start/end policy, session target, completion
on entry, and the unclassified fallback veto, alongside prompts, events,
participants, completion signals, and WIP policy.

Relation discovery inventories and locks every task-keyed table. An unmapped
workspace-owned relation blocks transfer when it contains rows for the selected
task; unrelated or empty tables do not block other tasks. Such rows retain their
original workspace fence until an explicit mapping contract exists.

The MCP error vocabulary maps stale, ambiguous, incompatible, unauthorized,
or idempotency-mismatched outcomes to `task transfer conflict` at that boundary,
without distinguishing task existence. The human Web adapter adds allowlisted
reason codes only after the caller is authorized for both workspaces.

## Control flow

1. The MCP surface decodes and validates the request; schema rejections are
   audited before answering.
2. The service authorizes task, workspace, and workflow reachability and
   resolves the attested actor. Denied attempts audit and answer without
   existence disclosure.
3. The repository transaction:
   - locks the task and reads its placement;
   - fails closed on any predicate, generation, boundary, lane-equivalence,
     capacity, or relation mismatch;
   - resolves the destination lane by stable ID or unique exact name;
   - persists the transfer with its receipt and audit row.
4. After commit, the broadcast path publishes the destination task update
   through the source notification boundary and reconciles caches and events.
   Reconciliation survives caller cancellation and runs exactly once.

## Failure and recovery

Every failure before commit leaves source and destination state unchanged and
returns the stable conflict result. Committed transfers are durable: a retry
after a caller-visible failure replays the stored receipt under the same actor,
session, and key. Post-commit reconciliation is idempotent under cancellation.

A caller must use a fresh key after its denied or failed attempt. Rejected
attempts from another actor or session cannot poison that caller's key.
Committed-receipt replay takes precedence over rejected-attempt history.
Audit-delivery retries share a server-generated attempt ID; that ID is accepted
only on the trusted internal audit path and never in a transfer request.

## Persistence

Transfer receipts and audit rows live in additive task-store tables owned by
the task repository. Migrations are additive; rolling back the application
binary leaves receipts and audit rows intact for a later upgrade. No rollback
path drops the transfer ledger.

## Security

Authorization is server-attested. The workspace boundary requires one shared
owner for source and destination. Ambiguous lane mapping, unattested callers,
and mismatched runner-seat mapping fail closed. Audit rows are redacted to
identifiers and outcomes, never transcript content.

## Observability

Each attempt writes one audit row with its outcome. Structured logs record
authorization decisions and post-commit reconciliation completion. The
receipt's preservation counts and digest make a transfer's inventory observable
without exposing content.

## Human HTTP boundary

Add `POST /api/v1/tasks/:id/transfer` beside the existing `/move` and
`/move-preview` routes in `task/handlers/task_handlers.go`. A proposed
`task_transfer_http_handlers.go` adapter calls `task/service.TransferTask`;
it must not call `MoveTaskWithOptions`, patch workspace IDs, or synthesize a
sequence of two moves.

The strict JSON request is:

```json
{
  "expected_source_workspace_id": "source-workspace",
  "expected_source_workflow_id": "source-workflow",
  "expected_source_workflow_step_id": "source-step",
  "expected_task_updated_at": "2026-11-05T12:00:00.123456789Z",
  "destination_workspace_id": "destination-workspace",
  "destination_workflow_id": "destination-workflow",
  "destination_workflow_step_id": "destination-step",
  "idempotency_key": "one-client-submission-identity"
}
```

Task identity comes from the route. Use destination IDs, not name matching.
Reject unknown fields and trailing JSON. Actor, owner predicates, policy,
audit attempt ID, entry options, and workflow-agent overrides are not request
fields. Construct `models.TaskTransferCommand` with
`TaskTransferPreservationPolicyV1`; retain the exact timestamp string on the
client and parse RFC3339Nano on the server. A JavaScript Date round trip would
truncate the bound generation.

Require the HTTP middleware's identity, including its synthetic identity in
auth-disabled mode. Never interpret absent HTTP identity as internal authority.
Attest `TaskTransferActorHuman` from `authn.IdentityFromContext`, using the
existing MCP adapter's `local-human` fallback only for a synthetic local caller;
do not bind a browser-supplied session. Human actor identity must remain stable
across browser retries.

Strengthen `Service.TransferTask` to require `authz.ScopeTaskWrite` in the
current task workspace and destination workspace, using the existing task and
workspace scope helpers before replay or mutation. Preserve reach-first 404
versus readable-but-forbidden 403 behavior. Apply the checks at the service
boundary, so existing MCP callers cannot bypass them. Retain the repository's
same-owner predicate and all actor-specific coordinator guards. Current task
and destination access are rechecked on replay; replay still precedes mutable
workflow configuration checks.

Return HTTP 200 with `{ "receipt": <TaskTransferReceipt> }`. The receipt is the
operation result, not necessarily the task's latest placement after subsequent
moves. Canonical task data is read through existing task navigation/detail
loaders. Never reconstruct current task state by treating an old receipt as a
fresh task snapshot.

Use 400 for invalid request syntax, 404 for missing reach/resources, 403 for
known-but-insufficient permission, 409 for a domain conflict, and a redacted 500
for unexpected failures. Add `Cache-Control: no-store`. Reuse
`AuditTaskTransferAttempt` for adapter-level rejection; domain-attempt auditing
remains owned by `TransferTask`. There is one audit owner per attempt, not a
handler plus repository duplicate.

## Safe conflict diagnostics

Introduce a typed reason alongside `repoerrors.ErrTaskTransferConflict`, not
string matching against Go error messages. Its `Unwrap`/`errors.Is` behavior
must preserve existing MCP classification. Repository guard conditions stay
unchanged. The Web operation's durable rejection rule below additionally binds
the outcome of a reviewed submission; it does not loosen any transfer guard.

The Web error body is `{ "error": "task transfer conflict", "code": "..." }`.
Allowlist these proposed codes and translate their recovery copy in the client:

| Code | Recovery shown to an authorized person |
| --- | --- |
| `source_changed` | Refresh and review current placement; an unresolved prior operation must first be resolved. |
| `destination_unavailable` | Reload destination choices and select a workspace, workflow, and step. |
| `step_incompatible` | Choose a step with the same name and saved behavior; ordinary workflow changes are separate. |
| `destination_full` | Free destination capacity or choose another compatible step. |
| `workspace_relation_unmapped` | Resolve workspace-scoped labels, project, group, or other associations before moving. |
| `profile_mapping_required` | Use compatible globally available profiles; workspace-specific profile transfer is unsupported. |
| `pending_step_unmapped` | Resolve the existing queued/deferred move or provide equivalent destination steps. |
| `cleanup_active` | Wait for task cleanup or recovery to finish. |
| `idempotency_conflict` | Review the operation identity; this alone cannot settle a previously uncertain move or enable a fresh key. |
| `operation_rejected` | The caller-bound key is durably barred and has no committed receipt; review current placement before a fresh submission. |
| `transfer_unsupported` | This attempt cannot safely transfer the task; a result check does not establish that the original attempt failed. |

Missing reach never returns a diagnostic reason, destination metadata, table
name, or raw internal error. Same-owner mismatch remains a non-disclosing
unavailable result. Unknown reasons fall back to `transfer_unsupported` only for
a known domain conflict; storage/commit failures remain uncertain 500 errors,
not a claim that nothing committed. Keep MCP error text and tool exposure
unchanged.

## Web movement navigation

Use one `Move to` parent for single-task movement across card right-click and
ellipsis menus, the task/session sidebar, detail/preview actions, Threads, and
command palette. Its children are `Workflow step`, `Different workflow...`,
and `Different workspace...`. The step child retains the existing inline
defaults/options path and progress details; the workflow child opens the
existing `ChangeWorkflowDialog` with its profile mapping and entry preview.
The workspace child opens the new transfer form. Do not merge mutation payloads
or suggest that a workspace transfer runs step-entry actions.

Ground integration in `kanban-card-menu-items.tsx`,
`TaskMoveContextMenuItems`, `useTaskActionsMenu`,
`task-switcher-context-menu-move-items.tsx`, the shared menu-entry builder, and
the command palette's existing nested entries. Keep bulk step/workflow actions
and their plural semantics; omit workspace transfer for multi-selection.
Preserve archived/ephemeral action eligibility. A single-step workflow must
still expose the workspace choice even when its step-move child is unavailable.

Replace the step-list `w-48` widths with a task-move-scoped preferred 20rem
width bounded by available viewport space. Retain
`TaskContextMenuSubContent` portaling and vertical overflow containment.
Let destination labels and progress/profile detail text wrap with `min-w-0`;
keep badges, color dots, and actions in non-overlapping columns. Do not widen
every unrelated submenu or rely on a hover tooltip to reveal clipped names.

## Workspace transfer form

Add proposed `TaskWorkspaceMoveDialog` and domain hook
`useTaskWorkspaceMove`. Reuse `ChangeWorkflowDialog` form anatomy, focus-return
handler, catalog loading conventions, ordered steps, and color markers; do not
extend its workflow-change payload with workspace mutation.

Shared form state contains the captured task ID, exact source snapshot,
destination workspace/workflow/step IDs, catalog request generations, and the
submitted operation snapshot. Keep dialogs above transient menu mounts.
Capture the subject when opened; close or keep that captured subject on an
unrelated task/session change, never silently retarget the request.

Read accessible workspaces through the existing workspace API/store and load
selected-workspace workflows through `listWorkflows`. Load steps with
`fetchWorkflowSnapshot`, following `use-change-workflow-data.ts` request
generation guards. Reads live in domain hooks, not component effects.
Exclude the source workspace and hidden workflows. Include personally hidden
steps in workflow order. Do not auto-select a destination step. Empty lists,
failed reads with retry, and loading each disable submission appropriately.
Late workspace-A/workflow-A responses cannot replace workspace-B choices.
Never call `setActiveWorkspace` merely to browse destination catalogs.

Show current placement, then Workspace, Workflow, and Step selectors, a short
preservation/compatibility notice, any submit error, and Cancel/Move actions.
Do not add a new eligibility-preview endpoint or mirror SQL equivalence rules
in TypeScript. The server is authoritative and revalidates at commit.
All copy uses `t()`/`Trans` in English and six shipped translations; generate
the Traditional Chinese pair through the existing locale script.

## Submission recovery

Before the first POST, capture the complete immutable request and allocate one
`generateUUID()` idempotency key. Prevent duplicate clicks with local admission
independent of render timing. No optimistic workspace removal occurs.

- A 200 receipt settles the operation. Reconcile canonical task placement and
  memberships before announcing success or offering navigation.
- A definitive rejection received for the original submission keeps still-valid
  draft choices. A `source_changed` response offers Refresh and requires a new
  explicit submit. A new reviewed attempt gets a fresh key.
- A network interruption or 5xx makes the submitted operation uncertain. Freeze
  destination choices and show `Check result`; that explicit action retries the
  identical body/key. Record that this is resolution of an uncertain operation,
  not a fresh submission, before interpreting its response.
- A result check returning 403/404 means unavailable to verify, not that the
  original transfer did not commit. Keep the immutable body/key across
  close/reopen, show no rollback or unchanged-placement claim, and do not enable
  a fresh-key move. The user can check the same operation after permissions
  recover. Other check failures likewise retain uncertainty unless the server
  supplies the authoritative terminal result described below.
- A receipt for the exact actor/request/key resolves the operation as committed,
  even if later moves have changed its placement. A separate canonical read
  reconciles current placement, but neither a matching destination nor a return
  to source proves which operation committed. A read may refresh the displayed
  task without clearing the unresolved operation.
- Resolve rejected Web operations using the existing audit storage and transfer
  serialization, not a new endpoint or schema. The human HTTP adapter sets a
  server-only durable-rejection mode on its internal command; it is not accepted
  from JSON and is excluded from the request digest. Existing MCP conflicts
  retain their current behavior and generic error text.
- Acquire transfer serialization before a mutation savepoint. Resolve an exact
  committed receipt first, then any caller/request-bound terminal rejection,
  before mutable source/lane validation. All same-key attempts, including ones
  already admitted at the service, must acquire this same serialization and
  check the existing failed-key barrier before any commit.
- If an authorized, well-formed Web operation fails a domain guard, including
  `source_changed` or `step_incompatible`, roll back its mutation savepoint
  while retaining the outer serialization lock. Recheck committed receipt
  precedence, then persist one durable `failed` attempt in the existing audit
  table and commit only that rejection record before returning its specific
  409. The rejected transfer changes no task/context data. This record is both
  the redacted attempt audit and the existing no-later-same-key-commit barrier;
  do not add a duplicate outside-transaction conflict audit.
- Bind Web terminal-result lookup to the exact task, expected source placement
  and generation, destination IDs, policy, key, and server-attested actor/session
  stored in the audit columns. A changed request or other actor's key collision
  is an idempotency conflict, never permission to create/replace a terminal
  marker or to poison the original operation. Legacy denied/failed records can
  establish terminal rejection only when these available Web predicates match.
  Ordinary historical `conflict` rows alone are not terminal evidence.
- An exact retry returns `operation_rejected` only after serialized receipt
  lookup finds no committed receipt and verifies the durable failed-key barrier.
  Thus a lost original domain-409 response can settle and release the form for
  a fresh reviewed key. An overlapping writer either committed first (receipt
  wins), or observes the barrier and cannot subsequently commit. Audit write,
  lookup, or commit failure remains uncertain; missing-key or arbitrary-409
  responses never establish terminal rejection. Current authorization precedes
  all disclosed outcomes, including after access restoration.
- Retain the unresolved submission in the owning domain state across
  dialog close/reopen within the page lifetime, scoped by task and authenticated
  caller; clear it on identity change. Dismissal is not rollback. On full page
  reload, canonical reads determine current placement and no POST replays
  automatically. Do not persist transcript data or authority in browser storage.
- If the user navigates to another task while the request settles, reconcile
  memberships but do not navigate, reopen the form, or steal focus.

The repository continues using the initiating context until commit. Caller
cancellation before commit rolls back; committed work retains the existing
bounded `context.WithoutCancel` event/WIP reconciliation. Test both cancellation
boundaries. There is no shared background transfer worker or extra retry loop.

## Client reconciliation

The existing service emits `task.updated` with destination placement,
`source_workspace_id`, `source_workflow_id`, and `transfer_operation_id`.
`TaskEventBroadcaster.broadcastTransferSourceRemoval` also emits a redacted
source `task.deleted` carrying that operation marker to source-only readers.

Currently `apps/web/lib/ws/handlers/tasks.ts` treats every `task.deleted` as a
real deletion and removes sessions, drafts, and other local data. Branch on the
transfer marker before those destructive effects. Transfer source removal must
only invalidate/remove the task's source membership, sidebar query/page cache,
source archive-list membership if present, and source preview selection.
Never invoke task deletion storage cleanup, session-read retirement, walkthrough
deletion, queue clearing, or global recent-task removal for a transfer.

Use a proposed small `task-transfer.ts` reconciler shared by HTTP settlement and
WS handlers. Treat redacted transfer removals as source-membership invalidations,
not ordered placement commands. The operation UUID is opaque, and source
workspace/workflow matching does not fence A -> B -> A. `updated_at` cannot
order placement either: completed-task transfers deliberately preserve it.

Before changing source membership or preview, perform an authorized canonical
task read or source-membership refresh through the existing read coordinator.
Remove only if the accepted result confirms that the task is no longer a source
member or access is definitively unavailable (403/404). A confirmed current source
row, including one installed after a return, is retained. A timeout, cancellation,
5xx, or other transient read failure keeps membership/preview and a retryable
invalidation; never infer departure. Do not reveal destination data to a
source-only reader.

Fence each reconciliation read with the captured task/source identity, current
workspace generation, and existing accepted overview/sidebar read revisions.
Revalidate those guards before committing membership or preview changes;
superseded reads cannot erase newer accepted state. New invalidations supersede
older reads, and duplicate markers must not directly mutate rows. Apply the same
canonical-read rule when reconciling delayed full destination updates or HTTP
receipts, so those hints cannot remove a newer source return either.

Existing read revisions also reject in-flight source snapshots after a confirmed
departure. Invalidate affected query counts and Office task projections where
loaded. Do not delete all caches by task ID, create a second identity store, or
add a placement token/schema merely to order these notifications.

The membership fence also covers untagged source full-entity upserts, not just
transfer-tagged hints. `task.updated`, `task.state_changed`, and any
`task.created`/snapshot/cache path that could insert a source member must pass
transfer-aware admission before writing task overview, board, multi-board,
sidebar membership, or preview state. A delayed source lifecycle event arriving
after confirmed departure cannot itself establish that the task returned.

Membership admission must not depend on observing a transfer notification.
Keep source membership control metadata in the authenticated read/reconciliation
owner, keyed by source workspace/workflow and task, not another task copy.
An accepted per-task read confirming departure or access loss installs the same
retained source guard as a transfer hint. For guarded source upserts, use event
payloads only to invalidate; admit membership and placement-bearing fields from
the accepted canonical result and never replay the held payload afterwards.

For every unverified event insertion or placement change, including ordinary
`task.created` and tasks never observed transferring, require canonical admission
before any source projection upsert. Existing resident rows with accepted current
membership can still receive non-membership fields under their normal freshness
rules; a cache row or event alone is not proof of recovered membership.

At startup, reconnect/re-subscription recovery, and workspace reactivation,
invalidate prior membership admission evidence while retaining readable cached
content until refresh settles. A task becomes admitted only through an accepted
current canonical task/list read in the new recovery/workspace generation.
Known resident tasks omitted from recovery reads are candidates for per-task
confirmation; filtered, partial, or paginated query absence is never authoritative
workspace departure. Positive canonical rows admit those rows only, not arbitrary
later event insertions. Unknown/absent event candidates also use the guarded
per-task read, so missed transfer markers cannot bootstrap stale membership.
Canonical real source returns and ordinary new tasks remain discoverable through
the same path. No timestamp or opaque event ID substitutes for admission.

Retain the guard for the authenticated page/store lifetime across workspace
switches, canonical returns, and ordinary browser reconnects. A new socket epoch
only fences callbacks from the replaced socket; an old event still queued in the
backend bus can fan out through the current socket. Transport replacement or a
fresh source listing is not evidence that upstream publications are drained.
Do not retire the guard on reconnect, by timer, or because a row is absent.
Discard it only with the owning page/store or authenticated identity. Canonical
source returns remain accepted under the same request/workspace/read guards.
Coalesce invalidations in the existing coordinator rather than issue an unbounded
read per event. Stable resident non-membership updates retain their existing
rules; unverified membership establishment always passes the gate. No producer
barrier, placement schema, or global subscription redesign is needed.

If a listing initiated the move, stay in its workspace/workflow, close a preview
of the departed task, and expose `Open task` via `linkToTask`. If the moved task
is still the active detail route, refetch its canonical task through
`fetchTaskNavigationData` in `lib/ssr/session-page-state.ts` and existing
navigation coordination, adopt its actual workspace, and preserve the selected
session and task-owned data. A receipt replay after a subsequent move follows
current canonical placement, not the receipt's historical destination.
Source-only readers refetch under ordinary authorization. Only definitive
unavailability returns an open detail to the source listing without destination
disclosure; a transient failure keeps its existing context with retry feedback.

## Responsive composition

Nearest shipped form exemplar: `ChangeWorkflowDialog`, which supplies shared
desktop Dialog/phone Drawer anatomy, one scroll body, and a fixed footer.
The curated inset geometry comes from
`components/task/mobile/mobile-picker-sheet.tsx`. Reuse domain state and
submission logic across both presentations.

Desktop uses a compact dialog; ordinary controls remain 28px. Phone below the
canonical 768px boundary uses an inset bottom Drawer with a capped dynamic
viewport height, not the workflow form's full-height takeover: this is a short
temporary three-choice form with no profile editor. Header and action footer
stay fixed; the form body owns vertical scrolling. Keep bottom safe-area padding
and phone/coarse-pointer targets at least 44px (48px nominal primary button).
Coarse-pointer wider layouts retain the dialog with touch-sized controls.
Resize preserves draft state; keyboard and touch use the same business rules.

The plan's UI-01 through UI-04 previews define structure, not pixel styling.
Rendered tests cover long translated names, 767/768px transitions, coarse
pointer, keyboard dismissal/focus return, safe-area containment, and zero
document horizontal overflow.

## Delivery and verification

- [Original domain/MCP delivery](../../../plans/cross-workspace-task-transfers/plan.md)
  remains the historical implementation record; its frontend exclusion applies
  only to that package.
- [Web workspace-move package](../../../plans/task-workspace-moves/plan.md)
  owns draft Web additions and exact TDD, desktop/mobile E2E, and documentation
  checks.

No new ADR is needed: this feature exposes the existing preservation boundary
rather than choosing a new transfer or ownership policy.

## Related decisions

- [Separate runtime permissions and advisory actions](../../../decisions/2026-09-07-separate-runtime-permissions-and-advisory-actions.md)
- [Task-owned worktree lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
