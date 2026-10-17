import type { Message, Turn } from "@/lib/types/http";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";

export type PromptDurationUnits = {
  s: string;
  m: string;
  h: string;
};

/**
 * Return a completed user prompt turn's wall-clock duration in whole seconds.
 * The bound is the turn's own completion timestamp — a caller that needs a
 * next-prompt bound derives it separately. Timestamps are parsed at full
 * nanosecond precision, floored to seconds, and clamped at zero so clock skew
 * cannot produce a negative duration.
 */
export function messageTurnDurationSeconds(message: Message, turn: Turn | null): number | null {
  if (
    message.author_type !== "user" ||
    !message.turn_id ||
    turn === null ||
    turn.id !== message.turn_id ||
    turn.session_id !== message.session_id
  ) {
    return null;
  }

  const createdAt = parseStrictRfc3339Timestamp(message.created_at);
  const completedAt = parseStrictRfc3339Timestamp(turn.completed_at);
  if (createdAt === null || completedAt === null) return null;

  const durationNs = completedAt - createdAt;
  return Number(durationNs < BigInt(0) ? BigInt(0) : durationNs / BigInt(1_000_000_000));
}

/** Format a duration in seconds as a compact `h m s` string using the given unit labels, omitting empty hour/minute parts. */
export function formatPromptDuration(seconds: number, units: PromptDurationUnits): string {
  const totalSeconds = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const remainingSeconds = totalSeconds % 60;

  if (hours > 0) return `${hours}${units.h} ${minutes}${units.m} ${remainingSeconds}${units.s}`;
  if (minutes > 0) return `${minutes}${units.m} ${remainingSeconds}${units.s}`;
  return `${remainingSeconds}${units.s}`;
}

/**
 * Format a time-to-first-token in milliseconds: whole milliseconds below one
 * second, one decimal of seconds below one minute, then the compact `h m s`
 * form. Sub-second values are never floored to `0s`.
 */
export function formatTtft(milliseconds: number, units: PromptDurationUnits): string {
  const ms = Math.max(0, milliseconds);
  if (Math.round(ms) < 1000) return `${Math.round(ms)} ms`; // i18n-exempt: SI unit symbol
  const tenths = Math.round(ms / 100);
  if (tenths < 600) return `${(tenths / 10).toFixed(1)}${units.s}`;
  return formatPromptDuration(Math.round(ms / 1000), units);
}
