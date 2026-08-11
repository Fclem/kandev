---
id: "08-conditional-pin-sync"
title: "Conditional pin sync"
status: done
wave: 5
depends_on: ["07-frontend-settings-plumbing"]
plan: "plan.md"
spec: "../../specs/ui/agent-todo-list-panel.md"
---

# Task 08: Conditional pin sync

Gate the automatic Todos pin on the "Only pin when todo list is not empty"
preference: when it is on, the sync hook must not add the `todos` panel while
the active session's todo list is empty, using the same two-source todo
fallback the panel content uses (live `sessionTodos.bySessionId` first, then
persisted `todo` messages via `buildTodoItems`). The sub-option never removes
an existing panel and never affects manual adds.

- **Acceptance:**
  1. `resolveConditionalTodoPanelAction` returns `"none"` (not `"add"`) when
     `onlyPinWhenNotEmpty` is true and `todoListNotEmpty` is false and the
     panel is absent; `"add"` when the list is non-empty; and never
     `"remove"` as a result of the sub-option (removal stays gated solely on
     `showTodoListPanel`).
  2. `useSyncTodoPanel` subscribes to the active session's todo state (live
     slice + persisted messages) and re-runs the sync when it changes, so the
     panel appears as soon as todo entries arrive (live WS or hydrated
     history).
  3. Unit tests go red before implementation and green after;
     `pnpm run typecheck` passes.
- **Verification:** `cd apps/web && pnpm exec vitest run components/task/dockview-todo-panel-sync.test.ts && pnpm run typecheck`
  (fresh worktree bootstrap first if needed: `cd apps && pnpm install --frozen-lockfile`).
- **Files likely touched:**
  - `apps/web/components/task/dockview-todo-panel-sync.ts`
  - `apps/web/components/task/dockview-todo-panel-sync.test.ts`
- **Dependencies:** Task 07 (store field `showTodoListPanelOnlyWhenNotEmpty`
  exists in `UserSettingsState`).
- **Parallelism:** `sequential`.
- **Inputs:** Spec What / Scenarios sections
  (`docs/specs/ui/agent-todo-list-panel.md`); plan's "Conditional-pin sync"
  section; `apps/web/components/task/todos-panel-content.tsx` as the exact
  two-source fallback reference; `buildTodoItems`
  (`apps/web/hooks/use-processed-messages.ts`).

Implementation notes:

- `resolveConditionalTodoPanelAction` gains `onlyPinWhenNotEmpty` and
  `todoListNotEmpty` params; insert the new guard after the
  restoring/maximized guards and only in the add path:
  `if (params.onlyPinWhenNotEmpty && !params.todoListNotEmpty) return "none";`.
- `syncConditionalTodoPanel` options gain the same two params and pass them
  through.
- In `useSyncTodoPanel`, derive the active session's todo state:
  - live: `useAppStore((s) => (sessionId ? s.sessionTodos.bySessionId[sessionId] : undefined))`
  - persisted: `useAppStore((s) => (sessionId ? s.messages.bySession[sessionId] : EMPTY))`
    then `useMemo(() => buildTodoItems(messages), [messages])`
  - `todoListNotEmpty = (live?.length ?? 0) > 0 || messageTodos.length > 0`
  Read `live.userSettings.showTodoListPanelOnlyWhenNotEmpty` inside the
  effect (alongside the existing `showTodoListPanel` read), pass both new
  values into `syncConditionalTodoPanel`, and add `todoListNotEmpty` +
  `onlyPinWhenNotEmpty` to the effect dependency array.
- Do not mount `useSessionMessages` here — the desktop workbench's chat
  panel always fetches the session's messages; subscribing to the store
  slice is enough and avoids duplicating fetch/subscription side effects.
- Test additions: extend the `it.each` table with
  `{ showTodoListPanel: true, onlyPinWhenNotEmpty: true, todoListNotEmpty: false, panelExists: false }`
  → `"none"` and the non-empty variant → `"add"`; add a
  `syncConditionalTodoPanel` case asserting `addPanel` is not called with the
  sub-option on and an empty list; assert an existing panel is untouched
  (no `close`) in that case.

## Results

Red first: extended `resolveConditionalTodoPanelAction`'s `it.each` table
with 5 sub-option rows (empty list + sub on → `"none"`; non-empty → `"add"`;
existing panel untouched; sub off → unchanged `"add"`; master off still
`"remove"`) and added two `syncConditionalTodoPanel` cases (no addPanel with
empty list + sub on; addPanel with non-empty list). `pnpm exec vitest run
components/task/dockview-todo-panel-sync.test.ts` → 2 failed (the empty-list
suppression rows returned `"add"`/called `addPanel`).

Implemented in `components/task/dockview-todo-panel-sync.ts`:
- `resolveConditionalTodoPanelAction` gains `onlyPinWhenNotEmpty` +
  `todoListNotEmpty` params; new guard `if (params.onlyPinWhenNotEmpty &&
  !params.todoListNotEmpty) return "none";` after the restoring/maximized
  guards, in the add path only — never affects `"remove"` or an existing
  panel.
- `syncConditionalTodoPanel` options gain both params and pass them through.
- `useSyncTodoPanel` subscribes to the active session's todo state
  (`sessionTodos.bySessionId[sessionId]` live + `messages.bySession[sessionId]`
  persisted via `buildTodoItems`), computes
  `todoListNotEmpty = liveTodos.length > 0 || buildTodoItems(messages).length > 0`,
  reads `live.userSettings.showTodoListPanelOnlyWhenNotEmpty`, passes both
  into `syncConditionalTodoPanel`, and adds `onlyPinWhenNotEmpty` +
  `todoListNotEmpty` to the effect deps so the panel appears as soon as todo
  entries arrive. No `useSessionMessages` mount (chat panel fetches messages).
- Test `DEFAULT_OPTIONS` widened with `onlyPinWhenNotEmpty: false,
  todoListNotEmpty: false` so existing `syncConditionalTodoPanel` calls stay
  assignable.

Commands:
- `cd apps/web && pnpm exec vitest run components/task/dockview-todo-panel-sync.test.ts`
  → 23 passed (2 red → green).
- `cd apps/web && pnpm run typecheck` (with
  `NODE_OPTIONS=--max-old-space-size=4096`) → clean.

### Reviewer-feedback refinement (adversarial review, APPROVE qualified)

The DeepSeek V4 Pro adversarial review (sub-task
`4c855132-fc54-4e8c-924e-3a45bcda460b`) found 3 items; two were acted on:

- **N1 (minor, suspected)** — the inner RAF callback captured the
  render-time `todoListNotEmpty` memo; a WS todo event landing between render
  and rAF dispatch (~16ms) could make the pin decision one frame stale.
  Fixed by extracting `todoListNotEmptyForSession(liveTodos, messages)` (the
  panel's exact two-source fallback, exported so the contract is testable)
  and recomputing it inside the callback from the dispatch-time
  `appStore.getState()` snapshot. The render-time memo stays as the effect's
  change signal in the dep array. Added 4 unit tests pinning the helper
  (empty, live-wins, persisted-fallback, empty-live-array-falls-through).
- **N3 (nit, confirmed)** — dropped the now-unnecessary
  `as unknown as Partial<BackendMessageMap[...]>` cast in
  `lib/ws/handlers/users.test.ts` (the field is statically typed).
- **N2 (nit, confirmed)** — card-level `data-settings-dirty` on both toggles;
  fixed in the round-2 pass as F1 (per-field dirty flags, see Task 07
  results).
- **F2 (minor, suspected, round 2)** — the render-time `todoListNotEmpty`
  memo and the RAF-dispatch recompute each called `buildTodoItems` (two O(n)
  scans per effect cycle). Fixed by removing the memo entirely: the effect's
  change signal is now the raw `liveTodos`/`messages` slice identities in the
  dependency array (they only change when the underlying todo/message data
  actually changes), so the predicate is computed exactly once per sync — in
  the dispatch callback, from the dispatch-time snapshot. `useMemo` import
  dropped.

Post-fix verification: sync+WS unit tests 49 passed; `pnpm run typecheck`
clean; `make fmt` + `make lint` clean; E2E
`todo-list-panel.spec.ts` 8/8 passed (web rebuilt). Committed as follow-up
`fix:` on the feature branch.

Blockers/risks: none.
