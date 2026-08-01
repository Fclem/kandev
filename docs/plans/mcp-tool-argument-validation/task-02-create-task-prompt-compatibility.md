---
id: "02-create-task-prompt-compatibility"
title: "Add create-task prompt compatibility"
status: done
wave: 2
depends_on: ["01-enforce-registered-schemas"]
plan: "plan.md"
spec: "../../specs/integrations/mcp-tool-argument-validation.md"
---

# Task 02: Add create-task prompt compatibility

## Acceptance

- `create_task_kandev` accepts unadvertised `prompt` as `description` and forwards the text unchanged to the existing backend payload.
- Calls containing both `prompt` and `description`, or any other unknown key, return a tool error without backend dispatch.
- The registered create-task schema and description continue advertising only canonical `description`; existing valid `description` calls remain unchanged.

## Verification

Follow strict TDD, then run:

```bash
cd apps/backend && go test -run 'TestCreateTask_(PromptCompatibility|DescriptionCompatibility|RejectsConflictingContext|RejectsUnknownArguments)' ./internal/mcp/server
```

## Files likely touched

- `apps/backend/internal/mcp/server/tool_argument_validation.go`
- `apps/backend/internal/mcp/server/handlers_test.go`

## Dependencies

- Task 01 provides the normalization/validation boundary and compiled create-task schema.

## Parallelism

`sequential` — this extends the shared validation path established by Task 01.

## Inputs

- The create-task scenarios in the spec.
- Issue #2123 reproduction: `description` contained a short label while unknown `prompt` contained the real instructions.
- Existing `createTaskHandler` forwarding to backend `description`.

## Risks

- Normalize a copied argument map so the original request cannot be mutated across hooks or logs.
- Do not advertise the alias or weaken rejection for any other unknown key.

## Output contract

Report red/green evidence, files changed, exact test result, blockers or residual risks, and task/plan status updates in the primary conversation.
