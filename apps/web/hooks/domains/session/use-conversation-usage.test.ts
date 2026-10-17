import { afterEach, expect, it, vi } from "vitest";
import { act, waitFor } from "@testing-library/react";
import type { SessionUsageTotals, UsageTurn } from "@/lib/types/conversation-usage";
import type { Turn } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { deferred, renderSessionRead } from "./session-read-test-helpers";
import { useConversationUsage } from "./use-conversation-usage";

const api = vi.hoisted(() => ({ totals: vi.fn(), list: vi.fn(), get: vi.fn() }));
vi.mock("@/lib/api/domains/conversation-usage-api", () => ({
  getSessionUsageTotals: api.totals,
  listSessionUsageTurns: api.list,
  getSessionUsageTurn: api.get,
}));
const bucket = {
  input_tokens: 1,
  input_complete: true,
  output_tokens: 20,
  output_complete: true,
  cached_read_tokens: 0,
  cached_write_tokens: 0,
  cached_read_tokens_present: false,
  cached_write_tokens_present: false,
  thought_tokens: 0,
  total_tokens: 21,
  event_count: 1,
  unpriced_count: 0,
  overflow: false,
};
function turn(output: number): UsageTurn {
  return {
    turn_id: "turn",
    completeness: "exact",
    direct: { ...bucket, output_tokens: output },
    child: { ...bucket, event_count: 0 },
    total: { ...bucket, output_tokens: output },
    cost_sources: [],
  };
}
const totals: SessionUsageTotals = {
  scope: "session",
  scope_id: "session",
  tokens_in: 1,
  tokens_cached_read: 0,
  tokens_cached_write: 0,
  tokens_out: 20,
  tokens_thought: 0,
  cost_subcents: 12500,
  output_tokens_complete: true,
  input_tokens_complete: true,
  total_tokens_complete: true,
  first_event_at: null,
  last_event_at: null,
  tokens_total: 21,
  cost_subcents_decimal: "12500",
  event_count: 1,
  unpriced_event_count: 0,
  estimated_event_count: 0,
};

function coverageOnlyTurn(): UsageTurn {
  const empty = {
    ...bucket,
    input_tokens: 0,
    input_complete: false,
    output_tokens: 0,
    output_complete: false,
    total_tokens: 0,
    event_count: 0,
  };
  return {
    turn_id: "coverage",
    completeness: "partial",
    direct: empty,
    child: empty,
    total: empty,
    cost_sources: [],
  };
}
afterEach(() => vi.resetAllMocks());

function conversationTurn(id = "turn", at = "2026-10-14T00:00:00Z"): Turn {
  return {
    id,
    session_id: sessionId("session"),
    task_id: taskId("task"),
    started_at: at,
    completed_at: at,
    created_at: at,
    updated_at: at,
  };
}

function seedCurrentTurn(store: StoreApi<AppState>) {
  store.getState().addTurn(conversationTurn());
  store.setState({ turns: { ...store.getState().turns, loadedBySession: { session: true } } });
}

it("never attributes an older usage turn to a newer completed conversation turn", async () => {
  api.totals.mockResolvedValue(totals);
  api.list.mockResolvedValue({ session_id: "session", turns: [turn(20)] });
  const { result } = renderSessionRead(
    () => useConversationUsage("task", "session"),
    undefined,
    (store) => {
      store.getState().addTurn(conversationTurn());
      store.setState({ turns: { ...store.getState().turns, loadedBySession: { session: true } } });
    },
  );
  await waitFor(() => expect(result.current.value.latestTurn?.turn_id).toBe("turn"));
  act(() =>
    result.current.store.getState().addTurn(conversationTurn("newer", "2026-10-14T00:00:02Z")),
  );
  expect(result.current.value.latestTurn).toBeNull();
  expect(result.current.value.totals?.cost_subcents_decimal).toBe("12500");
  await act(async () => result.current.value.refresh());
  expect(result.current.value.latestTurn).toBeNull();
  api.list.mockResolvedValue({
    session_id: "session",
    turns: [{ ...turn(40), turn_id: "newer" }, turn(20)],
  });
  await act(async () => result.current.value.refresh());
  expect(result.current.value.latestTurn?.turn_id).toBe("newer");
  expect(result.current.value.latestTurn?.total.output_tokens).toBe(40);
});

it("does not resurrect old usage when a newer turn arrives during a summary read", async () => {
  const list = deferred<{ session_id: string; turns: UsageTurn[] }>();
  api.totals.mockResolvedValue(totals);
  api.list.mockReturnValue(list.promise);
  const { result } = renderSessionRead(
    () => useConversationUsage("task", "session"),
    undefined,
    (store) => {
      store.getState().addTurn(conversationTurn());
      store.setState({ turns: { ...store.getState().turns, loadedBySession: { session: true } } });
    },
  );
  act(() =>
    result.current.store.getState().addTurn(conversationTurn("newer", "2026-10-14T00:00:02Z")),
  );
  await act(async () => list.resolve({ session_id: "session", turns: [turn(20)] }));
  expect(result.current.value.latestTurn).toBeNull();
  expect(result.current.value.totals?.tokens_total).toBe(21);
});

it("refreshes demanded same-turn detail after invalidation and rejects an older response", async () => {
  const old = deferred<UsageTurn>();
  api.totals.mockResolvedValue(null);
  api.list.mockResolvedValue({ session_id: "session", turns: [turn(40)] });
  api.get.mockReturnValueOnce(old.promise).mockResolvedValueOnce(turn(40));
  const { result } = renderSessionRead(
    () => useConversationUsage("task", "session"),
    undefined,
    seedCurrentTurn,
  );
  await act(async () => {
    void result.current.value.loadTurnDetail("turn");
  });
  act(() => result.current.store.getState().bumpSessionUsageInvalidation("session"));
  await waitFor(() => expect(result.current.value.detail?.turn?.total.output_tokens).toBe(40));
  await act(async () => old.resolve(turn(20)));
  expect(result.current.value.detail?.turn?.total.output_tokens).toBe(40);
});

it("reloads settled detail rather than retaining its obsolete response", async () => {
  api.totals.mockResolvedValue(null);
  api.list.mockResolvedValue({ session_id: "session", turns: [turn(40)] });
  api.get.mockResolvedValueOnce(turn(20)).mockResolvedValueOnce(turn(40));
  const { result } = renderSessionRead(
    () => useConversationUsage("task", "session"),
    undefined,
    seedCurrentTurn,
  );
  await act(async () => {
    await result.current.value.loadTurnDetail("turn");
  });
  expect(result.current.value.detail?.turn?.total.output_tokens).toBe(20);
  act(() => result.current.store.getState().bumpSessionUsageInvalidation("session"));
  await waitFor(() => expect(result.current.value.detail?.turn?.total.output_tokens).toBe(40));
});

it.each(["totals", "list"] as const)(
  "withholds invalidated summary after %s failure and admits coverage-only retry",
  async (failedRead) => {
    const pendingTotals = deferred<SessionUsageTotals>();
    const pendingList = deferred<{ session_id: string; turns: UsageTurn[] }>();
    api.totals.mockResolvedValueOnce(totals).mockReturnValueOnce(pendingTotals.promise);
    api.list
      .mockResolvedValueOnce({ session_id: "session", turns: [turn(20)] })
      .mockReturnValueOnce(pendingList.promise);
    const { result } = renderSessionRead(
      () => useConversationUsage("task", "session"),
      undefined,
      seedCurrentTurn,
    );
    await waitFor(() => expect(result.current.value.latestTurn?.turn_id).toBe("turn"));
    expect(result.current.value.totals?.cost_subcents_decimal).toBe("12500");
    act(() => {
      result.current.store.getState().addTurn(conversationTurn("coverage", "2026-10-14T00:00:02Z"));
      result.current.store.getState().bumpSessionUsageInvalidation("session");
    });
    await act(async () => {
      if (failedRead === "totals") {
        pendingTotals.reject(new Error("totals unavailable"));
        pendingList.resolve({ session_id: "session", turns: [coverageOnlyTurn()] });
      } else {
        pendingTotals.resolve(totals);
        pendingList.reject(new Error("turn page unavailable"));
      }
    });
    expect(result.current.value.error).toBe(true);
    expect(result.current.value.loading).toBe(false);
    expect(result.current.value.totals).toBeNull();
    expect(result.current.value.latestTurn).toBeNull();
    const retryTotals = deferred<SessionUsageTotals>();
    const retryList = deferred<{ session_id: string; turns: UsageTurn[] }>();
    api.totals.mockReturnValueOnce(retryTotals.promise);
    api.list.mockReturnValueOnce(retryList.promise);
    let retry!: Promise<void>;
    act(() => {
      retry = result.current.value.refresh();
    });
    expect(result.current.value.loading).toBe(true);
    expect(result.current.value.error).toBe(false);
    expect(result.current.value.totals).toBeNull();
    expect(result.current.value.latestTurn).toBeNull();
    retryTotals.resolve({
      ...totals,
      tokens_in: 0,
      tokens_out: 0,
      cost_subcents: 0,
      tokens_total: 0,
      cost_subcents_decimal: "0",
      event_count: 0,
    });
    retryList.resolve({ session_id: "session", turns: [coverageOnlyTurn()] });
    await act(async () => retry);
    expect(result.current.value.error).toBe(false);
    expect(result.current.value.latestTurn?.completeness).toBe("partial");
    expect(result.current.value.latestTurn?.total.event_count).toBe(0);
  },
);

it("retains accepted same-generation cache after an explicit refresh fails", async () => {
  api.totals.mockResolvedValueOnce(totals).mockRejectedValueOnce(new Error("unavailable"));
  api.list.mockResolvedValue({ session_id: "session", turns: [turn(20)] });
  const { result } = renderSessionRead(
    () => useConversationUsage("task", "session"),
    undefined,
    seedCurrentTurn,
  );
  await waitFor(() => expect(result.current.value.latestTurn?.turn_id).toBe("turn"));
  await act(async () => result.current.value.refresh());
  expect(result.current.value.error).toBe(true);
  expect(result.current.value.totals?.cost_subcents_decimal).toBe("12500");
  expect(result.current.value.latestTurn?.total.output_tokens).toBe(20);
});

it("bounds usage reads to visible demand and refreshes hidden invalidations on reopen", async () => {
  api.totals.mockResolvedValue(totals);
  api.list.mockResolvedValue({ session_id: "session", turns: [turn(20)] });
  const { result, rerender } = renderSessionRead(
    ({ active }: { active: boolean }) => useConversationUsage("task", "session", active),
    { active: false },
    seedCurrentTurn,
  );
  expect(api.totals).not.toHaveBeenCalled();
  expect(api.list).not.toHaveBeenCalled();
  rerender({ active: true });
  await waitFor(() => expect(result.current.value.latestTurn?.total.output_tokens).toBe(20));
  rerender({ active: false });
  expect(result.current.value.totals?.tokens_out).toBe(20);
  act(() => result.current.store.getState().bumpSessionUsageInvalidation("session"));
  await act(async () => result.current.value.refresh());
  expect(result.current.value.totals).toBeNull();
  expect(api.totals).toHaveBeenCalledTimes(1);
  expect(api.list).toHaveBeenCalledTimes(1);
  api.totals.mockResolvedValue({ ...totals, tokens_out: 40 });
  api.list.mockResolvedValue({ session_id: "session", turns: [turn(40)] });
  rerender({ active: true });
  await waitFor(() => expect(result.current.value.latestTurn?.total.output_tokens).toBe(40));
  expect(result.current.value.totals?.tokens_out).toBe(40);
  expect(api.totals).toHaveBeenCalledTimes(2);
  expect(api.list).toHaveBeenCalledTimes(2);
});
