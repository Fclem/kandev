---
id: "05-e2e-desktop-spec"
title: "Desktop E2E coverage"
status: pending
wave: 5
depends_on: ["04-integration-lifecycle"]
plan: "plan.md"
spec: "../../specs/ui/panel-pin-float.md"
---

# Task 05: Desktop E2E coverage

## Acceptance

- New Playwright spec `apps/web/e2e/tests/task/panel-pin.spec.ts` proves on a real task workbench:
  - unpin floats the group over the view with the freed space reflowed; outside click collapses to a **vertical** edge bar for a right-column group and a **horizontal** bottom bar for a center/bottom group;
  - a Plan group floating from the `plan` preset (generated group id in a root column right of center) collapses to a **vertical** right-edge bar, not horizontal;
  - clicking a bar title expands the window with that tab active; clicking the pin re-docks the group with tabs/order restored (including the last-group-in-column case);
  - a running terminal stays alive across float → collapse → expand → dock (no reconnect) — asserted against backend terminal state via causal-wait helpers, not DOM budgets;
  - **reload** while floating recreates the group floating with the same tabs/active/display state;
  - **maximize → float** of the maximized group restores the grid then floats at the pre-max placement;
  - **task switch away and back** with a floated chat tab keeps it floating and tracking the incoming session;
  - **keyboard collapse**: tabbing out of the expanded window collapses it without a pointer event; Escape collapses only when focus is inside the window and no descendant handled it;
  - **portaled-menu suppression**: opening a Radix DropdownMenu inside a floating window and pressing Escape closes the menu without collapsing the window; clicking outside while the menu is open keeps the window expanded until the menu closes;
  - **two groups on the same edge** stack without overlap and each bar's titles remain clickable;
  - **reset-merge** while groups float: reset-default panels (chat/terminal) are reused by id, floating tabs merge preserving order/active, no duplicate ids, floating storage cleared, groups do not re-float;
  - **right-groups-floated width**: with all right groups floated, a container resize does not resize the center column to the right target;
  - **toggle-right-panels with floated right groups**: hide→show does not re-insert floating ids (no duplicate across surfaces);
  - **portaled-menu pointerdown**: clicking outside while a menu inside the floating window is open keeps the window expanded until the menu closes (pending collapse), and a second outside click collapses it;
  - **unload mid-transaction**: reloading while a float transaction is in flight never persists a partial layout (the journaled pre-transaction layout is written on unload; the group either floats or stays pinned consistently);
  - **two-key divergence**: blob/layout split after a simulated crash restores consistently (no duplicate id, no lost panel);
  - **rollback portal safety**: forcing a mid-transaction failure does not release a terminal portal or stop its process.
- The spec uses the repo's causal-wait helpers (`e2e/helpers/causal-waits.ts`) and the `SessionPage` page object additions (`clickMaximize` style helpers for `dockview-pin-btn`); no new sleeps.
- Mobile: the dockview workbench does not render on phone viewports — the mobile task surface (`mobile-task-layout` + `session-mobile-bottom-nav`, exercised by existing mobile E2E specs) keeps Plan/Changes/Files/Terminal reachable, and floating/collapse is intentionally absent from the mobile state model. The retained-path mobile test is the existing mobile bottom-nav panel-access coverage; no new mobile E2E is added for this surface (documented in the spec).

## Verification

```bash
cd apps/web && pnpm e2e:raw tests/task/panel-pin.spec.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/e2e/tests/task/panel-pin.spec.ts` (new)
- `apps/web/e2e/pages/session-page.ts` (pin/floating helpers)
- `apps/web/e2e/helpers/dockview-persistence.ts` (floating-blob assertions)

## Inputs

- Spec: Scenarios (all), Persistence guarantees, Restore call sites, Focus ownership, Escape contract, Out of scope (mobile).
- `apps/web/e2e/README.md` and `/e2e` skill for fixtures and commands; existing `clickMaximize`/`expectMaximized` helpers in `e2e/pages/session-page.ts` as the shape to mirror.

## Dependencies and Risks

- Depends on task-04.
- Risk: E2E runs against the production Vite build served by the Go backend (`make test-e2e` rebuilds both); do not run against a dev server.
- Risk: terminal-liveness, reload-recreation, and portaled-menu assertions depend on backend state transitions and real Radix behavior — use `watchWs`/`waitForHttp` causal helpers and `expect.poll` against the API client, never hand-picked timeouts; the Escape test must use a real menu, not a synthetic keydown.

## Output Contract

Report the spec, helpers added, targeted E2E result, blockers, and update this task plus `plan.md` to done.
