import type { Message } from "@/lib/types/http";
import { kandevToolStemOf } from "@/components/task/chat/types";
import { hasKandevRendererStem } from "./registry-keys";

export function hasKandevRenderer(message: Message): boolean {
  return message.type === "tool_call" && hasKandevRendererStem(kandevToolStemOf(message));
}
