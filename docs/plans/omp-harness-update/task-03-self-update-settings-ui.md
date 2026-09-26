---
id: "03-self-update-settings-ui"
title: "Add the self-update Settings dialog variant"
status: done
wave: 3
depends_on:
  - "02-backend-self-update-pipeline"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
  - AC-AGENTS-RUNTIME-UPDATES-003.1
  - AC-AGENTS-RUNTIME-UPDATES-003.2
  - AC-AGENTS-RUNTIME-UPDATES-003.3
  - AC-AGENTS-RUNTIME-UPDATES-003.6
  - AC-AGENTS-RUNTIME-UPDATES-003.7
  - AC-AGENTS-RUNTIME-UPDATES-003.12
  - AC-AGENTS-RUNTIME-UPDATES-003.14
  - AC-AGENTS-RUNTIME-UPDATES-003.15
  - AC-AGENTS-RUNTIME-UPDATES-003.16
  - AC-AGENTS-RUNTIME-UPDATES-003.17
system_design:
  - ../../specs/agents/system-design/harness-self-update.md
---

# Task 03: Add the self-update Settings dialog variant

## Summary

Extend the existing Agents update control to render harness-owned updates without presenting a selectable version catalogue. Keep the same trigger, responsive dialog/drawer, update job progress, streamed output, and result handling used for pinned runtimes.

## In scope

- Add typed `update_mode` handling for runtime update, status, preview, and job data.
- Hide `RuntimeVersionPicker` for `self_update` mode and display installed/current plus `stable_latest_version` as a reference, never as an update target.
- Update the API client and approval hook so `self_update` uses `update_mode` to call the approval API without a target and serializes exactly `{}`; keep target-required and default-reset requests/guards for pinned updates.
- Use structural operation state for approval availability; self-update `update` and `repair` remain actionable with no target, while `up_to_date` is disabled. Distinguish metadata-unknown status from a repair preview when current ACP version is unknown, and keep the preview control available when metadata status is unknown.
- Explain in localized UI copy that `omp update` follows the configured OMP channel and may install a version different from the stable reference.
- Update the Agents settings approval path so `useAgentRuntimeUpdates` returns an empty-ID `up_to_date` result without calling `upsertAgentUpdateJob`; start runtime-update status refresh for every such response without awaiting it.
- Add component/helper tests for reference labeling, channel explanation, metadata-unknown resolution errors, no-target self-update approval with exact `{}`, structural `repair` actionability, and the pinned target-required guard. Approval coverage clicks the enabled self-update action from a targetless `update`/`repair` preview and verifies the self-update API call.
- When approval returns terminal `up_to_date` with an empty `job_id`, preserve it as dialog-local result state, render it without registering or polling a job, and clear it on reset or a new approval. Test repeated no-op approvals for no shared job entry and one status refresh each; keep the result visible while refresh is pending or rejected.
- Test status-map behavior with rejected refresh and with overlapping refreshes completed out of order: failure keeps the last good map, and an older completion cannot overwrite the latest-started request's status.
- Test the mixed page/status-hook race with deferred responses: resolve initial status load; let successful job A start a status request; make job B successful while A is pending; then approve and return empty-ID `up_to_date`, causing a newer page-triggered status request that fails. Resolve A's response successfully but stale and assert neither response observes A or B; exactly one successor covers both pending IDs. Apply that successor, assert both jobs are observed, then rerender with a new jobs-map object and assert no extra request.

## Out of scope

- Backend job behavior or API route changes.
- New standalone UI system, new dialog primitives, or changes to the pinned runtime version picker.

## Acceptance

- OMP's trigger and update dialog show the current version and `stable_latest_version` as a reference, clearly explain that updates follow OMP's configured channel and may install a different version, and offer no target picker or rollback/default controls.
- Approval submits neither a target version nor `use_default`; `update_mode` and structural `operation` enable targetless self-update `update`/`repair` actions and disable `up_to_date`, while pinned mode retains its target/default contract. Metadata-unknown status keeps the update control enabled and opens preview, where the resolution error appears without presenting a job as created. Repair remains actionable. If approval revalidation returns `up_to_date` with an empty `job_id`, show the no-op result from local dialog state without registering or polling a job, clearing it on reset/new approval.
- A user click on a targetless self-update preview reaches the approval API with exact `{}`; the same empty-target action remains blocked for pinned mode.
- Empty-ID terminal `up_to_date` responses are not upserted into the shared update-job store; each starts a status refresh. The result renders while refresh is pending or rejected. Failed reads preserve the last good status, and a delayed older read cannot overwrite the latest-started refresh.
- Each status request snapshots pending successful job IDs at start and can observe only those IDs after an applied response. Late successful jobs and superseded batches share one queued successor; never retry independently per job.
- Desktop dialog and phone drawer match UI-01/UI-02/UI-03 in the [plan preview](plan.md#ascii-ui-preview), use translated copy, and preserve existing job output/progress behavior.

## ASCII UI preview

See UI-01 and UI-02 plus terminal no-op state UI-03 in the [plan](plan.md#ascii-ui-preview). Required structure under AC-AGENTS-RUNTIME-UPDATES-003.1-.3, .14, and .15:

```text
+--------------------------------------+
| Update omp                     [X]   |
| Installed version: 18.3.1            |
| Stable latest (reference): 18.3.2    |
| Update follows OMP's channel.        |
| Installed version may differ.        |
| Update command: omp update           |
| Job output: streamed updater output  |
| [Update omp]                         |
+--------------------------------------+
```

### UI-03: Approval revalidates as up to date

```text
+--------------------------------------+
| Update omp                     [X]   |
| Installed version: 18.3.2            |
| Stable latest (reference): 18.3.2    |
|                                      |
| Already up to date                   |
|                                      |
| [Close]                              |
+--------------------------------------+
```

Show this terminal result from dialog-local state. Do not create or poll a job;
refresh runtime-update status after every empty-ID no-op response.

Phone uses the same terminal result inside the existing drawer, preserving its fixed header/footer and scrollable center; no picker appears in either composition.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/agent-runtime-update.test.ts lib/api/domains/agent-update-api.test.ts components/settings/agent-runtime-update-control.test.tsx components/settings/use-agent-update-dialog-state.test.ts hooks/domains/settings/use-agent-runtime-updates.test.tsx hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx app/settings/agents/page.test.tsx
pnpm run typecheck
pnpm run i18n:check
```

## Files likely touched

- `apps/web/lib/types/http-agents.ts`
- `apps/web/lib/api/domains/agent-update-api.ts`
- `apps/web/lib/agent-runtime-update.ts`
- `apps/web/lib/agent-runtime-update.test.ts`
- `apps/web/components/settings/agent-runtime-update-control.tsx`
- `apps/web/components/settings/agent-runtime-update-control.test.tsx`
- `apps/web/components/settings/runtime-version-picker.tsx`
- `apps/web/components/settings/use-agent-update-dialog-state.ts`
- `apps/web/components/settings/use-agent-update-dialog-state.test.ts`
- `apps/web/lib/api/domains/agent-update-api.test.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-updates.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-updates.test.tsx`
- `apps/web/app/settings/agents/page.tsx`
- `apps/web/app/settings/agents/page.test.tsx`
- `apps/web/hooks/domains/settings/use-agent-runtime-update-statuses.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-update-statuses.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/agents.json`

## Dependencies

Task 02 defines the `update_mode` wire contract and terminal no-job
`up_to_date` approval response.

## Risks

- Existing helper assumptions may require current/default/effective version fields even though self-update mode has no Kandev default. Keep the mode-specific UI explicit and do not invent a default selection.

## Parallelism

`sequential`

## Inputs

- Requirement AC-AGENTS-RUNTIME-UPDATES-003.1-.3, .6, .7, .12, and .14.
- [Harness self-update design](../../specs/agents/system-design/harness-self-update.md), Data and contracts and Control flow.
- Existing `AgentRuntimeUpdateControl`, `RuntimeVersionPicker`, `useAgentUpdateDialogState`, and runtime update unit tests.

## Results

Implemented mode-aware targetless approval, localized reference/channel UI, dialog-local terminal no-job state, page-triggered advisory refresh, and generation-fenced single-flight job-status observation. Seven focused web test files passed (69 tests); store and WebSocket update tests passed; `pnpm run typecheck` and `pnpm run i18n:check` passed.
