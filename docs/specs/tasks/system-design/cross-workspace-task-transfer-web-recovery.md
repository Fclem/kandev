---
status: current
system: tasks
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-004
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-005
---

# Cross-workspace task transfer web submission recovery System Design

Companion to [Audited cross-workspace task transfers System Design](cross-workspace-task-transfer.md).
Owns the Web submission and recovery rules, plus the Web durable-rejection outcome
precedence and the pre/post-commit cancellation boundary those operations depend on.
Workspace-first lock acquisition order and the SQLite serialization reservation stay
in the parent. The parent keeps the HTTP boundary, conflict-code table, persistence,
and observability.

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
- Acquire transfer serialization before a mutation savepoint. An exact
  committed receipt takes precedence over any caller/request-bound terminal
  rejection, and both take precedence over a mutable source/lane validation
  result. All same-key attempts, including ones already admitted at the
  service, must acquire this same serialization and check the existing
  failed-key barrier before any commit.
- If an authorized, well-formed Web operation fails a domain guard, including
  `source_changed` or `step_incompatible`, roll back its mutation savepoint
  while retaining the outer serialization lock. Recheck committed receipt
  precedence, then persist one durable `failed` attempt in the existing audit
  table and commit only that rejection record before returning its specific
  409. The rejected transfer changes no task/context data. This record is both
  the redacted attempt audit and the existing no-later-same-key-commit barrier;
  do not add a duplicate outside-transaction conflict audit.
- The source predicate is checked under the workspace-row locks, before the
  relation reservation. A `source_changed` found there takes no savepoint. It
  first resolves receipt and barrier precedence: a committed receipt replays, an
  exact recorded barrier returns `operation_rejected`, and a barrier for another
  request returns `idempotency_conflict`, writing no second `failed` row.
  Otherwise it commits the caller-bound failed barrier while holding only those
  workspace locks, which key every same-key barrier lookup.
- `relations_busy` (PostgreSQL `55P03` from the NOWAIT relation lock) is not a
  domain-guard rejection. After rollback it records one non-barrier `conflict` audit
  row outside the transaction and leaves the same key retryable. It writes no
  `failed` barrier and no `operation_rejected` terminal result.
- Every Web attempt that ends without a committed receipt or a domain-guard barrier
  (cancellation, authorization or boundary lookup failure, workspace or relation
  lock wait or failure, replay or precheck lookup, apply, or commit infrastructure
  error) records one detached non-barrier `conflict` row, leaves the same key
  retryable, and is uncertain to the client. A settled authorization outcome
  (forbidden, or task, workspace, or workflow not found) stays a `denied` barrier.
  A not-found that only fails closed because a membership or unit-reach lookup
  failed keeps its 404 response but is audited as `conflict`.
  The MCP path audits the same infrastructure failures as `failed` or `denied`;
  the Web path audits them as `conflict`.
- Bind Web terminal-result lookup to the exact task, expected source placement
  and generation, destination IDs, policy, key, and server-attested actor/session
  stored in the audit columns. A changed request or other actor's key collision
  is an idempotency conflict, never permission to create/replace a terminal
  marker or to poison the original operation. Legacy denied/failed records can
  establish terminal rejection only when these available Web predicates match.
  Ordinary historical `conflict` rows alone are not terminal evidence.
- An exact retry returns `operation_rejected` only after serialized receipt
  lookup finds no committed receipt and verifies the durable failed-key barrier.
  The service precheck reads the barrier without serialization; this is
  equivalent because the relation reservation holds the audit table, so neither a
  failed row nor a boundary `denied` row becomes visible beside an attempt that
  can still commit.
  Thus a lost original domain-409 response can settle and release the form for
  a fresh reviewed key. An overlapping writer either committed first (receipt
  wins), or observes the barrier and cannot subsequently commit. Audit write,
  lookup, or commit failure remains uncertain; missing-key or arbitrary-409
  responses never establish terminal rejection. Current authorization precedes
  all disclosed outcomes, including after access restoration.
- Retain the unresolved submission in the owning domain state across
  dialog close/reopen within the page lifetime, scoped by task and authenticated
  caller. Dismissal is not rollback. Identity change, logout, or navigation
  never clears an unresolved record; it is only cleared when an exact receipt or
  authoritative terminal rejection resolves it.
- Persist the unresolved operation record to `sessionStorage` (tab-scoped, not
  `localStorage`) so a full page reload keeps its identity. The record holds only
  the idempotency key, task ID, expected source placement and generation,
  destination workspace/workflow/step IDs, transfer policy, a digest of the
  immutable request, and the authenticated caller ID. It holds no transcript,
  prompt, credential, token, or authority. Reads of it on load are scoped to the
  current caller. Records of other callers stay stored but are hidden and never
  sent to the server; a different caller cannot check or act on them. Switching
  back to the original caller restores the record with the same key and body.
  Clearing happens only on the terminal resolution above, never on identity
  change or logout.
- On reload, canonical reads determine current placement and the restored
  record shows `Check result`. That action re-sends the identical body and key
  under current authorization, exactly as in an in-page check. No POST replays
  automatically, and a restored record never enables a fresh-key move until a
  terminal result is established by the rules above.
- If a restored record fails validation, the Web keeps placement reads only and
  shows that the earlier move cannot be checked from this browser. It does not
  claim the original move failed, and it enables no fresh-key move.
- If tab storage is unavailable, no move can be recorded durably. The Web keeps
  placement reads only, says that the browser is blocking site data and asks for
  a reload after allowing it, and enables no fresh-key move. It does not claim an
  earlier move exists.
- If the user navigates to another task while the request settles, reconcile
  memberships but do not navigate, reopen the form, or steal focus.

The repository continues using the initiating context until commit. Caller
cancellation before commit rolls back; committed work retains the existing
bounded `context.WithoutCancel` event/WIP reconciliation. Test both cancellation
boundaries. There is no shared background transfer worker or extra retry loop.
