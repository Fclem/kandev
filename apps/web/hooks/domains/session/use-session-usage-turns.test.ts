import { afterEach, describe, expect, it, vi } from "vitest";
import { act, waitFor } from "@testing-library/react";
import { ApiError } from "@/lib/api/client";
import type { UsageTurn, UsageTurnsPage } from "@/lib/types/conversation-usage";
import { deferred, renderSessionRead } from "./session-read-test-helpers";
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
      input_complete: true,
      cached_read_tokens: 0,
      cached_write_tokens: 0,
      cached_read_tokens_present: false,
      cached_write_tokens_present: false,
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
      input_complete: true,
      cached_read_tokens: 0,
      cached_write_tokens: 0,
      cached_read_tokens_present: false,
      cached_write_tokens_present: false,
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
      input_complete: true,
      cached_read_tokens: 0,
      cached_write_tokens: 0,
      cached_read_tokens_present: false,
      cached_write_tokens_present: false,
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
  vi.resetAllMocks();
});

it("withholds indexed aggregates when their invalidation refresh fails", async () => {
  USAGE_API.list
    .mockResolvedValueOnce(page([usageTurn("wanted")]))
    .mockRejectedValueOnce(new Error("offline"));
  const { result } = renderSessionRead(
    () => useSessionUsageTurn("task", "session", "wanted"),
    undefined,
  );
  await waitFor(() => expect(result.current.value.usageTurn?.turn_id).toBe("wanted"));
  act(() => result.current.store.getState().bumpSessionUsageInvalidation("session"));
  await waitFor(() => expect(result.current.value.isUnavailable).toBe(true));
  expect(result.current.value.usageTurn).toBeNull();
  expect(
    result.current.store.getState().usageTurns.bySessionId.session.byTurnId.wanted,
  ).toBeDefined();
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

  it("refresh merges the newest page and retains previously loaded turns", async () => {
    USAGE_API.list
      .mockResolvedValueOnce(page([usageTurn("newest")], "cursor-1"))
      .mockResolvedValueOnce(page([usageTurn("older")]))
      .mockResolvedValueOnce(page([usageTurn("newest-updated")], "cursor-new"));
    USAGE_API.get.mockResolvedValue(usageTurn("older"));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "older"),
      undefined,
    );

    await waitFor(() => expect(result.current.value.usageTurn?.turn_id).toBe("older"));
    result.current.store.getState().bumpSessionUsageInvalidation("session");
    await waitFor(() => expect(USAGE_API.list).toHaveBeenCalledTimes(3));

    expect(USAGE_API.list).toHaveBeenNthCalledWith(3, "task", "session", { limit: 100 });
    const index = result.current.store.getState().usageTurns.bySessionId.session;
    expect(index.byTurnId.older).toBeDefined();
    expect(index.byTurnId.newest).toBeDefined();
    expect(index.byTurnId["newest-updated"]).toBeDefined();
    expect(index.exhausted).toBe(true);
  });

  it("keeps a turn pushed past page one retrievable after refresh of an exhausted index", async () => {
    USAGE_API.list
      .mockResolvedValueOnce(page([usageTurn("t1")], "cursor-1"))
      .mockResolvedValueOnce(page([usageTurn("t0")]))
      .mockResolvedValueOnce(page([usageTurn("t2")], "cursor-fresh"));
    USAGE_API.get.mockResolvedValue(usageTurn("t1"));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "t1"),
      undefined,
    );

    await waitFor(() => expect(result.current.value.usageTurn?.turn_id).toBe("t1"));
    result.current.store.getState().setSessionUsageTurnPage("session", page([usageTurn("t0")]));
    result.current.store.getState().bumpSessionUsageInvalidation("session");
    await waitFor(() => expect(USAGE_API.list).toHaveBeenCalledTimes(2));

    const index = result.current.store.getState().usageTurns.bySessionId.session;
    expect(index.byTurnId.t1).toBeDefined();
    expect(index.exhausted).toBe(true);
    expect(index.nextCursor).toBeUndefined();
    expect(result.current.value.isNotFound).toBe(false);
  });
});

describe("useSessionUsageTurn newest-page refresh", () => {
  it("refreshes the newest page, not the cursor, when a new turn is demanded from an exhausted index", async () => {
    USAGE_API.list.mockResolvedValue(page([usageTurn("t2"), usageTurn("t1")], "cursor-fresh"));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "t2"),
      undefined,
      (store) =>
        store.getState().setSessionUsageTurnPage("session", page([usageTurn("t1")]), "refresh", 0),
    );

    await waitFor(() => expect(result.current.value.usageTurn?.turn_id).toBe("t2"));

    expect(USAGE_API.list).toHaveBeenCalledTimes(1);
    expect(USAGE_API.list).toHaveBeenCalledWith("task", "session", { limit: 100 });
    expect(result.current.value.isNotFound).toBe(false);
  });

  it("refreshes the newest page before walking the cursor for a new turn", async () => {
    USAGE_API.list.mockResolvedValue(page([usageTurn("t2"), usageTurn("t1")], "cursor-fresh"));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "t2"),
      undefined,
      (store) =>
        store
          .getState()
          .setSessionUsageTurnPage("session", page([usageTurn("t1")], "cursor-1"), "refresh", 0),
    );

    await waitFor(() => expect(result.current.value.usageTurn?.turn_id).toBe("t2"));

    expect(USAGE_API.list).toHaveBeenCalledTimes(1);
    expect(USAGE_API.list).not.toHaveBeenCalledWith(
      "task",
      "session",
      expect.objectContaining({ cursor: expect.anything() }),
    );
    expect(result.current.store.getState().usageTurns.bySessionId.session.nextCursor).toBe(
      "cursor-1",
    );
  });

  it("reports not found after one newest-page refresh when an exhausted index still lacks the turn", async () => {
    USAGE_API.list.mockResolvedValue(page([usageTurn("t1")]));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "ghost"),
      undefined,
      (store) =>
        store.getState().setSessionUsageTurnPage("session", page([usageTurn("t1")]), "refresh", 0),
    );

    await waitFor(() => expect(result.current.value.isNotFound).toBe(true));

    expect(USAGE_API.list).toHaveBeenCalledTimes(1);
    expect(USAGE_API.get).not.toHaveBeenCalled();
  });

  it("refreshes a stale index on mount when invalidations arrived while unmounted", async () => {
    USAGE_API.list.mockResolvedValue(page([usageTurn("t2"), usageTurn("t1")]));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "t1"),
      undefined,
      (store) => {
        store.getState().setSessionUsageTurnPage("session", page([usageTurn("t1")]), "refresh", 0);
        store.getState().bumpSessionUsageInvalidation("session");
      },
    );

    await waitFor(() => expect(USAGE_API.list).toHaveBeenCalledTimes(1));

    expect(USAGE_API.list).toHaveBeenCalledWith("task", "session", { limit: 100 });
    await waitFor(() =>
      expect(
        result.current.store.getState().usageTurns.bySessionId.session.refreshedInvalidation,
      ).toBe(1),
    );
    expect(
      result.current.store.getState().usageTurns.bySessionId.session.byTurnId.t2,
    ).toBeDefined();
  });
});

const REFRESHED_HISTORY_CURSOR = "new-cursor";

describe("useSessionUsageTurn historical refresh", () => {
  it.each([false, true])(
    "refreshes a demanded historical aggregate without walking history (exhausted=%s)",
    async (exhausted) => {
      const updated = usageTurn("historical");
      updated.total.input_tokens = 99;
      updated.total.cost_subcents = "123";
      USAGE_API.list.mockResolvedValue(page([usageTurn("newest")], REFRESHED_HISTORY_CURSOR));
      USAGE_API.get.mockResolvedValue(updated);
      const { result } = renderSessionRead(
        () => useSessionUsageTurn("task", "session", "historical"),
        undefined,
        (store) =>
          store
            .getState()
            .setSessionUsageTurnPage(
              "session",
              page([usageTurn("historical")], exhausted ? undefined : "saved-cursor"),
              "refresh",
              0,
            ),
      );
      await waitFor(() => expect(result.current.value.isLoading).toBe(false));
      await act(async () =>
        result.current.store.getState().bumpSessionUsageInvalidation("session"),
      );
      await waitFor(() => expect(result.current.value.usageTurn?.total.input_tokens).toBe(99));
      expect(result.current.value.usageTurn?.total.cost_subcents).toBe("123");
      expect(USAGE_API.list).toHaveBeenCalledTimes(1);
      expect(USAGE_API.get).toHaveBeenCalledTimes(1);
      const index = result.current.store.getState().usageTurns.bySessionId.session;
      expect(index.exhausted).toBe(exhausted);
      expect(index.nextCursor).toBe(exhausted ? undefined : "saved-cursor");
    },
  );

  it("refreshes a historical demand after the newest page was already refreshed while unmounted", async () => {
    const updated = usageTurn("historical");
    updated.total.output_tokens = 42;
    USAGE_API.get.mockResolvedValue(updated);
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "historical"),
      undefined,
      (store) => {
        store
          .getState()
          .setSessionUsageTurnPage("session", page([usageTurn("historical")]), "refresh", 0);
        store.getState().bumpSessionUsageInvalidation("session");
        store
          .getState()
          .setSessionUsageTurnPage(
            "session",
            page([usageTurn("newest")], REFRESHED_HISTORY_CURSOR),
            "refresh",
            1,
          );
      },
    );
    await waitFor(() => expect(result.current.value.usageTurn?.total.output_tokens).toBe(42));
    expect(USAGE_API.list).not.toHaveBeenCalled();
    expect(USAGE_API.get).toHaveBeenCalledTimes(1);
  });
});

describe("useSessionUsageTurn historical refresh races and reconnect", () => {
  it("catches invalidation during a historical read without publishing over a fresher page", async () => {
    const pending = deferred<UsageTurn>();
    const followup = deferred<UsageTurn>();
    const updated = usageTurn("historical");
    updated.total.input_tokens = 77;
    USAGE_API.list.mockResolvedValue(page([usageTurn("newest")], REFRESHED_HISTORY_CURSOR));
    USAGE_API.get.mockReturnValueOnce(pending.promise).mockReturnValueOnce(followup.promise);
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "historical"),
      undefined,
      (store) =>
        store
          .getState()
          .setSessionUsageTurnPage("session", page([usageTurn("historical")]), "refresh", 0),
    );
    await act(async () => result.current.store.getState().bumpSessionUsageInvalidation("session"));
    await waitFor(() => expect(USAGE_API.get).toHaveBeenCalledTimes(1));
    await act(async () => {
      result.current.store.getState().bumpSessionUsageInvalidation("session");
      result.current.store
        .getState()
        .setSessionUsageTurnPage("session", page([updated]), "refresh", 2);
      pending.resolve(usageTurn("historical"));
    });
    await waitFor(() => expect(USAGE_API.get).toHaveBeenCalledTimes(2));
    expect(
      result.current.store.getState().usageTurns.bySessionId.session.byTurnId.historical.total
        .input_tokens,
    ).toBe(77);
    await act(async () => followup.resolve(updated));
    await waitFor(() => expect(result.current.value.usageTurn?.total.input_tokens).toBe(77));
    expect(
      result.current.store.getState().usageTurns.bySessionId.session.byTurnId.historical.total
        .input_tokens,
    ).toBe(77);
  });

  it("refreshes a historical aggregate on reconnect without a delivered invalidation", async () => {
    const updated = usageTurn("historical");
    updated.total.input_tokens = 88;
    USAGE_API.list.mockResolvedValue(page([usageTurn("newest")], REFRESHED_HISTORY_CURSOR));
    USAGE_API.get.mockResolvedValue(updated);
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "historical"),
      undefined,
      (store) =>
        store
          .getState()
          .setSessionUsageTurnPage("session", page([usageTurn("historical")]), "refresh", 0),
    );
    await act(async () => result.current.store.getState().setConnectionStatus("reconnecting"));
    await act(async () => result.current.store.getState().setConnectionStatus("connected"));
    await waitFor(() => expect(result.current.value.usageTurn?.total.input_tokens).toBe(88));
    expect(USAGE_API.list).toHaveBeenCalledTimes(1);
    expect(USAGE_API.get).toHaveBeenCalledTimes(1);
  });
});

describe("useSessionUsageTurn in-flight invalidation", () => {
  it("coalesces invalidations arriving during a newest-page read and shows the newer usage", async () => {
    const pending = deferred<UsageTurnsPage>();
    const refreshed = usageTurn("t1");
    refreshed.total.input_tokens = 99;
    USAGE_API.list.mockReturnValueOnce(pending.promise).mockResolvedValueOnce(page([refreshed]));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "t1"),
      undefined,
      (store) =>
        store.getState().setSessionUsageTurnPage("session", page([usageTurn("t1")]), "refresh", 0),
    );

    await act(async () => result.current.store.getState().bumpSessionUsageInvalidation("session"));
    expect(USAGE_API.list).toHaveBeenCalledTimes(1);
    await act(async () => {
      result.current.store.getState().bumpSessionUsageInvalidation("session");
      result.current.store.getState().bumpSessionUsageInvalidation("session");
    });
    expect(USAGE_API.list).toHaveBeenCalledTimes(1);
    await act(async () => pending.resolve(page([usageTurn("t1")])));

    await waitFor(() => expect(result.current.value.usageTurn?.total.input_tokens).toBe(99));
    expect(USAGE_API.list).toHaveBeenCalledTimes(2);
    expect(USAGE_API.list).toHaveBeenNthCalledWith(2, "task", "session", { limit: 100 });
    expect(
      result.current.store.getState().usageTurns.bySessionId.session.refreshedInvalidation,
    ).toBe(3);
  });

  it("refreshes an initially empty index when invalidation arrives during the demand read", async () => {
    const pending = deferred<UsageTurnsPage>();
    USAGE_API.list
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValueOnce(page([usageTurn("wanted")]));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "wanted"),
      undefined,
    );

    await act(async () => result.current.store.getState().bumpSessionUsageInvalidation("session"));
    expect(USAGE_API.list).toHaveBeenCalledTimes(1);
    await act(async () => pending.resolve(page([])));

    await waitFor(() => expect(result.current.value.usageTurn?.turn_id).toBe("wanted"));
    expect(USAGE_API.list).toHaveBeenCalledTimes(2);
    expect(result.current.value.isLoading).toBe(false);
    expect(result.current.value.isNotFound).toBe(false);
  });
});

describe("useSessionUsageTurn reconnection", () => {
  it("refreshes the newest page when the connection is restored", async () => {
    USAGE_API.list.mockResolvedValue(page([usageTurn("t2"), usageTurn("t1")]));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "t1"),
      undefined,
      (store) =>
        store.getState().setSessionUsageTurnPage("session", page([usageTurn("t1")]), "refresh", 0),
    );

    await act(async () => result.current.store.getState().setConnectionStatus("reconnecting"));
    expect(USAGE_API.list).not.toHaveBeenCalled();
    await act(async () => result.current.store.getState().setConnectionStatus("connected"));

    await waitFor(() => expect(USAGE_API.list).toHaveBeenCalledTimes(1));
    expect(USAGE_API.list).toHaveBeenCalledWith("task", "session", { limit: 100 });
    expect(
      result.current.store.getState().usageTurns.bySessionId.session.byTurnId.t2,
    ).toBeDefined();
  });
});
describe("useSessionUsageTurn single-turn fallback", () => {
  it.each([false, true])(
    "does not read a missing turn after index exhaustion (preloaded=%s)",
    async (preloaded) => {
      USAGE_API.list.mockResolvedValue(page([]));
      USAGE_API.get.mockResolvedValue(usageTurn("missing"));
      const { result } = renderSessionRead(
        () => useSessionUsageTurn("task", "session", "missing", { fallbackToSingleTurn: true }),
        undefined,
        (store) => {
          if (preloaded) {
            store.getState().setSessionUsageTurnPage("session", page([usageTurn("existing")]));
          }
        },
      );
      await waitFor(() => expect(result.current.value.isLoading).toBe(false));
      expect(result.current.value.isNotFound).toBe(true);
      expect(result.current.value.usageTurn).toBeNull();
      expect(USAGE_API.get).not.toHaveBeenCalled();
    },
  );
});

describe("useSessionUsageTurn invalidated absent fallback", () => {
  it.each(["pending", "settled"] as const)(
    "refreshes an absent historical fallback when invalidation follows a %s 404 snapshot",
    async (phase) => {
      const pending = deferred<UsageTurn>();
      const updated = usageTurn("historical-missing");
      updated.total.input_tokens = 99;
      USAGE_API.list
        .mockResolvedValueOnce(page([usageTurn("newest")], "cursor-1"))
        .mockResolvedValueOnce(page([usageTurn("middle")], "cursor-2"))
        .mockResolvedValueOnce(page([usageTurn("old-page")], "cursor-3"))
        .mockResolvedValue(page([usageTurn("newest")], "ignored-refresh-cursor"));
      USAGE_API.get.mockReturnValueOnce(pending.promise).mockResolvedValue(updated);
      const { result } = renderSessionRead(
        () =>
          useSessionUsageTurn("task", "session", "historical-missing", {
            fallbackToSingleTurn: true,
          }),
        undefined,
      );
      await waitFor(() => expect(USAGE_API.get).toHaveBeenCalledTimes(1));
      if (phase === "settled") {
        await act(async () => pending.reject(new ApiError("not found", 404, null)));
        await waitFor(() => expect(result.current.value.isNotFound).toBe(true));
      }
      await act(async () =>
        result.current.store.getState().bumpSessionUsageInvalidation("session"),
      );
      await waitFor(() => expect(USAGE_API.list).toHaveBeenCalledTimes(4));
      if (phase === "pending") {
        await act(async () => pending.reject(new ApiError("not found", 404, null)));
      }
      await waitFor(() => expect(result.current.value.usageTurn?.total.input_tokens).toBe(99));
      expect(result.current.value.isNotFound).toBe(false);
      expect(USAGE_API.get).toHaveBeenCalledTimes(2);
      expect(USAGE_API.get).toHaveBeenLastCalledWith("task", "session", "historical-missing");
      const index = result.current.store.getState().usageTurns.bySessionId.session;
      expect(index.nextCursor).toBe("cursor-3");
      expect(index.exhausted).toBe(false);
      expect(index.byTurnId["old-page"]).toBeDefined();
    },
  );

  it("does not retry a raced missing fallback after the index becomes exhausted", async () => {
    const pending = deferred<UsageTurn>();
    USAGE_API.list
      .mockResolvedValueOnce(page([usageTurn("newest")], "cursor-1"))
      .mockResolvedValueOnce(page([usageTurn("middle")], "cursor-2"))
      .mockResolvedValueOnce(page([usageTurn("old-page")], "cursor-3"))
      .mockResolvedValue(page([usageTurn("newest")], "ignored-refresh-cursor"));
    USAGE_API.get.mockReturnValueOnce(pending.promise).mockResolvedValue(usageTurn("missing"));
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "missing", { fallbackToSingleTurn: true }),
      undefined,
    );
    await waitFor(() => expect(USAGE_API.get).toHaveBeenCalledTimes(1));
    await act(async () => {
      result.current.store.getState().setSessionUsageTurnPage("session", page([]), "append");
      result.current.store.getState().bumpSessionUsageInvalidation("session");
    });
    await waitFor(() => expect(USAGE_API.list).toHaveBeenCalledTimes(4));
    await act(async () => pending.reject(new ApiError("not found", 404, null)));
    await waitFor(() => expect(result.current.value.isNotFound).toBe(true));
    expect(USAGE_API.get).toHaveBeenCalledTimes(1);
    expect(result.current.store.getState().usageTurns.bySessionId.session.exhausted).toBe(true);
  });
});

describe("useSessionUsageTurn single-turn response admission", () => {
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
