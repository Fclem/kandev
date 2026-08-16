import { describe, expect, it } from "vitest";
import { defaultState, mergeInitialState } from "./default-state";
import type { HydrationState } from "./store";

describe("turn hydration state", () => {
  it("defaults and deep-merges hydrated session markers", () => {
    const state = mergeInitialState({
      turns: {
        bySession: {},
        activeBySession: {},
        hydratedBySession: { "session-1": true },
      },
    } as HydrationState);

    expect(defaultState.turns.hydratedBySession).toEqual({});
    expect(state.turns.hydratedBySession).toEqual({ "session-1": true });
  });
});
