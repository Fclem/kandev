import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import i18n from "i18next";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type {
  SessionUsageTotals,
  UsageTokenBreakdown,
  UsageTurn,
} from "@/lib/types/conversation-usage";

const usageTriggerTestId = "conversation-usage-trigger";
const measuredOutputText = "20";

const mocks = vi.hoisted(() => ({
  isMobile: false,
  isFinePointer: true,
  touchDrawer: false,
  loadTurnDetail: vi.fn(),
  usage: null as unknown,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.isMobile,
    isFinePointer: mocks.isFinePointer,
  }),
}));
vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => mocks.touchDrawer,
}));
vi.mock("@/hooks/domains/session/use-conversation-usage", () => ({
  useConversationUsage: () =>
    mocks.usage ?? {
      totals: {
        scope: "session",
        scope_id: "session-1",
        tokens_total: 1200,
        cost_subcents_decimal: "150000",
        event_count: 1,
        unpriced_event_count: 0,
        estimated_event_count: 0,
        input_tokens_complete: true,
        total_tokens_complete: true,
        output_tokens_complete: true,
      } as SessionUsageTotals,
      latestTurn: {
        turn_id: "turn-1",
        completeness: "exact",
        direct: {
          input_tokens: 400,
          input_complete: true,
          cached_read_tokens: 600,
          cached_write_tokens: 0,
          cached_read_tokens_present: true,
          cached_write_tokens_present: false,
          output_tokens: 200,
          thought_tokens: 0,
          reasoning_tokens: 80,
          total_tokens: 1200,
          output_complete: true,
          event_count: 1,
          unpriced_count: 0,
          cost_subcents: "150000",
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
          input_tokens: 400,
          input_complete: true,
          cached_read_tokens: 600,
          cached_write_tokens: 0,
          cached_read_tokens_present: true,
          cached_write_tokens_present: false,
          output_tokens: 200,
          thought_tokens: 0,
          reasoning_tokens: 80,
          total_tokens: 1200,
          output_complete: true,
          event_count: 1,
          unpriced_count: 0,
          cost_subcents: "150000",
          overflow: false,
        },
        cost_sources: ["models_dev_list"],
        last_response: {
          usage_event_id: "usage-1",
          provider_response_id: "response-1",
          input_tokens: 1000,
          input_tokens_present: true,
          cached_read_tokens: 600,
          output_tokens: 200,
          reasoning_output_tokens: 80,
          total_tokens: 1200,
          cost_source: "models_dev_list",
          estimated: false,
          occurred_at: "2026-09-24T00:00:00Z",
        },
      } as UsageTurn,
      detail: null,
      loading: false,
      error: false,
      refresh: vi.fn(),
      loadTurnDetail: mocks.loadTurnDetail,
    },
}));
vi.mock("@kandev/ui/drawer", () => ({
  DrawerClose: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
vi.mock("@/components/task/mobile/mobile-picker-sheet", () => ({
  MobilePickerSheet: ({
    open,
    title,
    children,
    headerAction,
    contentTestId,
  }: {
    open: boolean;
    title: string;
    children: React.ReactNode;
    headerAction?: React.ReactNode;
    contentTestId?: string;
  }) =>
    open ? (
      <section role="dialog" aria-label={title} data-testid={contentTestId}>
        <header>
          <h2>{title}</h2>
          {headerAction}
        </header>
        {children}
      </section>
    ) : null,
}));

import { ConversationUsageDisplay } from "./conversation-usage-display";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mocks.isMobile = false;
  mocks.isFinePointer = true;
  mocks.touchDrawer = false;
  mocks.usage = null;
});

describe("ConversationUsageDisplay", () => {
  it("shows an icon-only desktop trigger with its accessible Usage name", () => {
    render(
      <TooltipProvider>
        <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
      </TooltipProvider>,
    );

    const trigger = screen.getByRole("button", { name: "Usage" });
    expect(trigger.textContent).toBe("");
    expect(trigger.className).toContain("h-6");
    expect(trigger.className).toContain("w-6");
  });

  it("opens the touch drawer with the latest usage and requests persisted response detail", () => {
    mocks.touchDrawer = true;
    render(
      <TooltipProvider>
        <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
      </TooltipProvider>,
    );
    const trigger = screen.getByTestId(usageTriggerTestId);

    expect(trigger.className).toContain("h-11");
    fireEvent.click(trigger);

    expect(screen.getByRole("dialog", { name: "Conversation usage" })).toBeTruthy();
    expect(screen.getByTestId("usage-turn-summary").textContent).toContain("1,200");
    expect(mocks.loadTurnDetail).toHaveBeenCalledWith("turn-1");
    expect(screen.getByRole("button", { name: "View response details" }).className).toContain(
      "min-h-11",
    );
    const closeButtons = screen.getAllByRole("button", { name: "Close" });
    expect(closeButtons.at(-1)?.className).toContain("min-h-11");
  });

  it("uses the phone drawer and touch-sized trigger at a narrow fine-pointer breakpoint", () => {
    mocks.isMobile = true;
    render(
      <TooltipProvider>
        <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
      </TooltipProvider>,
    );

    const trigger = screen.getByRole("button", { name: "Usage" });
    expect(trigger.className).toContain("h-11");
    expect(trigger.className).toContain("w-11");
    fireEvent.click(trigger);

    expect(screen.getByRole("dialog", { name: "Conversation usage" })).toBeTruthy();
  });

  it("uses the shared touch drawer policy for a coarse pointer outside the phone breakpoint", () => {
    mocks.touchDrawer = true;
    render(
      <TooltipProvider>
        <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
      </TooltipProvider>,
    );

    const trigger = screen.getByRole("button", { name: "Usage" });
    expect(trigger.className).toContain("h-11");
    expect(trigger.className).toContain("w-11");
    fireEvent.click(trigger);

    expect(screen.getByRole("dialog", { name: "Conversation usage" })).toBeTruthy();
  });
});

describe("ConversationUsageDisplay coverage-only turns", () => {
  it("keeps coverage-only usage accessible without fabricating session amounts", () => {
    mocks.touchDrawer = true;
    const empty: UsageTokenBreakdown = {
      input_tokens: 0,
      input_complete: false,
      output_tokens: 0,
      output_complete: false,
      cached_read_tokens: 0,
      cached_write_tokens: 0,
      cached_read_tokens_present: false,
      cached_write_tokens_present: false,
      thought_tokens: 0,
      total_tokens: 0,
      event_count: 0,
      unpriced_count: 0,
      overflow: false,
    };
    mocks.usage = {
      totals: {
        scope: "session",
        scope_id: "session-1",
        tokens_total: 0,
        cost_subcents_decimal: "0",
        event_count: 0,
        unpriced_event_count: 0,
        estimated_event_count: 0,
      },
      latestTurn: {
        turn_id: "turn-1",
        completeness: "partial",
        direct: empty,
        child: empty,
        total: empty,
        cost_sources: [],
      },
      detail: null,
      loading: false,
      error: false,
      refresh: vi.fn(),
      loadTurnDetail: mocks.loadTurnDetail,
    };
    render(
      <TooltipProvider>
        <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Usage" }));
    const disclosure = screen.getByTestId("conversation-usage-content");
    expect(within(disclosure).queryByText("$0.00")).toBeNull();
    expect(within(disclosure).queryByText("Total tokens", { exact: true })).toBeNull();
    expect(within(disclosure).getByText("Usage data unavailable")).toBeTruthy();
  });
});

describe("ConversationUsageDisplay refreshed usage", () => {
  it("keeps the refreshed latest turn ahead of stale loaded detail", () => {
    mocks.touchDrawer = true;
    const makeTurn = (turnId: string, tokens: number): UsageTurn => {
      const breakdown = {
        input_tokens: tokens,
        input_complete: true,
        cached_read_tokens: 0,
        cached_write_tokens: 0,
        cached_read_tokens_present: true,
        cached_write_tokens_present: false,
        output_tokens: 0,
        thought_tokens: 0,
        total_tokens: tokens,
        output_complete: true,
        event_count: 1,
        unpriced_count: 0,
        overflow: false,
      };
      return {
        turn_id: turnId,
        completeness: "exact",
        direct: breakdown,
        child: {
          ...breakdown,
          input_tokens: 0,
          total_tokens: 0,
          event_count: 0,
          cached_read_tokens_present: false,
          cached_write_tokens_present: false,
        },
        total: breakdown,
        cost_sources: [],
        last_response: {
          usage_event_id: `usage-${turnId}`,
          input_tokens: tokens,
          input_tokens_present: true,
          cached_read_tokens: 0,
          output_tokens: 0,
          total_tokens: tokens,
          cost_source: "unpriced",
          estimated: false,
          occurred_at: "2026-09-24T00:00:00Z",
        },
      };
    };
    const latestTurn = makeTurn("turn-new", 1200);
    mocks.usage = {
      totals: {
        scope: "session",
        scope_id: "session-1",
        tokens_in: 1200,
        tokens_cached_read: 0,
        tokens_cached_write: 0,
        tokens_out: 0,
        tokens_thought: 0,
        tokens_total: 1200,
        cost_subcents: 0,
        event_count: 2,
        estimated_event_count: 0,
        unpriced_event_count: 2,
        input_tokens_complete: true,
        total_tokens_complete: true,
        output_tokens_complete: true,
        first_event_at: null,
        last_event_at: null,
      } as SessionUsageTotals,
      latestTurn,
      detail: {
        key: "task-1\u0000session-1",
        turnId: "turn-old",
        turn: makeTurn("turn-old", 800),
        loading: false,
        error: false,
      },
      loading: false,
      error: false,
      refresh: vi.fn(),
      loadTurnDetail: mocks.loadTurnDetail,
    };

    render(
      <TooltipProvider>
        <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByTestId(usageTriggerTestId));

    const summary = screen.getByTestId("usage-turn-summary").textContent;
    expect(summary).toContain("1,200");
    expect(summary).not.toContain("800");
  });
});

function measuredBucket(overrides: Partial<UsageTokenBreakdown> = {}): UsageTokenBreakdown {
  return {
    input_tokens: 5,
    input_complete: true,
    cached_read_tokens: 0,
    cached_write_tokens: 0,
    cached_read_tokens_present: false,
    cached_write_tokens_present: false,
    output_tokens: 2,
    output_complete: true,
    thought_tokens: 0,
    total_tokens: 7,
    event_count: 1,
    unpriced_count: 0,
    overflow: false,
    ...overrides,
  };
}

function openMeasuredUsage(
  direct: UsageTokenBreakdown,
  child = measuredBucket({ event_count: 0 }),
  total = direct,
) {
  const turn: UsageTurn = {
    turn_id: "turn-measured",
    direct,
    child,
    total,
    completeness: total.overflow || direct.overflow ? "partial" : "exact",
    cost_sources: ["provider_reported"],
  };
  mocks.touchDrawer = true;
  mocks.usage = {
    totals: {
      event_count: 1,
      tokens_total: 7,
      unpriced_event_count: 0,
      cost_subcents_decimal: "12345",
      input_tokens_complete: true,
      total_tokens_complete: true,
      output_tokens_complete: true,
    },
    latestTurn: turn,
    detail: { turnId: turn.turn_id, turn, loading: false, error: false },
    loading: false,
    error: false,
    refresh: vi.fn(),
    loadTurnDetail: mocks.loadTurnDetail,
  };
  render(
    <TooltipProvider>
      <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByTestId(usageTriggerTestId));
}

const usageSummaryTestId = "usage-turn-summary";
const directSessionLabel = "This session";
const totalTokensLabel = "Total tokens";

describe("ConversationUsageDisplay measured counts", () => {
  it("omits an empty child bucket instead of presenting unobserved zeros", () => {
    openMeasuredUsage(measuredBucket());
    expect(screen.queryByText("Child agents")).toBeNull();
    const direct = within(screen.getByText(directSessionLabel).closest("section")!);
    expect(direct.getByText("Input")).toBeTruthy();
    expect(direct.getByText("Output")).toBeTruthy();
    expect(direct.getByText("7")).toBeTruthy();
  });

  it("keeps known input but omits missing output and incomplete total counts", () => {
    openMeasuredUsage(
      measuredBucket({ output_tokens: 0, output_complete: false, total_tokens: 5 }),
    );
    const direct = within(screen.getByText(directSessionLabel).closest("section")!);
    expect(direct.getByText("Input").parentElement?.lastElementChild?.textContent).toBe("5");
    expect(direct.queryByText("Output")).toBeNull();
    expect(direct.queryByText(totalTokensLabel)).toBeNull();
    expect(within(screen.getByTestId(usageSummaryTestId)).queryByText(totalTokensLabel)).toBeNull();
  });

  it("keeps explicitly measured zero counts in a nonempty bucket", () => {
    openMeasuredUsage(
      measuredBucket({
        input_tokens: 0,
        output_tokens: 0,
        total_tokens: 0,
        cached_read_tokens_present: true,
        cached_write_tokens_present: true,
        reasoning_tokens: 0,
      }),
    );
    const direct = within(screen.getByText(directSessionLabel).closest("section")!);
    for (const label of [
      "Input",
      "Output",
      "Cached input",
      "Cache-write input",
      "Reasoning (included in output)",
      totalTokensLabel,
    ]) {
      expect(direct.getByText(label).parentElement?.lastElementChild?.textContent).toBe("0");
    }
    expect(within(screen.getByTestId(usageSummaryTestId)).getByText("0")).toBeTruthy();
  });

  it.each(["total", "direct"] as const)(
    "omits %s overflow caps while retaining unaffected buckets and cost",
    (scope) => {
      const cap = Number("9223372036854775807");
      const valid = measuredBucket({ output_tokens: 4, total_tokens: 9, reasoning_tokens: 3 });
      const saturated = measuredBucket({
        overflow: true,
        input_tokens: cap,
        output_tokens: cap,
        total_tokens: cap,
      });
      openMeasuredUsage(scope === "direct" ? saturated : valid, valid, {
        ...saturated,
        cost_subcents: "12345",
      });
      const summary = within(screen.getByTestId(usageSummaryTestId));
      expect(summary.queryByText(totalTokensLabel)).toBeNull();
      expect(summary.getByText("$1.23")).toBeTruthy();
      expect(summary.getByText("Partial usage")).toBeTruthy();
      const direct = within(screen.getByText(directSessionLabel).closest("section")!);
      expect(direct.queryByText("Input") !== null).toBe(scope !== "direct");
      expect(direct.queryByText("Output") !== null).toBe(scope !== "direct");
      const child = within(screen.getByText("Child agents").closest("section")!);
      expect(child.getByText("9")).toBeTruthy();
      expect(child.getByText("3")).toBeTruthy();
      expect(screen.queryByText(new Intl.NumberFormat().format(cap))).toBeNull();
    },
  );
});

describe("ConversationUsageDisplay session token completeness", () => {
  it.each([false, true])("shows an exact session total only when measured: %s", (complete) => {
    mocks.usage = {
      totals: {
        event_count: 1,
        tokens_total: 20,
        input_tokens_complete: complete,
        output_tokens_complete: true,
        total_tokens_complete: complete,
        unpriced_event_count: 1,
      },
      latestTurn: null,
      detail: null,
      loading: false,
      error: false,
      refresh: vi.fn(),
      loadTurnDetail: mocks.loadTurnDetail,
    };
    render(
      <TooltipProvider>
        <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByTestId(usageTriggerTestId));
    const summary = within(
      screen.getByText(i18n.t("task:conversationUsage.sessionTotal")).closest("section")!,
    );
    const total = summary.queryByText(totalTokensLabel);
    if (complete) {
      expect(total?.parentElement?.lastElementChild?.textContent).toBe(measuredOutputText);
    } else {
      expect(total).toBeNull();
      expect(summary.queryByText(measuredOutputText)).toBeNull();
    }
  });
});

it("retains a measured older response without labeling it latest after a coverage gap", () => {
  const partial = measuredBucket({ input_complete: false, output_complete: false });
  const turn: UsageTurn = {
    turn_id: "gap",
    completeness: "partial",
    direct: partial,
    child: measuredBucket({ event_count: 0 }),
    total: partial,
    cost_sources: [],
    responses: [
      {
        usage_event_id: "older",
        provider_response_id: "older",
        input_tokens: 0,
        input_tokens_present: true,
        output_tokens: 20,
        total_tokens: 20,
        reported_cache_write_tokens: 10,
        cost_source: "unpriced",
        estimated: false,
        occurred_at: "2026-10-14T00:00:00Z",
      },
    ],
  };
  mocks.touchDrawer = true;
  mocks.usage = {
    latestTurn: turn,
    totals: null,
    detail: { turnId: turn.turn_id, turn, loading: false, error: false },
    loading: false,
    error: false,
    refresh: vi.fn(),
    loadTurnDetail: mocks.loadTurnDetail,
  };
  render(
    <TooltipProvider>
      <ConversationUsageDisplay taskId="task-1" sessionId="session-1" />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByTestId(usageTriggerTestId));
  const latest = within(
    screen.getByText(i18n.t("task:conversationUsage.lastResponse")).closest("section")!,
  );
  expect(latest.queryByText(measuredOutputText)).toBeNull();
  const known = within(
    screen.getByText(i18n.t("task:conversationUsage.responseDetails")).closest("section")!,
  );
  expect(known.getByText(measuredOutputText)).toBeTruthy();
  expect(known.getByText("10")).toBeTruthy();
});
