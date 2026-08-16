import type { StateCreator } from "zustand";
import type { Draft } from "immer";
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
 * Returns the UTC-offset delta in SECONDS for a matched zone, or null when
 * the offset components are out of range.
 */
function parseOffsetSeconds(
  zone: string,
  offsetHourText: string | undefined,
  offsetMinuteText: string | undefined,
): number | null {
  if (zone === "Z") return 0;
  const hour = Number(offsetHourText);
  const minute = Number(offsetMinuteText);
  if (hour > 23 || minute > 59) return null;
  return (zone.startsWith("-") ? -1 : 1) * (hour * 60 + minute) * 60;
}

/**
 * Whether `incoming` is not newer than `existing`. A malformed incoming
 * timestamp counts as stale (never newer).
 */
function isNotNewerThan(incoming: string | undefined, existing: string | undefined): boolean {
  const incomingTs = parseTurnTimestamp(incoming);
  if (incomingTs === null) return true;
  const existingTs = parseTurnTimestamp(existing);
  if (existingTs === null) return false;
  return incomingTs <= existingTs;
}

/** Session states that settle a turn: no agent work is in progress. */
const SETTLED_SESSION_STATES = new Set<string>([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

/**
 * Clears the active marker and records the retired turn id so a delayed WS
 * `session.turn.started` for that turn cannot resurrect it.
 */
function retireActiveTurn(draft: Draft<SessionSlice>, sessionId: string): void {
  const activeId = draft.turns.activeBySession[sessionId];
  if (!activeId) return;
  draft.turns.activeBySession[sessionId] = null;
  const retired = draft.turns.retiredActiveTurnIdsBySession[sessionId] ?? [];
  if (!retired.includes(activeId)) retired.push(activeId);
  draft.turns.retiredActiveTurnIdsBySession[sessionId] = retired;
}

/** Whether a WS start (or hydration candidate) is for a retired turn. */
function isRetiredTurn(draft: Draft<SessionSlice>, sessionId: string, turnId: string): boolean {
  return (draft.turns.retiredActiveTurnIdsBySession[sessionId] ?? []).includes(turnId);
}

function isSettledSessionState(state: string): boolean {
  return SETTLED_SESSION_STATES.has(state);
}

/**
 * Returns the id of the latest incomplete turn in the list, or null when
 * every turn is completed. Compares via the strict timestamp parser so
 * fractional (RFC3339Nano) and whole-second (RFC3339) started_at values
 * order correctly.
 */
export function latestIncompleteTurnId(turns: Turn[]): string | null {
  let latestId: string | null = null;
  let latestStarted: bigint | null = null;
  for (const turn of turns) {
    if (turn.completed_at) continue;
    const started = parseTurnTimestamp(turn.started_at);
    if (started === null) continue;
    if (latestStarted === null || started > latestStarted) {
      latestId = turn.id;
      latestStarted = started;
    }
  }
  return latestId;
}

/**
 * Establishes (or clears) the active-turn marker after a full REST hydration,
 * applying the same settled-session rule as reconcileActiveTurnForIdleSession
 * and rejecting hydrations that started before an authoritative clear.
 */
function applyActiveTurnReconciliation(
  draft: Draft<SessionSlice>,
  sessionId: string,
  hydrationEpoch: number,
): void {
  if ((draft.turns.reconcileEpochBySession[sessionId] ?? 0) !== hydrationEpoch) return;
  const turns = (draft.turns.bySession[sessionId] ?? []).filter(
    (turn) => !isRetiredTurn(draft, sessionId, turn.id),
  );
  const latestId = latestIncompleteTurnId(turns);
  if (latestId === null) {
    draft.turns.activeBySession[sessionId] = null;
    return;
  }
  // Mirror reconcileActiveTurnForIdleSession: a settled session whose
  // snapshot is at least as new as the candidate turn's start has no active
  // turn — an orphaned incomplete turn must not report work in progress
  // (files-panel source gating reads this marker).
  const latestTurn = turns.find((turn) => turn.id === latestId);
  const session = draft.taskSessions.items[sessionId];
  if (session && latestTurn && isSettledSessionState(session.state)) {
    const sessionUpdatedAt = Date.parse(session.updated_at);
    const turnStartedAt = Date.parse(latestTurn.started_at);
    if (
      !Number.isNaN(sessionUpdatedAt) &&
      !Number.isNaN(turnStartedAt) &&
      turnStartedAt <= sessionUpdatedAt
    ) {
      draft.turns.activeBySession[sessionId] = null;
      return;
    }
  }
  draft.turns.activeBySession[sessionId] = latestId;
}

/**
 * Parses a turn `updated_at` into a comparable epoch in nanoseconds (BigInt).
 * Missing, empty, malformed, or non-RFC3339 values map to `null` (stale), so
 * they can never clobber a row with a valid timestamp. Calendar/time
 * components are validated explicitly because Date.parse NORMALIZES
 * semantically invalid values (e.g. `2026-02-30T10:00:00Z` becomes Mar 2)
 * instead of rejecting them. BigInt nanoseconds preserve Go RFC3339Nano
 * payloads exactly — float64 ms epochs have a ~244ns ULP around 2026, which
 * would collapse legitimate sub-100ns differences and misorder rows.
 */
export function parseTurnTimestamp(value: string | undefined): bigint | null {
  if (!value) return null;
  const match = RFC3339_PATTERN.exec(value);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  if (month < 1 || month > 12) return null;
  if (!validDayForMonth(year, month, Number(match[3]))) return null;
  if (Number(match[4]) > 23 || Number(match[5]) > 59 || Number(match[6]) > 59) {
    return null;
  }
  const offsetSeconds = parseOffsetSeconds(match[8], match[9], match[10]);
  if (offsetSeconds === null) return null;
  // RFC3339Nano caps the fraction at 9 digits; longer fractions would
  // overflow into the seconds component (padEnd does not cap).
  if (match[7] !== undefined && match[7].length > 9) return null;
  // setUTCFullYear handles years 0-99 correctly (Date.UTC maps them to
  // 1900+year); the components are validated above, so no normalization can
  // occur here.
  const utc = new Date(0);
  utc.setUTCFullYear(year, month - 1, Number(match[3]));
  utc.setUTCHours(Number(match[4]), Number(match[5]), Number(match[6]), 0);
  const wholeSeconds = BigInt(Math.floor(utc.getTime() / 1000)) - BigInt(offsetSeconds);
  const fractionNs = BigInt((match[7] ?? "").padEnd(9, "0"));
  return wholeSeconds * BigInt(1_000_000_000) + fractionNs;
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
  const incomingTs = parseTurnTimestamp(incoming.updated_at);
  const existingTs = parseTurnTimestamp(existing.updated_at);
  if (incomingTs === null) return false;
  if (existingTs === null) return true;
  return incomingTs > existingTs;
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
          // completion fields here when the event is not provably stale.
          // On an already-completed row, a missing, malformed, or
          // equal/older `updated_at` is stale — the same null policy as
          // isNotNewerThan, with an absent incoming timestamp treated as
          // stale too (the event type always carries updated_at).
          const stale =
            turn.completed_at !== undefined &&
            (updatedAt === undefined || isNotNewerThan(updatedAt, turn.updated_at));
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
    reconcileActiveTurnAfterHydration: (sessionId: string, hydrationEpoch: number) =>
      set((draft) => {
        applyActiveTurnReconciliation(draft, sessionId, hydrationEpoch);
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
          retireActiveTurn(draft, sessionId);
          // Bump the generation so an in-flight REST hydration cannot
          // resurrect the marker from a pre-adoption snapshot.
          draft.turns.reconcileEpochBySession[sessionId] =
            (draft.turns.reconcileEpochBySession[sessionId] ?? 0) + 1;
        }
      }),
  };
}
