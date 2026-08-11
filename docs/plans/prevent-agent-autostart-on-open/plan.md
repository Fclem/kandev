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
- `apps/web/lib/settings-discovery/catalog/general.ts` — new target
  `preventAutoStartOnOpen: "setting-prevent-auto-start-on-open"` plus a control
  definition under the task-actions page.
- i18n — add `preventAutoStartAgentOnOpen` and `preventAutoStartAgentOnOpenHelp`
  to `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json` (help text
  must use plain punctuation, no em dash).

### 3. Open-time gates

- `apps/web/hooks/domains/session/use-ensure-task-session.ts` —
  - Extend `EnsureTaskInput` with `workflowStepId?: string | null`.
  - Read `state.userSettings.preventAutoStartAgentOnOpen` and
    `state.kanban.steps`.
  - When the setting is on and the task's step is the final step of its
    workflow (max `position` among `state.kanban.steps`), call
    `ensureTaskSession(taskId, { autoStart: false })`; otherwise
    `ensureTaskSession(taskId)` as today.
  - New pure helper (exported for unit tests), e.g.
    `isFinalWorkflowStep(workflowStepId, steps)` in the same file or
    `lib/tasks/` — treat missing step/steps as "not final".
- `apps/web/hooks/domains/session/use-session-resumption.ts` —
  - Read `state.userSettings.preventAutoStartAgentOnOpen` in
    `useSessionResumption` and thread a `preventAutoStart` boolean into
    `useSessionResetAndCheck` → `checkAndResume`.
  - In `checkAndResume`, when `preventAutoStart` is true, skip the
    `status.needs_resume && status.is_resumable` branch (do not call
    `resumeWithSilentFallback`); set `resumptionState` to `"idle"` instead.
  - The manual `resumeSession()` action is NOT gated.
- Callers pass the richer task object already available:
  `components/task/task-page-content.tsx` and
  `components/kanban-with-preview.tsx` both pass kanban tasks that already
  carry `workflowStepId` — no signature change beyond the hook's input type.

---

## Tests

| Behavior (spec scenario) | File | How |
|---|---|---|
| PATCH accepts and persists the setting; GET/boot payload returns it | `apps/backend/internal/user/dto/dto_test.go`, `internal/user/service/service_test.go` (or existing settings-update test) | unit: round-trip `UpdateUserSettingsRequest` → model → DTO; assert blob round-trip |
| `session.ensure` with `auto_start: false` prepares instead of starts | `apps/backend/internal/orchestrator/session_ensure_test.go` (or `session_ensure_office_test.go` pattern) | integration: seed task + step with `auto_start_agent`; call `EnsureSession` with `AutoStart: &false`; assert `Source == "created_prepare"`, `State == "CREATED"`; control case without override still `created_start` |
| WS handler passes `auto_start` through | `apps/backend/internal/orchestrator/handlers/handlers_test.go` | unit: parse `{task_id, auto_start:false}` → handler calls service with the option |
| SSR defaults and hydration | `apps/web/lib/ssr/user-settings.test.ts` | unit: default `false`; hydrate from `prevent_auto_start_agent_on_open` |
| ensure payload carries `auto_start` | extend `apps/web/hooks/domains/session/use-ensure-task-session.test.ts` (mocks `ensureTaskSession`) | unit: final-step + setting-on → called with `{ autoStart: false }`; non-final → without |
| final-step helper | new test beside the helper | unit: max-position logic, missing step/steps → false |
| resume gate skips auto-resume | `apps/web/hooks/domains/session/use-session-resumption.test.ts` | unit: `checkAndResume` with `needs_resume && is_resumable` and preventAutoStart → no launch, state idle; manual `resumeSession` still launches |
| settings card renders and saves | `apps/web/components/settings/prevent-auto-start-agent-settings.test.tsx` (mirror `archive-confirmation-settings.test.tsx`) | component: switch toggles, save calls `updateUserSettings({ prevent_auto_start_agent_on_open })` |
| i18n ratchet + em-dash check | — | `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet` |

## E2E Tests

- **Scenario:** GIVEN setting ON and a task in the final workflow step with no
  session, WHEN the task page is opened, THEN the Start agent button
  (`[data-testid="task-description-start-button"]`) is visible and the agent
  never starts on its own.
- **File:** `apps/web/e2e/tests/settings/prevent-auto-start-on-open.spec.ts`
- **What to verify:** set the setting via `apiClient.updateUserSettings` (or
  the settings UI), create a task in a workflow whose final step has
  `auto_start_agent` (a custom workflow created through the API with
  `on_enter: [auto_start_agent]` on its last step), open `/t/:id`, assert the
  Start agent button appears, and (optionally) that clicking it starts the
  agent. A second case: seed a session in the recovered-idle shape (launch →
  stop backend-simulated restart via the existing recovery helpers) and assert
  no resume `session.launch` fires and the manual affordance is present.

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
