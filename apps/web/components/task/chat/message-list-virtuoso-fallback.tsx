"use client";

import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { MessageListFooter } from "./message-list-footer";
import { MessageListStatus } from "./message-list-shared";

export function VirtuosoMessageListFallback(props: {
  isLoadingMore: boolean;
  hasMore: boolean;
  showLoadingState: boolean;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  messages: Message[];
  loadMore: () => Promise<number>;
  sessionState?: TaskSessionState;
  sessionId: string | null;
  footerActions: Message[];
}) {
  return (
    <SessionPanelContent className="relative p-4 chat-message-list">
      <MessageListStatus
        isLoadingMore={props.isLoadingMore}
        hasMore={props.hasMore}
        showLoadingState={props.showLoadingState}
        messagesLoading={props.messagesLoading}
        isInitialLoading={props.isInitialLoading}
        messagesCount={props.messages.length}
        onLoadMore={props.loadMore}
      />
      <MessageListFooter
        sessionState={props.sessionState}
        sessionId={props.sessionId}
        messages={props.messages}
        footerActionMessages={props.footerActions}
      />
    </SessionPanelContent>
  );
}
