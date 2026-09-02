import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import { toast } from "@/lib/toast/sonner";
import {
  beginQueuedMessageEdit,
  endQueuedMessageEdit,
  renewQueuedMessageEdit,
  type QueueEditLease,
} from "@/lib/api/domains/queue-api";
import { useQueueEditProtection } from "./use-queue-edit-protection";

vi.mock("@/lib/api/domains/queue-api", () => ({
  beginQueuedMessageEdit: vi.fn(),
  endQueuedMessageEdit: vi.fn(),
  renewQueuedMessageEdit: vi.fn(),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: vi.fn() },
}));
const SESSION_A = "session-a";
const SESSION_B = "session-b";
const ENTRY_ID = "entry-1";

function entry(): QueuedMessage {
  return {
    id: ENTRY_ID,
    session_id: SESSION_A,
    task_id: "task-1",
    content: "queued",
    plan_mode: false,
    queued_at: "2026-08-28T00:00:00Z",
    queued_by: "user",
  };
}

beforeEach(() => {
  vi.mocked(toast.error).mockReset();
  vi.mocked(beginQueuedMessageEdit).mockClear();
  vi.mocked(endQueuedMessageEdit).mockClear();
  vi.mocked(renewQueuedMessageEdit).mockClear();
  vi.mocked(beginQueuedMessageEdit).mockResolvedValue({
    session_id: SESSION_A,
    entry_id: ENTRY_ID,
    lease_id: "lease-1",
    target_revision: 0,
  });
  vi.mocked(endQueuedMessageEdit).mockResolvedValue();
  vi.mocked(renewQueuedMessageEdit).mockResolvedValue({
    session_id: SESSION_A,
    entry_id: ENTRY_ID,
    lease_id: "lease-1",
    target_revision: 0,
  });
});

describe("useQueueEditProtection", () => {
  it("releases the target lease when the session changes during editing", async () => {
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) =>
        useQueueEditProtection({
          sessionId,
          entries: [entry()],
        }),
      { initialProps: { sessionId: SESSION_A } },
    );

    await act(async () => {
      expect(await result.current.beginEdit(ENTRY_ID)).toBe(true);
    });

    rerender({ sessionId: SESSION_B });

    await waitFor(() =>
      expect(endQueuedMessageEdit).toHaveBeenCalledWith({
        session_id: SESSION_A,
        entry_id: ENTRY_ID,
        lease_id: "lease-1",
        target_revision: 0,
      }),
    );
  });
});

it("allows only one lease acquisition while a begin request is pending", async () => {
  let resolveBegin!: (lease: QueueEditLease) => void;
  const pendingBegin = new Promise<QueueEditLease>((resolve) => {
    resolveBegin = resolve;
  });
  vi.mocked(beginQueuedMessageEdit).mockReturnValueOnce(pendingBegin);
  const { result } = renderHook(() =>
    useQueueEditProtection({
      sessionId: SESSION_A,
      entries: [entry()],
    }),
  );

  let firstEdit!: Promise<boolean>;
  act(() => {
    firstEdit = result.current.beginEdit(ENTRY_ID);
  });
  let secondEdit!: Promise<boolean>;
  act(() => {
    secondEdit = result.current.beginEdit("entry-2");
  });

  await expect(secondEdit).resolves.toBe(false);
  expect(beginQueuedMessageEdit).toHaveBeenCalledTimes(1);

  await act(async () => {
    resolveBegin({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-pending",
      target_revision: 0,
    });
    await expect(firstEdit).resolves.toBe(true);
  });
});

it("releases a lease when the target disappears during acquisition", async () => {
  let resolveBegin!: (lease: QueueEditLease) => void;
  const pendingBegin = new Promise<QueueEditLease>((resolve) => {
    resolveBegin = resolve;
  });
  vi.mocked(beginQueuedMessageEdit).mockReturnValueOnce(pendingBegin);
  const { result, rerender } = renderHook(
    ({ entries }: { entries: QueuedMessage[] }) =>
      useQueueEditProtection({ sessionId: SESSION_A, entries }),
    { initialProps: { entries: [entry()] } },
  );

  let pendingEdit!: Promise<boolean>;
  act(() => {
    pendingEdit = result.current.beginEdit(ENTRY_ID);
  });
  rerender({ entries: [] });

  await act(async () => {
    resolveBegin({
      session_id: SESSION_A,
      entry_id: ENTRY_ID,
      lease_id: "lease-gone",
      target_revision: 0,
    });
    await expect(pendingEdit).resolves.toBe(false);
  });

  expect(endQueuedMessageEdit).toHaveBeenCalledWith(
    expect.objectContaining({ lease_id: "lease-gone" }),
  );
});

describe("late queued edit lease operations", () => {
  it("keeps a replacement lease when an older renewal fails", async () => {
    vi.useFakeTimers();
    try {
      let rejectRenewal!: (error: Error) => void;
      const firstRenewal = new Promise<never>((_, reject) => {
        rejectRenewal = reject;
      });
      vi.mocked(renewQueuedMessageEdit).mockReturnValueOnce(firstRenewal);
      const replacementLease = {
        session_id: SESSION_A,
        entry_id: ENTRY_ID,
        lease_id: "lease-2",
        target_revision: 0,
      };
      const { result } = renderHook(() =>
        useQueueEditProtection({
          sessionId: SESSION_A,
          entries: [entry()],
        }),
      );

      await act(async () => {
        expect(await result.current.beginEdit(ENTRY_ID)).toBe(true);
      });
      await act(async () => {
        vi.advanceTimersByTime(20_000);
        await Promise.resolve();
      });
      expect(renewQueuedMessageEdit).toHaveBeenCalledTimes(1);

      await act(async () => {
        await result.current.completeEdit(ENTRY_ID);
      });
      vi.mocked(beginQueuedMessageEdit).mockResolvedValueOnce(replacementLease);
      await act(async () => {
        expect(await result.current.beginEdit(ENTRY_ID)).toBe(true);
      });

      await act(async () => {
        rejectRenewal(new Error("stale renewal failed"));
        await Promise.resolve();
      });

      expect(result.current.editLease?.lease_id).toBe("lease-2");
      expect(result.current.editingEntryId).toBe(ENTRY_ID);
    } finally {
      vi.useRealTimers();
    }
  });

  it("releases a lease when acquisition finishes after unmount", async () => {
    let resolveBegin!: (lease: QueueEditLease) => void;
    const pendingBegin = new Promise<QueueEditLease>((resolve) => {
      resolveBegin = resolve;
    });
    vi.mocked(beginQueuedMessageEdit).mockReturnValueOnce(pendingBegin);
    const { result, unmount } = renderHook(() =>
      useQueueEditProtection({
        sessionId: SESSION_A,
        entries: [entry()],
      }),
    );

    let pendingEdit!: Promise<boolean>;
    act(() => {
      pendingEdit = result.current.beginEdit(ENTRY_ID);
    });
    unmount();

    await act(async () => {
      resolveBegin({
        session_id: SESSION_A,
        entry_id: ENTRY_ID,
        lease_id: "lease-after-unmount",
        target_revision: 0,
      });
      await expect(pendingEdit).resolves.toBe(false);
    });

    expect(endQueuedMessageEdit).toHaveBeenCalledWith(
      expect.objectContaining({ lease_id: "lease-after-unmount" }),
    );
  });
});
