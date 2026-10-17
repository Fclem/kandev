import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { UsageTurn } from "@/lib/types/conversation-usage";
import type { Turn } from "@/lib/types/http";
import { MessageMetadataUsageItems } from "./message-metadata-usage-items";

const USAGE_READ = vi.hoisted(() => ({ usageTurn: null as UsageTurn | null }));

vi.mock("@/hooks/domains/session/use-session-usage-turns", () => ({
  useSessionUsageTurn: () => USAGE_READ,
}));

const direct = {
  input_tokens: 3,
  input_complete: true,
  cached_read_tokens: 1,
  cached_write_tokens: 0,
  cached_read_tokens_present: true,
  cached_write_tokens_present: false,
  output_tokens: 4,
  thought_tokens: 0,
  total_tokens: 8,
  output_complete: true,
  event_count: 1,
  unpriced_count: 0,
  overflow: false,
};

const turn: Turn = {
  id: "turn-1",
  session_id: "session-1" as Turn["session_id"],
  task_id: "task-1" as Turn["task_id"],
  started_at: "2026-09-24T10:00:00.000Z",
  completed_at: "2026-09-24T10:00:02.000Z",
  created_at: "2026-09-24T10:00:00.000Z",
  updated_at: "2026-09-24T10:00:02.000Z",
  metadata: { timing: { ttft_ms: 350, generation_ms: 2000 } },
};

afterEach(() => {
  cleanup();
  USAGE_READ.usageTurn = null;
});

describe("MessageMetadataUsageItems", () => {
  it("exposes the tokens-per-second definition through a keyboard-focusable tooltip", async () => {
    USAGE_READ.usageTurn = {
      turn_id: "turn-1",
      completeness: "exact",
      direct,
      child: direct,
      total: direct,
      cost_sources: [],
    };
    const { container } = render(
      <TooltipProvider>
        <MessageMetadataUsageItems
          taskId="task-1"
          sessionId="session-1"
          turn={turn}
          fields={["tps"]}
        />
      </TooltipProvider>,
    );

    const tps = screen.getByTestId("message-metadata-tps");
    expect(tps.getAttribute("tabindex")).toBe("0");
    expect(container.querySelector("[title]")).toBeNull();
    fireEvent.focus(tps);
    expect((await screen.findByRole("tooltip")).textContent).toContain(
      "Average direct output and reasoning token rate",
    );
  });
});
