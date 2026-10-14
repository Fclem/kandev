import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AgentUpdateJob, InstallJob } from "@/lib/api";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import type * as AgentApi from "@/lib/api";

const getAgentUpdateJobMock = vi.fn();
const getInstallJobMock = vi.fn();
const previewAgentUpdateMock = vi.fn();
const updateAgentMock = vi.fn();

vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof AgentApi>()),
  getAgentUpdateJob: (...args: unknown[]) => getAgentUpdateJobMock(...args),
  getInstallJob: (...args: unknown[]) => getInstallJobMock(...args),
  previewAgentUpdate: (...args: unknown[]) => previewAgentUpdateMock(...args),
  updateAgent: (...args: unknown[]) => updateAgentMock(...args),
}));

import { useAgentRuntimeUpdates } from "./use-agent-runtime-updates";

const AGENT = "grok-acp";

/** A 409 the backend returns when another maintenance job holds the agent. */
function conflict(activeKind: "install" | "update", jobId = "job-1") {
  return new ApiError("conflict", 409, { active_job_id: jobId, active_kind: activeKind });
}

function updateJob(overrides: Partial<AgentUpdateJob> = {}): AgentUpdateJob {
  return {
    update_mode: "pinned",
    job_id: "job-1",
    agent_name: AGENT,
    status: "updating",
    started_at: "2026-01-01T00:00:00.000Z",
    ...overrides,
  } as AgentUpdateJob;
}

function installJob(overrides: Partial<InstallJob> = {}): InstallJob {
  return {
    job_id: "job-1",
    agent_name: AGENT,
    status: "running",
    started_at: "2026-01-01T00:00:00.000Z",
    ...overrides,
  } as InstallJob;
}

function render() {
  return renderHook(() => ({ ...useAgentRuntimeUpdates(), store: useAppStoreApi() }), {
    wrapper: ({ children }) => <StateProvider>{children}</StateProvider>,
  });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useAgentRuntimeUpdates", () => {
  // The sibling branch must NOT throw — a concurrent *update* is adopted and
  // returned so the dialog attaches to the running job instead of erroring.
  it("adopts the running job when a 409 reports an active update job", async () => {
    updateAgentMock.mockRejectedValueOnce(conflict("update"));
    getAgentUpdateJobMock.mockResolvedValueOnce(updateJob());

    const { result } = render();

    await expect(result.current.startUpdate(AGENT, "1.1.0")).resolves.toMatchObject({
      job_id: "job-1",
    });
  });

  it("rethrows a non-conflict failure untouched", async () => {
    updateAgentMock.mockRejectedValueOnce(new Error("network down"));

    const { result } = render();

    await expect(result.current.startUpdate(AGENT, "1.1.0")).rejects.toThrow("network down");
    expect(getInstallJobMock).not.toHaveBeenCalled();
    expect(getAgentUpdateJobMock).not.toHaveBeenCalled();
  });

  it("returns a terminal self-update no-job result without registering a shared job", async () => {
    const terminal = updateJob({
      job_id: "",
      agent_name: "omp-acp",
      update_mode: "self_update",
      status: "succeeded",
      operation: "up_to_date",
      current_version: "1.1.0",
    });
    updateAgentMock.mockResolvedValueOnce(terminal).mockResolvedValueOnce(terminal);
    const { result } = render();
    await expect(result.current.startUpdate("omp-acp", "", false, "self_update")).resolves.toEqual(
      terminal,
    );
    await expect(result.current.startUpdate("omp-acp", "", false, "self_update")).resolves.toEqual(
      terminal,
    );
    expect(updateAgentMock).toHaveBeenCalledTimes(2);
    expect(updateAgentMock).toHaveBeenCalledWith("omp-acp", { update_mode: "self_update" });
    expect(result.current.updateJobs["omp-acp"]).toBeUndefined();
  });
});

describe("OMP conflict finish ordering", () => {
  it("adopts the conflicting job before GET so an earlier finish survives its stale snapshot", async () => {
    const automatic = updateJob({
      agent_name: "omp-acp",
      update_mode: "self_update",
      automatic: true,
      job_id: "periodic",
    });
    updateAgentMock.mockRejectedValueOnce(conflict("update", "periodic"));
    let resolveSnapshot!: (job: AgentUpdateJob) => void;
    getAgentUpdateJobMock.mockImplementationOnce(
      () =>
        new Promise<AgentUpdateJob>((resolve) => {
          resolveSnapshot = resolve;
        }),
    );
    const { result } = render();
    let pending!: Promise<AgentUpdateJob>;
    act(() => {
      pending = result.current.startUpdate("omp-acp", "", false, "self_update");
    });
    await waitFor(() => expect(getAgentUpdateJobMock).toHaveBeenCalledTimes(1));
    const handlers = registerAgentsHandlers(result.current.store);
    act(() => {
      handlers["agent.update.finished"]!({
        type: "notification",
        id: "finish",
        action: "agent.update.finished",
        timestamp: "2026-01-01T00:01:00Z",
        payload: {
          ...automatic,
          status: "succeeded",
          operation: "up_to_date",
          effective_version: "1.0.0",
        },
      });
    });
    try {
      expect(result.current.store.getState().updateJobs.byAgent["omp-acp"]?.status).toBe(
        "succeeded",
      );
    } finally {
      await act(async () => {
        resolveSnapshot(automatic);
        await pending;
      });
    }
    expect(result.current.updateJobs["omp-acp"]).toMatchObject({
      status: "succeeded",
      operation: "up_to_date",
    });
  });

  it("hydrates an install conflict without creating a second update", async () => {
    updateAgentMock.mockRejectedValueOnce(conflict("install"));
    getInstallJobMock.mockResolvedValueOnce(installJob());
    const { result } = render();
    await act(async () => {
      await expect(result.current.startUpdate(AGENT, "1.1.0")).rejects.toBeInstanceOf(Error);
    });
    expect(result.current.store.getState().installJobs.byAgent[AGENT]).toMatchObject({
      job_id: "job-1",
      status: "running",
    });
    expect(result.current.updateJobs[AGENT]).toBeUndefined();
  });
});
