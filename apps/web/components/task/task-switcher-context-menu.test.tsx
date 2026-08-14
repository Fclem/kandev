import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskItemWithContextMenu } from "./task-switcher-context-menu";
import type { TaskSwitcherItem } from "./task-switcher-types";

afterEach(() => cleanup());

function task(overrides: Partial<TaskSwitcherItem> = {}): TaskSwitcherItem {
  return { id: "task-1", title: "Task 1", state: "IN_PROGRESS", ...overrides };
}

/**
 * Stands in for the dnd-kit drag handle that wraps the row. In the real tree
 * the handle's sensor listeners (`onMouseDown`, `onTouchStart`, `onPointerDown`)
 * are fiber ancestors of the context-menu portal, so a pointer-start event on a
 * menu item fiber-bubbles into them and starts a row drag — unless the menu
 * stops it. `onClick` stands in for the row's `onSelectTask` click handler.
 */
function renderWithDragHandle(overrides: Partial<TaskSwitcherItem> = {}) {
  const onMouseDown = vi.fn();
  const onPointerDown = vi.fn();
  const onClick = vi.fn();
  const onArchiveTask = vi.fn();
  render(
    <StateProvider>
      <ToastProvider>
        <div
          data-testid="drag-handle"
          onMouseDown={onMouseDown}
          onPointerDown={onPointerDown}
          onClick={onClick}
        >
          <TaskItemWithContextMenu task={task(overrides)} onArchiveTask={onArchiveTask}>
            <div data-testid="task-row">Task 1</div>
          </TaskItemWithContextMenu>
        </div>
      </ToastProvider>
    </StateProvider>,
  );
  return { onMouseDown, onPointerDown, onClick, onArchiveTask };
}

async function openContextMenu() {
  fireEvent.contextMenu(screen.getByTestId("task-row"));
  await screen.findByRole("menuitem", { name: /color/i });
}

describe("TaskItemWithContextMenu — pointer containment", () => {
  // Regression: the menu renders in a portal whose fiber ancestors include the
  // drag handle. Without a guard, mousedown/pointerdown on any menu item
  // bubbles through the React fiber tree to the handle's dnd-kit sensor
  // listeners and starts a row drag (MouseSensor ignores only right-click).
  it("mousedown and pointerdown on the Color submenu trigger do not reach the drag handle", async () => {
    const { onMouseDown, onPointerDown } = renderWithDragHandle();
    await openContextMenu();

    fireEvent.mouseDown(screen.getByRole("menuitem", { name: /color/i }));
    fireEvent.pointerDown(screen.getByRole("menuitem", { name: /color/i }));

    expect(onMouseDown).not.toHaveBeenCalled();
    expect(onPointerDown).not.toHaveBeenCalled();
  });

  it("mousedown and pointerdown on a color swatch do not reach the drag handle", async () => {
    const { onMouseDown, onPointerDown } = renderWithDragHandle();
    await openContextMenu();

    fireEvent.pointerMove(screen.getByRole("menuitem", { name: /color/i }), {
      pointerType: "mouse",
    });
    const redSwatch = await screen.findByRole("menuitem", { name: /red/i });
    fireEvent.mouseDown(redSwatch);
    fireEvent.pointerDown(redSwatch);

    expect(onMouseDown).not.toHaveBeenCalled();
    expect(onPointerDown).not.toHaveBeenCalled();
  });

  it("clicking a menu item runs its action without activating the row", async () => {
    const { onClick, onArchiveTask } = renderWithDragHandle();
    await openContextMenu();

    fireEvent.click(screen.getByRole("menuitem", { name: /archive/i }));

    expect(onArchiveTask).toHaveBeenCalledTimes(1);
    expect(onArchiveTask).toHaveBeenCalledWith("task-1");
    expect(onClick).not.toHaveBeenCalled();
  });
});
