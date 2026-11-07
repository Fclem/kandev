---
created: 2026-11-05
status: draft
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-004
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-005
system_design:
  - ../../specs/tasks/system-design/cross-workspace-task-transfer.md
legacy_specs: []
---

# Implementation plan: Web task workspace moves

## Overview

Deliver [issue #5075](https://github.com/kdlbs/kandev/issues/5075) by exposing the existing audited transfer transaction through a human Web API and responsive task actions. Task identity and sessions follow the task; workspace transfer does not execute an unrelated step transition.

Sequence: safe HTTP command and diagnostics, transfer-aware client reconciliation, then the unified menu and desktop/phone flow. These are real verification boundaries: the domain command can be tested without React, and transfer notifications already exist independently of the new UI. Execute sequentially; no delegation is authorized.

## Scope

### In scope

- One-task Web transfer between same-owner workspaces under `preserve-task-identity-v1`.
- Explicit destination workspace, workflow, and step selection; atomic preservation and supported-move explanations.
- One movement menu containing separate step, workflow, and workspace choices; readable step/progress labels.
- Listing membership, route context, and selected-session continuity, including source-only notification handling and stale reads.
- Desktop, phone, keyboard, touch, all seven catalogs, and public how-to documentation.

### Out of scope

- Arbitrary cross-workspace step-semantic changes, cross-owner moves, relation/profile mapping, or silently dropping labels/projects/groups.
- Task copies, task-tree/bulk workspace moves, a new MCP/plugin capability, filesystem relocation, or new runtime/session policies.
- Workspace mutation through generic task update or the existing same-workspace `/move` route.
- Automatic retry loops, undo, or a new transfer eligibility-preview endpoint.

These are the existing transfer restrictions, not a new narrower preservation policy. A non-equivalent destination is selectable but rejected with an explanation. Default workflows with matching names alone are not necessarily equivalent: inherited prompts, events, profiles, participants, WIP, and session policies also matter.

## Technical approach

### Existing contracts and companion packages

- Authoritative [requirements](../../specs/tasks/requirements/cross-workspace-task-transfer.md) and [design](../../specs/tasks/system-design/cross-workspace-task-transfer.md), extended in place, own this whole vertical feature.
- [Original transfer package](../cross-workspace-task-transfers/plan.md): completed domain/MCP implementation. Its frontend exclusion does not apply to this new package. Its historical results are not evidence for new HTTP, scope, client, or UI behavior.
- [Change workflow](../change-workflow/plan.md) and [step colors](../change-workflow-step-colors/plan.md): preserve their form, replacement controls, entry preview, source-version guard, and color semantics. Only movement-menu placement changes. On delivery reconcile affected menu E2E scenarios and active presentation wording; do not mark those completed packages pending or claim their historical commands were rerun.
- [Task-owned worktree lifetime](../../decisions/2026-08-08-task-owned-worktree-lifetime.md) prohibits deleting task resources merely because a session or listing membership changes. No new ADR, schema, or runtime flag is needed.

### Backend command

Add `POST /api/v1/tasks/:id/transfer` in task HTTP handlers with strict body decoding, server-attested human identity, fixed preservation policy, and `{receipt}` response. Reuse `Service.TransferTask` and the repository transaction and audit ledger. Require `authz.ScopeTaskWrite` for source and destination at the service boundary, including replay. Add typed conflict reasons that retain `errors.Is(ErrTaskTransferConflict)`; expose only allowlisted codes after authorization, never raw database/error strings. MCP remains non-disclosing.

Bind the complete exact source and `updated_at` string plus stable destination IDs and one idempotency key. No JavaScript Date conversion of source generation. Keep identity/auth context through cancellation; preserve existing post-commit reconciliation ownership.

For reviewed Web operations only, terminalize source/lane/relation/capacity domain rejection under shared transfer serialization. Acquire the lock before a mutation savepoint, roll back task effects on a guard failure, recheck receipt precedence, and commit one existing `failed` audit row bound to the exact Web command and actor. That row is the same-key no-later-commit barrier. An exact retry can then settle a lost original 409 as `operation_rejected`; changed-body/actor probes cannot install or replace that barrier. Keep MCP conflict behavior and audit count unchanged. Marker persistence or lookup failure remains uncertain.

### Client membership and continuity

In `lib/ws/handlers/tasks.ts`, branch transfer-tagged source `task.deleted` before actual deletion cleanup and treat the marker as a membership invalidation. Reuse one small transfer reconciler for redacted removals, full destination updates, and HTTP settlement. Before removing source membership or preview, accept a guarded canonical task/source-membership read confirming departure or definitive access loss; a source return or transient read failure preserves the current row. Capture task/source identity, workspace generation, and accepted read revisions before the read and revalidate before applying it. Source IDs, opaque operation UUIDs, and completed-task `updated_at` do not order A -> B -> A. Retain sessions, transcripts, drafts, queue state, walkthrough, and task route; invalidate affected sidebar counts/pages and Office projections without weakening stale-read fences.

Membership admission cannot depend on observing a transfer marker. Untagged update/state/create events and cache/snapshot insertions cannot establish or change source membership without accepted canonical admission. Accepted per-task departure reads install the retained guard; startup/reconnect/re-subscription/workspace reactivation invalidates prior admission proof, while positive current-generation canonical rows admit only themselves. Missing known residents and unknown event candidates trigger guarded per-task confirmation; partial/filtered/paged absence never proves workspace departure. Retain source guards for authenticated page/store lifetime across reconnects and canonical returns, since upstream events may arrive on new sockets. Stable resident non-membership fields retain normal freshness rules. Canonical returns and ordinary new tasks remain discoverable; never replay held payloads. Keep only control metadata in the existing reconciliation owner, not a second task store or subscription redesign.

Do not insert foreign tasks into the active board. Listing actions stay in the source workspace and offer Open task. An already-open moved task stays on its route and selected session while the normal task navigation loader adopts canonical workspace context. Stale receipt replay never moves the client back to its historical destination.

### UI and submission

Add proposed `TaskWorkspaceMoveDialog` and `useTaskWorkspaceMove`. Reuse existing workspace/workflow APIs, `use-change-workflow-data.ts` request-generation patterns, and shared selector/focus primitives. Keep catalog browsing independent from the active workspace. Reset dependent selectors, require explicit step choice, and never calculate SQL equivalence in the browser.

Share movement entries across `kanban-card-menu-items.tsx`, `TaskMoveContextMenuItems`, `useTaskActionsMenu`, sidebar, Threads, and palette adapters. Keep existing step and workflow operations separate beneath the common parent. All single-task surfaces expose workspace transfer even if the current workflow has only one step; bulk transfer is not offered. Preserve existing archived/ephemeral eligibility.

A submission captures immutable predicates/key and prevents double clicks. Network/5xx uncertainty freezes that request and offers explicit Check result with the same body/key. Rejection of the original request can permit a fresh reviewed attempt; a check's 403/404 or arbitrary conflict cannot: retain the body/key in an unavailable-to-verify state, without rollback claims, until permissions recover and the same operation can be checked. Only its exact committed receipt or an authoritative `operation_rejected` result resolves it. The latter uses existing failed-key storage under transfer serialization, with committed receipt precedence and a durable barrier against any later same-key commit. Canonical reads reconcile current placement, not historical operation outcome. Preserve unresolved state through drawer reopen; logout clears it. Full reload restores the unresolved request from per-tab `sessionStorage` (opaque key, task, expected placement and generation, destination IDs, policy, request digest, and caller ID; no transcript, credential, or authority), keeps `Check result` available, and never auto-POSTs. Identity change clears the record.

### Compatibility boundaries

| Caller or shape | Intended behavior | Evidence |
| --- | --- | --- |
| Local synthetic human; same-owner equivalent steps | Transfer existing task, including running session | Handler/service tests; desktop and phone E2E |
| Authenticated human with task.write in both workspaces | Same transfer policy | Scoped handler/service tests |
| Read-only or foreign/unreachable workspace | 403 for known insufficient scope; otherwise non-disclosing 404 | Auth tests; no mutation assertions |
| Non-equivalent step or unmapped relations/profiles | Typed safe conflict; no task changes | Repository/handler tests and UI failure E2E |
| Existing MCP human/coordinator | Same tool exposure, policy, audit, generic error text; write scopes enforced | Existing transfer MCP/service regression suites |
| Source-only WS reader | Redacted membership removal, not deletion | Gateway and client state tests |
| Existing step/workflow and bulk moves | Existing payload/lifecycle; grouped navigation only | Existing move and workflow-form E2E |

Remote executors are not newly implemented or re-certified here. The existing transfer command preserves runtime artifacts without executor-specific migration; the Web adapter must not introduce executor gates or claim live SSH/Kubernetes coverage from mock-agent E2E.

## ASCII UI preview

Sketch text is illustrative localized copy, not a pixel specification. Required structure: common move parent, three distinct operations, current placement, ordered selectors, preservation notice, fixed form footer, and clear recoverable errors. Map: AC-004.1 through AC-004.6 and AC-005.3 below use the full `AC-TASKS-CROSS-WORKSPACE-TRANSFER-` prefix.

### UI-01: Desktop task movement menu, expanded

Entry: card ellipsis/right-click, sidebar, detail/preview, Threads; palette mirrors the same hierarchy.

```text
Task actions       Move to
+---------------+  +---------------------------+
| Edit          |  | Workflow step           > | -> wider step/progress list
| Move to     > |  | Different workflow...      | -> existing workflow form
| Archive       |  | Different workspace...     | -> UI-02
| Delete        |  +---------------------------+
+---------------+

Workflow step list (preferred 20rem, viewport bounded)
+---------------------------------------------+
| o Implementation with a long readable name   |
|   Agent name and progress wrap below         |
| o Review                            Current  |
+---------------------------------------------+
```

Labels/details wrap; dots, badges, options controls do not overlap. Vertical scroll remains inside the portaled submenu. Step options and workflow mapping remain available. Multi-selection omits workspace transfer. AC-004.1, AC-004.6.

### UI-02: Desktop workspace transfer, ready

```text
+------------------------------------------------------+
| Move task to another workspace                    X  |
| Task title                                           |
| From: Personal / Kanban / In progress                |
|                                                      |
| Workspace  [ Product team                         v ]|
| Workflow   [ Delivery                             v ]|
| Step       [ o In progress                         v ]|
|                                                      |
| Keeps history and sessions, including running work.   |
| Step settings must match. Some associations can block.|
|                                                      |
|                                  [Cancel] [Move task]|
+------------------------------------------------------+
```

Header and footer fixed; form body scrolls if needed. Desktop ordinary controls 28px; wider coarse-pointer controls at least 44px. No workflow-entry preview or profile replacement fields for workspace transfers. AC-004.2, AC-004.3.

### UI-03: Phone task movement and inset transfer drawer

Entry: visible task ellipsis, never right-click/hover only. Menu row navigation opens one form, not a stack of three drawers.

```text
+-----------------------------+
| Current task                |
|                             |
|   dimmed underlying task    |
| +-------------------------+ |
| | Move task             X | | <- fixed drawer header
| | From: Personal          | |
| | Kanban / In progress    | |
| |                         | |
| | Workspace [Product   v] | |
| | Workflow  [Delivery  v] | | <- one scrolling body
| | Step      [In prog.  v] | |
| |                         | |
| | History/sessions kept.  | |
| | Matching settings only. | |
| +-------------------------+ |
| | [Cancel] [Move task]     | | <- fixed footer, safe area
| +-------------------------+ |
+-----------------------------+
```

Inset bottom Drawer; bounded by dynamic viewport rather than a desktop sheet stretched full-screen. Phone below 768px uses at least 44px targets even with a fine pointer. Primary nominal target 48px. Keyboard, safe areas, open selector lists, and long copy must not create page overflow. AC-004.5.

### UI-04: Loading, unsupported, uncertain, and completed states

```text
Loading:     Workspace [Product v]  Workflow [Loading...]
             Step [Choose workflow]          [Move disabled]
Empty:       No workflows in this workspace. [Move disabled]
Load failed: Could not load workflows. [Retry] [Move disabled]
Rejected:    This step has different saved settings.
             Choose a matching step.          [Move task]
Stale:       Task changed. [Refresh task]      [Move disabled]
Uncertain:   Move result not confirmed.
             [Check result]  (submitted choices locked)
Cannot check: Move result cannot be checked with current access.
              [Check again] (same request retained; no new move)
Success:     Task moved. [Open task]
```

Original rejection and unavailable-to-verify are different states: a check denied after an uncertain submit does not prove the original move failed. Keep submitted choices/key locked until an exact receipt or authoritative terminal failed-key result resolves it. Canonical placement alone does not resolve operation outcome. Errors/status are localized and announced; valid choices survive and the fixed footer remains reachable. AC-004.2, AC-004.3, AC-004.4, AC-005.2, AC-005.3.

## Tests

Proposed test names below describe behavioral evidence, not source-text/wiring assertions. Add tests before implementation and record expected assertion failure, then passing result.

| Criteria | Test file and evidence |
| --- | --- |
| 001.1-001.4, 005.1 | New `task/handlers/task_transfer_http_handlers_test.go`: `TestHTTPTransferTaskPreservesIdentityAndRunningSession`, `TestHTTPTransferTaskRequiresWriteInBothWorkspaces`, `TestHTTPTransferTaskRejectsActorFields` |
| 001.2, 005.2 | Same handler file: `TestHTTPTransferTaskReportsSafeConflictReasons`, including incompatible lane, full lane, project/label/group/profile/pending mapping, cleanup, and stale source; assert no partial task changes |
| 003.1-003.5, 005.3 | New handler tests `TestHTTPTransferTaskReplaysExactRequest`, `TestHTTPTransferTaskCheckAfterPermissionRevocation`, `TestHTTPTransferTaskTerminalRejectedKey`, `TestHTTPTransferTaskLostDomainConflictResponse`: identity/non-disclosure, access revocation, receipt precedence, lost source_changed/step_incompatible 409 recovery, rejection-savepoint rollback, deterministic writer-first/rejection-first races, no later same-key commit, and no changed-body/actor poisoning |
| 001.5, 004.4, 005.4 | New `lib/ws/handlers/task-transfer.test.ts`, `lib/ws/client.test.ts`, and gateway `TestTaskNotificationTransferDepartureAcrossReconnect`: both marker-delivered retention and NO-marker recovery bootstrap; upstream-held ordinary event released on CURRENT socket after offline transfer and fresh source read; ordinary/completed unchanged-timestamp tasks; all source projections; real canonical return and ordinary new-task admission; partial/filtered/paged omission is not false departure; A -> B -> A and real deletion unchanged |
| 004.2, 005.3 | New `hooks/domains/kanban/use-task-workspace-move.test.ts`: dependent selection reset, late reads, exact generation/double-click admission, original rejection versus denied check, lost domain-409 -> authoritative terminal rejection -> new reviewed key, 403/404 across reopen/access restoration, canonical placement versus operation outcome, and new-key eligibility only after authoritative resolution |
| 004.1, 004.3, 004.5 | New `components/task/task-workspace-move-dialog.test.tsx` plus existing menu/form tests: visible state transitions, cancellation, keyboard/focus behavior, and retained workflow/step actions |
| Existing safety | Existing `task_transfer*_test.go`, `service_transfer_test.go`, MCP transfer handler tests, and gateway `task_notifications_test.go` |

## E2E tests

Add `apps/web/e2e/tests/kanban/task-workspace-move.spec.ts` and `mobile-task-workspace-move.spec.ts`, using `../../fixtures/test-base` and a shared page object/helper for destination seeding. Projects: `chromium` and `mobile-chrome`, respectively.

1. **Move and reopen** (001.1, 001.3, 004.1, 004.2, 004.4): create two same-owner workspaces with semantically equivalent steps, a task with plan/history and two sessions, one executing a deterministic mock turn. Transfer through visible UI. Source card disappears, source listing stays selected, Open task shows destination context and the same task/session IDs, plan and transcript; reload retains placement. Verify active turn continuity through its next deterministic output, not just a stored state label.
2. **Open-task continuity** (004.4, 005.4): same route/selected session with destination context; source invalidation/departure and A -> B -> A with delayed events. Task 02 must independently prove both retained-guard reconnect and missed-marker bootstrap: transfer while disconnected with NO transfer hint/receipt delivered to recovering store, then fresh complete source read and old upstream publication on CURRENT socket. All source projections stay absent; real canonical returns/new tasks remain discoverable, partial query omission is not departure. Cover completed unchanged-timestamp tasks, post-return stale events and both delivery orders; old-socket callback fencing alone is not proof.
3. **Supported failure** (001.2, 001.4, 004.3, 005.2): mismatched step settings and a workspace-owned label/project produce specific localized recovery; no placement, session, history, or association loss. User can repair choices/cancel.
4. **Stale/uncertain submit** (003.1, 004.3, 005.3): concurrent source move requires refresh/review; lost committed response resolves through the same-key receipt. Revoke destination write/read reach and check (403/404): keep unavailable-to-verify, immutable request across reopen, no rollback/new-key claim; restore access and get the same receipt. Separately drop real original source_changed/step_incompatible 409 responses: same-key check returns authoritative operation_rejected, unchanged task/session data, then a refreshed/reviewed fresh key is allowed. Ordinary placement reads cannot settle historical operation outcome. Use real backend effects and intercepted responses.
5. **Phone parity and layout** (004.5, 004.6): `.tap()` visible action, hierarchy to drawer, select and complete transfer; long names/copy, empty/error catalog, one scroll body, measured >=44px targets, safe footer, zero horizontal overflow. Also test 767px fine-pointer and 768px boundary with computed style/geometry checks; do not overwrite saved desktop preferences.
6. **Existing moves and labels** (004.1, 004.6): update `cross-workflow-task-move.spec.ts` and affected task-menu/palette tests for the grouped route without losing same-step options, workflow replacement fields, or bulk semantics. Replace implementation-only wording assertions rather than re-pin an incidental menu-array order.

Run headed-free managed builds through `pnpm e2e:run`, sequential projects, default shard/worker limits. Capture desktop/phone rendered screenshots and compare UI-01..UI-04 during the same run. Do not claim this design sketch is rendered proof.

## Work orders

- [ ] [Task 01: Expose safe human workspace transfer](task-01-human-transfer-api.md)
- [ ] [Task 02: Preserve client task context across transfers](task-02-transfer-reconciliation.md), depends on 01
- [ ] [Task 03: Deliver unified responsive movement actions](task-03-responsive-move-flow.md), depends on 01 and 02

## Verification results

Design validation completed on 2026-11-05; all three implementation work orders remain pending.

- `python3 scripts/list-docs.py validate`: passed (431 decisions and 1920 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/task-workspace-moves docs/plans/cross-workspace-task-transfers`: passed.
- Repository `.github/scripts/pr-docs.cjs::validateCoverage`: passed for all three new work orders, using a representative planned production path to exercise coverage instead of the documentation-only exemption. No artifact-reference errors.
- Local Markdown links in the owning requirement/design and new package: all resolve. Requirement and design sizes remain below their standard limits.

Exploratory existing-command smoke attempted on 2026-11-05:

```bash
(cd apps/backend && go test ./internal/task/service -run '^TestServiceTransferTask(AuthorizesBothWorkspacesAndPublishesOneCanonicalUpdate|RejectsForeignDestinationWithoutMutation|FinishesCommittedReconciliationAfterCancellation)$' -count=1)
```

Blocked before compilation: download of `github.com/kdlbs/acp-go-sdk` version `v0.13.6-0.20261101161427-70cc752b95c6` from `proxy.golang.org` failed with connection refused. No transfer runtime behavior was verified in this design turn. Implementation needs dependency access or the exact module already cached.

### Adversarial design review

- Round 1, OMP Sol profile, child `f9a38bdc-9815-4bc4-8edf-97253eec80d1`: two actionable P2 findings accepted. The design/work orders now require guarded canonical reconciliation for A -> B -> A and distinguish denied result checks from original-operation rejection. Added completed-task unchanged-timestamp, transient-read, revoked-access, and durable terminal failed-key test obligations. These are specification corrections, not implemented or runtime-verified fixes.
- Review loop complete: 8 rounds, with 3 consecutive clean rounds (6, 7, 8). Six actionable P2 findings from rounds 1, 3, 4, and 5 were accepted and addressed in the design package. Stop condition met; no round 9. All implementation work orders remain pending.
- Round 1 remediation validation: catalog validation, 36 specification-validator tests, full specification lint, scoped whitespace check, revised three-work-order coverage preflight, and local-link validation passed. Production/runtime verification remains deferred.
- Round 3, OMP Sol profile, child `7fc037e0-2cfc-44b8-8aa4-ba3b89f7133e`: accepted missing terminal recovery after a lost domain-conflict response, and missing admission fencing for delayed untagged source events. Added Web-only serialized savepoint/rejection-audit ownership, exact-request/no-poison rules, and retained transfer-source admission guards across ordinary events and canonical returns. Work orders and regression/E2E obligations are synchronized; these remain unimplemented design changes.
- Round 3 remediation validation: catalog validation, 36 validator tests, full spec lint, scoped whitespace, revised three-work-order coverage preflight, and local-link checks passed. Requirement/design remain within standard size limits. No production code, permanent tests, runtime or rendered verification were added or claimed.
- Round 4, OMP Sol profile, child `a6ca71f2-82e0-4617-8f6f-1e25dcb503eb`: accepted one P2 finding that browser transport replacement cannot drain upstream queued events. The guard now lasts for the authenticated page/store lifetime across reconnects and fresh source reads. Added the upstream-held publication delivered through the current replacement socket schedule, including completed tasks and later canonical returns. Static design correction only.
- Round 4 remediation validation: catalog validation, 36 validator tests, full specification lint, scoped whitespace, revised three-work-order coverage preflight, and local links passed. No runtime or browser verification was run; the updated reconnect/current-socket regression remains an implementation obligation.
- Round 5, OMP Sol profile, child `f164150c-b431-45cd-b341-6e2a033d5493`: accepted one P2 missed-marker bootstrap gap. Canonical per-task departure now installs source guards independently; unverified membership insertion requires current-generation canonical admission even for never-transfer-observed tasks. Added offline transfer without any delivered marker/receipt, recovery listing, current-socket upstream-event regression, new-task/return discovery, and partial-query omission checks. Static design change only.
- Round 5 remediation validation: catalog, 36 specification-validator tests, full spec lint, scoped whitespace, revised three-work-order coverage preflight, and local links passed. Requirement/design sizes are within standard limits. Recovery/bootstrap behavior remains a future implementation and deterministic-test obligation, not runtime proof from this design turn.
- Clean round 6: child `2fe53f14-a305-417e-b950-b4e0ed037398`; clean round 7: child `aefdaefd-3950-4408-9c97-2922087d9e0b`; clean round 8: child `ae9c1dab-85c1-4e67-bc07-173f2225ca92`. Each reported zero actionable findings after independent complete-package/source review, matching the requested Sol profile and unchanged seven-file fingerprints. These are static design-readiness findings, not production approval or runtime test evidence. Reviewers reported self-archival after sending their findings.
- The three clean rounds reviewed the same content over base `56af766dd9e9571e00e3c948426d75bf1c88b5a9`: requirement SHA256 `a2fa3cf52e03311ae9e8be814ba707696e0b5662640a1b4f697d2b3ee28e2cf4`, design SHA256 `bca7551013ff27f58d78ed9601e5f6c723f96295bde74b814e12fe13bae2ef3a`, and manifest SHA256 `8ae0439e6793dae9d0a0d6adcf07f5db2e78451e2750f32e17a705e2e4aad6d6`. Post-review manifest changes record only review completion; requirements, technical design, and work orders are unchanged.

## Risks

- Equivalent-step restrictions are strict and intentional. Same names are insufficient; the UI must explain incompatibility rather than imply any lane is supported.
- Current source `task.deleted` handling destroys client session state. Task 02 is required before exposing the Web action, not optional polish.
- Existing transfer service uses reach checks; source/destination write-scope tests must guard the new human path and replay without weakening MCP non-disclosure.
- A committed response can be lost, and a replay receipt can be historical. Resolve operation identity separately from current task placement.
- No mapping contract exists for labels/projects/shared groups/workspace-specific profiles. Do not unblock tests by silently dropping these relations.
- The requested broad implementation gate is `make fmt` followed by `make typecheck test lint`; run only after code exists, preserve unrelated user changes if formatting touches them, and record actual results. It is not a design validation command.
