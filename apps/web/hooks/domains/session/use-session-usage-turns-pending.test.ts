import { afterEach, expect, it, vi } from "vitest";
import { act, waitFor } from "@testing-library/react";
import type { UsageTurn, UsageTurnsPage } from "@/lib/types/conversation-usage";
import type { Turn } from "@/lib/types/http";
import { deriveTurnMetrics } from "@/lib/turn-metrics";
import { deferred, renderSessionRead } from "./session-read-test-helpers";
import { useSessionUsageTurn } from "./use-session-usage-turns";

const USAGE_API = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn() }));
vi.mock("@/lib/api/domains/conversation-usage-api", () => ({
  listSessionUsageTurns: USAGE_API.list,
  getSessionUsageTurn: USAGE_API.get,
}));
afterEach(() => vi.resetAllMocks());

function usageTurn(turnId: string): UsageTurn {
  const bucket = {
    input_tokens: 1,
    input_complete: true,
    output_tokens: 1,
    output_complete: true,
    cached_read_tokens: 0,
    cached_write_tokens: 0,
    cached_read_tokens_present: false,
    cached_write_tokens_present: false,
    thought_tokens: 0,
    total_tokens: 2,
    event_count: 1,
    unpriced_count: 0,
    overflow: false,
  };
  return {
    turn_id: turnId,
    completeness: "exact",
    direct: { ...bucket },
    child: { ...bucket, input_tokens: 0, output_tokens: 0, total_tokens: 0, event_count: 0 },
    total: { ...bucket },
    cost_sources: [],
  };
}

function page(turns: UsageTurn[], next_cursor?: string): UsageTurnsPage {
  return { session_id: "session", turns, next_cursor };
}

it.each(["newest", "historical"])(
  "withholds stale metrics while the %s refresh is pending",
  async (location) => {
    const newest = deferred<UsageTurnsPage>();
    const historical = deferred<UsageTurn>();
    const old = usageTurn("wanted");
    old.total.cost_subcents = "12500";
    const updated = usageTurn("wanted");
    updated.total.input_tokens = 55;
    updated.total.output_tokens = updated.direct.output_tokens = 7;
    updated.total.cost_subcents = "25000";
    const turn: Turn = {
      id: "wanted",
      session_id: "session" as Turn["session_id"],
      task_id: "task" as Turn["task_id"],
      started_at: "2026-10-14T00:00:00Z",
      completed_at: "2026-10-14T00:00:01Z",
      created_at: "2026-10-14T00:00:00Z",
      updated_at: "2026-10-14T00:00:01Z",
      metadata: { timing: { ttft_ms: 250, generation_ms: 1000 } },
    };
    USAGE_API.list.mockReturnValueOnce(newest.promise);
    USAGE_API.get.mockReturnValueOnce(historical.promise);
    const { result } = renderSessionRead(
      () => useSessionUsageTurn("task", "session", "wanted"),
      undefined,
      (store) =>
        store.getState().setSessionUsageTurnPage("session", page([old], "saved"), "refresh", 0),
    );
    await waitFor(() => expect(result.current.value.isLoading).toBe(false));
    expect(deriveTurnMetrics(turn, result.current.value.usageTurn)).toMatchObject({
      inputTokens: 1,
      outputTokens: 1,
      tokensPerSecond: 1,
      costSubcents: "12500",
    });
    try {
      await act(async () =>
        result.current.store.getState().bumpSessionUsageInvalidation("session"),
      );
      expect(result.current.value.usageTurn).toBeNull();
      expect(result.current.value.isLoading).toBe(true);
      expect(deriveTurnMetrics(turn, result.current.value.usageTurn)).toMatchObject({
        inputTokens: null,
        outputTokens: null,
        tokensPerSecond: null,
        costSubcents: null,
        wallTimeMs: 1000,
        ttftMs: 250,
      });
      if (location === "historical") {
        await act(async () => newest.resolve(page([usageTurn("newest")], "ignored")));
        await waitFor(() => expect(USAGE_API.get).toHaveBeenCalledTimes(1));
        expect(result.current.value.usageTurn).toBeNull();
        expect(result.current.value.isLoading).toBe(true);
        await act(async () => historical.resolve(updated));
      } else {
        await act(async () => newest.resolve(page([updated])));
      }
      await waitFor(() => expect(result.current.value.isLoading).toBe(false));
      expect(deriveTurnMetrics(turn, result.current.value.usageTurn)).toMatchObject({
        inputTokens: 55,
        outputTokens: 7,
        tokensPerSecond: 7,
        costSubcents: "25000",
      });
      expect(result.current.store.getState().usageTurns.bySessionId.session.nextCursor).toBe(
        "saved",
      );
    } finally {
      await act(async () => {
        newest.resolve(page([updated]));
        historical.resolve(updated);
      });
    }
  },
);

it("keeps current-generation cached usage visible when reconnect refresh fails", async () => {
  const old = usageTurn("wanted");
  old.total.cost_subcents = "12500";
  USAGE_API.list.mockRejectedValueOnce(new Error("offline"));
  const { result } = renderSessionRead(
    () => useSessionUsageTurn("task", "session", "wanted"),
    undefined,
    (store) => store.getState().setSessionUsageTurnPage("session", page([old]), "refresh", 0),
  );
  await waitFor(() => expect(result.current.value.isLoading).toBe(false));
  await act(async () => result.current.store.getState().setConnectionStatus("reconnecting"));
  await act(async () => result.current.store.getState().setConnectionStatus("connected"));
  expect(USAGE_API.list).toHaveBeenCalledTimes(1);
  expect(result.current.value.isUnavailable).toBe(false);
  expect(result.current.value.usageTurn?.total.cost_subcents).toBe("12500");
  expect(result.current.store.getState().usageInvalidation.bySessionId.session ?? 0).toBe(0);
});
