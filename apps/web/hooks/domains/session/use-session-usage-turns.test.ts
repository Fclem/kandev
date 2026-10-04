import { afterEach, describe, expect, it, vi } from "vitest";
import { waitFor } from "@testing-library/react";
import { ApiError } from "@/lib/api/client";
import type { UsageTurn } from "@/lib/types/conversation-usage";
import { renderSessionRead } from "./session-read-test-helpers";
import { useSessionUsageTurn } from "./use-session-usage-turns";

const USAGE_API = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn() }));

vi.mock("@/lib/api/domains/conversation-usage-api", () => ({
  listSessionUsageTurns: USAGE_API.list,
  getSessionUsageTurn: USAGE_API.get,
}));

function usageTurn(turnId: string): UsageTurn {
  return {
    turn_id: turnId,
    completeness: "exact",
    direct: {
      input_tokens: 1,
      cached_read_tokens: 0,
      cached_write_tokens: 0,
      output_tokens: 1,
      thought_tokens: 0,
      total_tokens: 2,
      output_complete: true,
      event_count: 1,
      unpriced_count: 0,
      overflow: false,
    },
    child: {
      input_tokens: 0,
      cached_read_tokens: 0,
      cached_write_tokens: 0,
      output_tokens: 0,
      thought_tokens: 0,
      total_tokens: 0,
      output_complete: true,
      event_count: 0,
      unpriced_count: 0,
      overflow: false,
    },
    total: {
      input_tokens: 1,
      cached_read_tokens: 0,
      cached_write_tokens: 0,
      output_tokens: 1,
      thought_tokens: 0,
      total_tokens: 2,
      output_complete: true,
      event_count: 1,
      unpriced_count: 0,
      overflow: false,
    },
    cost_sources: [],
  };
}

function page(turns: UsageTurn[], next_cursor?: string) {
  return { session_id: "session", turns, ...(next_cursor ? { next_cursor } : {}) };
}

afterEach(() => {
  vi.clearAllMocks();
});

describe("useSessionUsageTurn page indexing", () => {
  it("pages until the demanded turn is indexed and marks the final page exhausted", async () => {
    USAGE_API.list
      .mockResolvedValueOnce(page([usageTurn("newest")], "cursor-1"))
      .mockResolvedValueOnce(page([usageTurn("wanted")]));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "wanted"),
      undefined,
    );

    await waitFor(() => expect(result.current.value.usageTurn?.turn_id).toBe("wanted"));

    expect(USAGE_API.list).toHaveBeenNthCalledWith(1, "task", "session", {
      limit: 100,
    });
    expect(USAGE_API.list).toHaveBeenNthCalledWith(2, "task", "session", {
      limit: 100,
      cursor: "cursor-1",
    });
    expect(result.current.store.getState().usageTurns.bySessionId.session.exhausted).toBe(true);
  });

  it("treats an empty page as exhausted and does not repeat missing-turn reads", async () => {
    USAGE_API.list.mockResolvedValue(page([], "ignored-cursor"));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "missing"),
      undefined,
    );

    await waitFor(() => expect(result.current.value.isNotFound).toBe(true));

    expect(USAGE_API.list).toHaveBeenCalledTimes(1);
  });

  it("bounds each demand to three pages", async () => {
    let pageNumber = 0;
    USAGE_API.list.mockImplementation(() => {
      pageNumber += 1;
      return Promise.resolve(page([usageTurn(`turn-${pageNumber}`)], `cursor-${pageNumber}`));
    });
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "missing"),
      undefined,
    );

    await waitFor(() => expect(USAGE_API.list).toHaveBeenCalledTimes(3));

    expect(result.current.value.usageTurn).toBeNull();
    expect(result.current.value.isLoading).toBe(false);
  });

  it("refreshes only the newest page on invalidation and retains older indexed turns", async () => {
    USAGE_API.list
      .mockResolvedValueOnce(page([usageTurn("newest")], "cursor-1"))
      .mockResolvedValueOnce(page([usageTurn("older")]))
      .mockResolvedValueOnce(page([usageTurn("newest-updated")], "cursor-1"));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "older"),
      undefined,
    );

    await waitFor(() => expect(result.current.value.usageTurn?.turn_id).toBe("older"));
    result.current.store.getState().bumpSessionUsageInvalidation("session");
    await waitFor(() => expect(USAGE_API.list).toHaveBeenCalledTimes(3));

    expect(USAGE_API.list).toHaveBeenNthCalledWith(3, "task", "session", { limit: 100 });
    expect(
      result.current.store.getState().usageTurns.bySessionId.session.byTurnId.older,
    ).toBeDefined();
    expect(
      result.current.store.getState().usageTurns.bySessionId.session.byTurnId.newest,
    ).toBeUndefined();
    expect(
      result.current.store.getState().usageTurns.bySessionId.session.byTurnId["newest-updated"],
    ).toBeDefined();
  });
});
describe("useSessionUsageTurn single-turn fallback", () => {
  it("falls back to a single-turn read and treats only 404 as no measurement", async () => {
    let cursorNumber = 0;
    USAGE_API.list.mockImplementation(
      (_task: string, _session: string, options: { cursor?: string }) => {
        cursorNumber += 1;
        return Promise.resolve(
          page([usageTurn(`page-${options.cursor ?? "root"}`)], `cursor-${cursorNumber}`),
        );
      },
    );
    USAGE_API.get.mockRejectedValue(new ApiError("not found", 404, null));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "outside-page", { fallbackToSingleTurn: true }),
      undefined,
    );

    await waitFor(() => expect(result.current.value.isNotFound).toBe(true));

    expect(USAGE_API.list).toHaveBeenCalledTimes(3);
    expect(USAGE_API.get).toHaveBeenCalledWith("task", "session", "outside-page");
    expect(result.current.value.isUnavailable).toBe(false);
  });
  it("reports non-404 fallback failures as unavailable", async () => {
    let cursorNumber = 0;
    USAGE_API.list.mockImplementation(
      (_task: string, _session: string, options: { cursor?: string }) => {
        cursorNumber += 1;
        return Promise.resolve(
          page([usageTurn(`page-${options.cursor ?? "root"}`)], `cursor-${cursorNumber}`),
        );
      },
    );
    USAGE_API.get.mockRejectedValue(new ApiError("server error", 500, null));
    const { result } = renderSessionRead(
      () =>
        useSessionUsageTurn("task", "session", "outside-page", {
          fallbackToSingleTurn: true,
        }),
      undefined,
    );

    await waitFor(() => expect(result.current.value.isUnavailable).toBe(true));

    expect(result.current.value.isNotFound).toBe(false);
  });
});
