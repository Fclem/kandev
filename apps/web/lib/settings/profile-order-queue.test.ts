import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { ApiError } from "@/lib/api/client";
import { ProfileOrderQueue } from "./profile-order-queue";

const store = createAppStore();

describe("ProfileOrderQueue", () => {
  beforeEach(() => {
    store.getState().setSettingsAgents([
      {
        id: "a",
        name: "Agent",
        profiles: [
          { id: "x", name: "X" },
          { id: "y", name: "Y" },
        ],
      } as never,
    ]);
    store.setState((state) => {
      state.agentProfiles.orderByAgent.a = {
        revision: 1,
        order: ["x", "y"],
        inFlight: null,
        queued: null,
      };
    });
  });

  it("submits the latest intent after an earlier save rejects", async () => {
    let rejectFirst!: (error: Error) => void;
    const first = new Promise<{ profile_ids: string[]; revision: number }>((_, reject) => {
      rejectFirst = reject;
    });
    const save = vi
      .fn()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({ profile_ids: ["x", "y"], revision: 2 });
    const queue = new ProfileOrderQueue(store, { save });
    queue.requestProfileOrder("a", ["y", "x"]);
    queue.requestProfileOrder("a", ["x", "y"]);
    rejectFirst(new Error("failed"));
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(store.getState().agentProfiles.orderByAgent.a.queued).toBeNull());
    expect(save.mock.calls.map((call) => call[1])).toEqual([
      ["y", "x"],
      ["x", "y"],
    ]);
    expect(store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      "x",
      "y",
    ]);
  });

  it("clears the optimistic order after the latest save fails", async () => {
    const save = vi.fn().mockRejectedValue(new Error("failed"));
    const onError = vi.fn();
    const queue = new ProfileOrderQueue(store, { save, onError });
    queue.requestProfileOrder("a", ["y", "x"]);
    await vi.waitFor(() => expect(onError).toHaveBeenCalledOnce());
    expect(store.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      "x",
      "y",
    ]);
    expect(store.getState().agentProfiles.orderByAgent.a.inFlight).toBeNull();
  });

  it("refetches on conflict and replays only surviving queued profiles after new IDs", async () => {
    let rejectFirst!: (error: Error) => void;
    const first = new Promise<{ profile_ids: string[]; revision: number }>((_, reject) => {
      rejectFirst = reject;
    });
    const save = vi
      .fn()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({ profile_ids: ["new", "x", "y"], revision: 4 });
    const read = vi.fn().mockResolvedValue({
      agents: [
        {
          id: "a",
          name: "Agent",
          profile_order_revision: 3,
          profiles: [
            { id: "new", name: "New" },
            { id: "x", name: "X" },
            { id: "y", name: "Y" },
          ],
        },
      ],
    });
    const queue = new ProfileOrderQueue(store, { save, read });
    queue.requestProfileOrder("a", ["y", "x"]);
    queue.requestProfileOrder("a", ["y", "deleted", "x"]);
    rejectFirst(new ApiError("stale", 409, {}));
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(2));
    expect(read).toHaveBeenCalledOnce();
    expect(save.mock.calls[1][1]).toEqual(["new", "y", "x"]);
  });
});
