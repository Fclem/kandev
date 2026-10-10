import type { MessageAction } from "@/components/task/chat/types";
import { ActionButtons } from "./action-message-actions";

export function RunningActionNotice({
  actions,
  message,
  taskId,
}: {
  actions?: MessageAction[];
  message: string;
  taskId?: string;
}) {
  return (
    <div
      data-testid="running-action-notice"
      className="flex min-w-0 items-center gap-2 py-1 text-muted-foreground"
    >
      <span className="min-w-0 flex-1 truncate text-xs">{message}</span>
      {actions && actions.length > 0 && <ActionButtons actions={actions} taskId={taskId} compact />}
    </div>
  );
}
