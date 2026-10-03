import { describe, expect, it } from "vitest";
import { applyProfileDuplicated } from "./use-profile-duplicate";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

const profile = (id: string, name: string) => ({ id, name, agentId: "a" }) as AgentProfile;
const option = (id: string): AgentProfileOption =>
  ({
    id,
    label: id,
    agent_id: "a",
    agent_name: "Agent",
    cli_passthrough: false,
  }) as AgentProfileOption;

describe("profile duplication ordering", () => {
  it("prepends a duplicated profile within its agent in both mirrored lists", () => {
    const agent = {
      id: "a",
      name: "Agent",
      profiles: [profile("old-1", "One"), profile("old-2", "Two")],
    } as Agent;
    const state = {
      settingsAgents: { items: [agent] },
      agentProfiles: { items: [option("old-1"), option("old-2")], version: 0, orderByAgent: {} },
    };
    const next = applyProfileDuplicated(state, agent, profile("new", "Copy"));
    expect(next.settingsAgents.items[0].profiles.map((item) => item.id)).toEqual([
      "new",
      "old-1",
      "old-2",
    ]);
    expect(next.agentProfiles.items.map((item) => item.id)).toEqual(["new", "old-1", "old-2"]);
  });
});
