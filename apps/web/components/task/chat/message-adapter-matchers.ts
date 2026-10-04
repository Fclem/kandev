import type { Message } from "@/lib/types/http";
import type { ToolCallMetadata } from "./types";
import { payloadRetentionMarker } from "@/lib/utils/tool-payload-retention";
import { hasKandevRenderer } from "./messages/kandev/has-renderer";

type MessageAdapterContext = {
  isTaskDescription?: boolean;
  childrenByParentToolCallId?: Map<string, Message[]>;
};

export type MessageAdapterMatcher = {
  matches: (message: Message, context: MessageAdapterContext) => boolean;
  rendersActions: (message: Message, context: MessageAdapterContext) => boolean;
};

const neverRendersActions = () => false;

export const messageAdapterMatchers: readonly MessageAdapterMatcher[] = [
  {
    matches: (message) =>
      message.type !== "tool_execute" && Boolean(payloadRetentionMarker(message.metadata)),
    rendersActions: neverRendersActions,
  },
  { matches: (message) => message.type === "thinking", rendersActions: neverRendersActions },
  { matches: (message) => message.type === "todo", rendersActions: neverRendersActions },
  { matches: (message) => message.type === "tool_edit", rendersActions: neverRendersActions },
  { matches: (message) => message.type === "tool_read", rendersActions: neverRendersActions },
  { matches: (message) => message.type === "tool_search", rendersActions: neverRendersActions },
  { matches: (message) => message.type === "tool_execute", rendersActions: neverRendersActions },
  {
    matches: (message) => {
      if (message.type !== "tool_call") return false;
      const metadata = message.metadata as ToolCallMetadata | undefined;
      const output = metadata?.normalized?.generic?.output as { monitor?: unknown } | undefined;
      return !!output && typeof output === "object" && !!output.monitor;
    },
    rendersActions: neverRendersActions,
  },
  {
    matches: (message, context) => {
      if (message.type !== "tool_call") return false;
      const metadata = message.metadata as ToolCallMetadata | undefined;
      const toolCallId = metadata?.tool_call_id;
      return (
        metadata?.normalized?.kind === "subagent_task" ||
        (!!toolCallId && (context.childrenByParentToolCallId?.has(toolCallId) ?? false))
      );
    },
    rendersActions: neverRendersActions,
  },
  {
    matches: (message) => hasKandevRenderer(message),
    rendersActions: neverRendersActions,
  },
  { matches: (message) => message.type === "tool_call", rendersActions: neverRendersActions },
  {
    matches: (message) => {
      const metadata = message.metadata as Record<string, unknown> | undefined;
      const hasActions = Array.isArray(metadata?.actions) && metadata.actions.length > 0;
      const isRetainedTurnFailure =
        metadata?.variant === "error" &&
        metadata.failure_scope === "turn" &&
        metadata.runtime_retained === true;
      return hasActions || isRetainedTurnFailure;
    },
    rendersActions: neverRendersActions,
  },
  {
    matches: (message) =>
      message.type === "error" || message.type === "status" || message.type === "progress",
    rendersActions: neverRendersActions,
  },
  {
    matches: (message) => message.type === "permission_request",
    rendersActions: neverRendersActions,
  },
  {
    matches: (message) => message.type === "clarification_request",
    rendersActions: neverRendersActions,
  },
  { matches: (message) => message.type === "agent_plan", rendersActions: neverRendersActions },
  {
    matches: (message) => message.type === "script_execution",
    rendersActions: neverRendersActions,
  },
  {
    matches: () => true,
    rendersActions: (message, context) =>
      !context.isTaskDescription && message.author_type === "agent",
  },
];

export function messageAdapterRendersActions(
  message: Message,
  context: MessageAdapterContext = {},
): boolean {
  const adapter = messageAdapterMatchers.find((candidate) => candidate.matches(message, context));
  return adapter?.rendersActions(message, context) ?? false;
}
