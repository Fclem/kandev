---
spec: docs/specs/prevent-agent-autostart-on-open/spec.md
created: 2026-08-11
status: draft
---

# Implementation Plan: Prevent Agent Auto-Start On Open

## Overview

Add a per-user setting, `prevent_auto_start_agent_on_open`, that stops the web
UI from automatically starting or resuming an agent when a task is *opened* in
two situations: the post-restart recovered-idle shape (session exists, agent
process not running) and tasks sitting in the final step of their workflow.
When the setting is on, those tasks open with the agent stopped and the
existing manual start affordance (the "Start agent" button for never-started
sessions) is shown instead.

The change spans: backend settings plumbing (models/DTO/service/controller/boot
payload), a small opt-in `auto_start` override on the `session.ensure` WS
contract, frontend settings plumbing (types/SSR/store/API client), the settings
UI card, and the two open-time gates (`useEnsureTaskSession` for the final-step
no-session case, `useSessionResumption` for the resume case). Order follows the
dependency chain: backend contracts first, settings plumbing, UI, then the
gating hooks, then E2E.

---

## Backend

### 1. User settings field

- `apps/backend/internal/user/models/models.go` — add
  `PreventAutoStartAgentOnOpen bool` with json tag
  `prevent_auto_start_agent_on_open` to `UserSettings` (after
  `ConfirmTaskArchive`). No DB migration: settings are a JSON blob.
- `apps/backend/internal/user/dto/dto.go` — add the field to `UserSettingsDTO`
  (`FromUserSettings` at `:238`) and `*bool` to `UpdateUserSettingsRequest`
  (after `ConfirmTaskArchive` at `:105`).
- `apps/backend/internal/user/service/service.go` — add `PreventAutoStartAgentOnOpen *bool`
  to the service-level `UpdateUserSettingsRequest` (`:52`); apply it in
  `applyTaskActionPreferences` (`:346` block); emit it in
  `publishUserSettingsEvent` (`:773` map).
- `apps/backend/internal/user/controller/controller.go` — map
  `req.PreventAutoStartAgentOnOpen` in `UpdateUserSettings` (`:61` block).
- `apps/backend/internal/user/store/sqlite.go` — persist the field in the
  settings JSON blob: add `"prevent_auto_start_agent_on_open"` to the
  `marshalUserSettingsPayload` map (`:519-573`) and a
  `PreventAutoStartAgentOnOpen *bool` field to the `scanUserSettings` payload
  struct (`:707-760`) with the pointer-guarded assignment. Default is `false`
  (the zero value in `defaultUserSettings` at `:651`).
- `apps/backend/internal/backendapp/boot_state_routes.go` — add
  `"preventAutoStartAgentOnOpen": settings.PreventAutoStartAgentOnOpen` to the
  boot-payload map (`:459` block).

### 2. `session.ensure` `auto_start` override

- `apps/backend/internal/orchestrator/session_ensure.go` — add
  `AutoStart *bool` to `EnsureSessionOptions`; in `EnsureSession`, when
  `o.AutoStart != nil`, override the step-derived decision:
  `autoStart := stepAllowsAutoStart(step); if o.AutoStart != nil { autoStart = *o.AutoStart }`.
- `apps/backend/internal/orchestrator/handlers/handlers.go` — add
  `AutoStart *bool \`json:"auto_start,omitempty"\`` to `wsEnsureSessionRequest`
  and pass it through as `EnsureSessionOptions{AutoStart: req.AutoStart}`.
- Behavior: `auto_start: false` → `IntentPrepare` / `created_prepare` even when
  the step has `auto_start_agent`; absent/`true` → unchanged.

---

## Frontend

### 1. Settings plumbing

- `apps/web/lib/types/http-user-settings.ts` — add
  `prevent_auto_start_agent_on_open?: boolean` to `UserSettings` and
  `UserSettingsUpdatePayload`.
- `apps/web/lib/ssr/user-settings.ts` — add `preventAutoStartAgentOnOpen: false`
  to `createDefaultUserSettings`; hydrate in `buildBehaviorFields`
  (`s.prevent_auto_start_agent_on_open ?? current.preventAutoStartAgentOnOpen`).
- `apps/web/lib/state/slices/settings/types.ts` — add
  `preventAutoStartAgentOnOpen: boolean` to `UserSettingsState`.
- `apps/web/lib/services/session-launch-service.ts` — extend
  `ensureTaskSession` opts with `autoStart?: boolean`; include
  `auto_start: opts?.autoStart` in the `session.ensure` payload.

### 2. Settings UI

- `apps/web/components/settings/prevent-auto-start-agent-settings.tsx` (new) —
  clone the `archive-confirmation-settings.tsx` pattern (SettingsCard + Switch +
  `useSettingsSaveContributor`), persisting `prevent_auto_start_agent_on_open`.
- `apps/web/components/settings/general-settings.tsx` — render the new card in
  `TaskActionsSettings` (first slot of the task-actions section).
- `apps/web/lib/settings-discovery/catalog/preferences.ts` — new target
  `preventAutoStartOnOpen: "setting-prevent-auto-start-on-open"` on
  `GENERAL_SETTINGS_TARGETS` plus a control definition under the task-actions
  page. (The PageShell restructure #2322 moved the catalog here from
  `catalog/general.ts`.)
- i18n — add `preventAutoStartAgentOnOpen` and `preventAutoStartAgentOnOpenHelp`
  to `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json` (help text
  must use plain punctuation, no em dash).

### 3. Open-time gates

- `apps/web/hooks/domains/session/use-ensure-task-session.ts` —
  - Extend `EnsureTaskInput` with `workflowStepId?: string | null` and
    `workflowId?: string | null`.
  - Read `state.userSettings.preventAutoStartAgentOnOpen` and resolve the
    task's workflow step list workflow-aware: `state.kanban.steps` when the
    task's workflow is the active one (`state.kanban.workflowId`), otherwise
    `state.kanbanMulti.snapshots[workflowId]?.steps`. Missing workflow id or
    step list → treated as "not final" (no gate).
  - When the setting is on and the task's step is the final step of that
    workflow (max `position`), call `ensureTaskSession(taskId, { autoStart: false })`;
    otherwise `ensureTaskSession(taskId)` as today.
  - New pure helper (exported for unit tests),
    `isFinalWorkflowStep(workflowStepId, steps)`.
- `apps/web/hooks/domains/session/use-session-resumption.ts` —
  - Read `state.userSettings.preventAutoStartAgentOnOpen` in
    `useSessionResumption` and thread a `preventAutoStart` boolean into
    `useSessionResetAndCheck` → `checkAndResume`.
  - In `checkAndResume`, when `preventAutoStart` is true, skip the
    `status.needs_resume && status.is_resumable` branch (do not call
    `resumeWithSilentFallback`); set `resumptionState` to `"idle"` and record
    the skip in the store (a `resumeSkippedSessionIds` set on the kanban tasks
    slice via a new `setResumeSkipped(sessionId, boolean)` action). The flag is
    keyed by session id, so it cannot leak across sessions.
  - Skip-flag lifecycle (all three clear points MUST be specified):
    - `resumeSession()` (`use-session-resumption.ts:491-522`) clears the flag
      for the session before launching.
    - The Start agent button click (`message-renderer.tsx:55-64`) clears it
      before dispatching.
    - The WS `session.state_changed` handler (`lib/ws/handlers/agent-session.ts:692-750`)
      clears it when the session transitions to a state where the agent is
      running (STARTING/RUNNING), so a late state event cannot leave a stale
      button behind.
  - The manual `resumeSession()` action is NOT gated.
- Start agent button for the recovered-idle case —
  `apps/web/components/task/chat/message-renderer.tsx`:
  `TaskDescriptionStartButton` currently renders only for
  `sessionState === "CREATED"`. Extend the visibility condition so it also
  renders when the store marks the session as resume-skipped, BUT only for
  non-FAILED sessions: `TaskDescriptionMessage` returns early for FAILED
  sessions at `:119-134` (agent-styled message, no button slot), and FAILED
  sessions keep their existing recovery actions (`recovery-resume-button` /
  `recovery-fresh-button` in `action-message.tsx`). Dispatch the matching
  intent: `buildStartCreatedRequest` for CREATED sessions, the resume request
  builder for the skipped-resume case.
- Callers:
  - `components/task/task-page-content.tsx` passes the normalized input
    `{ id: task?.id, workflowStepId: task?.workflow_step_id, workflowId: task?.workflow_id }`
    (the effective task is the HTTP `Task`, whose fields are snake_case).
  - `components/kanban-with-preview.tsx` — `useSelectedTask` must include
    `workflowId` in its returned subset (it currently drops it at `:171-180`),
    so cross-workflow preview tasks resolve their own workflow's steps.

---

## Tests

| Behavior (spec scenario) | File | How |
|---|---|---|
| PATCH accepts and persists the setting; GET/boot payload returns it | `apps/backend/internal/user/dto/dto_test.go`, `internal/user/service/service_test.go` (or existing settings-update test) | unit: round-trip `UpdateUserSettingsRequest` → model → DTO; assert blob round-trip |
| Settings blob survives reload (true/false/omitted-legacy) | `apps/backend/internal/user/store/sqlite_test.go` | repo test: `SaveUserSettings` then `GetUserSettings` preserves the value; legacy JSON without the key loads the default `false` |
| `session.ensure` with `auto_start: false` prepares instead of starts | `apps/backend/internal/orchestrator/session_ensure_test.go` (or `session_ensure_office_test.go` pattern) | integration: seed task + step with `auto_start_agent`; call `EnsureSession` with `AutoStart: &false`; assert `Source == "created_prepare"`, `State == "CREATED"`; control case without override still `created_start` |
| WS handler passes `auto_start` through | `apps/backend/internal/orchestrator/handlers/handlers_test.go` | unit: parse `{task_id, auto_start:false}` → handler calls service with the option |
| SSR defaults and hydration | `apps/web/lib/ssr/user-settings.test.ts` | unit: default `false`; hydrate from `prevent_auto_start_agent_on_open` |
| ensure payload carries `auto_start` | extend `apps/web/hooks/domains/session/use-ensure-task-session.test.ts` (mocks `ensureTaskSession`) | unit: final-step + setting-on → called with `{ autoStart: false }`; non-final → without |
| final-step helper | new test beside the helper | unit: max-position logic, missing step/steps → false |
| caller normalization (snake → camel input) | `apps/web/components/task/task-page-content` test or hook caller test | unit: task page passes `workflowStepId` from `workflow_step_id` |
| cross-workflow preview resolves the right steps | `apps/web/hooks/domains/session/use-ensure-task-session.test.ts` + preview test | unit: task from `kanbanMulti.snapshots[w]` uses snapshot steps, not the active workflow's |
| resume gate skips auto-resume and records the skip | `apps/web/hooks/domains/session/use-session-resumption.test.ts` | unit: `checkAndResume` with `needs_resume && is_resumable` and preventAutoStart → no launch, state idle, skip recorded in store; manual `resumeSession` still launches AND clears the flag |
| Start button renders for resume-skipped sessions (non-FAILED) | `apps/web/components/task/chat/message-renderer` test (or component test) | unit: `sessionState === "CREATED"` OR store resume-skipped flag (non-FAILED state) → button visible; FAILED + resume-skipped → no new button (recovery actions remain); button dispatches resume for the skipped case and clears the flag |
| skip flag clears on WS running state | `apps/web/lib/ws/handlers/agent-session.test.ts` (or slice test) | unit: `session.state_changed` to STARTING/RUNNING clears `resumeSkippedSessionIds[sessionId]` |
| Start button renders for resume-skipped sessions | `apps/web/components/task/chat/message-renderer` test (or component test) | unit: `sessionState === "CREATED"` OR store resume-skipped flag → button visible; button dispatches resume for the skipped case |
| settings card renders and saves | `apps/web/components/settings/prevent-auto-start-agent-settings.test.tsx` (mirror `archive-confirmation-settings.test.tsx`) | component: switch toggles, save calls `updateUserSettings({ prevent_auto_start_agent_on_open })` |
| i18n ratchet + em-dash check | — | `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet` |

## E2E Tests

- **Scenario:** GIVEN setting ON and a task in the final workflow step with no
  session, WHEN the task page is opened, THEN the Start agent button
  (`[data-testid="task-description-start-button"]`) is visible and the agent
  never starts on its own.
- **File:** `apps/web/e2e/tests/settings/prevent-auto-start-on-open.spec.ts`
- **What to verify:**
  - Set the setting via `apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true })`.
  - Create a custom workflow whose final step has
    `on_enter: [{ type: "auto_start_agent" }]` — extend
    `e2e/helpers/api-client.ts` `createWorkflowStep` with an `events` opt
    (the backend `POST /api/v1/workflow/steps` already accepts `events`,
    `internal/workflow/controller/controller.go` `CreateStepRequest`).
  - Final-step case: create a task in that final step, open `/t/:id`, assert
    the Start agent button appears and the agent stays stopped until clicked.
  - Setting-off control: create a SEPARATE fresh task in the same final step
    (never opened before, no session) — `useEnsureTaskSession` no-ops when a
    session already exists, so reusing the setting-on task cannot exercise the
    control.
  - Recovered-idle case: seed a task+session, let the first turn finish, then
    `backend.restart()` + `testPage.reload()` (the restart pattern from
    `e2e/tests/session/session-resume.spec.ts`), assert no automatic resume
    (the agent does not reach a running state on its own and the Start agent
    button is visible), then click the button and assert the agent resumes.
  - Control: with the setting off, the same final-step task auto-starts on
    open (no start button; agent reaches a running/ready state).
  - Isolation: `e2e/fixtures/test-base.ts` per-test settings reset (`:190-225`)
    gains `prevent_auto_start_agent_on_open: false` so a test enabling the
    setting cannot leak into later tests in the same worker.
  - Mobile: add `e2e/tests/settings/mobile-prevent-auto-start-on-open.spec.ts`
    (same fixtures, phone viewport via `testPage.setViewportSize`, following
    `mobile-general-settings.spec.ts`) asserting the Start agent button on the
    final-step case. The gating hooks run on mobile through the shared
    responsive `TaskPageContent` (`useResponsiveBreakpoint` at
    `task-page-content.tsx:325`).

## Verification Results

Pending. On completion, synchronize this section with each task's `## Results`:
record exact commands and outcomes/counts, generated artifact paths, and
cleanup/teardown evidence.

## Implementation Waves And Parallel Candidates

```
Wave 1 (sequential):
- [ ] [task-01-backend-settings-field](task-01-backend-settings-field.md)
- [ ] [task-02-backend-ensure-override](task-02-backend-ensure-override.md)
- [ ] [task-03-frontend-settings-plumbing](task-03-frontend-settings-plumbing.md)
- [ ] [task-04-settings-ui-card](task-04-settings-ui-card.md)
- [ ] [task-05-open-time-gates](task-05-open-time-gates.md)

Wave 2:
- [ ] [task-06-e2e](task-06-e2e.md)
```

All tasks are sequential: 01→02 share the backend settings surface, 03 depends
on 01's contract, 04 depends on 03's store field, 05 depends on 03's API
client change, and 06 depends on 05's behavior. No parallel-safe candidates.

## Open Questions

- Office advanced-mode `ensureExecution` resume stays a no-op (the WS handler
  drops `ensure_execution` today); the spec parks it out of scope until that
  flow is wired.
