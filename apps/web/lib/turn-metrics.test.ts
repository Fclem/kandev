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
    input_complete: true,
    cached_read_tokens: 2,
    cached_write_tokens: 1,
    cached_read_tokens_present: true,
    cached_write_tokens_present: true,
    output_tokens: 2,
    thought_tokens: 3,
    reasoning_tokens: 3,
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
      lastResponseReportedCacheWriteTokens: null,
      reasoningTokens: 3,
      costSubcents: null,
      costSources: [],
    });
  });
});

describe("deriveTurnMetrics input measurements", () => {
  it.each([
    ["absent", false, 0, null],
    ["measured zero", true, 0, 0],
    ["measured positive", true, 5, 5],
    ["partly measured aggregate", false, 5, null],
  ] as const)(
    "keeps %s input distinct without discarding output",
    (_, complete, input, expected) => {
      const measured = usage({
        total: {
          ...usage().total,
          input_complete: complete,
          input_tokens: input,
          output_tokens: 20,
        },
      });
      const metrics = deriveTurnMetrics(turn(), measured);
      expect(metrics.inputTokens).toBe(expected);
      expect(metrics.outputTokens).toBe(20);
    },
  );
});

it("omits positive output subtotals when another ledger row has unknown output", () => {
  const measured = usage({
    total: { ...usage().total, event_count: 2, output_tokens: 20, output_complete: false },
  });
  const metrics = deriveTurnMetrics(turn(), measured);
  expect(metrics.outputTokens).toBeNull();
  expect(metrics.inputTokens).toBe(17);
});

it("retains reported cache-write as response detail without pricing or additive cache write", () => {
  const measured = usage({
    total: {
      ...usage().total,
      cached_write_tokens_present: false,
      cached_write_tokens: 0,
      unpriced_count: 1,
    },
    last_response: {
      usage_event_id: "native-response",
      input_tokens: 5,
      input_tokens_present: true,
      output_tokens: 20,
      reported_cache_write_tokens: 10,
      total_tokens: 25,
      cost_source: "unpriced",
      estimated: false,
      occurred_at: "2026-09-24T10:00:02Z",
    },
  });
  expect(deriveTurnMetrics(turn(), measured).lastResponseReportedCacheWriteTokens).toBe(10);
  expect(deriveTurnMetrics(turn(), measured).cachedWriteTokens).toBeNull();
  expect(deriveTurnMetrics(turn(), measured).costSubcents).toBeNull();
});

describe("deriveTurnMetrics token precision", () => {
  it("omits rounded wire counts without discarding safe output, timing or cost", () => {
    const rounded = JSON.parse("9007199254740993") as number;
    const measured = usage();
    measured.total.input_tokens = rounded;
    measured.total.cached_read_tokens = rounded;
    measured.total.cached_write_tokens = rounded;
    measured.direct.reasoning_tokens = rounded;
    measured.total.cost_subcents = "12345";
    const metrics = deriveTurnMetrics(turn(), measured);
    expect(metrics.inputTokens).toBeNull();
    expect(metrics.cachedReadTokens).toBeNull();
    expect(metrics.cachedWriteTokens).toBeNull();
    expect(metrics.reasoningTokens).toBeNull();
    expect(metrics.outputTokens).toBe(12);
    expect(metrics.ttftMs).toBe(350);
    expect(metrics.costSubcents).toBe("12345");
  });

  it.each([
    [JSON.parse("9007199254740993") as number, 0],
    [Number.MAX_SAFE_INTEGER, 1],
  ])("omits an unsafe direct rate numerator %s plus %s", (output, thought) => {
    const measured = usage();
    measured.direct.output_tokens = output;
    measured.direct.thought_tokens = thought;
    measured.total.output_tokens = output;
    const metrics = deriveTurnMetrics(turn(), measured);
    expect(metrics.tokensPerSecond).toBeNull();
    if (!Number.isSafeInteger(output)) expect(metrics.outputTokens).toBeNull();
  });
  it("retains the exact safe-boundary half-token rate over two seconds", () => {
    const measured = usage();
    measured.direct.output_tokens = Number.MAX_SAFE_INTEGER;
    measured.direct.thought_tokens = 0;
    expect(deriveTurnMetrics(turn(), measured).tokensPerSecond).toBe(Number.MAX_SAFE_INTEGER / 2);
  });
});

describe("deriveTurnMetrics wall time", () => {
  it.each([
    ["0", turn().completed_at],
    [turn().started_at, "2026-09-24 10:00:02"],
    ["2026-02-30T10:00:00Z", "2026-03-02T10:00:02Z"],
    ["2026-02-28T10:00:00Z", "2026-02-30T10:00:02Z"],
  ])("omits wall time for malformed turn bounds %s and %s", (started_at, completed_at) => {
    expect(deriveTurnMetrics(turn({ started_at, completed_at }), null).wallTimeMs).toBeNull();
  });

  it("derives wall time from valid offset timestamps", () => {
    const metrics = deriveTurnMetrics(
      turn({
        started_at: "2026-09-24T12:00:00.000+02:00",
        completed_at: turn().completed_at,
      }),
      null,
    );
    expect(metrics.wallTimeMs).toBe(2000);
  });
});

describe("deriveTurnMetrics usage eligibility", () => {
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
  it("omits zero output counts when the total output measurement is incomplete", () => {
    const partialOutput = usage({
      total: { ...usage().total, output_tokens: 0, output_complete: false },
    });
    expect(deriveTurnMetrics(turn(), partialOutput).outputTokens).toBeNull();
  });
});
describe("deriveTurnMetrics zero-count rows", () => {
  it.each([
    [false, 0, true, 0, { cachedReadTokens: null, cachedWriteTokens: 0 }],
    [true, 0, false, 0, { cachedReadTokens: 0, cachedWriteTokens: null }],
    [true, 2, true, 1, { cachedReadTokens: 2, cachedWriteTokens: 1 }],
    [false, 0, false, 0, { cachedReadTokens: null, cachedWriteTokens: null }],
  ] as const)(
    "projects cache measurements by presence (%s, %s; %s, %s)",
    (readPresent, readTokens, writePresent, writeTokens, expected) => {
      const total = {
        ...usage().total,
        cached_read_tokens: readTokens,
        cached_read_tokens_present: readPresent,
        cached_write_tokens: writeTokens,
        cached_write_tokens_present: writePresent,
      };
      const metrics = deriveTurnMetrics(turn(), usage({ total }));
      expect({
        cachedReadTokens: metrics.cachedReadTokens,
        cachedWriteTokens: metrics.cachedWriteTokens,
      }).toEqual(expected);
    },
  );
  it("shows aggregated ACP reasoning when response details are absent", () => {
    expect(deriveTurnMetrics(turn(), usage()).reasoningTokens).toBe(3);
  });
});
describe("deriveTurnMetrics indexed reasoning", () => {
  it.each([
    [1, 0],
    [1, 20],
    [2, 30],
  ])(
    "shows measured direct reasoning for %s indexed response(s)",
    (eventCount, reasoningTokens) => {
      const direct = {
        ...usage().direct,
        thought_tokens: 0,
        reasoning_tokens: reasoningTokens,
        event_count: eventCount,
      };
      const child = { ...usage().child, reasoning_tokens: 500 };
      const indexed = usage({
        direct,
        child,
        responses: undefined,
        last_response: {
          usage_event_id: "latest-response",
          scope: "direct",
          input_tokens: 7,
          input_tokens_present: true,
          reasoning_output_tokens: Math.min(reasoningTokens, 20),
          total_tokens: 20,
          cost_source: "unpriced",
          estimated: false,
          occurred_at: "2026-09-24T10:00:01.000Z",
        },
      });
      expect(deriveTurnMetrics(turn(), indexed).reasoningTokens).toBe(reasoningTokens);
    },
  );
});

describe("deriveTurnMetrics saturated counts", () => {
  it("omits an overflowed total bucket but retains valid direct reasoning, timing, and cost", () => {
    const overflowing = usage({
      completeness: "partial",
      total: {
        ...usage().total,
        input_tokens: Number("9223372036854775807"),
        overflow: true,
        cost_subcents: "123",
      },
    });
    const metrics = deriveTurnMetrics(turn(), overflowing);
    expect(metrics.inputTokens).toBeNull();
    expect(metrics.outputTokens).toBeNull();
    expect(metrics.cachedReadTokens).toBeNull();
    expect(metrics.cachedWriteTokens).toBeNull();
    expect(metrics.reasoningTokens).toBe(3);
    expect(metrics.costSubcents).toBe("123");
    expect(metrics.ttftMs).toBe(350);
    expect(metrics.tokensPerSecond).toBeNull();
  });

  it("omits direct reasoning and TPS when the direct bucket overflowed", () => {
    const overflowing = usage({
      completeness: "partial",
      direct: {
        ...usage().direct,
        reasoning_tokens: Number("9223372036854775807"),
        overflow: true,
      },
      total: { ...usage().total, overflow: true },
    });
    const metrics = deriveTurnMetrics(turn(), overflowing);
    expect(metrics.reasoningTokens).toBeNull();
    expect(metrics.tokensPerSecond).toBeNull();
  });

  it("keeps non-overflowed partial measurements visible", () => {
    const metrics = deriveTurnMetrics(turn(), usage({ completeness: "partial" }));
    expect(metrics.inputTokens).toBe(17);
    expect(metrics.outputTokens).toBe(12);
    expect(metrics.cachedReadTokens).toBe(2);
    expect(metrics.cachedWriteTokens).toBe(1);
    expect(metrics.reasoningTokens).toBe(3);
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
