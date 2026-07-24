import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  listTaskSessionMessages: vi.fn(),
  prependMessages: vi.fn(),
  setMessagesMetadata: vi.fn(),
  state: {
    messages: {
      metaBySession: {
        "session-1": { hasMore: true, oldestCursor: "cursor-1", isLoading: false },
        "session-2": { hasMore: true, oldestCursor: "cursor-2", isLoading: false },
      },
    },
  },
}));

vi.mock("@/lib/api", () => ({
  listTaskSessionMessages: mocks.listTaskSessionMessages,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      ...mocks.state,
      prependMessages: mocks.prependMessages,
      setMessagesMetadata: mocks.setMessagesMetadata,
    }),
}));

import { useLazyLoadMessages } from "./use-lazy-load-messages";

beforeEach(() => {
  mocks.state.messages.metaBySession["session-1"].isLoading = false;
  mocks.state.messages.metaBySession["session-2"].isLoading = false;
  mocks.listTaskSessionMessages.mockReset();
  mocks.prependMessages.mockReset();
  mocks.setMessagesMetadata.mockReset();
});

describe("useLazyLoadMessages", () => {
  it("shares an in-flight page request between hook instances for the same session", async () => {
    let resolvePage!: (value: { messages: unknown[]; has_more: boolean }) => void;
    mocks.listTaskSessionMessages.mockReturnValue(
      new Promise((resolve) => {
        resolvePage = resolve;
      }),
    );
    const { result } = renderHook(() => ({
      automatic: useLazyLoadMessages("session-1"),
      navigation: useLazyLoadMessages("session-1"),
    }));

    let automaticLoad!: Promise<number>;
    let navigationLoad!: Promise<number>;
    act(() => {
      automaticLoad = result.current.automatic.loadMore();
      navigationLoad = result.current.navigation.loadMore();
    });

    expect(mocks.listTaskSessionMessages).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolvePage({
        messages: [
          {
            id: "older-1",
            created_at: "2026-07-21T00:00:00Z",
            author_type: "agent",
            type: "message",
          },
        ],
        has_more: true,
      });
    });

    await expect(automaticLoad).resolves.toBe(1);
    await expect(navigationLoad).resolves.toBe(1);
    expect(mocks.prependMessages).toHaveBeenCalledTimes(1);
  });

  it("does not let a stale store loading flag block a cursor-backed page request", async () => {
    mocks.state.messages.metaBySession["session-1"].isLoading = true;
    mocks.listTaskSessionMessages.mockResolvedValue({ messages: [], has_more: false });
    const { result } = renderHook(() => useLazyLoadMessages("session-1"));

    await act(() => result.current.loadMore());

    expect(mocks.listTaskSessionMessages).toHaveBeenCalledTimes(1);
    mocks.state.messages.metaBySession["session-1"].isLoading = false;
  });
});

describe("useLazyLoadMessages session changes", () => {
  it("starts a new-session request and preserves its cursor after the prior request resolves", async () => {
    let resolveFirstPage!: (value: { messages: unknown[]; has_more: boolean }) => void;
    let resolveSecondPage!: (value: { messages: unknown[]; has_more: boolean }) => void;
    mocks.listTaskSessionMessages
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirstPage = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveSecondPage = resolve;
        }),
      );
    const { result, rerender } = renderHook(({ sessionId }) => useLazyLoadMessages(sessionId), {
      initialProps: { sessionId: "session-1" },
    });

    act(() => {
      void result.current.loadMore();
    });
    rerender({ sessionId: "session-2" });
    await act(async () => {
      resolveFirstPage({ messages: [], has_more: false });
    });

    act(() => {
      void result.current.loadMore();
    });
    expect(mocks.listTaskSessionMessages).toHaveBeenNthCalledWith(
      2,
      "session-2",
      expect.objectContaining({ before: "cursor-2" }),
    );

    await act(async () => {
      resolveSecondPage({ messages: [], has_more: false });
    });
  });

  it("retries the requested cursor after an empty page that reports more messages", async () => {
    mocks.listTaskSessionMessages
      .mockResolvedValueOnce({ messages: [], has_more: true })
      .mockResolvedValueOnce({ messages: [], has_more: false });
    const { result } = renderHook(() => useLazyLoadMessages("session-1"));

    await act(() => result.current.loadMore());
    await act(() => result.current.loadMore());

    expect(mocks.listTaskSessionMessages).toHaveBeenNthCalledWith(
      2,
      "session-1",
      expect.objectContaining({ before: "cursor-1" }),
    );
  });
});
