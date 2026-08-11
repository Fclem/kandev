import { describe, expect, it, vi } from "vitest";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { applyProfileDuplicated } from "./use-profile-duplicate";

vi.mock("@/app/actions/agents", () => ({ duplicateAgentProfileAction: vi.fn() }));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: vi.fn() }));
vi.mock("@/components/toast-provider", () => ({ useToast: vi.fn() }));

const COPY_NAME = "Default Copy";

function profile(id: string, agentId: string, name = id): AgentProfile {
  return {
    id,
    agentId,
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
  } as unknown as AgentProfile;
}

function agent(id: string, ...profiles: AgentProfile[]): Agent {
  return { id, name: id, profiles } as unknown as Agent;
}

function option(id: string): AgentProfileOption {
  return { id, label: id, agent_id: "a1", agent_name: "a1", cli_passthrough: false };
}

function stateWith(agents: Agent[], options: AgentProfileOption[]) {
  return {
    settingsAgents: { items: agents, version: 0 },
    agentProfiles: { items: options, version: 0 },
  };
}

describe("applyProfileDuplicated", () => {
  it("appends the copy to the owning agent and surfaces it as an option", () => {
    const source = profile("p1", "a1", "Default");
    const copy = profile("p2", "a1", COPY_NAME);
    const initial = stateWith([agent("a1", source)], []);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    const profiles = next.settingsAgents.items.find((a) => a.id === "a1")?.profiles ?? [];
    expect(profiles.map((p) => p.id)).toEqual(["p1", "p2"]);
    // Every profile (source + copy) is rebuilt into the options slice.
    expect(next.agentProfiles.items.map((o) => o.id)).toEqual(["p1", "p2"]);
  });

  it("dedupes by id so a WS-delivered copy is not double-inserted", () => {
    const source = profile("p1", "a1");
    const copy = profile("p2", "a1", COPY_NAME);
    // The WS agent.profile.created handler delivered the copy first.
    const initial = stateWith([agent("a1", source)], [option("p2")]);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    const profiles = next.settingsAgents.items.find((a) => a.id === "a1")?.profiles ?? [];
    expect(profiles.filter((p) => p.id === "p2")).toHaveLength(1);
    expect(next.agentProfiles.items.filter((o) => o.id === "p2")).toHaveLength(1);
  });

  it("keeps the copy visible when the owning agent left the store mid-request", () => {
    const copy = profile("p2", "a1", COPY_NAME);
    const initial = stateWith([], []);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    // settingsAgents is unchanged (no agent to attach to), but the copy must
    // still surface in the agentProfiles slice instead of being dropped.
    expect(next.settingsAgents.items).toEqual([]);
    expect(next.agentProfiles.items.map((o) => o.id)).toEqual(["p2"]);
  });

  it("does not erase a WS-delivered copy when the agent is missing", () => {
    const copy = profile("p2", "a1", COPY_NAME);
    // WS delivered the option first while the owning agent is absent.
    const initial = stateWith([], [option("p2")]);

    const next = applyProfileDuplicated(initial, agent("a1"), copy);

    expect(next.agentProfiles.items.filter((o) => o.id === "p2")).toHaveLength(1);
  });

  it("preserves WS-delivered orphan options for agents absent from the store", () => {
    // p-orphan was delivered by WS for an agent not present in settingsAgents;
    // duplicating a profile of a DIFFERENT agent must not wipe it.
    const source = profile("p1", "a2");
    const copy = profile("p2", "a2", COPY_NAME);
    const initial = stateWith([agent("a2", source)], [option("p-orphan")]);

    const next = applyProfileDuplicated(initial, agent("a2"), copy);

    const ids = next.agentProfiles.items.map((o) => o.id);
    expect(ids).toContain("p-orphan");
    expect(ids).toContain("p2");
    expect(ids.filter((id) => id === "p2")).toHaveLength(1);
  });
});
