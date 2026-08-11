"use client";

import type { Message, TaskSessionState } from "@/lib/types/http";
import { AgentStatus } from "@/components/task/chat/messages/agent-status";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { SessionResumeStartButton } from "@/components/task/chat/session-resume-start-button";
import { useAppStore } from "@/components/state-provider";

type MessageListFooterProps = {
  sessionState?: TaskSessionState;
  sessionId: string | null;
  messages: Message[];
  isWorking?: boolean;
  footerActionMessages?: Message[];
};

function isMissingBranchFailure(message: Message): boolean {
  const metadata = message.metadata as Record<string, unknown> | undefined;
  const actions = metadata?.actions;
  if (!Array.isArray(actions) || actions.length === 0) return false;
  return metadata?.failure_kind === "missing_pr_branch";
}

function isActionableFailure(message: Message): boolean {
  const metadata = message.metadata as Record<string, unknown> | undefined;
  return Array.isArray(metadata?.actions) && metadata.actions.length > 0;
}

function findCurrentActionableFailure(
  messages: Message[],
  footerActionMessages: Message[],
): Message | undefined {
  for (let index = messages.length - 1; index >= 0; index--) {
    if (isActionableFailure(messages[index])) return messages[index];
  }
  return footerActionMessages.at(-1);
}

export function MessageListFooter({
  sessionState,
  sessionId,
  messages,
  isWorking,
  footerActionMessages = [],
}: MessageListFooterProps) {
  const currentActionableFailure = findCurrentActionableFailure(messages, footerActionMessages);
  const recoveryOwnsFailure =
    sessionState === "FAILED" &&
    currentActionableFailure !== undefined &&
    isMissingBranchFailure(currentActionableFailure);
  const visibleFooterActionMessages = footerActionMessages.filter(
    (message) => !isMissingBranchFailure(message) || message.id === currentActionableFailure?.id,
  );
  // The task-description start button only renders for EMPTY sessions. A
  // resume-skipped session (prevent-auto-start-on-open preference, agent
  // stopped with history) gets its Start agent button here instead — but only
  // for non-FAILED states, where the existing recovery actions already provide
  // the manual affordance.
  const resumeSkipped = useAppStore((state) =>
    sessionId ? state.tasks.resumeSkippedSessionIds[sessionId] === true : false,
  );
  const showResumeStartButton =
    resumeSkipped && sessionState !== "FAILED" && sessionId !== null && messages.length > 0;
  return (
    <>
      {!recoveryOwnsFailure && (
        <AgentStatus
          sessionState={sessionState}
          sessionId={sessionId}
          messages={messages}
          isWorking={isWorking}
        />
      )}
      {showResumeStartButton && sessionId !== null && (
        <SessionResumeStartButton sessionId={sessionId} />
      )}
      {visibleFooterActionMessages.map((message) => (
        <MessageRenderer key={message.id} comment={message} isTaskDescription={false} />
      ))}
    </>
  );
}
