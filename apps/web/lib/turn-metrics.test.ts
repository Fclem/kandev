import { describe, expect, it } from "vitest";
import type { UsageTurn } from "@/lib/types/conversation-usage";
import type { Message, Turn } from "@/lib/types/http";
import { deriveTurnMetrics, lastQualifyingMessageId } from "./turn-metrics";
function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "message-1",
    session_id: "session-1" as Message["session_id"],
    task_id: "task-1" as Message["task_id"],
    turn_id: "turn-1",
    author_type: "agent",
    content: "output",
    type: "message",
    created_at: "2026-09-24T10:00:01.000Z",
    ...overrides,
  };
}

function turn(overrides: Partial<Turn> = {}): Turn {
  return {
    id: "turn-1",
    session_id: "session-1" as Turn["session_id"],
    task_id: "task-1" as Turn["task_id"],
    started_at: "2026-09-24T10:00:00.000Z",
    completed_at: "2026-09-24T10:00:02.000Z",
    created_at: "2026-09-24T10:00:00.000Z",
    updated_at: "2026-09-24T10:00:02.000Z",
    metadata: { timing: { ttft_ms: 350, generation_ms: 2000 } },
    ...overrides,
  };
}

function usage(overrides: Partial<UsageTurn> = {}): UsageTurn {
  const direct = {
    input_tokens: 7,
    cached_read_tokens: 2,
    cached_write_tokens: 1,
    output_tokens: 2,
    thought_tokens: 3,
    total_tokens: 15,
    output_complete: true,
    event_count: 1,
    unpriced_count: 0,
    overflow: false,
  };
  const total = {
    ...direct,
    input_tokens: 17,
    output_tokens: 12,
    total_tokens: 35,
  };
  return {
    turn_id: "turn-1",
    completeness: "exact",
    direct,
    child: { ...direct, output_tokens: 10 },
    total,
    cost_sources: ["models_dev_list"],
    ...overrides,
  };
}

describe("deriveTurnMetrics", () => {
  it("projects turn timing and ledger totals without counting concurrent child output in TPS", () => {
    expect(deriveTurnMetrics(turn(), usage())).toEqual({
      wallTimeMs: 2000,
      ttftMs: 350,
      tokensPerSecond: 2.5,
      inputTokens: 17,
      outputTokens: 12,
      cachedReadTokens: 2,
      cachedWriteTokens: 1,
      reasoningTokens: 3,
      costSubcents: null,
      costSources: [],
    });
  });

  it.each([
    ["estimated", true, null],
    ["partial", true, null],
    ["unknown", false, null],
    ["unknown", true, 2.5],
    ["exact", false, null],
  ] as const)(
    "uses direct output completeness for %s turns",
    (completeness, complete, expected) => {
      const metrics = deriveTurnMetrics(
        turn(),
        usage({ completeness, direct: { ...usage().direct, output_complete: complete } }),
      );
      expect(metrics.tokensPerSecond).toBe(expected);
    },
  );

  it("omits TPS for zero direct output and zero generation windows", () => {
    const noOutput = usage({ direct: { ...usage().direct, output_tokens: 0, thought_tokens: 0 } });
    expect(deriveTurnMetrics(turn(), noOutput).tokensPerSecond).toBeNull();
    expect(
      deriveTurnMetrics(turn({ metadata: { timing: { ttft_ms: 350, generation_ms: 0 } } }), usage())
        .tokensPerSecond,
    ).toBeNull();
  });

  it("omits cost for unpriced turns and keeps measured cost with provenance", () => {
    const measured = usage({ total: { ...usage().total, cost_subcents: "123" } });
    expect(deriveTurnMetrics(turn(), measured).costSubcents).toBe("123");
    expect(
      deriveTurnMetrics(
        turn(),
        usage({
          total: { ...usage().total, cost_subcents: "123", unpriced_count: 1 },
        }),
      ).costSubcents,
    ).toBeNull();
  });

  it("omits unmeasured usage values instead of projecting zero counts", () => {
    const empty = usage({
      total: { ...usage().total, event_count: 0, input_tokens: 0, output_tokens: 0 },
    });
    const metrics = deriveTurnMetrics(turn(), empty);
    expect(metrics.inputTokens).toBeNull();
    expect(metrics.outputTokens).toBeNull();
    expect(metrics.cachedReadTokens).toBeNull();
    expect(metrics.cachedWriteTokens).toBeNull();
  });
  it("uses only direct response reasoning tokens when response details are present", () => {
    const detailed = usage({
      responses: [
        {
          usage_event_id: "direct",
          scope: "direct",
          input_tokens: 0,
          reasoning_output_tokens: 4,
          total_tokens: 4,
          cost_source: "provider_reported",
          estimated: false,
          occurred_at: "2026-09-24T10:00:01.000Z",
        },
        {
          usage_event_id: "child",
          scope: "child",
          input_tokens: 0,
          reasoning_output_tokens: 40,
          total_tokens: 40,
          cost_source: "provider_reported",
          estimated: false,
          occurred_at: "2026-09-24T10:00:01.000Z",
        },
      ],
    });
    expect(deriveTurnMetrics(turn(), detailed).reasoningTokens).toBe(4);
  });
  it("omits zero output counts when the total output measurement is incomplete", () => {
    const partialOutput = usage({
      total: { ...usage().total, output_tokens: 0, output_complete: false },
    });
    expect(deriveTurnMetrics(turn(), partialOutput).outputTokens).toBeNull();
  });
});
describe("lastQualifyingMessageId", () => {
  it("selects the last generic agent message and excludes non-chat adapters", () => {
    const result = lastQualifyingMessageId(
      [
        message({ id: "first" }),
        message({ id: "actions", type: "message", metadata: { actions: ["open"] } }),
        message({ id: "tool", type: "tool_call" }),
        message({ id: "last", type: "content" }),
        message({ id: "later-tool", type: "tool_call" }),
        message({ id: "later-diagnostic", type: "error" }),
        message({
          id: "tool-meta-message",
          type: "message",
          metadata: { tool_name: "mcp__kandev__list_tasks_kandev" },
        }),
      ],
      turn(),
    );
    expect(result).toBe("tool-meta-message");
  });

  it("requires a completed turn and an agent message", () => {
    expect(lastQualifyingMessageId([message()], turn({ completed_at: undefined }))).toBeNull();
    expect(lastQualifyingMessageId([message({ author_type: "user" })], turn())).toBeNull();
    expect(lastQualifyingMessageId([message({ id: "task-description" })], turn())).toBeNull();
  });
});
