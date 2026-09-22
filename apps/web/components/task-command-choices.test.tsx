import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginCommandChoices, useTaskMoveChoices } from "./task-command-choices";

afterEach(cleanup);

it("omits workflows without destinations while preserving both destination actions", () => {
  const openMoveOptions = vi.fn();
  const moveImmediately = vi.fn();
  const target = { id: "review", title: "Review", color: "bg-blue-500" };
  const { result } = renderHook(() =>
    useTaskMoveChoices({
      task: { id: "task", title: "Task", workflowId: "current" },
      workflows: [
        { id: "current", name: "Current" },
        { id: "empty", name: "Empty" },
        { id: "missing", name: "Missing" },
        { id: "target", name: "Target" },
      ],
      stepsByWorkflowId: { empty: [], target: [target] },
      openMoveOptions,
      moveImmediately,
    }),
  );
  expect(result.current.workflows.map((item) => item.label)).toEqual(["Target"]);
  const destination = result.current.workflows[0].children![0];
  destination.action?.();
  destination.immediateAction?.();
  expect(openMoveOptions).toHaveBeenCalledWith({ ...target, workflow_id: "target" });
  expect(moveImmediately).toHaveBeenCalledWith({ ...target, workflow_id: "target" });
});

/**
 * A plugin submenu carries no action of its own (its label is a trigger), so
 * the command palette has to reach its children -- dropping the entry would
 * silently remove a plugin's actions from the palette and the sidebar's task
 * commands (see task-commands.test.tsx's plugin parity assertions).
 */
describe("pluginCommandChoices", () => {
  const item = (label: string, key = label) => ({
    kind: "item" as const,
    key,
    label,
    onSelect: vi.fn(),
  });

  it("turns a flat plugin entry into one command", () => {
    const entry = { ...item("Add tag"), disabled: true, icon: "icon" };
    expect(pluginCommandChoices(entry, "Tasks")).toEqual([
      {
        id: "Add tag",
        label: "Add tag",
        group: "Tasks",
        action: entry.onSelect,
        disabled: true,
        icon: "icon",
      },
    ]);
  });

  it("flattens a plugin submenu into its item children, keeping their disabled flag", () => {
    const first = item("Blocked", "plugin-primary-tags-add-tag-more");
    const second = { ...item("Urgent"), disabled: true };
    const entry = {
      kind: "submenu" as const,
      key: "plugin-primary-tags-add-tag",
      label: "Add tag...",
      children: [first, second, { kind: "separator" as const, key: "sep" }],
    };

    expect(pluginCommandChoices(entry, "Tasks").map((command) => command.id)).toEqual([
      "plugin-primary-tags-add-tag-more::child",
      "Urgent::child",
    ]);
    const [more, urgent] = pluginCommandChoices(entry, "Tasks");
    more.action?.();
    expect(first.onSelect).toHaveBeenCalledTimes(1);
    expect(urgent.disabled).toBe(true);
    expect(more.context).toBe("Add tag...");
  });

  it("keeps a child's command id distinct from a sibling action's key", () => {
    // A child key is its parent's key plus its own id, joined by a dash, so a
    // plugin can name an action such that its key equals another action's child
    // key. A palette row's id is also cmdk's value, where a duplicate makes
    // filtering and selection ambiguous.
    const flat = {
      kind: "item" as const,
      key: "plugin-primary-p-quick-pick",
      label: "Quick pick",
      onSelect: vi.fn(),
    };
    const submenu = {
      kind: "submenu" as const,
      key: "plugin-primary-p-quick",
      label: "Quick",
      children: [{ ...flat, label: "Pick" }],
    };

    const ids = [
      ...pluginCommandChoices(submenu, "Tasks"),
      ...pluginCommandChoices(flat, "Tasks"),
    ].map((command) => command.id);

    expect(ids).toEqual(["plugin-primary-p-quick-pick::child", "plugin-primary-p-quick-pick"]);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("ignores a label the palette cannot render", () => {
    const entry = { ...item("x"), label: {} as never };
    expect(pluginCommandChoices(entry, "Tasks")).toEqual([]);
  });
});
