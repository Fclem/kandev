import type { Agent } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import {
  orderProfilesForSelection,
  toSelectorProfileOptions,
} from "@/lib/settings/agent-profile-selector-order";

export function syncSavedAgentToStore(
  store: StoreApi<AppState>,
  agent: Agent,
  profileVersionAtSaveStart: number,
) {
  const state = store.getState();
  const settingsAgents = state.settingsAgents.items;
  const existing = settingsAgents.find((item) => item.id === agent.id);
  if (!existing && state.agentProfiles.version !== profileVersionAtSaveStart) return;
  let profiles = agent.profiles;
  let membershipChanged = false;

  if (state.agentProfiles.version === profileVersionAtSaveStart) {
    const incomingById = new Map(agent.profiles.map((profile) => [profile.id, profile]));
    const existingIds = new Set(existing?.profiles.map((profile) => profile.id) ?? []);
    const newProfiles = agent.profiles.filter((profile) => !existingIds.has(profile.id));
    const existingProfiles = (existing?.profiles ?? [])
      .map((profile) => {
        const incoming = incomingById.get(profile.id);
        const currentTime = parseTurnTimestamp(profile.updatedAt);
        const incomingTime = parseTurnTimestamp(incoming?.updatedAt);
        return incoming &&
          currentTime !== null &&
          (incomingTime === null || incomingTime < currentTime)
          ? profile
          : incoming;
      })
      .filter((profile): profile is Agent["profiles"][number] => profile !== undefined);
    membershipChanged =
      newProfiles.length > 0 ||
      (existing !== undefined &&
        existing.profiles.some((profile) => !incomingById.has(profile.id)));
    profiles = [...orderProfilesForSelection(newProfiles), ...existingProfiles];
  } else if (existing) {
    profiles = existing.profiles;
  }

  const reconciled = existing
    ? {
        ...existing,
        name: agent.name,
        workspace_id: agent.workspace_id,
        mcp_config_path: agent.mcp_config_path,
        profiles,
      }
    : { ...agent, profiles };
  const nextAgents = existing
    ? settingsAgents.map((item) => (item.id === agent.id ? reconciled : item))
    : [...settingsAgents, reconciled];
  state.setSettingsAgents(nextAgents);
  const savedProfileOptions = toSelectorProfileOptions([reconciled]);
  const nextProfileOptions: typeof state.agentProfiles.items = [];
  const savedIds = new Set(savedProfileOptions.map((profile) => profile.id));
  const previousIds = new Set(existing?.profiles.map((profile) => profile.id) ?? []);
  let savedGroupInserted = false;
  for (const option of state.agentProfiles.items) {
    if (option.agent_id === reconciled.id) {
      if (!savedGroupInserted) {
        nextProfileOptions.push(...savedProfileOptions);
        savedGroupInserted = true;
      }
      if (!savedIds.has(option.id) && (option.workspace_id || !previousIds.has(option.id)))
        nextProfileOptions.push(option);
      continue;
    }
    nextProfileOptions.push(option);
  }
  if (!savedGroupInserted) nextProfileOptions.push(...savedProfileOptions);
  state.setAgentProfiles(nextProfileOptions);
  if (membershipChanged) state.bumpAgentProfilesVersion();
  return reconciled;
}
