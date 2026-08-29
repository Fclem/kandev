import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";
import { sessionId, taskId, type Message } from "@/lib/types/http";

const SESSION = "prompts-session";

function makeStore() {
  return create<SessionSlice>()(
    // Zustand's immer middleware erases the slice mutator tuple in this test harness.
    immer((set) => ({
      ...(createSessionSlice as unknown as (setter: typeof set) => SessionSlice)(set),
    })),
  );
}

function message(id: string, author_type: "user" | "agent"): Message {
  return {
    id,
    session_id: sessionId(SESSION),
    task_id: taskId("task"),
    author_type,
    type: "message",
    content: id,
    created_at: "2026-08-22T00:00:00Z",
  } as Message;
}

describe("prompt cache fan-out", () => {
  it("keeps only user messages in the independent prompts cache", () => {
    const store = makeStore();
    store.getState().addMessage(message("user", "user"));
    store.getState().addMessage(message("agent", "agent"));

    expect(store.getState().messagePrompts.bySession[SESSION].map((entry) => entry.id)).toEqual([
      "user",
    ]);
    expect(store.getState().messages.bySession[SESSION]).toHaveLength(2);
  });
});

it("updates prompts even when the transcript cache is absent", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(SESSION, [message("user", "user")]);
  store.setState((state) => ({ ...state, messages: { bySession: {}, metaBySession: {} } }));

  store.getState().updateMessage({ ...message("user", "user"), content: "updated" });

  expect(store.getState().messagePrompts.bySession[SESSION][0].content).toBe("updated");
});

it("unions user rows from transcript snapshots without dropping deep prompt pages", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(SESSION, [message("deep", "user")]);

  store
    .getState()
    .mergeMessages(SESSION, [
      { ...message("new", "user"), created_at: "2026-08-22T00:00:01Z" },
      message("agent", "agent"),
    ]);

  expect(store.getState().messagePrompts.bySession[SESSION].map((entry) => entry.id)).toEqual([
    "deep",
    "new",
  ]);
});

it("repairs the prompt cursor after deleting the oldest cached prompt", () => {
  const store = makeStore();
  store.getState().replacePromptMessages(
    SESSION,
    [
      { ...message("oldest", "user"), created_at: "2026-08-22T00:00:00Z" },
      { ...message("newest", "user"), created_at: "2026-08-22T00:00:01Z" },
    ],
    { hasMore: true, oldestCursor: "oldest" },
  );

  store.getState().removeMessage(SESSION, "oldest");

  expect(store.getState().messagePrompts.metaBySession[SESSION].oldestCursor).toBe("newest");
});
