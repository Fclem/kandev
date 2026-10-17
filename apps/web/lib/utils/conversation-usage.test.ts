import { describe, expect, it } from "vitest";
import type { UsageTurn } from "@/lib/types/conversation-usage";
import type { Turn } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import {
  formatUsageCost,
  formatUsageTokens,
  usageDetailForTurn,
  usageForCurrentTurn,
} from "./conversation-usage";

describe("conversation usage formatting", () => {
  it("formats token counts and int64 subcent prices without float conversion", () => {
    expect(formatUsageTokens(1200)).toBe("1,200");
    expect(formatUsageCost("150000")).toBe("$15.00");
    expect(formatUsageCost("9223372036854775807")).toBe("$922,337,203,685,477.58");
    expect(formatUsageCost("0")).toBe("$0.00");
  });

  it("omits JSON-rounded or invalid counts and retains the largest safe integer", () => {
    const rounded = JSON.parse("9007199254740993") as number;
    expect(formatUsageTokens(rounded)).toBeNull();
    expect(formatUsageTokens(-1)).toBeNull();
    expect(formatUsageTokens(0.5)).toBeNull();
    expect(formatUsageTokens(Number.MAX_SAFE_INTEGER)).toBe("9,007,199,254,740,991");
    expect(formatUsageTokens(0)).toBe("0");
  });

  it("returns unavailable for missing or malformed prices", () => {
    expect(formatUsageCost(undefined)).toBeNull();
    expect(formatUsageCost("not-a-number")).toBeNull();
  });

  it("uses detail only when it belongs to the refreshed latest turn", () => {
    const latest = { turn_id: "new" } as UsageTurn;
    const stale = {
      turnId: "old",
      turn: { turn_id: "old" } as UsageTurn,
      loading: false,
    };
    expect(usageDetailForTurn(latest, stale)).toBeNull();

    const loading = { turnId: "new", turn: null, loading: true };
    expect(usageDetailForTurn(latest, loading)).toBe(loading);

    const mismatchedResponse = {
      turnId: "new",
      turn: { turn_id: "old" } as UsageTurn,
      loading: false,
    };
    expect(usageDetailForTurn(latest, mismatchedResponse)).toBeNull();
  });
});

describe("current conversation turn usage", () => {
  const conversation = (id: string, created_at: string): Turn => ({
    id,
    created_at,
    started_at: created_at,
    updated_at: created_at,
    session_id: sessionId("session"),
    task_id: taskId("task"),
  });
  const usage = (turn_id: string) => ({ turn_id }) as UsageTurn;

  it("selects matching usage using exact conversation chronology, not index or arrival order", () => {
    const older = conversation("older", "2026-10-14T00:00:00.000000001Z");
    const newer = conversation("newer", "2026-10-14T01:00:00.000000002+01:00");
    expect(usageForCurrentTurn([usage("older"), usage("newer")], [newer, older])?.turn_id).toBe(
      "newer",
    );
  });

  it("omits current usage when only an older turn has measurements or chronology is unknown", () => {
    const older = conversation("older", "2026-10-14T00:00:00Z");
    const newer = conversation("newer", "2026-10-14T00:00:02Z");
    expect(usageForCurrentTurn([usage("older")], [older, newer])).toBeNull();
    expect(usageForCurrentTurn([usage("older")], [])).toBeNull();
    expect(
      usageForCurrentTurn(
        [usage("older")],
        [older, conversation("unknown", "2026-02-30T00:00:00Z")],
      ),
    ).toBeNull();
  });

  it("does not attribute usage when distinct latest turns share an indistinguishable timestamp", () => {
    const at = "2026-10-14T00:00:00Z";
    expect(
      usageForCurrentTurn(
        [usage("first")],
        [conversation("first", at), conversation("second", at)],
      ),
    ).toBeNull();
  });
});
