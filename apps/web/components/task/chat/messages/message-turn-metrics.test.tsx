import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { t } from "i18next";
import type { UsageTurn } from "@/lib/types/conversation-usage";
import type { Turn } from "@/lib/types/http";
import { MessageTurnMetrics } from "./message-turn-metrics";

const USAGE_READ = vi.hoisted(() => ({
  usageTurn: null as UsageTurn | null,
  isLoading: false,
  isUnavailable: false,
  isNotFound: false,
}));

vi.mock("@/hooks/domains/session/use-session-usage-turns", () => ({
  useSessionUsageTurn: () => USAGE_READ,
}));

function turn(): Turn {
  return {
    id: "turn-1",
    session_id: "session-1" as Turn["session_id"],
    task_id: "task-1" as Turn["task_id"],
    started_at: "2026-09-24T10:00:00.000Z",
    completed_at: "2026-09-24T10:00:02.000Z",
    created_at: "2026-09-24T10:00:00.000Z",
    updated_at: "2026-09-24T10:00:02.000Z",
    metadata: { timing: { ttft_ms: 350, generation_ms: 2000 } },
  };
}

function usageTurn(): UsageTurn {
  const direct = {
    input_tokens: 3,
    input_complete: true,
    cached_read_tokens: 1,
    cached_write_tokens: 1,
    cached_read_tokens_present: true,
    cached_write_tokens_present: true,
    output_tokens: 2,
    thought_tokens: 1,
    reasoning_tokens: 1,
    total_tokens: 8,
    output_complete: true,
    event_count: 1,
    unpriced_count: 0,
    cost_subcents: "123",
    overflow: false,
  };
  return {
    turn_id: "turn-1",
    completeness: "exact",
    direct,
    child: { ...direct, output_tokens: 50 },
    total: { ...direct, input_tokens: 3, output_tokens: 52, total_tokens: 58 },
    cost_sources: ["provider_reported"],
  };
}

function rowValue(label: string): string | undefined {
  return screen.getByText(label).parentElement?.querySelector("dd")?.textContent ?? undefined;
}

afterEach(() => {
  cleanup();
  USAGE_READ.usageTurn = null;
  USAGE_READ.isLoading = false;
  USAGE_READ.isUnavailable = false;
  USAGE_READ.isNotFound = false;
});

describe("MessageTurnMetrics", () => {
  it("shows all available metrics and uses the total only for token rows", () => {
    USAGE_READ.usageTurn = usageTurn();
    render(<MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />);

    expect(screen.getByText("Turn metrics")).toBeTruthy();
    expect(screen.getByText("2s")).toBeTruthy();
    expect(screen.getByText("350 ms")).toBeTruthy();
    expect(rowValue("Input")).toBe("3");
    expect(rowValue("Output")).toBe("52");
    expect(rowValue("Cached input")).toBe("1");
    expect(rowValue("Reasoning (included in output)")).toBe("1");
    expect(screen.getByText("$0.01")).toBeTruthy();
    expect(screen.getByText("Provider-reported cost")).toBeTruthy();
  });

  it("renders TTFT and TPS definitions as visible text instead of hover-only titles", () => {
    USAGE_READ.usageTurn = usageTurn();
    const { container } = render(
      <MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />,
    );

    expect(screen.getByText(/Backend-measured time from receipt of turn_started/)).toBeTruthy();
    expect(screen.getByText(/Average direct output and reasoning token rate/)).toBeTruthy();
    expect(container.querySelector("[title]")).toBeNull();
  });

  it("shows pending or unavailable usage as status instead of fabricated token zeros", () => {
    USAGE_READ.isLoading = true;
    const { rerender } = render(
      <MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />,
    );
    expect(screen.getByText("Usage pending")).toBeTruthy();
    expect(screen.queryByText("0")).toBeNull();

    USAGE_READ.isLoading = false;
    USAGE_READ.isUnavailable = true;
    rerender(<MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />);
    expect(screen.getByText("Usage data unavailable")).toBeTruthy();
  });

  it("omits unmeasured values and reports an empty usage turn without a zero row", () => {
    const empty = usageTurn();
    empty.direct.event_count = 0;
    empty.total.event_count = 0;
    USAGE_READ.usageTurn = empty;
    USAGE_READ.isNotFound = true;
    render(<MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />);

    expect(screen.getByText("No usage recorded")).toBeTruthy();
    expect(screen.queryByText("0")).toBeNull();
    expect(screen.queryByText("$0.00")).toBeNull();
  });
});

describe("MessageTurnMetrics usage availability and formatting", () => {
  it.each(["partial", "estimated"] as const)(
    "discloses %s coverage alongside independently measured response values",
    (completeness) => {
      const usage = usageTurn();
      usage.completeness = completeness;
      if (completeness === "partial") {
        usage.total.input_complete = false;
        usage.total.output_complete = false;
      }
      USAGE_READ.usageTurn = usage;
      render(<MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />);

      expect(screen.getByRole("status").textContent).toBe(
        t(`task:conversationUsage.${completeness}`),
      );
      expect(rowValue("Time to first token")).toBe("350 ms");
      expect(rowValue(t("task:conversationUsage.reasoning"))).toBe("1");
      expect(screen.getByText("$0.01")).toBeTruthy();
      if (completeness === "partial") {
        expect(screen.queryByText("Input", { exact: true })).toBeNull();
        expect(screen.queryByText("Output", { exact: true })).toBeNull();
      } else {
        expect(rowValue("Input")).toBe("3");
        expect(rowValue("Output")).toBe("52");
      }
    },
  );

  it("reports observed but unmeasured partial coverage as unavailable, not absent", () => {
    const partial = usageTurn();
    partial.completeness = "partial";
    for (const bucket of [partial.direct, partial.child, partial.total]) {
      bucket.event_count = 0;
      bucket.input_complete = false;
      bucket.output_complete = false;
      bucket.cached_read_tokens_present = false;
      bucket.cached_write_tokens_present = false;
      delete bucket.cost_subcents;
      delete bucket.reasoning_tokens;
    }
    USAGE_READ.usageTurn = partial;
    render(<MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />);

    expect(screen.getByRole("status").textContent).toBe("Usage data unavailable");
    expect(screen.queryByText("No usage recorded")).toBeNull();
    expect(screen.queryByText("Input", { exact: true })).toBeNull();
    expect(screen.queryByText("Output", { exact: true })).toBeNull();
    expect(screen.queryByText("$0.00")).toBeNull();
  });

  it("retains measurements without claiming a nonempty overflowed turn has no usage", () => {
    const overflowed = usageTurn();
    overflowed.completeness = "partial";
    overflowed.total.overflow = true;
    USAGE_READ.usageTurn = overflowed;
    render(<MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />);

    expect(screen.queryByText("No usage recorded")).toBeNull();
    expect(screen.queryByText("Input", { exact: true })).toBeNull();
    expect(screen.queryByText("Output", { exact: true })).toBeNull();
    expect(rowValue("Reasoning (included in output)")).toBe("1");
    expect(rowValue("Time to first token")).toBe("350 ms");
    expect(screen.getByText("$0.01")).toBeTruthy();
  });

  it("omits unmeasured cache-write and reasoning rows", () => {
    const codex = usageTurn();
    codex.direct.cached_write_tokens = 0;
    codex.direct.cached_write_tokens_present = false;
    codex.direct.thought_tokens = 0;
    delete codex.direct.reasoning_tokens;
    codex.total.cached_write_tokens = 0;
    codex.total.cached_write_tokens_present = false;
    codex.total.thought_tokens = 0;
    delete codex.total.reasoning_tokens;
    USAGE_READ.usageTurn = codex;
    render(<MessageTurnMetrics turn={turn()} taskId="task-1" sessionId="session-1" />);

    expect(rowValue("Input")).toBe("3");
    expect(screen.queryByText("Reasoning (included in output)")).toBeNull();
    expect(screen.queryByText("Cache-write input")).toBeNull();
  });

  it("renders TTFT above one second with one decimal instead of flooring", () => {
    const slow = { ...turn(), metadata: { timing: { ttft_ms: 1900, generation_ms: 2000 } } };
    render(<MessageTurnMetrics turn={slow} taskId="task-1" sessionId="session-1" />);

    expect(rowValue("Time to first token")).toBe("1.9s");
  });
});
