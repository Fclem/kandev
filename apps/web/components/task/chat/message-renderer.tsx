"use client";

import { memo, useState, useCallback, useMemo, type ReactElement } from "react";
import { IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { sessionId as toSessionId, taskId as toTaskId } from "@/lib/types/http";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { launchSession } from "@/lib/services/session-launch-service";
import { isLaunchStateRegression } from "@/lib/session-state";
import { buildStartCreatedRequest } from "@/lib/services/session-launch-helpers";
import { useAppStore } from "@/components/state-provider";
import { useTask } from "@/hooks/use-task";
import { ChatMotionItem } from "./chat-motion";
import { ChatMessage } from "@/components/task/chat/messages/chat-message";
import { PermissionRequestMessage } from "@/components/task/chat/messages/permission-request-message";
import { StatusMessage } from "@/components/task/chat/messages/status-message";
import { ToolCallMessage } from "@/components/task/chat/messages/tool-call-message";
import { ToolEditMessage } from "@/components/task/chat/messages/tool-edit-message";
import { ToolReadMessage } from "@/components/task/chat/messages/tool-read-message";
import { ToolSearchMessage } from "@/components/task/chat/messages/tool-search-message";
import { ToolPayloadRemovedMessage } from "./messages/tool-payload-removed-message";
import { ToolExecuteMessage } from "@/components/task/chat/messages/tool-execute-message";
import { messageAdapterMatchers } from "./message-adapter-matchers";
import { ThinkingMessage } from "@/components/task/chat/messages/thinking-message";
import { TodoMessage } from "@/components/task/chat/messages/todo-message";
import { ScriptExecutionMessage } from "@/components/task/chat/messages/script-execution-message";
import { ClarificationRequestMessage } from "@/components/task/chat/messages/clarification-request-message";
import { useLateClarificationMessage } from "@/hooks/use-late-clarification-message";
import { ToolSubagentMessage } from "@/components/task/chat/messages/tool-subagent-message";
import { MonitorMessage } from "@/components/task/chat/messages/monitor-message";
import { AgentPlanMessage } from "@/components/task/chat/messages/agent-plan-message";
import { ActionMessage } from "@/components/task/chat/messages/action-message";
import { KandevToolMessage } from "@/components/task/chat/messages/kandev-tool-message";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { isDismissedGitPushErrorMessage } from "@/lib/utils/git-push-error-message";

type AdapterContext = {
  isTaskDescription: boolean;
  taskId?: string;
  permissionsByToolCallId?: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string, repo?: string) => void;
  onScrollToMessage?: (messageId: string) => void;
  isTurnActive?: boolean;
  isContainingTurnActive?: boolean;
};

/**
 * Decides whether the task-description message renders the Start agent
 * button. Shown only for never-started (CREATED) sessions; resume-skipped
 * (prevent-auto-start-on-open) sessions get the composer hint affordance
 * instead (which also covers empty sessions and empty descriptions). Hidden
 * while the task is SCHEDULING (the launch is in flight) and when no
 * task/session context is bound.
 */
export function shouldShowDescriptionStartButton({
  sessionState,
  taskState,
  taskId,
  sessionId,
}: {
  sessionState: TaskSessionState | undefined;
  taskState?: string;
  taskId?: string;
  sessionId?: string;
}): boolean {
  return sessionState === "CREATED" && taskState !== "SCHEDULING" && !!taskId && !!sessionId;
}

/**
 * "Start agent" button inside the task-description message, shown only for
 * never-started (CREATED) sessions. Dispatches the start_created launch and
 * hides while the workspace environment is being prepared.
 */
function TaskDescriptionStartButton({ taskId, sessionId }: { taskId: string; sessionId: string }) {
  const { t } = useTranslation();
  const [isStarting, setIsStarting] = useState(false);
  const prepareStatus = useAppStore(
    (state) => state.prepareProgress.bySessionId[sessionId]?.status ?? null,
  );
  const session = useAppStore((state) => state.taskSessions.items[sessionId] ?? null);
  const setTaskSession = useAppStore((state) => state.setTaskSession);

  const handleStart = useCallback(async () => {
    setIsStarting(true);
    try {
      // This button only renders for never-started (CREATED) sessions
      // (shouldShowDescriptionStartButton); resume-skipped (recovered-idle)
      // sessions get their affordance from the composer hint instead, so the
      // launch intent is always start_created.
      const { request } = buildStartCreatedRequest(taskId, sessionId);
      const response = await launchSession(request);
      if (response.success && response.state) {
        // Hydrate the launch state (commonly STARTING) so the button hides
        // immediately and repeated start_created requests cannot fire
        // against an already-starting session before the WS transition lands.
        // Never apply it over a newer live state: a WS RUNNING/FAILED
        // transition can land before the launch response resolves, and the
        // delayed STARTING must not hide a running agent or a failure's
        // recovery affordances.
        if (!isLaunchStateRegression(session?.state, response.state)) {
          setTaskSession({
            id: toSessionId(sessionId),
            task_id: toTaskId(taskId),
            state: response.state as TaskSessionState,
            started_at: session?.started_at ?? "",
            updated_at: session?.updated_at ?? "",
          });
        }
      }
    } catch (error) {
      console.error("Failed to start agent:", error);
    } finally {
      setIsStarting(false);
    }
  }, [taskId, sessionId, session, setTaskSession]);

  // Hide while environment is being prepared
  if (prepareStatus === "preparing") return null;

  return (
    <div className="flex justify-end mt-1.5">
      <Button
        size="sm"
        variant="default"
        className="cursor-pointer gap-1.5"
        onClick={handleStart}
        disabled={isStarting}
        data-testid="task-description-start-button"
      >
        <IconPlayerPlay className="h-3.5 w-3.5" />
        {isStarting ? t("task:startingEllipsis") : t("task:startAgent")}
      </Button>
    </div>
  );
}

function useSessionStateValue(sessionId?: string): TaskSessionState | undefined {
  return useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.state ?? undefined) : undefined,
  );
}

/**
 * The task-description message is the only row whose rendering depends on the
 * live session/task state, so it reads them from the store directly. Keeping
 * sessionState/taskState off the shared MessageRenderer props means a state
 * transition no longer re-renders every message in the list.
 */
function TaskDescriptionMessage({
  comment,
  taskId,
  sessionId,
  worktreePath,
  onOpenFile,
  onScrollToMessage,
  isTurnActive = false,
}: {
  comment: Message;
  taskId?: string;
  sessionId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  onScrollToMessage?: (messageId: string) => void;
  isTurnActive?: boolean;
}) {
  const { t } = useTranslation();
  const sessionState = useSessionStateValue(sessionId);
  const task = useTask(taskId ?? null);
  const renderAsUser = comment.author_type === "user" || sessionState !== "FAILED";
  if (!renderAsUser) {
    return (
      <ChatMessage
        comment={comment}
        label={t("task:agent")}
        className="bg-muted/40 text-foreground border-border/60"
        showRichBlocks={comment.type === "message" || comment.type === "content" || !comment.type}
        sessionId={sessionId}
        isTurnActive={isTurnActive}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        onScrollToMessage={onScrollToMessage}
      />
    );
  }
  const showStartButton = shouldShowDescriptionStartButton({
    sessionState,
    taskState: task?.state,
    taskId,
    sessionId,
  });
  return (
    <>
      <ChatMessage
        comment={comment}
        label={t("task:you")}
        className="bg-primary/10 text-foreground border-primary/30"
        sessionId={sessionId}
        isTurnActive={isTurnActive}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        onScrollToMessage={onScrollToMessage}
      />
      {showStartButton && <TaskDescriptionStartButton taskId={taskId!} sessionId={sessionId!} />}
    </>
  );
}

type MessageAdapter = {
  matches: (comment: Message, ctx: AdapterContext) => boolean;
  render: (comment: Message, ctx: AdapterContext) => ReactElement;
};

function ClarificationRequestMessageAdapter({
  comment,
  isCurrentTurn,
}: {
  comment: Message;
  isCurrentTurn: boolean;
}) {
  const lateAnswer = useLateClarificationMessage(comment);
  const sessionMessages = useAppStore(
    useCallback(
      (state) => state.messages.bySession[comment.session_id] ?? [],
      [comment.session_id],
    ),
  );
  const bundle = useMemo(() => {
    const pendingId = (comment.metadata as { pending_id?: string } | undefined)?.pending_id;
    if (!pendingId) return [comment];
    const matching = sessionMessages.filter(
      (message) =>
        message.type === "clarification_request" &&
        (message.metadata as { pending_id?: string } | undefined)?.pending_id === pendingId,
    );
    return matching.length > 0 ? matching : [comment];
  }, [comment, sessionMessages]);
  return (
    <ClarificationRequestMessage
      comment={comment}
      messages={bundle}
      onLateAnswer={lateAnswer.send}
      lateAnswerSnapshot={lateAnswer.state.snapshot}
      lateAnswerState={lateAnswer.state}
      onResetLateAnswer={lateAnswer.reset}
      isCurrentTurn={isCurrentTurn}
    />
  );
}

const adapterRenderers: MessageAdapter["render"][] = [
  (comment) => <ToolPayloadRemovedMessage comment={comment} />,
  (comment, ctx) => (
    <ThinkingMessage
      comment={comment}
      worktreePath={ctx.worktreePath}
      onOpenFile={ctx.onOpenFile}
    />
  ),
  (comment) => <TodoMessage comment={comment} />,
  (comment, ctx) => (
    <ToolEditMessage
      comment={comment}
      worktreePath={ctx.worktreePath}
      sessionId={ctx.sessionId}
      onOpenFile={ctx.onOpenFile}
    />
  ),
  (comment, ctx) => (
    <ToolReadMessage
      comment={comment}
      worktreePath={ctx.worktreePath}
      sessionId={ctx.sessionId}
      onOpenFile={ctx.onOpenFile}
    />
  ),
  (comment, ctx) => (
    <ToolSearchMessage
      comment={comment}
      worktreePath={ctx.worktreePath}
      onOpenFile={ctx.onOpenFile}
    />
  ),
  (comment, ctx) => <ToolExecuteMessage comment={comment} worktreePath={ctx.worktreePath} />,
  (comment) => <MonitorMessage comment={comment} />,
  (comment, ctx) => {
    const toolCallId = (comment.metadata as ToolCallMetadata | undefined)?.tool_call_id;
    const childMessages = toolCallId ? (ctx.childrenByParentToolCallId?.get(toolCallId) ?? []) : [];
    const renderChild = (child: Message) => {
      const childCtx = { ...ctx, childrenByParentToolCallId: undefined };
      const adapter =
        adapters.find((entry) => entry.matches(child, childCtx)) ?? adapters[adapters.length - 1];
      return adapter.render(child, childCtx);
    };
    return (
      <ToolSubagentMessage
        comment={comment}
        childMessages={childMessages}
        isContainingTurnActive={ctx.isContainingTurnActive}
        worktreePath={ctx.worktreePath}
        onOpenFile={ctx.onOpenFile}
        renderChild={renderChild}
      />
    );
  },
  (comment, ctx) => {
    const toolCallId = (comment.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
    const permissionMessage = toolCallId ? ctx.permissionsByToolCallId?.get(toolCallId) : undefined;
    return (
      <KandevToolMessage
        comment={comment}
        permissionMessage={permissionMessage}
        sessionId={ctx.sessionId}
        onOpenFile={ctx.onOpenFile}
      />
    );
  },
  (comment, ctx) => {
    const toolCallId = (comment.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
    const permissionMessage = toolCallId ? ctx.permissionsByToolCallId?.get(toolCallId) : undefined;
    return (
      <ToolCallMessage
        comment={comment}
        permissionMessage={permissionMessage}
        worktreePath={ctx.worktreePath}
      />
    );
  },
  (comment) => <ActionMessage comment={comment} />,
  (comment) => <StatusMessage comment={comment} />,
  (comment) => <PermissionRequestMessage comment={comment} />,
  (comment, ctx) => (
    <ClarificationRequestMessageAdapter
      comment={comment}
      isCurrentTurn={Boolean(ctx.isContainingTurnActive)}
    />
  ),
  (comment, ctx) => (
    <AgentPlanMessage
      comment={comment}
      worktreePath={ctx.worktreePath}
      onOpenFile={ctx.onOpenFile}
    />
  ),
  (comment) => <ScriptExecutionMessage comment={comment} />,
  (comment, ctx) => {
    if (ctx.isTaskDescription) {
      return (
        <TaskDescriptionMessage
          comment={comment}
          taskId={ctx.taskId}
          sessionId={ctx.sessionId}
          worktreePath={ctx.worktreePath}
          onOpenFile={ctx.onOpenFile}
          onScrollToMessage={ctx.onScrollToMessage}
          isTurnActive={ctx.isTurnActive}
        />
      );
    }
    if (comment.author_type === "user") {
      return (
        <ChatMessage
          comment={comment}
          label={t("task:you")}
          className="bg-primary/10 text-foreground border-primary/30"
          sessionId={ctx.sessionId}
          worktreePath={ctx.worktreePath}
          onOpenFile={ctx.onOpenFile}
          onScrollToMessage={ctx.onScrollToMessage}
          isTurnActive={ctx.isTurnActive}
        />
      );
    }
    return (
      <ChatMessage
        comment={comment}
        label={t("task:agent")}
        className="bg-muted/40 text-foreground border-border/60"
        showRichBlocks={comment.type === "message" || comment.type === "content" || !comment.type}
        sessionId={ctx.sessionId}
        worktreePath={ctx.worktreePath}
        onOpenFile={ctx.onOpenFile}
        onScrollToMessage={ctx.onScrollToMessage}
        isTurnActive={ctx.isTurnActive}
      />
    );
  },
];

const adapters: MessageAdapter[] = messageAdapterMatchers.map((matcher, index) => {
  const render = adapterRenderers[index];
  if (!render) throw new Error("Message adapter renderer order mismatch");
  return { matches: matcher.matches, render };
});

type MessageRendererProps = {
  comment: Message;
  isTaskDescription: boolean;
  taskId?: string;
  permissionsByToolCallId?: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string, repo?: string) => void;
  onScrollToMessage?: (messageId: string) => void;
  isTurnActive?: boolean;
  isContainingTurnActive?: boolean;
};

export const MessageRenderer = memo(function MessageRenderer({
  comment,
  isTaskDescription,
  taskId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  worktreePath,
  sessionId,
  onOpenFile,
  onScrollToMessage,
  isTurnActive = false,
  isContainingTurnActive = false,
}: MessageRendererProps) {
  if (isDismissedGitPushErrorMessage(comment)) return null;

  const ctx = {
    isTaskDescription,
    taskId,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    worktreePath,
    sessionId,
    onOpenFile,
    onScrollToMessage,
    isTurnActive,
    isContainingTurnActive,
  };
  const adapter =
    adapters.find((entry) => entry.matches(comment, ctx)) ?? adapters[adapters.length - 1];
  const content = adapter.render(comment, ctx);
  return isTaskDescription ? (
    content
  ) : (
    <ChatMotionItem messageId={comment.id}>{content}</ChatMotionItem>
  );
});
