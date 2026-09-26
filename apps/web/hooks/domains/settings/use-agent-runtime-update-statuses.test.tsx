import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob, AgentUpdateStatus } from "@/lib/api";

const listAgentUpdateStatusesMock = vi.fn();

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  listAgentUpdateStatuses: (...args: unknown[]) => listAgentUpdateStatusesMock(...args),
}));

import { useAgentRuntimeUpdateStatuses } from "./use-agent-runtime-update-statuses";

function job(overrides: Partial<AgentUpdateJob> = {}): AgentUpdateJob {
  return {
    job_id: "job-1",
    agent_name: "claude-acp",
    status: "succeeded",
    update_mode: "pinned",
    started_at: "2026-01-01T00:00:00.000Z",
    ...overrides,
  };
}

function status(
  agent_name: string,
  check_state: AgentUpdateStatus["check_state"],
): AgentUpdateStatus {
  return {
    agent_name,
    update_mode: "pinned",
    package: "@agentclientprotocol/claude-agent-acp",
    default_version: "0.70.0",
    effective_version: "0.70.0",
    check_state,
  };
}

afterEach(() => vi.clearAllMocks());

describe("useAgentRuntimeUpdateStatuses", () => {
  it("loads structural statuses into a page-local agent map", async () => {
    listAgentUpdateStatusesMock.mockResolvedValueOnce({
      statuses: [
        {
          agent_name: "claude-acp",
          package: "@agentclientprotocol/claude-agent-acp",
          update_mode: "pinned",
          default_version: "0.70.0",
          effective_version: "0.70.0",
          latest_version: "0.71.0",
          check_state: "update_available",
        },
        {
          agent_name: "codex-acp",
          package: "@agentclientprotocol/codex-acp",
          default_version: "1.6.0",
          update_mode: "pinned",
          effective_version: "1.6.0",
          check_state: "unknown",
        },
      ],
    });

    const { result } = renderHook(() => useAgentRuntimeUpdateStatuses({}));

    await waitFor(() => expect(result.current.statusByAgent["claude-acp"]).toBeDefined());
    expect(result.current.statusByAgent["claude-acp"]?.check_state).toBe("update_available");
    expect(result.current.statusByAgent["codex-acp"]?.check_state).toBe("unknown");
    expect(listAgentUpdateStatusesMock).toHaveBeenCalledWith({ cache: "no-store" });
  });

  it("refreshes once after a successful update job", async () => {
    listAgentUpdateStatusesMock
      .mockResolvedValueOnce({ statuses: [] })
      .mockResolvedValueOnce({ statuses: [] });
    const { result, rerender } = renderHook(({ jobs }) => useAgentRuntimeUpdateStatuses(jobs), {
      initialProps: { jobs: {} as Record<string, AgentUpdateJob> },
    });

    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(1));
    rerender({ jobs: { "claude-acp": job() } });
    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(2));
    expect(result.current.statusByAgent).toEqual({});
  });

  it("does not erase a last good map when a refresh fails", async () => {
    listAgentUpdateStatusesMock
      .mockResolvedValueOnce({
        statuses: [
          {
            agent_name: "claude-acp",
            update_mode: "pinned",
            package: "@agentclientprotocol/claude-agent-acp",
            default_version: "0.70.0",
            effective_version: "0.70.0",
            latest_version: "0.71.0",
            check_state: "update_available",
          },
        ],
      })
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ statuses: [] });
    const { result, rerender } = renderHook(({ jobs }) => useAgentRuntimeUpdateStatuses(jobs), {
      initialProps: { jobs: {} as Record<string, AgentUpdateJob> },
    });

    await waitFor(() => expect(result.current.statusByAgent["claude-acp"]).toBeDefined());
    rerender({ jobs: { "claude-acp": job({ job_id: "job-2" }) } });
    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(2));
    expect(result.current.statusByAgent["claude-acp"]?.check_state).toBe("update_available");

    // A rerender of the same jobs map must not retry a failed request in a tight loop.
    rerender({ jobs: { "claude-acp": job({ job_id: "job-2" }) } });
    expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(2);
    await act(async () => {
      expect(await result.current.refresh()).toBe("applied");
    });
    expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(3);
  });
});

describe("useAgentRuntimeUpdateStatuses concurrent refreshes", () => {
  it("applies only the newest response when refreshes settle out of order", async () => {
    const old = Promise.withResolvers<{ statuses: AgentUpdateStatus[] }>();
    const newest = Promise.withResolvers<{ statuses: AgentUpdateStatus[] }>();
    listAgentUpdateStatusesMock
      .mockResolvedValueOnce({ statuses: [] })
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(newest.promise);
    const { result } = renderHook(() => useAgentRuntimeUpdateStatuses({}));
    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(1));
    let older!: Promise<unknown>;
    let newer!: Promise<unknown>;
    act(() => {
      older = result.current.refresh();
      newer = result.current.refresh();
    });
    await act(async () => {
      newest.resolve({ statuses: [status("newer", "update_available")] });
      expect(await newer).toBe("applied");
      old.resolve({ statuses: [status("older", "unknown")] });
      expect(await older).toBe("superseded");
    });
    expect(result.current.statusByAgent.newer?.check_state).toBe("update_available");
    expect(result.current.statusByAgent.older).toBeUndefined();
  });

  it("coalesces late successes and a failed no-job refresh after a superseded job refresh", async () => {
    const a = Promise.withResolvers<{ statuses: AgentUpdateStatus[] }>();
    const noJobRequest = Promise.withResolvers<{ statuses: AgentUpdateStatus[] }>();
    const successor = Promise.withResolvers<{ statuses: AgentUpdateStatus[] }>();
    listAgentUpdateStatusesMock
      .mockResolvedValueOnce({ statuses: [status("baseline", "unknown")] })
      .mockReturnValueOnce(a.promise)
      .mockReturnValueOnce(noJobRequest.promise)
      .mockReturnValueOnce(successor.promise);
    const { result, rerender } = renderHook(({ jobs }) => useAgentRuntimeUpdateStatuses(jobs), {
      initialProps: { jobs: {} as Record<string, AgentUpdateJob> },
    });
    await waitFor(() => expect(result.current.statusByAgent.baseline).toBeDefined());
    rerender({ jobs: { a: job({ job_id: "a", agent_name: "a" }) } });
    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(2));
    rerender({
      jobs: {
        a: job({ job_id: "a", agent_name: "a" }),
        b: job({ job_id: "b", agent_name: "b" }),
      },
    });
    let noJob!: Promise<unknown>;
    act(() => {
      noJob = result.current.refresh();
    });
    await act(async () => {
      noJobRequest.reject(new Error("offline"));
      expect(await noJob).toBe("failed");
      a.resolve({ statuses: [status("stale", "unknown")] });
    });
    await waitFor(() => expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(4));
    expect(result.current.statusByAgent.baseline).toBeDefined();
    expect(result.current.statusByAgent.stale).toBeUndefined();
    await act(async () => {
      successor.resolve({ statuses: [status("a", "unknown"), status("b", "update_available")] });
    });
    expect(result.current.statusByAgent.a).toBeDefined();
    expect(result.current.statusByAgent.b?.check_state).toBe("update_available");
    rerender({
      jobs: {
        b: job({ job_id: "b", agent_name: "b" }),
        a: job({ job_id: "a", agent_name: "a" }),
      },
    });
    expect(listAgentUpdateStatusesMock).toHaveBeenCalledTimes(4);
  });
});
