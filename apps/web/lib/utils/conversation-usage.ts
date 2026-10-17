import type { UsageTurn } from "@/lib/types/conversation-usage";
import type { Turn } from "@/lib/types/http";
import { parseStrictRfc3339Timestamp } from "./strict-timestamp";

const integerFormatter = new Intl.NumberFormat();

export function usageTokenCount(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

export function formatUsageTokens(value: number): string | null {
  return usageTokenCount(value) === null ? null : integerFormatter.format(value);
}

/** Format ledger subcents (1/100 of a cent) without converting int64 to float. */
export function formatUsageCost(subcents?: string): string | null {
  if (subcents === undefined || !/^-?\d+$/.test(subcents)) return null;
  let amount: bigint;
  try {
    amount = BigInt(subcents);
  } catch {
    return null;
  }
  const zero = BigInt(0);
  const negative = amount < zero;
  const absolute = negative ? -amount : amount;
  const cents = (absolute + BigInt(50)) / BigInt(100);
  const dollars = cents / BigInt(100);
  const remainder = (cents % BigInt(100)).toString().padStart(2, "0");
  return `${negative ? "-$" : "$"}${dollars.toLocaleString()}.${remainder}`;
}

export function usageForCurrentTurn(usage: UsageTurn[], turns: Turn[]): UsageTurn | null {
  let current: Turn | undefined;
  let latestAt: bigint | undefined;
  let ambiguous = false;
  for (const turn of turns) {
    const at = parseStrictRfc3339Timestamp(turn.created_at);
    if (at === null) return null;
    if (latestAt === undefined || at > latestAt) {
      current = turn;
      latestAt = at;
      ambiguous = false;
    } else if (at === latestAt && turn.id !== current?.id) {
      ambiguous = true;
    }
  }
  return ambiguous ? null : (usage.find((turn) => turn.turn_id === current?.id) ?? null);
}

export function usageDetailForTurn<T extends { turnId: string; turn: UsageTurn | null }>(
  latestTurn: UsageTurn | null,
  detail: T | null,
): T | null {
  if (!latestTurn || !detail || detail.turnId !== latestTurn.turn_id) return null;
  if (detail.turn && detail.turn.turn_id !== latestTurn.turn_id) return null;
  return detail;
}
