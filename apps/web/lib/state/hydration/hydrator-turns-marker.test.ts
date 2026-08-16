import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

// Dedicated file for the turns `loadedBySession` marker predicates so
// hydrator.test.ts stays under the repo's 600-line file cap.

const SESSION_ID = "session-1";
const BOUNDARY = "2026-01-01T00:00:00Z";
const FRESH_TURN = "fresh-turn";

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

describe("hydrateState — turns loadedBySession marker predicates", () => {
  it.each([
    ["active", "active-session"],
    ["pre-existing non-active", SESSION_ID],
  ])("does not mark a skipped %s session loaded", (_name, sessionId) => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.bySession[sessionId] = [{ id: "live-turn" }] as never;
      hydrateState(
        draft,
        {
          turns: { bySession: { [sessionId]: [{ id: "stale-turn" }] } },
        } as unknown as Partial<AppState>,
        { activeSessionId: sessionId === "active-session" ? sessionId : null },
      );
    });
    // The skipped merge must keep the live turns and must NOT claim the
    // session's full history is loaded.
    expect(result.turns.bySession[sessionId]).toEqual([{ id: "live-turn" }]);
    expect(result.turns.loadedBySession[sessionId]).toBeUndefined();
  });

  it("marks a force-merged session loaded even when it already existed", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.bySession[SESSION_ID] = [{ id: "live-turn" }] as never;
      hydrateState(
        draft,
        {
          turns: { bySession: { [SESSION_ID]: [{ id: FRESH_TURN }] } },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: SESSION_ID },
      );
    });
    expect(result.turns.bySession[SESSION_ID]).toEqual([{ id: FRESH_TURN }]);
    expect(result.turns.loadedBySession[SESSION_ID]).toBe(true);
  });

  it("never force-merges a stale active marker for a boundary-settled turn", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      // The turn was retired by an authoritative boundary (source adoption /
      // settled-session clear) while the server snapshot still names it as
      // the active turn.
      draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
      hydrateState(
        draft,
        {
          turns: {
            activeBySession: { [SESSION_ID]: "retired-turn" },
            bySession: {
              [SESSION_ID]: [
                { id: "retired-turn", started_at: "2025-12-31T00:00:00Z", completed_at: null },
              ],
            },
          },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: SESSION_ID },
      );
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("keeps a force-merged active marker for a turn started after the boundary", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
      hydrateState(
        draft,
        {
          turns: {
            activeBySession: { [SESSION_ID]: FRESH_TURN },
            bySession: {
              [SESSION_ID]: [
                { id: FRESH_TURN, started_at: "2026-01-02T00:00:00Z", completed_at: null },
              ],
            },
          },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: SESSION_ID },
      );
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBe(FRESH_TURN);
  });
});
