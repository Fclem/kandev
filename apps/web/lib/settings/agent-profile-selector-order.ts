import type { Agent } from "@/lib/types/http";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { normalizeAgentProfiles } from "@/lib/api/domains/agent-profile-normalize";

type CreatedProfile = { id: string; createdAt?: string };

/** Settings order is not a selector input. Retain source order for unstamped legacy options. */
export function orderProfilesForSelection<T extends CreatedProfile>(profiles: T[]): T[] {
  if (profiles.some((profile) => !profile.createdAt)) return profiles;
  const keyed = profiles.map((profile) => ({
    profile,
    milliseconds: Date.parse(profile.createdAt!),
    submillisecond: Number(
      (profile.createdAt!.match(/\.(\d+)(?:Z|[+-]\d{2}:\d{2})$/)?.[1] ?? "")
        .padEnd(9, "0")
        .slice(3),
    ),
  }));
  return keyed
    .toSorted(
      (left, right) =>
        right.milliseconds - left.milliseconds ||
        right.submillisecond - left.submillisecond ||
        (left.profile.id < right.profile.id ? -1 : Number(left.profile.id > right.profile.id)),
    )
    .map(({ profile }) => profile);
}

/** Restore global profile baselines while keeping scoped options and orphan slots fixed. */
export function orderProfileOptionsForSelection(
  options: AgentProfileOption[],
): AgentProfileOption[] {
  const groups = new Map<string, AgentProfileOption[]>();
  for (const option of options) {
    if (!option.createdAt || option.workspace_id) continue;
    const group = groups.get(option.agent_id);
    if (group) group.push(option);
    else groups.set(option.agent_id, [option]);
  }
  const ordered = new Map(
    [...groups].map(([id, profiles]) => [id, orderProfilesForSelection(profiles)]),
  );
  const offsets = new Map<string, number>();
  const next = options.map((option) => {
    if (!option.createdAt || option.workspace_id) return option;
    const offset = offsets.get(option.agent_id) ?? 0;
    offsets.set(option.agent_id, offset + 1);
    return ordered.get(option.agent_id)![offset];
  });
  return next.every((option, index) => option === options[index]) ? options : next;
}

export function toSelectorProfileOptions(agents: Agent[]): AgentProfileOption[] {
  return agents.flatMap((agent) =>
    orderProfilesForSelection(agent.profiles).map((profile) =>
      toAgentProfileOption(agent, profile),
    ),
  );
}

/** Boot options omit creation timestamps; recover them from the matching profile snapshot. */
export function hydrateSelectorProfileOptions(
  options: AgentProfileOption[],
  agents: Agent[],
): AgentProfileOption[] {
  const creationById = new Map<string, string | undefined>(
    agents.flatMap((agent) =>
      normalizeAgentProfiles(agent).profiles.map(
        (profile) => [profile.id, profile.createdAt] as const,
      ),
    ),
  );
  return orderProfileOptionsForSelection(
    options.map((option) => ({
      ...option,
      createdAt: option.createdAt ?? creationById.get(option.id),
    })),
  );
}
