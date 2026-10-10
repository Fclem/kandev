import { describe, expect, it } from "vitest";
import { workflowId, workspaceId } from "@/lib/types/http";
import { toWorkflowStoreItem } from "./workflow-store-item";

const CREATED_AT = "2026-01-01T00:00:00Z";

describe("toWorkflowStoreItem", () => {
  it("keeps visibility and template identity together for hidden Improve workflows", () => {
    expect(
      toWorkflowStoreItem({
        id: workflowId("wf-improve"),
        workspace_id: workspaceId("ws-1"),
        name: "Improve",
        sort_order: 2,
        hidden: true,
        style: "kanban",
        workflow_template_id: "improve-kandev",
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      }),
    ).toMatchObject({
      id: "wf-improve",
      workspaceId: "ws-1",
      sortOrder: 2,
      hidden: true,
      style: "kanban",
      workflowTemplateId: "improve-kandev",
    });
  });

  it("defaults missing description and sort order", () => {
    expect(
      toWorkflowStoreItem({
        id: workflowId("wf-1"),
        workspace_id: workspaceId("ws-1"),
        name: "Default",
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      }),
    ).toMatchObject({ description: null, sortOrder: 0 });
  });
});
