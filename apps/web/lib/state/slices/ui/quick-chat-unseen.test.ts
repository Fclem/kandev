import { describe, expect, it } from "vitest";
import { selectQuickChatHasUnseenIdle } from "./quick-chat-unseen-selectors";
import { createAppStore } from "@/lib/state/store";
type QuickChatWithUnseenMarkers = {
  unseenIdleByWorkspace: Record<string, Record<string, true>>;
};

describe("quick chat unseen idle markers", () => {
  it("clears all markers when opening quick chat", () => {
    const store = createAppStore();
    store.setState({
      quickChat: {
        ...store.getState().quickChat,
        unseenIdleByWorkspace: { "workspace-1": { "session-1": true } },
      } as never,
    });

    store.getState().openQuickChat("session-1", "workspace-1");

    expect(
      (store.getState().quickChat as unknown as QuickChatWithUnseenMarkers).unseenIdleByWorkspace,
    ).toEqual({});
  });

  it("scopes markers to their workspace and clears an individual session", () => {
    const store = createAppStore();

    store.getState().markQuickChatUnseenIdle("session-a", "workspace-a");
    store.getState().markQuickChatUnseenIdle("session-b", "workspace-b");
    store.getState().clearQuickChatUnseenIdle("session-a", "workspace-a");

    expect(selectQuickChatHasUnseenIdle(store.getState(), "workspace-a")).toBe(false);
    expect(selectQuickChatHasUnseenIdle(store.getState(), "workspace-b")).toBe(true);
    expect(selectQuickChatHasUnseenIdle(store.getState(), null)).toBe(false);
  });
});
