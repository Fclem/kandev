import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { Message } from "@/lib/types/http";

const listTaskSessionMessages = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessionMessages }));

import { loadMessageWindowAround } from "./load-message-window";

function message(id: string, created_at: string): Message {
  return { id, created_at } as Message;
}

describe("loadMessageWindowAround", () => {
  const mergeMessages = vi.fn();
  const store = {
    getState: () => ({
      messages: { bySession: { session: [message("new", "2026-08-22T00:00:02Z")] } },
      mergeMessages,
    }),
  } as unknown as StoreApi<AppState>;

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("requests and merges the target-containing around window", async () => {
    listTaskSessionMessages.mockResolvedValue({
      messages: [
        message("target", "2026-08-22T00:00:00Z"),
        message("middle", "2026-08-22T00:00:01Z"),
      ],
    });

    const result = await loadMessageWindowAround("session", "target", () => true, store);

    expect(result).toEqual({ kind: "merged", merged: true, current: true, targetFound: true });
    expect(listTaskSessionMessages).toHaveBeenCalledWith("session", {
      around: "target",
      limit: 100,
      sort: "desc",
    });
    expect(mergeMessages).toHaveBeenCalledWith("session", [
      message("target", "2026-08-22T00:00:00Z"),
      message("middle", "2026-08-22T00:00:01Z"),
      message("new", "2026-08-22T00:00:02Z"),
    ]);
  });
});
