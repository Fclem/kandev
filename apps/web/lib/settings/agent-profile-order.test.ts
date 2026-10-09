import { describe, expect, it } from "vitest";
import {
  acceptServerOrder,
  insertFirstInAgentGroup,
  reorderIds,
  reconcileAgentOrders,
  type ProfileOrderState,
} from "./agent-profile-order";
import type { Agent, AgentProfile } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";

const profile = (id: string, name: string) => ({ id, name }) as AgentProfile;
const agent = (id: string, profiles: AgentProfile[]) => ({ id, profiles }) as Agent;
const option = (id: string, agentId: string): AgentProfileOption =>
  ({
    id,
    agent_id: agentId,
    agent_name: agentId,
    label: id,
    cli_passthrough: false,
  }) as AgentProfileOption;

describe("agent profile order helpers", () => {
  it("moves only valid active and over IDs", () => {
    expect(reorderIds(["a", "b", "c"], "a", "c")).toEqual(["b", "c", "a"]);
    expect(reorderIds(["a", "b"], "x", "b")).toEqual(["a", "b"]);
  });

  it("accepts only newer server revisions while retaining pending intents", () => {
    const initial: ProfileOrderState = {
      a: { revision: 4, order: ["x", "y"], inFlight: ["y", "x"], queued: null },
    };
    expect(acceptServerOrder(initial, "a", ["x", "y"], 4)).toBe(initial);
    expect(acceptServerOrder(initial, "a", ["y", "x"], 5).a).toEqual({
      revision: 5,
      order: ["y", "x"],
      inFlight: ["y", "x"],
      queued: null,
    });
  });

  it("reconciles snapshots with new profiles first, queued order next, and deleted IDs omitted", () => {
    const agents = [agent("a", [profile("new", "New"), profile("x", "X"), profile("y", "Y")])];
    const sync: ProfileOrderState = {
      a: { revision: 3, order: ["x", "y"], inFlight: ["y", "x"], queued: ["x", "deleted", "y"] },
    };
    const reconciled = reconcileAgentOrders(agents, sync);
    expect(reconciled[0].profiles.map((item) => item.id)).toEqual(["new", "x", "y"]);
  });

  it("inserts a created profile at the start of its agent group without moving other groups", () => {
    const items = [option("a1", "a"), option("b1", "b"), option("a2", "a")];
    expect(insertFirstInAgentGroup(items, "a", option("a3", "a")).map((item) => item.id)).toEqual([
      "a3",
      "a1",
      "b1",
      "a2",
    ]);
    expect(
      insertFirstInAgentGroup(items, "missing", option("m1", "missing")).map((item) => item.id),
    ).toEqual(["a1", "b1", "a2", "m1"]);
  });
});
