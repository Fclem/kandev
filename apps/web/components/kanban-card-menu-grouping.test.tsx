import { afterEach, describe, expect, it, vi } from "vitest";
import { buildKanbanCardMenuEntries } from "./kanban-card-menu-builder";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";
import { pluginRegistry } from "@/lib/plugins/registry";

const PLUGIN_ID = "kandev-plugin-tags";
const ACTION_ID = "quick-tag";
const ACTION_LABEL = "Quick tag";
const CHANGE_WORKFLOW_ENTRY_KEY = "change-workflow";
const MOVE_GROUP_KEY = "move-to";
const WORKFLOW_ONE = "wf-1";
const WORKFLOW_TWO = "wf-2";

const PluginBitbucketIcon = () => null;

afterEach(() => {
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

function entryKeys(entries: KanbanCardMenuEntry[]) {
  return entries.map((entry) => entry.key);
}

function moveGroupChildren(entries: KanbanCardMenuEntry[]) {
  const group = entries.find((entry) => entry.key === MOVE_GROUP_KEY);
  return group?.kind === "submenu" ? group.children : [];
}

function registerPrimaryAction(visible?: () => boolean) {
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
    id: ACTION_ID,
    label: ACTION_LABEL,
    icon: PluginBitbucketIcon,
    group: "primary",
    visible,
    run: vi.fn(),
  });
}

function movementArgs() {
  return {
    currentWorkflowId: WORKFLOW_ONE,
    workflows: [
      { id: WORKFLOW_ONE, name: "Workflow 1" },
      { id: WORKFLOW_TWO, name: "Workflow 2" },
    ],
    stepsByWorkflowId: {
      [WORKFLOW_ONE]: [
        { id: "s1", title: "Step 1" },
        { id: "s2", title: "Step 2" },
      ],
      [WORKFLOW_TWO]: [{ id: "s3", title: "Step 3" }],
    },
  };
}

describe("buildKanbanCardMenuEntries — 'primary' group plugin actions", () => {
  it("renders a 'primary' group action after movement and before removal", () => {
    registerPrimaryAction();

    const entries = buildKanbanCardMenuEntries({
      ...movementArgs(),
      onChangeWorkflow: vi.fn(),
      onLinkPullRequest: vi.fn(),
    });

    const keys = entryKeys(entries);
    const moveIndex = keys.indexOf(MOVE_GROUP_KEY);
    const primaryIndex = keys.indexOf(`plugin-primary-${PLUGIN_ID}:${ACTION_ID}`);
    const linkIndex = keys.indexOf("link");

    expect(moveIndex).toBeGreaterThanOrEqual(0);
    expect(primaryIndex).toBeGreaterThanOrEqual(0);
    expect(linkIndex).toBeGreaterThanOrEqual(0);
    expect(linkIndex).toBeLessThan(moveIndex);
    expect(moveIndex).toBeLessThan(primaryIndex);

    const archiveIndex = keys.indexOf("archive");
    expect(primaryIndex).toBeLessThan(archiveIndex);

    const primaryEntry = entries[primaryIndex];
    expect(primaryEntry.kind).toBe("item");
    if (primaryEntry.kind === "item") {
      expect(primaryEntry.label).toBe(ACTION_LABEL);
      expect((primaryEntry.icon as { type?: unknown })?.type).toBe(PluginBitbucketIcon);
    }
  });

  it("keeps exactly one separator between each nonempty card group", () => {
    registerPrimaryAction();

    const entries = buildKanbanCardMenuEntries({
      ...movementArgs(),
      currentStepId: "s1",
      currentPriority: "high",
      onSelectPriority: vi.fn(),
      onEdit: vi.fn(),
      onLinkPullRequest: vi.fn(),
      parentTaskId: "parent-1",
      onDetach: vi.fn(),
      onMoveToStep: vi.fn(),
      onChangeWorkflow: vi.fn(),
      onArchive: vi.fn(),
      onDelete: vi.fn(),
    });

    expect(entryKeys(entries)).toEqual([
      "priority",
      "edit-separator",
      "edit",
      "relationships-separator",
      "link",
      "detach",
      "move-separator",
      MOVE_GROUP_KEY,
      "plugins-separator",
      `plugin-primary-${PLUGIN_ID}:${ACTION_ID}`,
      "remove-separator",
      "archive",
      "delete",
    ]);
  });

  it("does not add a 'primary' entry when visible(context) returns false", () => {
    registerPrimaryAction(() => false);

    const entries = buildKanbanCardMenuEntries({ workflows: [], stepsByWorkflowId: {} });

    expect(entryKeys(entries)).not.toContain(`plugin-primary-${PLUGIN_ID}:${ACTION_ID}`);
  });

  it("leaves the 'edit' group submenu unaffected by 'primary' group registrations", () => {
    registerPrimaryAction();

    const entries = buildKanbanCardMenuEntries({ workflows: [], stepsByWorkflowId: {} });
    const editMenu = entries.find((entry) => entry.key === "edit");

    expect(editMenu?.kind).toBe("item");
  });
});

describe("buildKanbanCardMenuEntries — single-task Move to group", () => {
  // @covers AC-TASKS-CROSS-WORKSPACE-TRANSFER-004.1
  it("groups step, workflow, and workspace moves for one task under Move to", () => {
    const onChangeWorkflow = vi.fn();
    const onChangeWorkspace = vi.fn();
    const entries = buildKanbanCardMenuEntries({
      ...movementArgs(),
      currentStepId: "s1",
      onMoveToStep: vi.fn(),
      onChangeWorkflow,
      onChangeWorkspace,
    });

    expect(entryKeys(entries)).not.toContain(CHANGE_WORKFLOW_ENTRY_KEY);
    const children = moveGroupChildren(entries);
    expect(entryKeys(children)).toEqual([
      "move-step",
      CHANGE_WORKFLOW_ENTRY_KEY,
      "change-workspace",
    ]);
    for (const child of children) if (child.kind === "item") child.onSelect?.();
    expect(onChangeWorkflow).toHaveBeenCalledOnce();
    expect(onChangeWorkspace).toHaveBeenCalledOnce();
  });

  it("keeps the workspace move for a one-step workflow with no other workflow", () => {
    const entries = buildKanbanCardMenuEntries({
      currentWorkflowId: WORKFLOW_ONE,
      workflows: [{ id: WORKFLOW_ONE, name: "Workflow 1" }],
      stepsByWorkflowId: { [WORKFLOW_ONE]: [{ id: "s1", title: "Step 1" }] },
      currentStepId: "s1",
      onMoveToStep: vi.fn(),
      onChangeWorkflow: vi.fn(),
      onChangeWorkspace: vi.fn(),
    });

    expect(entryKeys(moveGroupChildren(entries))).toEqual(["change-workspace"]);
  });

  it("keeps plural step and workflow moves and offers no workspace move for a selection", () => {
    const entries = buildKanbanCardMenuEntries({
      ...movementArgs(),
      isBulkSelection: true,
      onMoveToStep: vi.fn(),
      onSendToWorkflow: vi.fn(),
      onChangeWorkspace: vi.fn(),
    });

    expect(entryKeys(entries)).toEqual(
      expect.arrayContaining([MOVE_GROUP_KEY, "change-workflow-selection"]),
    );
    const steps = moveGroupChildren(entries);
    expect(entryKeys(steps)).toEqual(["step-s1", "step-s2"]);
    expect(JSON.stringify(entryKeys(entries))).not.toContain("change-workspace");
  });
});
