import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { createAppStore } from "@/lib/state/store";
import type { AppState } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/http";
import { sessionCatalogRecordSchema } from "@/lib/types/session-catalog";
import { useSessionRestart } from "./use-session-restart";

const mockRequest = vi.fn();
const mockToast = vi.fn().mockReturnValue("restart-toast");
const mockUpdateToast = vi.fn();
let mockStore: StoreApi<AppState>;

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => mockStore,
  useOptionalAppStoreApi: () => mockStore,
  useAppStore: (selector: (state: AppState) => unknown) => selector(mockStore.getState()),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: mockUpdateToast }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

const acceptedOwner = {
  execution_id: "replacement-execution",
  attempt_id: "replacement-attempt",
  source_generation: 1,
};
beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-16T16:00:00Z"));
  mockRequest.mockResolvedValue({ accepted_launch_owner: acceptedOwner });
  mockStore = createAppStore();
  mockStore.getState().upsertTaskSessionFromEvent("t1", {
    id: sessionId("s1"),
    task_id: taskId("t1"),
    state: "RUNNING",
    started_at: "2026-10-20T00:00:00Z",
    updated_at: "2026-10-20T00:00:00Z",
  });
});
afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
});

function seedNotice() {
  const record = sessionCatalogRecordSchema.parse({
    version: 1,
    revision: 4,
    profile_id: "profile",
    digest: "host",
    owner: { execution_id: "old-execution", attempt_id: "old-attempt", source_generation: 1 },
    disposition: "host_candidates",
    reason: "new_candidates",
    models: [{ id: "candidate", name: "Candidate" }],
    source: "task_probe_v1",
    probe: {
      outcome: "complete",
      models: { present: true, supported: true, choices: [{ id: "candidate", name: "Candidate" }] },
      modes: { present: false, supported: false, choices: null },
    },
  });
  mockStore.getState().applySessionCatalogRefresh("t1", "s1", record);
  return record;
}

it("uses one 60-second deadline after ACK and never retries runtime actions on timeout", async () => {
  const notice = seedNotice();
  const first = renderHook(() => useSessionRestart("s1", "t1"));
  const second = renderHook(() => useSessionRestart("s1", "t1"));
  await act(async () => {
    expect(await first.result.current.restart()).toBe(true);
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.checking).toMatchObject({
    noticeRevision: notice.revision,
    owner: acceptedOwner,
    status: "checking",
    deadlineAt: Date.now() + 60000,
  });
  expect(first.result.current.isRestarting).toBe(false);
  first.unmount();
  second.unmount();
  mockStore.getState().setActiveSessionAuto("other-task", "other-session");
  act(() => {
    vi.advanceTimersByTime(59999);
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.checking?.status).toBe(
    "checking",
  );
  act(() => {
    vi.advanceTimersByTime(1);
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.checking?.status).toBe(
    "unavailable",
  );
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.record).toEqual(notice);
  expect(mockStore.getState().tasks.activeSessionId).toBe("other-session");
  expect(mockRequest).toHaveBeenCalledTimes(2);
});

it("does not arm a checking spinner when its matching result arrived before ACK", async () => {
  const notice = seedNotice();
  mockRequest.mockImplementation((action: string) => {
    if (action === "session.launch")
      mockStore.getState().applySessionCatalogRefresh(
        "t1",
        "s1",
        sessionCatalogRecordSchema.parse({
          ...notice,
          revision: 5,
          owner: acceptedOwner,
          disposition: "cleared",
          reason: "discovery_complete",
          models: undefined,
          result: {
            outcome: "complete",
            models: { present: true, supported: true, choices: [] },
            modes: { present: false, supported: false, choices: null },
          },
        }),
      );
    return Promise.resolve({ accepted_launch_owner: acceptedOwner });
  });
  const hook = renderHook(() => useSessionRestart("s1", "t1"));
  await act(async () => {
    await hook.result.current.restart();
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.checking).toBeUndefined();
  act(() => {
    vi.advanceTimersByTime(60000);
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.record?.disposition).toBe(
    "cleared",
  );
  expect(mockRequest).toHaveBeenCalledTimes(2);
  hook.unmount();
});

it("accepts a matching late result after timeout without another stop or resume", async () => {
  const notice = seedNotice();
  const hook = renderHook(() => useSessionRestart("s1", "t1"));
  await act(async () => {
    await hook.result.current.restart();
  });
  act(() => {
    vi.advanceTimersByTime(60000);
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.checking?.status).toBe(
    "unavailable",
  );
  act(() => {
    mockStore.getState().applySessionCatalogRefresh(
      "t1",
      "s1",
      sessionCatalogRecordSchema.parse({
        ...notice,
        revision: 5,
        owner: acceptedOwner,
        disposition: "cleared",
        reason: "discovery_complete",
        models: undefined,
        result: {
          outcome: "complete",
          models: { present: true, supported: true, choices: [] },
          modes: { present: false, supported: false, choices: null },
        },
      }),
    );
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.checking).toBeUndefined();
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.record?.disposition).toBe(
    "cleared",
  );
  expect(mockRequest).toHaveBeenCalledTimes(2);
  hook.unmount();
});

it("lets a newer notice supersede the old deadline and does not hydrate a spinner on reload", async () => {
  const notice = seedNotice();
  const hook = renderHook(() => useSessionRestart("s1", "t1"));
  await act(async () => {
    await hook.result.current.restart();
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.checking?.status).toBe(
    "checking",
  );
  act(() => {
    mockStore
      .getState()
      .applySessionCatalogRefresh("t1", "s1", { ...notice, revision: 6, digest: "newer-host" });
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.checking).toBeUndefined();
  act(() => {
    vi.advanceTimersByTime(60000);
  });
  expect(mockStore.getState().sessionCatalogRefresh.bySessionId.s1.record?.revision).toBe(6);
  const reloaded = createAppStore();
  reloaded.getState().applySessionCatalogRefresh("t1", "s1", { ...notice, revision: 6 });
  expect(reloaded.getState().sessionCatalogRefresh.bySessionId.s1.checking).toBeUndefined();
  expect(reloaded.getState().sessionCatalogRefresh.bySessionId.s1.record?.revision).toBe(6);
  hook.unmount();
});
