---
id: "01-backend-composite-semantics"
title: "Backend composite re-parent semantics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/subtask-reparenting-drag-drop.md"
---

# Task 01: Backend composite re-parent semantics

## Acceptance

- Non-empty reparenting delegates to the narrow `Store.SetTaskParent` aggregate
  command; it normalizes inherited mode as required by the reparenting contract,
  while preserving other modes and root tasks.
- Clearing the parent delegates to `Store.DetachTask`, including its conditional
  stewardship transfer and lifecycle outbox. Office uses the same command.
- All existing reparent/detach service and handler tests pass unchanged.

## Verification

```bash
cd apps/backend
go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard -run '^TestSetTaskParentModeBranch$' -count=1
go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard
```

## Files likely touched

- `apps/backend/internal/task/archivecascade/` (`SetTaskParent`, `DetachTask`)
- `apps/backend/internal/task/service/service_tasks.go` (command delegation)
- `apps/backend/internal/task/service/service_reparent_test.go` (new cases)
- `apps/backend/internal/office/dashboard/service_tasks.go` (command delegation)
- `apps/backend/internal/office/dashboard/service_detachment_test.go` (parity test)

## Dependencies

None.

## Inputs

- Spec sections: What (composite semantics), Data model, API surface, Failure modes.
- Existing pattern: `Store.SetTaskParent` / `Store.DetachTask` aggregate commands,
  `resolveParentID` / `validateReparentDepth` in `service_tasks.go`, and the
  lifecycle outbox publisher.
- Do NOT change `resolveParentID` / `validateReparentDepth`; validation behavior is already shipped and tested.

## Output contract

Report the service/repo changes, exact commands and results, files changed, blockers, residual risks; update this task and `plan.md` when acceptance passes.

## Results

- `cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard` — service and office/dashboard `ok`; handlers `ok` (run with `umask 022`; this sandbox's default umask 002 makes 3 pre-existing local-repository handler tests fail — they pass under standard umask and fail identically on the stash-clean base).
- Reparent tests cover `SetTaskParent` delegation and inherited-mode
  normalization; detach tests cover `Store.DetachTask` conditional ownership,
  publication, and non-inherited mode preservation.
- Office parity delegates empty-parent updates to `DetachTask` and non-empty
  updates to `SetTaskParent`; no repository method writes parent or metadata.
- Files changed: `internal/task/archivecascade`, task/Office service callers,
  and focused service/handler/dashboard tests.
