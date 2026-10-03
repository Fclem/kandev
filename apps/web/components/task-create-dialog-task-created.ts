import { createContext } from "react";
import type { Task } from "@/lib/types/http";

type CreatedTaskIdentity = Pick<Task, "id" | "workspace_id">;
export type TaskCreatedHandler = (task: CreatedTaskIdentity) => void;
export type RegisterTaskCreatedHandler = (handler: TaskCreatedHandler) => () => void;
export interface TaskCreatedHandlerRegistry {
  register: RegisterTaskCreatedHandler;
  notify(task: CreatedTaskIdentity): void;
}

export const TaskCreateDialogTaskCreatedContext = createContext<RegisterTaskCreatedHandler | null>(null);

export function createTaskCreatedHandlerRegistry(): TaskCreatedHandlerRegistry {
  const handlers = new Set<TaskCreatedHandler>();
  return {
    register(handler: TaskCreatedHandler) {
      handlers.add(handler);
      return () => handlers.delete(handler);
    },
    notify(task: CreatedTaskIdentity) {
      for (const handler of handlers) {
        try {
          handler(task);
        } catch (error) {
          console.error("[plugins] Task-create completion handler failed", error);
        }
      }
    },
  };
}

export function notifyTaskCreatedHandlers(
  registry: TaskCreatedHandlerRegistry,
  task: CreatedTaskIdentity,
  mode: "create" | "edit",
) {
  if (mode !== "create") return;
  registry.notify(task);
}
