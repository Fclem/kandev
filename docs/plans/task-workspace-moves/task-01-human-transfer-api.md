---
id: "01-human-transfer-api"
title: "Expose safe human workspace transfer"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-005
acceptance_criteria:
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.3
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.4
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.3
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.4
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.5
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.3
system_design:
  - ../../specs/tasks/system-design/cross-workspace-task-transfer.md
---

# Task 01: Expose safe human workspace transfer

## Summary

Expose the existing atomic transfer command to authenticated human Web requests. Preserve transaction, session, actor, audit, and replay invariants while adding safe typed diagnostic codes and explicit write authorization.

## In scope

- Add strict `POST /api/v1/tasks/:id/transfer` adapter and `{receipt}` success response per design section Human HTTP boundary. Task ID is route-owned; actor and policy are server-owned. Reject unknown/trailing fields, including actor, owner, policy, audit identity, and entry options.
- Bind a human actor from middleware identity. Synthetic local identity is supported; missing identity is never internal authority. Keep human actor stable across a browser retry and do not accept a caller session ID.
- Require `authz.ScopeTaskWrite` in both workspaces inside `Service.TransferTask`, before replay or mutation. Preserve read/write reach distinctions, same-owner policy, and all MCP/coordinator guards.
- Introduce typed conflict diagnostics preserving `errors.Is(ErrTaskTransferConflict)`. Convert existing guard errors, including lane/profile/pending/relation/cleanup/idempotency guards, without changing their predicates. Map only allowlisted safe codes to Web responses; unknown internal failures do not claim rollback.
- Add server-only durable Web rejection mode and the `operation_rejected` terminal replay result using existing receipt/audit storage, not a new schema or endpoint. Acquire transfer serialization before a mutation savepoint. If a source/lane/relation/capacity domain guard rejects the exact reviewed Web request, roll back mutation effects to the savepoint, recheck receipt precedence, then persist one caller/request-bound `failed` audit/barrier and commit it before returning the specific 409. Keep existing MCP conflict retry behavior.
- Bind terminal lookup to all available Web command identity columns: actor/session, task, source placement/generation, destination IDs, fixed policy, and key. Serialization must prevent an already-admitted same-key attempt from committing after rejection; exact receipt wins if it committed first. Changed-body/other-actor conflicts cannot create or replace the original barrier. Audit write/lookup/commit failures remain uncertain. The rejection record is the single attempt audit; do not append a second outside-transaction conflict audit.
- Add real-repository handler integration tests alongside existing transfer tests; extend scope/replay service tests before changing behavior.
- Add `TestHTTPTransferTaskCheckAfterPermissionRevocation`: commit, lose response, revoke destination write (403) or reach (404), restore access, and replay the same receipt. Add `TestHTTPTransferTaskTerminalRejectedKey`: terminal rejection requires the durable key barrier and wins no race against an already committed receipt; a rejected key cannot commit later. Keep missing-key and lookup-failure cases nonterminal.
- Add `TestHTTPTransferTaskLostDomainConflictResponse`: drop original `source_changed` and `step_incompatible` 409 responses, check the same exact key, obtain authoritative terminal rejection, and permit a newly reviewed key without task/session changes. Deterministically race rejection publication with an already-admitted same-key writer; test both writer-first receipt precedence and rejection-first no-later-commit. Changed-body/actor probes must not poison an operation.

## Out of scope

- UI, new MCP tool exposure, new transfer policy, schema, relation mapping, automatic retry, or modified step-entry behavior.
- Fixing unrelated transfer-engine internals or promising new executor compatibility.

## Acceptance

1. A strict human HTTP command transfers the same task UUID, history, sessions, and running-turn identity into a valid destination and returns the existing durable receipt; invalid/rejected commands leave all task data unchanged, apart from the required redacted attempt audit.
2. Source/destination read-only callers cannot transfer or replay; inaccessible targets do not leak existence. Authorized conflict reasons are safe and specific while existing MCP text/classification remain unchanged.
3. Same-body/key/actor retries return one operation; changed identity conflicts without poisoning another request. Lost Web domain-409 responses recover through serialized durable terminal rejection; check-attempt permission failures remain nonterminal. Rejection barriers prevent every later same-key commit, while a prior committed receipt wins. Preserve both cancellation boundaries.

## Verification

All commands are repo-root-relative and run sequentially. Start with behavioral RED tests named in the plan's Tests section, not a missing-route/selector-only failure. Then execute:

```bash
(cd apps/backend && go test ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/mcp/handlers -run 'TransferTask|TaskTransfer' -count=1)
(cd apps/backend && go test -race ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite -run 'TransferTask|TaskTransfer' -count=1)
(cd apps/backend && go build ./...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

For API smoke, use a disposable backend/data directory, seed two equivalent workflows through its existing HTTP API, and invoke the new route with the exact generation. Observe the persisted destination task and the same session IDs through normal reads, then replay the identical request and observe the same operation ID. Submit a mismatched-step request with a fresh key and observe 409 plus unchanged placement. Use a throwaway script rather than adding wiring-only permanent tests; remove it after recording command/output. The final UI work order supplies production-build browser proof.

The design turn's attempted existing service test could not fetch the pinned ACP SDK from `proxy.golang.org`; do not label that setup failure a failing behavioral test. Restore module access/cache before collecting RED/GREEN evidence.

## Files likely touched

- `apps/backend/internal/task/handlers/task_handlers.go`
- New `apps/backend/internal/task/handlers/task_transfer_http_handlers.go` and `task_transfer_http_handlers_test.go`
- `apps/backend/internal/task/service/service_transfer.go`, `service_transfer_test.go`
- `apps/backend/internal/task/models/task_transfer.go` for server-only Web rejection mode, excluded from JSON/request identity
- `apps/backend/internal/task/repository/repoerrors/errors.go` or a focused new transfer-error file
- `apps/backend/internal/task/repository/sqlite/task_transfer.go`, `task_transfer_relations.go`, `task_transfer_profiles.go`, `task_transfer_pending.go`, `task_transfer_cleanup.go`, `task_transfer_audit.go`, and relevant adjacent tests
- `apps/backend/internal/mcp/handlers/task_transfer_handler_test.go` for preserved boundary behavior

## Dependencies

None. Existing transfer storage/service implementation is present. Read its original completed work package as historical context, not authorization to change its preservation policy.

## Risks

- `Service.TransferTask` currently performs reach checks, not task-write checks. Enforce write on replay as well as new mutations.
- Existing MCP generic errors must not begin exposing raw repository reasons.
- Timestamp precision and actor/session binding are part of replay identity. Do not round source generation or switch human actor identity between transport attempts.
- A 500/transport error can be uncertain; never render it as a definitive unchanged-task rejection.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/cross-workspace-task-transfer.md), 001-003 and 005.
- [System design](../../specs/tasks/system-design/cross-workspace-task-transfer.md): Human HTTP boundary, Safe conflict diagnostics, Submission recovery.
- `apps/backend/AGENTS.md`, `/tdd`, existing `service_transfer_test.go`, repository `task_transfer*_test.go`, and `task_transfer_handler.go` strict decoding/actor pattern.

## Results

Pending. Record RED/GREEN assertions, exact commands/results, and disposable API smoke evidence. Do not update historical companion test counts as if rerun.
