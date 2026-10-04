import type { Message, Turn } from "@/lib/types/http";
import { messageAdapterRendersActions } from "@/components/task/chat/message-adapter-matchers";
import type { UsageTurn } from "@/lib/types/conversation-usage";
const qualifyingMessageCache = new WeakMap<Message[], WeakMap<Turn, string | null>>();

export function lastQualifyingMessageId(messages: Message[], turn: Turn): string | null {
  let turnCache = qualifyingMessageCache.get(messages);
  if (!turnCache) {
    turnCache = new WeakMap();
    qualifyingMessageCache.set(messages, turnCache);
  }
  if (turnCache.has(turn)) return turnCache.get(turn) ?? null;
  if (!turn.completed_at) {
    turnCache.set(turn, null);
    return null;
  }
  const childrenByParentToolCallId = new Map<string, Message[]>();
  for (const message of messages) {
    const parentId = message.metadata?.parent_tool_call_id;
    if (typeof parentId !== "string") continue;
    const children = childrenByParentToolCallId.get(parentId) ?? [];
    children.push(message);
    childrenByParentToolCallId.set(parentId, children);
  }
  let lastMessageId: string | null = null;
  for (const message of messages) {
    if (
      message.turn_id === turn.id &&
      message.author_type === "agent" &&
      messageAdapterRendersActions(message, {
        childrenByParentToolCallId,
        isTaskDescription: message.id === "task-description",
      })
    ) {
      lastMessageId = message.id;
    }
  }
  turnCache.set(turn, lastMessageId);
  return lastMessageId;
}

export type TurnMetrics = {
  wallTimeMs: number | null;
  ttftMs: number | null;
  tokensPerSecond: number | null;
  inputTokens: number | null;
  outputTokens: number | null;
  cachedReadTokens: number | null;
  cachedWriteTokens: number | null;
  reasoningTokens: number | null;
  costSubcents: string | null;
  costSources: string[];
};

function nonNegativeNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function timingValue(turn: Turn, key: "ttft_ms" | "generation_ms"): number | null {
  const timing = turn.metadata?.timing;
  if (!timing || typeof timing !== "object") return null;
  return nonNegativeNumber((timing as Record<string, unknown>)[key]);
}

function wallTime(turn: Turn): number | null {
  if (!turn.completed_at) return null;
  const startedAt = Date.parse(turn.started_at);
  const completedAt = Date.parse(turn.completed_at);
  if (!Number.isFinite(startedAt) || !Number.isFinite(completedAt) || completedAt < startedAt) {
    return null;
  }
  return completedAt - startedAt;
}

function directOutputRate(turn: Turn, usage: UsageTurn | null): number | null {
  if (!usage || usage.completeness === "estimated" || usage.completeness === "partial") {
    return null;
  }
  const generationMs = timingValue(turn, "generation_ms");
  const outputTokens = nonNegativeNumber(usage.direct.output_tokens);
  const thoughtTokens = nonNegativeNumber(usage.direct.thought_tokens);
  if (
    !usage.direct.output_complete ||
    usage.direct.event_count === 0 ||
    generationMs === null ||
    generationMs === 0 ||
    outputTokens === null ||
    thoughtTokens === null
  ) {
    return null;
  }
  const generatedTokens = outputTokens + thoughtTokens;
  return generatedTokens > 0 ? (generatedTokens * 1000) / generationMs : null;
}

function reasoningTokenCount(usage: UsageTurn): number {
  const responseCounts = usage.responses
    ?.filter((response) => response.scope !== "child")
    .map((response) => response.reasoning_output_tokens)
    .filter(
      (value): value is number => typeof value === "number" && Number.isFinite(value) && value >= 0,
    );
  if (responseCounts && responseCounts.length > 0) {
    return responseCounts.reduce((total, value) => total + value, 0);
  }
  return usage.direct.thought_tokens;
}

function measuredUsage(usage: UsageTurn | null): UsageTurn | null {
  if (!usage || usage.total.event_count === 0) return null;
  return usage;
}

function pricedCost(usage: UsageTurn | null): string | null {
  if (!usage || usage.total.unpriced_count !== 0) return null;
  return usage.total.cost_subcents ?? null;
}

function totalOutputTokens(usage: UsageTurn | null): number | null {
  if (!usage) return null;
  if (!usage.total.output_complete && usage.total.output_tokens === 0) return null;
  return usage.total.output_tokens;
}

function costSources(usage: UsageTurn | null, cost: string | null): string[] {
  return usage && cost !== null ? [...usage.cost_sources] : [];
}

export function deriveTurnMetrics(turn: Turn, usage: UsageTurn | null): TurnMetrics {
  const measured = measuredUsage(usage);
  const cost = pricedCost(measured);
  return {
    wallTimeMs: wallTime(turn),
    ttftMs: timingValue(turn, "ttft_ms"),
    tokensPerSecond: directOutputRate(turn, usage),
    inputTokens: measured?.total.input_tokens ?? null,
    outputTokens: totalOutputTokens(measured),
    cachedReadTokens: measured?.total.cached_read_tokens ?? null,
    cachedWriteTokens: measured?.total.cached_write_tokens ?? null,
    reasoningTokens: measured ? reasoningTokenCount(measured) : null,
    costSubcents: cost,
    costSources: costSources(measured, cost),
  };
}
