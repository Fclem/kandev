import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";

export type WorkspaceMoveFixture = Awaited<ReturnType<typeof seedWorkspaceMove>>;

/**
 * Two same-owner workspaces whose `Work` steps have identical saved settings,
 * plus a destination `Review` step whose prompt differs (an incompatible step).
 */
export async function seedWorkspaceMove(api: ApiClient, seed: SeedData, label: string) {
  const sourceWorkflow = await api.createWorkflow(seed.workspaceId, `${label} board`);
  const sourceStep = await api.createWorkflowStep(sourceWorkflow.id, "Work", 0);
  const destination = await api.createWorkspace(`${label} destination workspace`);
  const destinationWorkflow = await api.createWorkflow(destination.id, `${label} delivery`);
  const destinationStep = await api.createWorkflowStep(destinationWorkflow.id, "Work", 0);
  const incompatibleStep = await api.createWorkflowStep(destinationWorkflow.id, "Review", 1);
  await api.updateWorkflowStep(incompatibleStep.id, { prompt: "Different review instructions" });
  return {
    sourceWorkflow,
    sourceStep,
    destination,
    destinationWorkflow,
    destinationStep,
    incompatibleStep,
  };
}

/** A running mock turn that writes a marker before and waits on the gate file before finishing. */
export function runningTurnScript(label: string, gatePath: string) {
  return [
    `e2e:message("${label} before move")`,
    `e2e:await_file(${JSON.stringify(gatePath)})`,
    `e2e:message("${label} after move")`,
  ].join("\n");
}

/** Builds the exact transfer body from a canonical task read. */
export async function transferBody(
  api: ApiClient,
  taskId: string,
  destination: { workspaceId: string; workflowId: string; stepId: string },
  key: string,
) {
  const task = await api.getTask(taskId);
  return {
    expected_source_workspace_id: task.workspace_id ?? "",
    expected_source_workflow_id: task.workflow_id ?? "",
    expected_source_workflow_step_id: task.workflow_step_id ?? "",
    expected_task_updated_at: task.updated_at ?? "",
    destination_workspace_id: destination.workspaceId,
    destination_workflow_id: destination.workflowId,
    destination_workflow_step_id: destination.stepId,
    idempotency_key: key,
  };
}
