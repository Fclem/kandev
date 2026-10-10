import type { WorkflowsState } from "@/lib/state/slices/kanban/types";
import type { Workflow } from "@/lib/types/http";

export type WorkflowStoreItem = WorkflowsState["items"][number];

/**
 * Maps a workflow API record to its store item. Every path that writes
 * `workflows.items` uses this so visibility (`hidden`, `style`) and template
 * identity always travel together; dropping either changes which lanes the
 * board shows.
 */
export function toWorkflowStoreItem(workflow: Workflow): WorkflowStoreItem {
  return {
    id: workflow.id,
    workspaceId: workflow.workspace_id,
    name: workflow.name,
    description: workflow.description ?? null,
    prompt: workflow.prompt,
    sortOrder: workflow.sort_order ?? 0,
    agent_profile_id: workflow.agent_profile_id,
    hidden: workflow.hidden,
    style: workflow.style,
    workflowTemplateId: workflow.workflow_template_id,
  };
}
