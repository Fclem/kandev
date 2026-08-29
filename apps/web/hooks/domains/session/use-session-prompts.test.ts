import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";

const listTaskSessionMessages = vi.hoisted(() => vi.fn());
const state = vi.hoisted(() => ({
  messagePrompts: {
    bySession: { session: [] as Message[] },
    metaBySession: {
      session: { isLoading: false, isLoadingMore: false, hasMore: false, oldestCursor: null },
    },
  },
  connection: { status: "connected" },
  setPromptMessagesLoading: vi.fn(),
  replacePromptMessages: vi.fn(),
}));

vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessionMessages }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => ({ getState: () => state }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    subscribeSessionWithReady: () => ({ ready: Promise.resolve(), unsubscribe: vi.fn() }),
  }),
}));
import { useSessionPrompts } from "./use-session-prompts";

describe("useSessionPrompts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false, cursor: null });
  });

  it("requests only user-authored prompt messages", async () => {
    renderHook(() => useSessionPrompts("session"));

    await waitFor(() => expect(listTaskSessionMessages).toHaveBeenCalledTimes(1));
    expect(listTaskSessionMessages).toHaveBeenCalledWith("session", {
      author_type: "user",
      limit: 20,
      sort: "desc",
    });
  });
});
