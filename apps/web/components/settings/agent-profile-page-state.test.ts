import { describe, expect, it, vi } from "vitest";
import type { Agent } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { reconcileAgentProfileOptions } from "./agent-profile-page-state";

vi.mock("@/app/actions/agents", () => ({
  deleteAgentProfileAction: vi.fn(),
  updateAgentProfileAction: vi.fn(),
}));
vi.mock("@/components/state-provider", () => ({ useAppStore: vi.fn(), useAppStoreApi: vi.fn() }));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

function agent(id: string, ...profileNames: string[]): Agent {
  return {
    id,
    name: id,
    profiles: profileNames.map((name) => ({
      id: name,
      agentId: id,
      name,
      agentDisplayName: "Test Agent",
      model: "",
      allowIndexing: false,
      autoApprove: false,
      cliFlags: [],
      cliPassthrough: false,
      enabled: true,
      createdAt: "",
      updatedAt: "",
    })),
  } as unknown as Agent;
}

function option(id: string, label = id): AgentProfileOption {
  return { id, label, agent_id: "a1", agent_name: "a1", cli_passthrough: false };
}

describe("reconcileAgentProfileOptions", () => {
  it("rebuilds options from the next agent list", () => {
    const next = reconcileAgentProfileOptions([], [agent("a1", "p1", "p2")]);
    expect(next.map((o) => o.id).sort()).toEqual(["p1", "p2"]);
  });

  it("preserves WS-delivered orphan options the rebuild does not represent", () => {
    // p-orphan was delivered by WS for an agent absent from the next list.
    const next = reconcileAgentProfileOptions([option("p-orphan")], [agent("a1", "p1")]);
    const ids = next.map((o) => o.id);
    expect(ids).toContain("p-orphan");
    expect(ids).toContain("p1");
  });

  it("replaces stale options with the rebuilt versions by id", () => {
    const next = reconcileAgentProfileOptions([option("p1", "stale label")], [agent("a1", "p1")]);
    expect(next.filter((o) => o.id === "p1")).toHaveLength(1);
    expect(next.find((o) => o.id === "p1")?.label).toContain("p1");
  });
});
