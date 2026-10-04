const kandevRendererKeys = [
  "list_workspaces",
  "list_workflows",
  "list_workflow_steps",
  "list_tasks",
  "list_related_tasks",
  "list_agents",
  "list_executor_profiles",
  "list_task_documents",
  "create_task",
  "update_task",
  "move_task",
  "message_task",
  "get_task_plan",
  "create_task_plan",
  "update_task_plan",
  "delete_task_plan",
  "get_task_document",
  "write_task_document",
  "get_task_conversation",
  "ask_user_question",
  "show_walkthrough",
  "show_rich_output",
] as const;

export type KandevRendererKey = (typeof kandevRendererKeys)[number];

export function hasKandevRendererStem(stem: string | null): stem is KandevRendererKey {
  return stem !== null && (kandevRendererKeys as readonly string[]).includes(stem);
}
