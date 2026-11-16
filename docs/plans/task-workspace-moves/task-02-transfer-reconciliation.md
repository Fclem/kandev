---
id: "02-transfer-reconciliation"
title: "Preserve client task context across transfers"
status: done
wave: 2
depends_on:
  - "01-human-transfer-api"
plan: "plan.md"
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-004
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-005
acceptance_criteria:
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.5
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.4
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-005.4
system_design:
  - ../../specs/tasks/system-design/cross-workspace-task-transfer.md
---

# Task 02: Preserve client task context across transfers

## Summary

Make existing transfer notifications represent workspace-membership changes rather than task deletion. Provide shared reconciliation that the upcoming HTTP UI can use without deleting sessions, transcript caches, drafts, or destination membership.

## In scope

- Inspect gateway `broadcastTransferSourceRemoval` and its existing `task.deleted` marker. Keep source-only delivery redacted; extend WebSocket payload types to accept `transfer_operation_id` where needed.
- Branch transfer-tagged `task.deleted` before destructive deletion side effects in `lib/ws/handlers/tasks.ts`. Treat it as a source-membership invalidation, not an immediate removal. Preserve task-wide storage, queues, selected session, walkthrough, and recent-task identity.
- Reconcile redacted removals, full destination `task.updated`, and HTTP receipts through a guarded canonical task/source-membership read before changing source membership or preview. Remove only on confirmed departure or definitive access loss; preserve membership on a confirmed source return or transient read failure. This rule also applies when the active-workspace filter declines destination insertion.
- Share a small proposed `lib/ws/handlers/task-transfer.ts` helper with HTTP settlement. Capture task/source identity, workspace generation, and accepted sidebar/task-overview read revisions; revalidate before applying a result. Do not order removals by operation UUID, source IDs, or `updated_at`: A -> B -> A can return to the same source and completed-task timestamps are preserved. Existing read fences also prevent stale source responses from restoring a confirmed departure.
- Gate untagged `task.updated`/`task.state_changed`, ordinary `task.created`, and full-entity cache/snapshot paths that establish or change source membership, even when this store never received a transfer marker. Unverified event candidates must obtain accepted canonical membership before any projection upsert. For retained source guards, payloads only invalidate and are never replayed after the read; stable resident non-membership fields keep normal freshness rules.
- Retain source admission control metadata for the authenticated page/store lifetime after departure and canonical return, across workspace switches AND ordinary socket reconnects. Replaced-socket callback fencing does not drain old events still queued upstream; neither a new transport epoch nor a fresh listing permits guard retirement. Discard only with page/store or authenticated identity, not by timeout or row absence. Keep metadata in the existing reconciliation owner, not a second task store; coalesce invalidations.
- Bootstrap admission from normal reads and recovery, not just transfer hints: an accepted per-task departure/access-loss read installs the retained source guard. On startup/reconnect/re-subscription or workspace reactivation, invalidate prior admission proof and accept positive canonical membership only for rows actually returned under current recovery/workspace/read generations. An omitted known resident or unknown event candidate requires a guarded per-task read; filtered/partial/paged absence is not workspace departure. Ordinary new tasks and genuine source returns must be admitted through this same path.
- Rehydrate an active moved task through existing task navigation coordination using its canonical identity. Keep the route and selected session; adopt the actual destination workspace only if this task is still open. Do not follow a historical receipt over a newer move or steal focus after unrelated navigation.
- Refresh affected Office task projections and source archive/sidebar memberships when loaded without broad cache clearing.
- Add behavioral state tests before the change; preserve actual deletion and same-workspace move regression coverage.
- Add a deferred-transport/read regression: complete A -> B -> A, install the returned source row through a normal read, then deliver the old redacted removal and a duplicate. Keep the current row and preview, including for a completed task whose `updated_at` never changes. Also cover a real departure, transient lookup failure, and a late reconciliation response superseded by a newer accepted source read.
- Add a deferred source-event regression: hold an ordinary full A `task.state_changed`/`task.updated`, transfer A -> B, finish canonical departure removal, then release the old untagged event. Board, multi-board, overview/sidebar, and preview must not regain source membership. Cover ordinary/completed tasks, unchanged timestamps, both event orders, workspace switches with the same transport, a later genuine canonical A return, and an old event after that return.
- Add an upstream-queue reconnect regression: hold the ordinary source lifecycle publication before gateway fan-out, deliver transfer/departure confirmation, replace the browser socket while retaining the authenticated store, accept a fresh source listing, then release the held event through the CURRENT socket. No source board/multi-board/overview/sidebar/preview resurrection or task/session loss is allowed. Cover ordinary/completed unchanged-timestamp tasks, a later canonical source return, and another upstream stale event after that return. Invoking the old socket callback alone is insufficient proof.
- Add the missed-marker bootstrap regression independently of marker-retention coverage: hold an ordinary source lifecycle publication before gateway fan-out, disconnect the source browser, transfer from another browser while delivering NO transfer hint/receipt to the recovering store, reconnect that same store, accept a fresh complete source listing without the previously resident task, then release the old publication through its CURRENT socket. No source projection or preview may reappear. Include completed/unchanged-timestamp tasks, genuine later source return, and ordinary newly created tasks. Add partial/filtered/paged omission cases proving they trigger confirmation rather than false departure.

## Out of scope

- Rendered form/menu changes, transfer policy changes, a new identity cache, general deletion refactoring, or global subscription changes.

## Acceptance

1. A transfer marker invalidates source membership without deleting task-owned data. A guarded authoritative read removes only a confirmed departed/inaccessible source member; transient read failure preserves the row/preview and offers retry. Actual deletion retains normal cleanup.
2. Membership admission bootstraps even when transfer hints were missed during disconnect. Old transfer/ordinary events, duplicate delivery, HTTP/WS overlap and stale reads cannot remove a canonical return or resurrect a confirmed departure, including through a replacement socket and for completed unchanged-timestamp tasks. Query omission alone is not workspace departure; genuine returns and new tasks remain discoverable.
3. An open transferred task retains its route/session and adopts its actual workspace through normal navigation hydration. A source listing stays selected; source-only readers receive no destination data and recover through authorized reads, without treating transient failures as confirmed access loss.

## Verification

Install workspace dependencies once if this worktree has not been bootstrapped. Commands run from the repo root, sequentially:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/task-transfer.test.ts lib/ws/client.test.ts lib/ws/handlers/tasks-archive.test.ts lib/ws/handlers/task-lifecycle-diagnostics.test.ts lib/ws/handlers/task-dependencies.test.ts lib/state/task-navigation-reads.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/backend && go test ./internal/gateway/websocket -run 'Transfer|TaskNotification' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

For a smoke run before the new form exists, trigger the Task 01 HTTP transfer from a throwaway client against a disposable running app while source and task-detail browser tabs are open. Observe source removal, no session loss, destination context on the still-open task, and state after reload. Record the exact runtime/browser steps and screenshots; remove the throwaway client. The Task 03 E2E suite then covers the complete visible action.

This work changes data reconciliation, not UI composition; existing phone surfaces remain unchanged. New phone flow/geometry evidence is owned by Task 03.

## Files likely touched

- `apps/web/lib/ws/handlers/tasks.ts`
- New `apps/web/lib/ws/handlers/task-transfer.ts` and `task-transfer.test.ts`
- `apps/web/lib/ws/handlers/task-archive-cache.ts`, `task-lifecycle-side-effects.ts` only for focused membership/navigation reuse
- `apps/web/lib/sidebar/sidebar-task-page-cache.ts`
- `apps/web/lib/state/task-navigation-reads.ts` and existing task-overview read/revision code as required by the shared integration
- `apps/web/lib/state/slices/task-overview.ts`, `task-overview-types.ts`, `task-overview-merge.ts`, and their existing mutation/read tests for transfer-aware admission; source guard control metadata stays in the authenticated reconciliation owner
- `apps/web/lib/ssr/session-page-state.ts` through its existing navigation consumer, not a duplicate loader
- WebSocket payload types discovered from existing `WsHandlers`
- `apps/web/lib/ws/client.test.ts` for same-store reconnect/current-socket delivery integration
- `apps/backend/internal/gateway/websocket/task_notifications_test.go` for the controlled upstream lifecycle queue and current-reader/current-socket fan-out regression; production gateway routing stays unchanged

## Dependencies

Task 01 provides the supported human command for disposable runtime smoke and defines the receipt contract. Existing transfer event production is reused, not rewritten.

## Risks

- `task.deleted` currently retires reads and clears sessions/storage before cache removal. The transfer branch must precede every destructive side effect.
- Source and destination delivery are different reader boundaries; do not solve membership by broadcasting private destination payloads globally.
- Source workspace/workflow IDs are not an ordering fence for a returned task. Do not delete before the guarded canonical membership read or use completed-task `updated_at` as a placement revision.
- Task navigation and catalog reads use different scopes. Refreshing active task context must not publish old source data over the new workspace generation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/cross-workspace-task-transfer.md): 001.5, 004.4, 005.4.
- [System design](../../specs/tasks/system-design/cross-workspace-task-transfer.md): Client reconciliation.
- `apps/web/AGENTS.md`, `/tdd`, existing `tasks.ts`, `task-archive-cache.ts`, `task-navigation-reads.test.ts`, and gateway transfer source-removal implementation.

## Results

Completed 2026-11-07.

Implementation:

- `lib/ws/handlers/task-transfer.ts` is the membership reconciler shared by WS handlers and HTTP settlement (`reconcileTaskMembership` in `tasks.ts`). It coalesces one canonical `GET /tasks/:id` per task, supersedes in-flight results on a newer invalidation, reconnect/overview generation change, or newer accepted admission, and applies only the accepted canonical row. A read that places the task in another workspace, or a 403/404, removes only listing membership (boards, overview with a journaled deletion, archive list, sidebar pages, Office projection refresh) and installs a retained guard; an open task keeps its route and session and adopts the canonical workspace, or returns to the listing only on definitive unavailability. Transient failures keep the row and retry with bounded backoff.
- `tasks.ts` branches transfer-tagged `task.deleted` before every destructive cleanup. `task.created`/`task.updated`/`task.state_changed` pass `admitTaskEvent`: an event that would insert a task, change its workspace/workflow, carry a transfer source, or touch a guarded task becomes a canonical read; archived-only rows and admitted residents keep their normal freshness path.
- `lib/state/task-membership-control.ts` holds store-lifetime control metadata: per-task admission tagged with the task-overview generation (advanced by reconnects and workspace switches) and retained per-workspace departure guards, reset only on authenticated identity change. Accepted read journals (`finishTaskOverviewRead`, `retainTaskOverviews` with a read ID) admit their rows; cached pages do not.
- `use-workflow-snapshot.ts` confirms each resident omitted from a complete workflow listing through the same canonical read instead of treating absence as departure.
- Gateway routing is unchanged; `task_notifications_transfer_reconnect_test.go` pins the held-upstream-event-on-current-socket schedule, with and without a delivered marker, and source-only redaction.

RED: `vitest run lib/ws/handlers/task-transfer.test.ts` failed 12 of 15 behavioral cases before the change (sessions deleted by the marker, departed rows resurrected by delayed `task.updated`/`task.state_changed`, stale `step-a-stale` applied after a canonical return and after reconnect, unknown candidates inserted without a read). The client reconnect and omitted-resident tests were written alongside the implementation; a deliberate mutation (gate always admits, omission confirmation removed) made all four fail (`client.test.ts` 2, `use-workflow-snapshot-membership.test.ts` 2), and restoring the code returned 50/50 passing.

GREEN, sequential:

- `(cd apps/web && pnpm exec vitest run lib/ws/handlers/task-transfer.test.ts lib/ws/client.test.ts lib/ws/handlers/tasks-archive.test.ts lib/ws/handlers/task-lifecycle-diagnostics.test.ts lib/ws/handlers/task-dependencies.test.ts lib/state/task-navigation-reads.test.ts)`: 6 files, 80 tests passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/backend && go test ./internal/gateway/websocket -run 'Transfer|TaskNotification' -count=1)`: ok.
- Broader affected web suites (`lib/ws lib/state hooks/domains/kanban lib/sidebar`): 325 files, 3280 tests passed after updating tests that inserted tasks directly from unadmitted events to answer the canonical read.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.

Runtime evidence: the Task 03 production-build Playwright scenario `keeps an open task and its session while it leaves and returns` (chromium, disposable backend) triggers the human HTTP transfer A -> B -> A from the API while `/t/<id>` is open; the route and transcript stay, and the source board shows the returned card after both moves. The phone/desktop form scenarios also observe the source card disappearing after a committed move without session loss. No layout change is claimed by this work order.
