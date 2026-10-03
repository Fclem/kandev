import { describe, expect, it, vi } from "vitest";
import { createTaskCreatedHandlerRegistry, notifyTaskCreatedHandlers } from "./task-create-dialog-task-created";

describe("task-create completion handlers", () => {
  it("routes only successful create-mode tasks to handlers registered by that dialog", () => {
    const firstDialog = createTaskCreatedHandlerRegistry();
    const secondDialog = createTaskCreatedHandlerRegistry();
    const firstHandler = vi.fn();
    const secondHandler = vi.fn();
    firstDialog.register(firstHandler);
    secondDialog.register(secondHandler);
    const task = { id: "task-1", workspace_id: "workspace-1" };

    notifyTaskCreatedHandlers(firstDialog, task, "edit");
    expect(firstHandler).not.toHaveBeenCalled();
    expect(secondHandler).not.toHaveBeenCalled();

    notifyTaskCreatedHandlers(firstDialog, task, "create");
    expect(firstHandler).toHaveBeenCalledExactlyOnceWith(task);
    expect(secondHandler).not.toHaveBeenCalled();
  });
});
