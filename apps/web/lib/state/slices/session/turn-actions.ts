import type { StateCreator } from "zustand";
import type { SessionSlice } from "./types";
import type { Turn } from "@/lib/types/http";

type ImmerSet = Parameters<
  StateCreator<SessionSlice, [["zustand/immer", never]], [], SessionSlice>
>[0];

// Strict wire format: full RFC3339 with explicit offset/UTC marker. JS
// Date.parse is permissive (e.g. "0" parses as 2000-01-01, partial dates as
// midnight, timezone-less values in the local zone), so shape validation must
// come first or malformed rows would win freshness comparisons.
const RFC3339_PATTERN =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-](\d{2}):(\d{2}))$/;

const DAYS_IN_MONTH = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

function isLeapYear(year: number): boolean {
  return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
}

/** Whether the day exists in the month (leap-year aware). */
function validDayForMonth(year: number, month: number, day: number): boolean {
  if (day < 1) return false;
  const maxDay = month === 2 && isLeapYear(year) ? 29 : DAYS_IN_MONTH[month - 1];
  return day <= maxDay;
}

/**
 * Returns the UTC-offset delta in milliseconds for a matched zone, or null
 * when the offset components are out of range.
 */
function parseOffsetDelta(
  zone: string,
  offsetHourText: string | undefined,
  offsetMinuteText: string | undefined,
): number | null {
  if (zone === "Z") return 0;
  const hour = Number(offsetHourText);
  const minute = Number(offsetMinuteText);
  if (hour > 23 || minute > 59) return null;
  return (zone.startsWith("-") ? -1 : 1) * (hour * 60 + minute) * 60_000;
}

/**
 * Parses a turn `updated_at` into a comparable epoch. Missing, empty,
 * malformed, or non-RFC3339 values map to `-Infinity` (stale), so they can
 * never clobber a row with a valid timestamp. Calendar/time components are
 * validated explicitly because Date.parse NORMALIZES semantically invalid
 * values (e.g. `2026-02-30T10:00:00Z` becomes Mar 2) instead of rejecting
 * them, which would let malformed rows win freshness comparisons.
 */
export function parseTurnTimestamp(value: string | undefined): number {
  if (!value) return -Infinity;
  const match = RFC3339_PATTERN.exec(value);
  if (!match) return -Infinity;
  const year = Number(match[1]);
  const month = Number(match[2]);
  if (month < 1 || month > 12) return -Infinity;
  if (!validDayForMonth(year, month, Number(match[3]))) return -Infinity;
  if (Number(match[4]) > 23 || Number(match[5]) > 59 || Number(match[6]) > 59) {
    return -Infinity;
  }
  const offsetDelta = parseOffsetDelta(match[8], match[9], match[10]);
  if (offsetDelta === null) return -Infinity;
  // Add the fraction EXACTLY (no rounding): Math.round collapses
  // .9995 onto the next second's epoch, and genuinely newer sub-millisecond
  // WS timestamps (RFC3339Nano) would compare equal and lose the strict `>`.
  const fractionMs = match[7] ? Number(`0.${match[7]}`) * 1000 : 0;
  // setUTCFullYear handles years 0-99 correctly (Date.UTC maps them to
  // 1900+year); the components are validated above, so no normalization can
  // occur here.
  const utc = new Date(0);
  utc.setUTCFullYear(year, month - 1, Number(match[3]));
  utc.setUTCHours(Number(match[4]), Number(match[5]), Number(match[6]), 0);
  const epoch = utc.getTime() + fractionMs - offsetDelta;
  return Number.isFinite(epoch) ? epoch : -Infinity;
}

/**
 * Decides whether an incoming turn row should replace the store's existing
 * row for the same turn. Completion state takes precedence over timestamps:
 * WS rows carry RFC3339Nano fractions while the REST DTO truncates to whole
 * seconds, so a completion within the same second can look equal or older —
 * but a completed row is always more advanced than an incomplete one, and an
 * existing completion must never be rolled back. Only when the completion
 * states agree do we fall back to `updated_at` freshness. This guards every
 * write path (WS events, REST hydration) against stale rows clobbering newer
 * live data.
 */
export function shouldApplyTurnUpdate(existing: Turn, incoming: Turn): boolean {
  const incomingCompleted = !!incoming.completed_at;
  const existingCompleted = !!existing.completed_at;
  if (incomingCompleted && !existingCompleted) return true;
  if (existingCompleted && !incomingCompleted) return false;
  return parseTurnTimestamp(incoming.updated_at) > parseTurnTimestamp(existing.updated_at);
}

export function buildTurnActions(set: ImmerSet) {
  return {
    addTurn: (turn: Parameters<SessionSlice["addTurn"]>[0]) =>
      set((draft) => {
        const sessionId = turn.session_id;
        if (!draft.turns.bySession[sessionId]) draft.turns.bySession[sessionId] = [];
        const existing = draft.turns.bySession[sessionId].find((item) => item.id === turn.id);
        if (!existing) {
          draft.turns.bySession[sessionId].push(turn);
          return;
        }
        if (!shouldApplyTurnUpdate(existing, turn)) return;
        Object.assign(existing, turn, { completed_at: existing.completed_at ?? turn.completed_at });
      }),
    completeTurn: (
      sessionId: Parameters<SessionSlice["completeTurn"]>[0],
      turnId: Parameters<SessionSlice["completeTurn"]>[1],
      completedAt: Parameters<SessionSlice["completeTurn"]>[2],
      metadata: Parameters<SessionSlice["completeTurn"]>[3],
      updatedAt: Parameters<SessionSlice["completeTurn"]>[4],
    ) =>
      set((draft) => {
        const turn = draft.turns.bySession[sessionId]?.find((item) => item.id === turnId);
        if (turn) {
          // addTurn already applied the completed payload; only apply the
          // completion fields here when the event is not provably stale
          // (equal/older updated_at on an already-completed row).
          const stale =
            updatedAt !== undefined &&
            turn.updated_at !== undefined &&
            turn.completed_at !== undefined &&
            parseTurnTimestamp(updatedAt) <= parseTurnTimestamp(turn.updated_at);
          if (!stale) {
            turn.completed_at = completedAt;
            if (metadata) turn.metadata = metadata;
          }
        }
        if (draft.turns.activeBySession[sessionId] === turnId) {
          draft.turns.activeBySession[sessionId] = null;
        }
      }),
    markTurnsLoaded: (sessionId: string) =>
      set((draft) => {
        draft.turns.loadedBySession[sessionId] = true;
      }),
    setActiveTurn: (
      sessionId: Parameters<SessionSlice["setActiveTurn"]>[0],
      turnId: Parameters<SessionSlice["setActiveTurn"]>[1],
    ) =>
      set((draft) => {
        if (!turnId) {
          draft.turns.activeBySession[sessionId] = null;
          return;
        }
        const turns = draft.turns.bySession[sessionId] ?? [];
        const next = turns.find((turn) => turn.id === turnId);
        if (!next || next.completed_at) return;

        const currentId = draft.turns.activeBySession[sessionId];
        const current = turns.find((turn) => turn.id === currentId);
        if (!current || current.completed_at) {
          draft.turns.activeBySession[sessionId] = turnId;
          return;
        }
        if (current.id === turnId) return;

        const currentStartedAt = Date.parse(current.started_at);
        const nextStartedAt = Date.parse(next.started_at);
        if (
          !Number.isNaN(currentStartedAt) &&
          !Number.isNaN(nextStartedAt) &&
          nextStartedAt > currentStartedAt
        ) {
          draft.turns.activeBySession[sessionId] = turnId;
        }
      }),
    reconcileWorkspaceSourcesAdopted: (
      sessionIds: Parameters<SessionSlice["reconcileWorkspaceSourcesAdopted"]>[0],
    ) =>
      set((draft) => {
        for (const sessionId of new Set(sessionIds)) {
          draft.turns.activeBySession[sessionId] = null;
        }
      }),
  };
}
