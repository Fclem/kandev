import type { Agent, AgentProfile } from "@/lib/types/http";
import type { AgentProfileOption, ProfileOrderSync } from "@/lib/state/slices/settings/types";

export type ProfileOrderState = Record<string, ProfileOrderSync>;

export function sortProfileIdsByName(
  profiles: Pick<AgentProfile, "id" | "name">[],
  locale: string,
): string[] {
  const collator = new Intl.Collator(locale, { sensitivity: "accent", numeric: true });
  return profiles
    .map((profile, index) => ({ profile, index }))
    .sort(
      (left, right) =>
        collator.compare(left.profile.name, right.profile.name) || left.index - right.index,
    )
    .map(({ profile }) => profile.id);
}
export function acceptAgentOrdersFromSnapshot(
  state: ProfileOrderState,
  agents: Agent[],
): ProfileOrderState {
  let next = state;
  for (const agent of agents) {
    if (agent.profile_order_revision === undefined) continue;
    next = acceptServerOrder(
      next,
      agent.id,
      agent.profiles.map((profile) => profile.id),
      agent.profile_order_revision,
    );
  }
  return next;
}

export function reorderIds(ids: string[], activeId: string, overId: string): string[] {
  const from = ids.indexOf(activeId);
  const to = ids.indexOf(overId);
  if (from < 0 || to < 0 || from === to) return ids;
  const next = ids.slice();
  next.splice(from, 1);
  next.splice(to, 0, activeId);
  return next;
}

export function acceptServerOrder(
  state: ProfileOrderState,
  agentId: string,
  ids: string[],
  revision: number,
): ProfileOrderState {
  const current = state[agentId];
  if (current?.order !== null && current?.order !== undefined && revision <= current.revision)
    return state;
  return {
    ...state,
    [agentId]: {
      revision,
      order: [...ids],
      inFlight: current?.inFlight ?? null,
      queued: current?.queued ?? null,
    },
  };
}

function reorderProfileGroup<T extends { id: string }>(items: T[], order: string[]): T[] {
  const rank = new Map(order.map((id, index) => [id, index]));
  return items
    .map((item, index) => ({ item, index }))
    .sort((left, right) => {
      const leftRank = rank.get(left.item.id);
      const rightRank = rank.get(right.item.id);
      if (leftRank === undefined && rightRank === undefined) return left.index - right.index;
      if (leftRank === undefined) return -1;
      if (rightRank === undefined) return 1;
      return leftRank - rightRank || left.index - right.index;
    })
    .map(({ item }) => item);
}

export function reconcileAgentOrders(agents: Agent[], sync: ProfileOrderState): Agent[] {
  let changed = false;
  const next = agents.map((agent) => {
    const state = sync[agent.id];
    if (!state) return agent;
    const overlay = state.queued ?? state.inFlight;
    const order = overlay ?? state.order;
    if (!order) return agent;
    const profiles = reorderProfileGroup(agent.profiles, order);
    if (profiles.every((profile, index) => profile === agent.profiles[index])) return agent;
    changed = true;
    return { ...agent, profiles };
  });
  return changed ? next : agents;
}

export function reorderFlatOptions(
  options: AgentProfileOption[],
  agentId: string,
  ids: string[],
): AgentProfileOption[] {
  const groupIndexes: number[] = [];
  const group: AgentProfileOption[] = [];
  options.forEach((option, index) => {
    if (option.agent_id === agentId) {
      groupIndexes.push(index);
      group.push(option);
    }
  });
  const ordered = reorderProfileGroup(group, ids);
  if (ordered.every((option, index) => option === group[index])) return options;
  const next = options.slice();
  groupIndexes.forEach((index, offset) => {
    next[index] = ordered[offset];
  });
  return next;
}

export function insertFirstInAgentGroup(
  options: AgentProfileOption[],
  agentId: string,
  option: AgentProfileOption,
): AgentProfileOption[] {
  const withoutDuplicate = options.filter((existing) => existing.id !== option.id);
  const firstIndex = withoutDuplicate.findIndex((existing) => existing.agent_id === agentId);
  withoutDuplicate.splice(firstIndex < 0 ? withoutDuplicate.length : firstIndex, 0, option);
  return withoutDuplicate;
}
export function reconcileFlatAgentOrders(
  options: AgentProfileOption[],
  sync: ProfileOrderState,
): AgentProfileOption[] {
  let next = options;
  for (const [agentId, state] of Object.entries(sync)) {
    const order = state.queued ?? state.inFlight ?? state.order;
    if (order) next = reorderFlatOptions(next, agentId, order);
  }
  return next;
}

export function getAgentProfileIds(agent: Agent): string[] {
  return agent.profiles.map((profile) => profile.id);
}
