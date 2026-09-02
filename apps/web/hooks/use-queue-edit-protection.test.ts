import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import { toast } from "@/lib/toast/sonner";
import {
  beginQueuedMessageEdit,
  endQueuedMessageEdit,
  renewQueuedMessageEdit,
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

describe("useQueueEditProtection", () => {
  beforeEach(() => {
    vi.mocked(toast.error).mockReset();
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
