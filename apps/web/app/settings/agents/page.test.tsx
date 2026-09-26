import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AgentUpdateJob } from "@/lib/api";

const startUpdateMock = vi.fn();
const refreshStatusesMock = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({
      settingsAgents: { items: [] },
      installJobs: { byAgent: {} },
      setAgentDiscovery: vi.fn(),
      setSettingsAgents: vi.fn(),
      setAvailableAgents: vi.fn(),
      setAgentProfiles: vi.fn(),
    }),
}));
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => true }));
vi.mock("@/hooks/domains/settings/use-agent-discovery", () => ({
  useAgentDiscovery: () => ({
    items: [
      {
        name: "omp-acp",
        available: true,
        installation_paths: [],
        matched_path: "omp",
        supports_mcp: false,
      },
    ],
    loading: false,
  }),
}));
vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => ({
    items: [
      {
        name: "omp-acp",
        display_name: "omp",
        runtime_update: { supported: true, update_mode: "self_update" },
      },
    ],
  }),
}));
vi.mock("@/hooks/domains/settings/use-agent-runtime-updates", () => ({
  useAgentRuntimeUpdates: () => ({
    updateJobs: {},
    previewUpdate: vi.fn(),
    startUpdate: startUpdateMock,
  }),
}));
vi.mock("@/hooks/domains/settings/use-agent-runtime-update-statuses", () => ({
  useAgentRuntimeUpdateStatuses: () => ({ statusByAgent: {}, refresh: refreshStatusesMock }),
}));
vi.mock("@/components/settings/installed-agent-card", () => ({
  InstalledAgentCard: ({
    onUpdate,
  }: {
    onUpdate?: (
      name: string,
      target: string,
      useDefault: boolean,
      mode: "self_update",
    ) => Promise<AgentUpdateJob>;
  }) => (
    <button type="button" onClick={() => void onUpdate?.("omp-acp", "", false, "self_update")}>
      Approve self-update
    </button>
  ),
}));
vi.mock("@/components/settings/agents/agent-profiles-section", () => ({
  AgentProfilesSubList: () => null,
}));
vi.mock("@/components/settings/custom-tui-mcp-card", () => ({ CustomTUIMcpCard: () => null }));
vi.mock("@/components/settings/dynamic-agents-card", () => ({ DynamicAgentsCard: () => null }));
vi.mock("@/components/settings/host-shell-dialog", () => ({ HostShellDialog: () => null }));
vi.mock("@/components/settings/add-tui-agent-dialog", () => ({ AddTUIAgentDialog: () => null }));
vi.mock("./hide-disabled-agent-profiles-setting", () => ({
  HideDisabledAgentProfilesSetting: () => null,
}));

import AgentsSettingsPage from "./page";

afterEach(() => vi.clearAllMocks());

describe("Agents settings self-update approval", () => {
  it("starts one status refresh per terminal no-job response without awaiting a slow or failed read", async () => {
    const pending = Promise.withResolvers<void>();
    refreshStatusesMock
      .mockReturnValueOnce(pending.promise)
      .mockRejectedValueOnce(new Error("offline"));
    const terminal: AgentUpdateJob = {
      update_mode: "self_update",
      job_id: "",
      agent_name: "omp-acp",
      status: "succeeded",
      operation: "up_to_date",
      started_at: "2026-09-26T12:00:00Z",
    };
    startUpdateMock.mockResolvedValue(terminal);
    render(<AgentsSettingsPage />);

    fireEvent.click(screen.getByRole("button", { name: "Approve self-update" }));
    await waitFor(() => expect(refreshStatusesMock).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole("button", { name: "Approve self-update" }));
    await waitFor(() => expect(refreshStatusesMock).toHaveBeenCalledTimes(2));
    expect(startUpdateMock).toHaveBeenCalledTimes(2);
    pending.resolve();
  });
});
