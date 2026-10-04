import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
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
    cached_read_tokens: 1,
    cached_write_tokens: 1,
    output_tokens: 2,
    thought_tokens: 1,
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
