---
status: draft
system: tasks
created: 2026-09-07
owners:
  - kandev
---

# Audited cross-workspace task transfers Requirements

## Overview

A task belongs to one workspace, but work outgrows that boundary: a platform
reorganization moves a board between workspaces, a delivery coordinator needs to
hand an in-flight task to the workspace that now owns its lane, and a human
restructures workspaces without abandoning running sessions. Recreating a task
elsewhere loses its plan history, sessions, and audit trail, while stopping the
agent first wastes an active turn.

The cross-workspace transfer capability moves one exact task identity — the
same UUID, sessions, plan, relations, and history — from an authorized source
workspace to an equivalent lane in a destination workspace owned by the same
owner. The transfer is one atomic operation with an idempotent receipt and a
redacted audit row for every attempt.

The task system owns this contract because task identity, placement, lifecycle,
and movement authorization are durable task behavior. The MCP surface exposes
it; the Web UI also exposes it through a human request boundary. Authorization
is server-attested, never taken from a request payload.

## Terminology

- **Transfer:** One atomic movement of an existing task's identity from a source
  workspace to a same-owner destination workspace, preserving every durable and
  runtime artifact.
- **Placement predicate:** The exact source workspace, workflow, and lane, plus
  the task's current `updated_at` generation, that a request must bind before
  the transfer is attempted.
- **Equivalent lane:** A destination lane whose configurable semantics match the
  source lane: name, effective prompt, inherited workflow prompt, events,
  effective agent profile, stage type, pull source, manual-move and start flags,
  archive timer, signal-gating, cancel-completion behavior, WIP limit, and the
  durable participant slate.
- **Idempotency key:** A caller-supplied identity for one transfer request. An
  exact retry returns the stored receipt; the same key with a changed request,
  actor, or session conflicts.
- **Preservation policy:** The versioned contract `preserve-task-identity-v1`
  that defines what the transfer must preserve and what the receipt reports.
- **Receipt:** The durable per-operation result containing identifiers, counts,
  and digests — never prompts, message bodies, secrets, or credentials.
- **Server-attested actor:** The transfer caller identity the backend resolves
  from the authenticated session or principal. A request payload can never
  assert it.

## Requirements

### REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001: Atomic identity transfer

**Intent:** An authorized caller can move one exact task to a same-owner
workspace without recreating data, stopping sessions, or exposing a
half-transferred state.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.1:** When a transfer request names an
  existing task, binds the exact source workspace, workflow, and lane, the
  current task generation, a destination workspace and workflow, one
  destination lane resolved by stable lane ID or a unique exact lane name, a
  unique idempotency key, and the `preserve-task-identity-v1` policy, the system
  shall move that task into the destination placement in one atomic operation
  and return a receipt for the same task UUID.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.2:** When any bound predicate no
  longer matches the task's persisted placement or generation, when the
  destination lane is missing or not uniquely mapped, when the destination lane
  is not equivalent to the source lane, when the destination lane is at WIP
  capacity, or when the source and destination workspaces do not share one
  owner, the system shall reject the transfer with one stable conflict result
  and shall not change any source or destination state.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.3:** When a transfer completes, the
  system shall preserve, without cancellation, duplication, or loss: all
  sessions including a running writer turn, plans and messages, task
  repositories, worktrees, and branches, parent and dependency relations,
  pull-request associations, pending moves, status summaries, terminal Done
  archive timing, and Blocked preservation state.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.4:** When the task carries
  workspace-owned relations that cannot follow it — labels scoped to another
  workspace, workspace groups, project bindings, or other relations without a
  destination mapping — the system shall reject the transfer before moving the
  task rather than silently dropping or re-scoping those relations.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.5:** When a transfer commits, the
  system shall rebind workspace-owned projections of the task in the same
  transaction, emit the destination task update through the source notification
  boundary, and reconcile dependent caches and events exactly once, so a
  subsequent read observes the destination placement without a duplicate or
  missing event.

### REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002: Server-attested authorization

**Intent:** Only an authorized human or server-attested coordinator can transfer
across workspaces, and no request payload can grant that authority.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.1:** When the transfer surface is
  invoked, the system shall resolve the actor from the authenticated server
  context and shall admit only a human caller or a server-attested coordinator
  holding destination-workspace authority; request-supplied actor fields shall
  never influence this resolution.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.2:** When an ordinary task agent, an
  automation coordinator without destination reach, or any unattested caller
  invokes the transfer surface, the system shall deny the attempt without
  revealing whether the named task exists.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.3:** When a coordinator-session
  transfer is invoked, the system shall map at most its own runner seat to the
  destination workspace's unique active authority and shall reject the transfer
  when that mapping is missing or ambiguous.

### REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003: Idempotent replay and audit

**Intent:** Retrying a completed transfer is safe and every attempt is
auditable, without leaking task existence or transcript content.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.1:** When the same bound actor and
  session retries the same request — same idempotency key and unchanged
  request identity — the system shall return the stored receipt without
  re-executing the transfer and without depending on mutable workflow or lane
  configuration.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.2:** When the same idempotency key is
  reused with a changed request, a different actor, or a different session, the
  system shall return one stable conflict result and shall not alter the
  committed transfer.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.3:** When a caller retries after the
  caller's request context is cancelled, the system shall still complete the
  committed transfer's post-commit reconciliation exactly once.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.4:** When any transfer attempt is
  accepted, rejected, or denied, the system shall write one audit record that
  identifies the operation, the bound placement, the idempotency key, and the
  outcome, and that never contains prompts, message bodies, secrets, or
  credentials.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.5:** When a caller reuses the
  idempotency key of a denied or failed attempt, or attempts a replay without
  current task or destination-workspace access, the system shall reject the
  retry or replay and shall not fabricate a successful receipt.

### REQ-TASKS-CROSS-WORKSPACE-TRANSFER-004: Web task movement

**Intent:** A person can reorganize an existing task without agent assistance
or database edits. This extends the existing transfer contract to desktop and
phone task actions.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.1:** Single-task card, sidebar, detail,
  preview, Threads, and command-palette actions shall group movement under
  `Move to`, with separate choices for a workflow step, a different workflow,
  and a different workspace. Existing step entry options and workflow agent
  mapping shall remain available with their existing semantics. Multiple-task
  selection shall not offer a workspace transfer.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.2:** The workspace form shall show the
  task's current placement and let the user explicitly choose a different
  accessible workspace, a visible workflow in it, and a step in that workflow.
  Steps shall follow workflow order, including personally hidden steps.
  Changing a workspace shall clear the workflow and step; changing a workflow
  shall clear the step. Browsing choices shall not switch the active workspace
  or mutate the task. Empty, loading, and failed catalog states shall be distinct;
  failed loads shall offer retry and incomplete choices shall prevent submission.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.3:** Before submission the form shall
  explain that a workspace move preserves sessions rather than entering a new
  step, requires equivalent step settings, and can be blocked by workspace-owned
  associations. Cancel shall leave the task unchanged. A rejected transfer shall
  preserve still-valid choices, explain the supported recovery, and leave task
  placement and context unchanged.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.4:** After success the task shall
  disappear from source-workspace listings and appear in the selected destination
  workflow and step, including after reload. A listing-initiated transfer shall
  keep that listing's selected workspace and workflow and offer an action to open
  the transferred task. When the transferred task is already open, its route and
  selected session shall remain open while its displayed workspace context follows
  the destination. Other open tasks shall not be retargeted.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.5:** Phone users shall open the move
  flow from a visible task action and complete the same choices in an inset
  bottom drawer with one scrolling form body and a fixed action footer.
  The surface shall fit the dynamic viewport, clear safe areas, and provide
  phone/coarse-pointer action targets of at least 44 CSS pixels. Desktop shall
  retain compact controls. Labels, keyboard navigation, focus return, and
  localized status/error copy shall cover both presentations.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.6:** Movement menus shall provide room
  for step names and progress/agent details without overlapping their action
  controls. Long names shall wrap within the viewport rather than become
  indistinguishable clipped choices. Pointer, keyboard, and touch users shall
  be able to read and activate each destination without horizontal page scrolling.

### REQ-TASKS-CROSS-WORKSPACE-TRANSFER-005: Safe human submission and recovery

**Intent:** The supported Web action retains the atomic preservation boundary
and explains failures without creating a new permission or exposing private data.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.1:** A human caller shall need task-write
  permission in both workspaces, in addition to the existing same-owner and
  preservation-policy conditions. Read-only access shall not permit transfer.
  The server shall bind the actor; client-supplied authority shall be rejected.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.2:** When the caller has the required
  access, unsupported transfers shall distinguish stale source placement,
  unavailable destination, incompatible step settings, destination capacity,
  unmapped workspace associations or profiles, pending-step mapping, and active
  cleanup. Explanations shall describe corrective action without exposing
  internal table names, prompts, or credentials. Missing reach shall retain
  non-disclosing errors; existing MCP errors shall remain non-disclosing.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.3:** Submission shall prevent duplicate
  requests. After an uncertain network outcome, the user shall be able to check
  the same submitted operation without creating another transfer. Choices and
  request identity shall remain bound until its result is resolved. Denial or
  unavailability of a result check shall not establish that the original move
  failed, release its identity, or permit a replacement move. A confirmed
  committed receipt or authoritative terminal rejection of that operation shall
  resolve it; an ordinary placement read shall establish only current placement.
  A definitive original rejection or a resolved, changed and reviewed request
  shall use a fresh operation identity. A stale-source rejection shall require
  refresh and another explicit submission, never silently overwrite a concurrent
  move or edit.
  When the original response for a domain-guard rejection is lost, a later exact
  check with current access shall be able to establish terminal rejection and
  permit a new reviewed submission. Once terminal rejection is established, no
  concurrent or later attempt of that same operation shall commit.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.4:** Source-removal notifications shall
  remove only source-workspace listing membership, not delete the task's local
  transcript, sessions, drafts, plan, or selected-session state. Destination
  updates and transfer responses in either order shall converge without deleting
  destination membership. After a task leaves and returns to its original
  workspace/workflow, an old or duplicated departure notification shall not remove
  its current row or preview, including for completed tasks with unchanged edit
  timestamps. A stale source listing response or delayed untagged task update
  shall not resurrect a departed task. A later genuine source return shall
  remain discoverable through authorized canonical reads, not inferred from an
  unverified event. Transient reconciliation-read failure shall not count as
  confirmed departure. A source-only reader shall receive no private destination
  payload.
  This guarantee shall survive browser reconnection within the same authenticated
  page/store, including when an old source event arrives through the new socket.
  It shall also hold when the transfer occurred while the browser was disconnected
  and no transfer notification was received. Partial-query absence shall not
  falsely establish departure, and ordinary new tasks and genuine returning
  tasks shall remain discoverable through authorized reads.

The new Web requirements are draft additions for
[issue #5075](https://github.com/kdlbs/kandev/issues/5075). The existing
same-workspace [change-workflow contract](change-workflow.md) continues to own
agent mapping and step-entry behavior: its command becomes a child of the common
movement group, not a workspace-transfer implementation.

## Out of scope

- Recreating, copying, or splitting a task; the transfer moves one identity and
  creates no task.
- Cross-owner or cross-trust-boundary movement; both workspaces must share one
  owner, enforced by REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001.2.
- Resolving project membership, workspace groups, or labels across the
  boundary; transfers that carry them fail closed by
  REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001.4 until a mapping contract exists.
- Changing same-workspace move behavior; the existing manual-move contract
  remains unchanged.

## Implementation plans

- [Existing audited transfer delivery](../../../plans/cross-workspace-task-transfers/plan.md)
  records the implemented domain command and MCP surface.
- [Web task workspace moves](../../../plans/task-workspace-moves/plan.md) adds
  the human Web API, transfer-aware client reconciliation, and responsive UI.
