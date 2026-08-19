import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RunEvent } from "@/lib/api/domains/office-extended-api";
import { useRunLiveSync } from "./use-run-live-sync";

const clients = vi.hoisted(() => ({
  active: { subscribeRun: vi.fn() },
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => clients.active,
  useWebSocketClient: () => clients.active,
}));

function runEvent(seq: number, eventType: string): RunEvent {
  return {
    seq,
    event_type: eventType,
    level: "info",
    payload: "{}",
    created_at: "2026-01-01T00:00:00Z",
  };
}

type Props = {
  runId: string;
  initialEvents: RunEvent[];
  initialStatus: "claimed";
};

function renderLiveSync(initialProps: Props) {
  return renderHook(
    (props: Props) => useRunLiveSync(props.runId, props.initialEvents, props.initialStatus),
    { initialProps },
  );
}

const initialProps: Props = { runId: "run-1", initialEvents: [], initialStatus: "claimed" };

describe("useRunLiveSync", () => {
  beforeEach(() => {
    clients.active = { subscribeRun: vi.fn(() => vi.fn()) };
  });

  it("does not loop when the snapshot reference changes but its content does not", () => {
    // A fresh array literal on every render changes `initialEvents` identity
    // without changing its content. Without the content guard the snapshot
    // sync effect re-writes state on every render, feeding an endless
    // render->effect cycle that exhausts the worker (this test times out
    // rather than completing).
    const { result, rerender } = renderHook(() => useRunLiveSync("run-1", [], "claimed"));

    rerender();
    rerender();

    expect(clients.active.subscribeRun).toHaveBeenCalledOnce();
    expect(result.current.events).toEqual([]);
    expect(result.current.status).toBe("claimed");
  });

  it("re-syncs events when the snapshot content actually changes", () => {
    const started = runEvent(1, "started");
    const { result, rerender } = renderLiveSync(initialProps);

    rerender({ ...initialProps, initialEvents: [started] });

    expect(result.current.events).toEqual([started]);
  });
});
