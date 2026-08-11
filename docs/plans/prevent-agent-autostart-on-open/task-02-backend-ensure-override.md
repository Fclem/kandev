---
id: "02-backend-ensure-override"
title: "Backend session.ensure auto_start override"
status: pending
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/prevent-agent-autostart-on-open/spec.md"
---

# Task 02: Backend `session.ensure` `auto_start` override

## Acceptance

- `EnsureSessionOptions` gains `AutoStart *bool`; `EnsureSession` honors it:
  when `AutoStart` is `&false`, the session is created with
  `IntentPrepare` / source `created_prepare` even when the resolved workflow
  step has `auto_start_agent` in its on-enter actions. When `AutoStart` is nil
  or `&true`, the step-derived decision is unchanged.
- The WS `session.ensure` handler parses an optional `auto_start` field and
  passes it through as the option.
- No behavior change for existing callers that never send `auto_start`.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator/ -race -run 'TestEnsureSession|TestWsEnsureSession')
```

```bash
(cd apps/backend && go test ./internal/orchestrator/... ./internal/gateway/... -race)
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/session_ensure.go` (`EnsureSessionOptions` at `:26`, decision at `:87-94`)
- `apps/backend/internal/orchestrator/handlers/handlers.go` (`wsEnsureSessionRequest` at `:100`, handler at `:104`)
- `apps/backend/internal/orchestrator/session_ensure_test.go` (or a new focused test file)
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`

## Dependencies

None (backend-only; the frontend sends the field from task 03).

## Inputs

- Spec "API surface → WebSocket `session.ensure`".
- `stepAllowsAutoStart` (`session_ensure.go:268`) and the existing
  `session_ensure_test.go` / `session_ensure_office_test.go` seeding patterns
  (a step whose on-enter has `auto_start_agent` → control case starts;
  override case prepares).

## Output Contract

`EnsureSession` accepts an explicit start-vs-prepare override; the WS contract
gains the optional `auto_start` field without breaking existing clients. Tests
pin: override-false prepares, override-absent starts, handler passthrough.
