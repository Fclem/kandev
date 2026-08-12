import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";
import type { AgentProfile } from "@/lib/types/agent-profile";

function buildProfileEntry(profile: unknown): AgentProfile {
  return normalizeAgentProfile(profile);
}

function getAgentId(raw: unknown): string {
  const obj = (raw ?? {}) as Record<string, unknown>;
  const value = obj.agentId ?? obj.agent_id;
  return typeof value === "string" ? value : "";
}

// Office-scoped profiles are owned by the office channels, never the kanban
// settings surface (the HTTP agent list hides them via filterGlobalProfiles).
// Defense in depth: ignore their events here so a stray broadcast cannot leak
// them into the global settings store and selectors.
function isOfficeScoped(normalized: { workspaceId?: string }): boolean {
  return Boolean(normalized.workspaceId);
}

// Deletion tombstones keyed by profile id -> deletion event timestamp, so a
// delayed create/update event cannot resurrect a profile that was deleted
// after the event was produced. Cleared when a genuinely newer create arrives.
const deletionTombstones = new Map<string, string>();

function findExistingProfile(state: AppState, profileId: string): AgentProfile | undefined {
  for (const item of state.settingsAgents.items) {
    const found = item.profiles.find((p) => p.id === profileId);
    if (found) return found;
  }
  return undefined;
}

/**
 * A profile event is stale when the store already holds a newer revision of
 * the profile or its option (e.g. a delayed duplicate response or a newer
 * WebSocket update arrived first), or when the profile was deleted after the
 * event was produced (tombstone). Events must never regress newer state.
 * Missing event timestamps (never produced by the backend) are treated as
 * not-stale so they cannot trip the guards.
 */
function isStaleProfileEvent(
  state: AppState,
  normalized: { id: string; updatedAt?: string },
  eventTimestamp: string | undefined,
): boolean {
  const tombstone = deletionTombstones.get(normalized.id);
  if (tombstone !== undefined && eventTimestamp && tombstone >= eventTimestamp) return true;
  const existingProfile = findExistingProfile(state, normalized.id);
  if (existingProfile && (existingProfile.updatedAt ?? "") > (normalized.updatedAt ?? "")) {
    return true;
  }
  const existingOption = state.agentProfiles.items.find((o) => o.id === normalized.id);
  return Boolean(existingOption && (existingOption.updatedAt ?? "") > (normalized.updatedAt ?? ""));
}

function handleProfileCreated(
  state: AppState,
  profile: unknown,
  eventTimestamp: string | undefined,
): Partial<AppState> {
  const normalized = normalizeAgentProfile(profile);
  if (isOfficeScoped(normalized)) return {};
  if (isStaleProfileEvent(state, normalized, eventTimestamp)) return {};
  deletionTombstones.delete(normalized.id); // a genuinely newer create wins
  const agentId = getAgentId(profile);
  const agent = state.settingsAgents.items.find((a) => a.id === agentId);
  const agentStub = { id: agentId, name: agent?.name ?? "" };
  const nextProfiles = [
    ...state.agentProfiles.items.filter((p) => p.id !== normalized.id),
    toAgentProfileOption(agentStub, normalized),
  ];
  const nextAgents = state.settingsAgents.items.map((item) =>
    item.id === agentId
      ? {
          ...item,
          profiles: [
            ...item.profiles.filter((p) => p.id !== normalized.id),
            buildProfileEntry(profile),
          ],
        }
      : item,
  );
  return {
    agentProfiles: { ...state.agentProfiles, items: nextProfiles },
    settingsAgents: { items: nextAgents },
  };
}

function handleProfileUpdated(
  state: AppState,
  profile: unknown,
  eventTimestamp: string | undefined,
): Partial<AppState> {
  const normalized = normalizeAgentProfile(profile);
  if (isOfficeScoped(normalized)) return {};
  if (isStaleProfileEvent(state, normalized, eventTimestamp)) return {};
  const agentId = getAgentId(profile);
  const agent = state.settingsAgents.items.find((a) => a.id === agentId);
  const agentStub = { id: agentId, name: agent?.name ?? "" };
  const nextProfiles = state.agentProfiles.items.map((p) =>
    p.id === normalized.id ? toAgentProfileOption(agentStub, normalized) : p,
  );
  const nextAgents = state.settingsAgents.items.map((item) =>
    item.id === agentId
      ? {
          ...item,
          profiles: item.profiles.map((p) => (p.id === normalized.id ? normalized : p)),
        }
      : item,
  );
  return {
    agentProfiles: { ...state.agentProfiles, items: nextProfiles },
    settingsAgents: { items: nextAgents },
  };
}

function handleProfileDeleted(
  state: AppState,
  profile: unknown,
  eventTimestamp: string | undefined,
): Partial<AppState> {
  const normalized = normalizeAgentProfile(profile);
  if (isOfficeScoped(normalized)) return {};
  if (eventTimestamp) deletionTombstones.set(normalized.id, eventTimestamp);
  const agentId = getAgentId(profile);
  const nextAgents = state.settingsAgents.items.map((item) =>
    item.id === agentId
      ? {
          ...item,
          profiles: item.profiles.filter((p) => p.id !== normalized.id),
        }
      : item,
  );
  return {
    agentProfiles: {
      ...state.agentProfiles,
      items: state.agentProfiles.items.filter((p) => p.id !== normalized.id),
    },
    settingsAgents: { items: nextAgents },
  };
}

export function registerAgentsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "agent.available.updated": (message) => {
      store.setState((state) => ({
        ...state,
        availableAgents: {
          items: message.payload.agents ?? [],
          tools: message.payload.tools ?? state.availableAgents.tools,
          loaded: true,
          loading: false,
        },
      }));
    },
    "agent.install.started": (message) => {
      // Payload is the full job snapshot (queued → running transitions both emit this).
      store.getState().upsertInstallJob(message.payload);
    },
    "agent.install.output": (message) => {
      const { agent_name, chunk } = message.payload as {
        agent_name: string;
        chunk: string;
      };
      store.getState().appendInstallOutput(agent_name, chunk);
    },
    "agent.install.finished": (message) => {
      store.getState().upsertInstallJob(message.payload);
    },
    "agent.update.started": (message) => {
      store.getState().upsertAgentUpdateJob(message.payload);
    },
    "agent.update.output": (message) => {
      const { agent_name, job_id, chunk } = message.payload;
      store.getState().appendAgentUpdateOutput(agent_name, job_id, chunk);
    },
    "agent.update.finished": (message) => {
      store.getState().upsertAgentUpdateJob(message.payload);
    },
    "agent.updated": (message) => {
      store.setState((state) => ({
        ...state,
        agents: {
          agents: state.agents.agents.some((a) => a.id === message.payload.agentId)
            ? state.agents.agents.map((a) =>
                a.id === message.payload.agentId ? { ...a, status: message.payload.status } : a,
              )
            : [
                ...state.agents.agents,
                { id: message.payload.agentId, status: message.payload.status },
              ],
        },
      }));
    },
    "agent.profile.created": (message) => {
      store.setState((state) => ({
        ...state,
        ...handleProfileCreated(state, message.payload.profile, message.timestamp),
      }));
    },
    "agent.profile.updated": (message) => {
      store.setState((state) => ({
        ...state,
        ...handleProfileUpdated(state, message.payload.profile, message.timestamp),
      }));
    },
    "agent.profile.deleted": (message) => {
      store.setState((state) => ({
        ...state,
        ...handleProfileDeleted(state, message.payload.profile, message.timestamp),
      }));
    },
  };
}
