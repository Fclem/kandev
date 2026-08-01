import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { UpdatesResponse } from "@/lib/types/system";

const mocks = vi.hoisted(() => ({
  checkUpdates: vi.fn(),
  fetchUpdates: vi.fn(),
  saveUpdatesChannel: vi.fn(),
  setSystemUpdates: vi.fn(),
  currentUpdates: null as UpdatesResponse | null,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      system: { updates: UpdatesResponse | null };
      setSystemUpdates: typeof mocks.setSystemUpdates;
    }) => unknown,
  ) =>
    selector({
      system: { updates: mocks.currentUpdates },
      setSystemUpdates: mocks.setSystemUpdates,
    }),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  checkUpdates: mocks.checkUpdates,
  fetchUpdates: mocks.fetchUpdates,
  saveUpdatesChannel: mocks.saveUpdatesChannel,
}));

import { useUpdates } from "./use-updates";

function updates(channel: UpdatesResponse["channel"]): UpdatesResponse {
  const nightly = channel === "nightly";
  return {
    current: "v1.0.0",
    latest: nightly ? "v1.0.1-nightly.shaabcdef123456" : "v1.0.1",
    latest_url: "https://example.test/update",
    latest_checked_at: "2026-08-01T00:00:00Z",
    update_available: true,
    channel,
    channel_editable: true,
    channel_unsupported_reason: "",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

beforeEach(() => {
  mocks.checkUpdates.mockReset();
  mocks.fetchUpdates.mockReset();
  mocks.saveUpdatesChannel.mockReset();
  mocks.setSystemUpdates.mockReset();
  mocks.currentUpdates = updates("stable");
});

describe("useUpdates", () => {
  it("saves a channel and publishes the returned state", async () => {
    const nightly = updates("nightly");
    mocks.saveUpdatesChannel.mockResolvedValue(nightly);
    const { result } = renderHook(() => useUpdates());

    let response!: UpdatesResponse;
    await act(async () => {
      response = await result.current.saveChannel("nightly");
    });

    expect(mocks.saveUpdatesChannel).toHaveBeenCalledWith("nightly");
    expect(mocks.setSystemUpdates).toHaveBeenCalledWith(nightly);
    expect(response).toBe(nightly);
    expect(result.current.error).toBeNull();
  });

  it("surfaces a channel save failure without replacing update state", async () => {
    const failure = new Error("save failed");
    mocks.saveUpdatesChannel.mockRejectedValue(failure);
    const { result } = renderHook(() => useUpdates());

    await act(async () => {
      await expect(result.current.saveChannel("nightly")).rejects.toBe(failure);
    });

    expect(result.current.error).toBe("save failed");
    expect(mocks.setSystemUpdates).not.toHaveBeenCalled();
  });

  it("ignores an older check response after a channel save starts", async () => {
    const pendingCheck = deferred<UpdatesResponse>();
    const pendingSave = deferred<UpdatesResponse>();
    mocks.checkUpdates.mockReturnValue(pendingCheck.promise);
    mocks.saveUpdatesChannel.mockReturnValue(pendingSave.promise);
    const { result } = renderHook(() => useUpdates());

    let checkPromise!: Promise<UpdatesResponse>;
    let savePromise!: Promise<UpdatesResponse>;
    act(() => {
      checkPromise = result.current.check();
      savePromise = result.current.saveChannel("nightly");
    });

    const nightly = updates("nightly");
    await act(async () => {
      pendingSave.resolve(nightly);
      await savePromise;
    });
    await act(async () => {
      pendingCheck.resolve(updates("stable"));
      await checkPromise;
    });

    expect(mocks.setSystemUpdates).toHaveBeenCalledOnce();
    expect(mocks.setSystemUpdates).toHaveBeenLastCalledWith(nightly);
  });
});
