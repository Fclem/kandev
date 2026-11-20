---
id: "01-human-transfer-api"
title: "Expose safe human workspace transfer"
status: done
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
  - ../../specs/tasks/system-design/cross-workspace-task-transfer-web-recovery.md
---

# Task 01: Expose safe human workspace transfer

## Summary

Expose the existing atomic transfer command to authenticated human Web requests. Preserve transaction, session, actor, audit, and replay invariants while adding safe typed diagnostic codes and explicit write authorization.

## In scope

- Add strict `POST /api/v1/tasks/:id/transfer` adapter and `{receipt}` success response per design section Human HTTP boundary. Task ID is route-owned; actor and policy are server-owned. Reject unknown/trailing fields, including actor, owner, policy, audit identity, and entry options.
- Bind a human actor from middleware identity. Synthetic local identity is supported; missing identity is never internal authority. Keep human actor stable across a browser retry and do not accept a caller session ID.
- Require `authz.ScopeTaskWrite` in both workspaces inside `Service.TransferTask`, before replay or mutation. Preserve read/write reach distinctions, same-owner policy, and all MCP/coordinator guards.
- Introduce typed conflict diagnostics preserving `errors.Is(ErrTaskTransferConflict)`. Convert existing guard errors, including lane/profile/pending/relation/cleanup/idempotency guards, without changing their predicates. Map only allowlisted safe codes to Web responses; unknown internal failures do not claim rollback.
- Add server-only durable Web rejection mode and the `operation_rejected` terminal replay result using existing receipt/audit storage, not a new schema or endpoint. Acquire transfer serialization before a mutation savepoint. If a source/lane/workspace_relation_unmapped/capacity domain guard rejects the exact reviewed Web request, roll back mutation effects to the savepoint, recheck receipt precedence, then persist one caller/request-bound `failed` audit/barrier and commit it before returning the specific 409. `relations_busy` is not a domain guard: it stays a non-barrier conflict audit. Keep existing MCP conflict retry behavior.
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
- [System design](../../specs/tasks/system-design/cross-workspace-task-transfer.md): Human HTTP boundary, Safe conflict diagnostics. [Companion](../../specs/tasks/system-design/cross-workspace-task-transfer-web-recovery.md): durable-rejection, serialization, and cancellation rules for this endpoint.
- `apps/backend/AGENTS.md`, `/tdd`, existing `service_transfer_test.go`, repository `task_transfer*_test.go`, and `task_transfer_handler.go` strict decoding/actor pattern.

## Results

Completed 2026-11-07.

Implementation:

- `task_transfer_http_handlers.go` adds the strict `POST /api/v1/tasks/:id/transfer` adapter (route registered in `task_handlers.go`). Unknown/trailing fields, an unparsable generation, or a missing field return 400; a missing middleware identity returns non-disclosing 404. The adapter attests `TaskTransferActorHuman` from `authn.IdentityFromContext` (`local-human` only for a synthetic identity without user ID), fixes the policy, sets the server-only `DurableRejection` mode, returns `{receipt}` with `Cache-Control: no-store`, and audits adapter rejections as non-barrier `conflict` rows.
- `repoerrors/task_transfer.go` adds `TaskTransferConflictError` reasons that keep `errors.Is(ErrTaskTransferConflict)` and the existing error text. Every repository guard (lane, capacity, relations, project, profiles, pending moves, cleanup, idempotency, source predicate) is typed without predicate changes. The Web adapter maps an allowlist; MCP keeps `task transfer conflict` and now maps the new write-scope `ErrForbidden` to its existing non-disclosing `task transfer unavailable`.
- `Service.TransferTask` requires `authz.ScopeTaskWrite` for the task's current workspace and the destination before replay or mutation (reach-first 404, readable-but-forbidden 403).
- `task_transfer_durable_rejection.go` runs Web commands under the existing transfer serialization: committed receipt first, then caller-bound barrier lookup (exact request match returns `operation_rejected`, another request under the key returns `idempotency_conflict`), then `SAVEPOINT`; a domain guard rolls back to the savepoint, rechecks receipt precedence, and commits exactly one `failed` audit row (the barrier). Lookup/storage/commit failures stay uncertain 500s.

RED (behavioral, after the route existed): `go test ./internal/task/handlers -run TestHTTPTransferTask` failed `RequiresWriteInBothWorkspaces` (read-only source/destination expected 403, got 200), `CheckAfterPermissionRevocation` (expected 403, got 200), `TerminalRejectedKey` (failed barrier rows 0, want 1; lookup failure expected 500, got 409), `LostDomainConflictResponse` (expected `operation_rejected`, got `source_changed`/`step_incompatible`; the admitted same-key writer committed with 200 after the rejection). The first run before the route existed failed only on the missing route (404 and a gate that never admitted) and is not counted as behavioral evidence. MCP: `TestHandleTransferTaskKeepsMCPErrorsNonDisclosing/write_scope_denied` expected `NOT_FOUND`, got `INTERNAL_ERROR`.

GREEN, run sequentially with module cache only (`GOPROXY=off`; the pinned ACP SDK was already cached):

- `(cd apps/backend && go test ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/mcp/handlers -run 'TransferTask|TaskTransfer' -count=1)`: all four packages ok.
- `(cd apps/backend && go test -race ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite -run 'TransferTask|TaskTransfer' -count=1)`: ok (handlers 4.8s, service 2.7s, sqlite 38.5s).
- `(cd apps/backend && go build ./...)`: exit 0.
- New permanent tests: `task_transfer_http_handlers_test.go` (`PreservesIdentityAndRunningSession`, `RejectsActorFields`, `RequiresWriteInBothWorkspaces`, `ReportsSafeConflictReasons`, `ReplaysExactRequest`, `CheckAfterPermissionRevocation`, `TerminalRejectedKey`, `LostDomainConflictResponse` with deterministic writer-first and rejection-first gates, `CancelledBeforeCommitLeavesTaskUnchanged`) and `TestHandleTransferTaskKeepsMCPErrorsNonDisclosing`.

Disposable API smoke (`/tmp` backend `bin/kandev __backend`, port 38711, throwaway DB; script removed afterwards): seeded two same-owner workspaces with equivalent `Work` steps plus an incompatible `Review` step through the HTTP API. A fresh-key request to `Review` returned `409 {"code":"step_incompatible"}` with unchanged placement; the same key then returned `409 {"code":"operation_rejected"}`. The compatible request returned 200 (`Cache-Control: no-store`), operation `ee6b3c09-…`; `GET /tasks/:id` showed the destination workspace/workflow/step. The identical replay returned the same operation ID, and a body with `actor` returned 400. The seeded task had no session through the plain API (the session-preservation evidence is the running session/turn in the handler test and the Task 03 browser scenario).
