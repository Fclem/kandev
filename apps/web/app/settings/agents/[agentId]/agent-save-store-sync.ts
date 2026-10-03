import type { Agent } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

export function syncSavedAgentToStore(
  store: StoreApi<AppState>,
  agent: Agent,
  profileVersionAtSaveStart: number,
) {
  const state = store.getState();
  const settingsAgents = state.settingsAgents.items;
  const existing = settingsAgents.find((item) => item.id === agent.id);
  let profiles = agent.profiles;
  let membershipChanged = false;

  if (state.agentProfiles.version === profileVersionAtSaveStart) {
    const incomingById = new Map(agent.profiles.map((profile) => [profile.id, profile]));
    const existingIds = new Set(existing?.profiles.map((profile) => profile.id) ?? []);
    const newProfiles = agent.profiles.filter((profile) => !existingIds.has(profile.id));
    const existingProfiles = (existing?.profiles ?? [])
      .map((profile) => incomingById.get(profile.id))
      .filter((profile): profile is Agent["profiles"][number] => profile !== undefined);
    membershipChanged =
      newProfiles.length > 0 ||
      (existing !== undefined &&
        existing.profiles.some((profile) => !incomingById.has(profile.id)));
    profiles = [...newProfiles, ...existingProfiles];
  } else if (existing) {
    profiles = existing.profiles;
  }

  const reconciled = { ...agent, profiles };
  const nextAgents = existing
    ? settingsAgents.map((item) => (item.id === agent.id ? reconciled : item))
    : [...settingsAgents, reconciled];
  state.setSettingsAgents(nextAgents);
  state.setAgentProfiles(
    nextAgents.flatMap((item) =>
      item.profiles.map((profile) => toAgentProfileOption(item, profile)),
    ),
  );
  if (membershipChanged) state.bumpAgentProfilesVersion();
}
