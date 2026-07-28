import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerSystemEventsHandlers, SYSTEM_ERROR_FALLBACK_MESSAGE } from "./system-events";

const ERROR_TEXT = "Database is locked";

function dispatchSystemError(payload: unknown) {
  const store = createAppStore({});
  const message = {
    id: "message-1",
    type: "error" as const,
    action: "system.error" as const,
    payload,
  } as BackendMessageMap["system.error"];
  registerSystemEventsHandlers(store)["system.error"]!(message);
  return store.getState().systemErrorNotification;
}

describe("system.error", () => {
  it("stores a well-formed payload verbatim", () => {
    expect(dispatchSystemError({ message: ERROR_TEXT, code: "db_locked" })).toEqual({
      message: ERROR_TEXT,
      code: "db_locked",
    });
  });

  it("omits the code when the payload carries only a message", () => {
    expect(dispatchSystemError({ message: ERROR_TEXT })).toEqual({ message: ERROR_TEXT });
  });

  it("trims surrounding whitespace from message and code", () => {
    expect(dispatchSystemError({ message: "  boom  ", code: "  E1  " })).toEqual({
      message: "boom",
      code: "E1",
    });
  });

  it.each<[string, unknown]>([
    ["an empty object", {}],
    ["a blank message", { message: "   " }],
    ["a non-string message", { message: 42 }],
    ["a null payload", null],
    ["an undefined payload", undefined],
    ["a non-object payload", "boom"],
  ])("falls back to a generic message for %s", (_label, payload) => {
    expect(dispatchSystemError(payload)).toEqual({ message: SYSTEM_ERROR_FALLBACK_MESSAGE });
  });

  it("keeps a usable code when only the message is malformed", () => {
    expect(dispatchSystemError({ code: "db_locked" })).toEqual({
      message: SYSTEM_ERROR_FALLBACK_MESSAGE,
      code: "db_locked",
    });
  });

  it("drops a non-string code instead of rendering it", () => {
    expect(dispatchSystemError({ message: "boom", code: { nested: true } })).toEqual({
      message: "boom",
    });
  });

  it("does not throw when the frame itself is missing a payload", () => {
    const store = createAppStore({});
    const handler = registerSystemEventsHandlers(store)["system.error"]!;
    expect(() =>
      handler({ type: "error", action: "system.error" } as BackendMessageMap["system.error"]),
    ).not.toThrow();
    expect(store.getState().systemErrorNotification).toEqual({
      message: SYSTEM_ERROR_FALLBACK_MESSAGE,
    });
  });
});
