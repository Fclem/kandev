---
id: "01-backend-settings-field"
title: "Backend user-settings field"
status: pending
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/prevent-agent-autostart-on-open/spec.md"
---

# Task 01: Backend user-settings field

## Acceptance

- `UserSettings` gains `PreventAutoStartAgentOnOpen bool` with json tag
  `prevent_auto_start_agent_on_open`; the value round-trips through
  `FromUserSettings`, `UpdateUserSettingsRequest`, the service apply path, and
  `publishUserSettingsEvent`.
- `PATCH /api/v1/user/settings` with `{"prevent_auto_start_agent_on_open": true}`
  persists the value; omitting the key leaves it unchanged (pointer semantics).
- The SSR boot payload exposes it as `preventAutoStartAgentOnOpen`.
- No DB migration: the settings blob already accepts new JSON fields.

## Verification

```bash
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/... -race)
```

```bash
(cd apps/backend && make lint)
```

## Files Likely Touched

- `apps/backend/internal/user/models/models.go` (`UserSettings` struct)
- `apps/backend/internal/user/dto/dto.go` (`UserSettingsDTO` + `FromUserSettings` at `:238`, `UpdateUserSettingsRequest` at `:105`)
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/service/service.go` (service `UpdateUserSettingsRequest` at `:52`, `applyTaskActionPreferences` at `:346`, `publishUserSettingsEvent` at `:773`)
- `apps/backend/internal/user/controller/controller.go` (`UpdateUserSettings` mapping at `:61`)
- `apps/backend/internal/backendapp/boot_state_routes.go` (boot-payload map at `:459`)

## Dependencies

None.

## Inputs

- Spec "Data model" and "API surface" sections.
- Existing pattern: `ConfirmTaskArchive` plumbing across the same five files.

## Output Contract

The field exists end to end on the backend: model → DTO → service apply →
controller → boot payload, with pointer-based PATCH semantics. Tests pin the
round-trip and the omitted-key behavior.
