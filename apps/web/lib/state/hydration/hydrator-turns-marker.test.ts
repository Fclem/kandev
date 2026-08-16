import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

// Dedicated file for the turns `loadedBySession` marker predicates so
// hydrator.test.ts stays under the repo's 600-line file cap.

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

describe("hydrateState — turns loadedBySession marker predicates", () => {
  it.each([
    ["active", "active-session"],
    ["pre-existing non-active", "session-1"],
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
    const sessionId = "session-1";
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.bySession[sessionId] = [{ id: "live-turn" }] as never;
      hydrateState(
        draft,
        {
          turns: { bySession: { [sessionId]: [{ id: "fresh-turn" }] } },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: sessionId },
      );
    });
    expect(result.turns.bySession[sessionId]).toEqual([{ id: "fresh-turn" }]);
    expect(result.turns.loadedBySession[sessionId]).toBe(true);
  });
});
