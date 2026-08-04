---
id: "02-frontend-nest-drop-infrastructure"
title: "Frontend nest-drop infrastructure"
status: pending
wave: 2
depends_on: ["01-backend-composite-semantics"]
plan: "plan.md"
spec: "../../specs/tasks/subtask-reparenting-drag-drop.md"
---

# Task 02: Frontend nest-drop infrastructure

## Acceptance

- A single `DndContext` per sidebar group section replaces the per-level contexts, so a drag can target rows outside its own sibling level; sibling reorder behavior is unchanged.
- `resolveSidebarDrop` (pure) and `computeNestTargets` (pure) exist with unit tests; `NestDropZone` renders on candidate rows with `data-testid="nest-drop-zone"` and `data-task-id`.
- Existing `TaskSwitcher` / `SortableTaskLevel` tests pass (no behavior regression when no group-level context or reorder is configured).

## Verification

```bash
cd apps/web && pnpm test -- components/task/task-switcher-subtask-dnd.test.ts components/task/task-switcher.test.tsx lib/sidebar/nest-candidates.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/task/task-switcher-subtask-dnd.tsx` — hoist `DndContext` out of `SortableTaskLevel` (new `externalDragContext` mode), add `NestDropZone`, `resolveSidebarDrop`, `computeNestTargets`, custom collision (`pointerWithin` → `nest:` ids, else `closestCenter`), sensors, group-level `onDragStart`/`onDragEnd`.
- `apps/web/components/task/task-switcher.tsx` — `GroupSection` renders the hoisted `DndContext`; `TaskTreeContext` carries candidate ids + `onReparentTask`; `TaskTreeNode` renders `NestDropZone` on candidate rows; new optional `onReparentTask` prop on `TaskSwitcher` (unwired for now).
- `apps/web/components/task/task-switcher-subtask-dnd.test.ts` (new) — `resolveSidebarDrop` unit tests: nest id mapping, group-root reorder, subtask-level reorder, cross-level drop → null, no-over → null.

## Dependencies

Task 01 (semantics only — infra works against the existing `updateTask` path regardless).

## Inputs

- Spec sections: What (gesture, targets, reorder preservation), Scenarios.
- Existing code: `useLevelDnd` / `SortableTaskLevel` / `DraggableSortableTaskNode` in `task-switcher-subtask-dnd.tsx`; `TaskTreeLevel`/`GroupSection`/`TaskTreeContext` in `task-switcher.tsx`; `computeNestCandidates` in `apps/web/lib/sidebar/nest-candidates.ts`; activation constraints constants.
- Reorder callbacks keep their signatures: `onReorderGroup(orderedTaskIds)`, `onReorderSubtasks(parentTaskId, orderedTaskIds)`.

## Output contract

Report the component/helper changes, exact commands and results, files changed, blockers, residual risks; update this task and `plan.md` when acceptance passes.
