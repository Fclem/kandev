import { useMemo } from "react";
import { usableConfigOptions } from "@/components/model-config-selector";
import { useAppStore } from "@/components/state-provider";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { prioritizeSelectedOption } from "@/lib/utils/selector-options";
import type { Agent, AgentProfile, AvailableAgent } from "@/lib/types/http";
import type { SessionCatalogOwner, SessionCatalogRefreshEntry } from "@/lib/types/session-catalog";
import { projectSessionModeChoices } from "./mode-selector-catalog-refresh";

export type ModeOption = {
  id: string;
  name: string;
  description?: string;
  requiresRefresh?: boolean;
  disabled?: boolean;
};

export type ModeSelectorState = {
  thinking?: boolean;
  currentModeId: string;
  availableModes: ModeOption[];
  requestedModeId?: string;
};

function resolveSnapshotMode(snapshot: unknown): string | null {
  if (!snapshot || typeof snapshot !== "object") return null;
  const mode = (snapshot as Record<string, unknown>).mode;
  return typeof mode === "string" && mode ? mode : null;
}

function resolveProfileMode(profileId: string | null | undefined, agents: Agent[]): string | null {
  if (!profileId) return null;
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (profile?.mode) return profile.mode;
  }
  return null;
}

function resolveStaticModes(
  agents: Agent[],
  profileId: string | null | undefined,
  availableAgents: AvailableAgent[],
): ModeOption[] {
  if (!profileId) return [];
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (!profile) continue;
    const available = availableAgents.find((a: AvailableAgent) => a.name === agent.name);
    return available?.model_config?.available_modes ?? [];
  }
  return [];
}

export function formatModeName(modeId: string): string {
  return modeId
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

// withRequestedMode carries the requested mode onto the resolved state only
// when it differs from what the session is actually in.
function withRequestedMode(
  state: ModeSelectorState | undefined,
  requestedModeId: string | undefined,
): ModeSelectorState | undefined {
  if (!state || !requestedModeId || requestedModeId === state.currentModeId) return state;
  return { ...state, requestedModeId };
}

function buildModeState(
  currentModeId: string | null,
  liveModes: ModeOption[] | undefined,
  staticModes: ModeOption[],
): ModeSelectorState | undefined {
  const availableModes = liveModes?.length ? liveModes : staticModes;
  if (!currentModeId) {
    return availableModes.length > 0 ? { currentModeId: "", availableModes } : undefined;
  }
  if (availableModes.length === 0) {
    if (currentModeId === "default") return undefined;
    return {
      currentModeId,
      availableModes: [{ id: currentModeId, name: formatModeName(currentModeId) }],
    };
  }
  if (availableModes.length <= 1) return undefined;
  if (availableModes.some((m) => m.id === currentModeId)) {
    return {
      currentModeId,
      availableModes: prioritizeSelectedOption(availableModes, currentModeId, (mode) => mode.id),
    };
  }
  return {
    currentModeId,
    availableModes: [{ id: currentModeId, name: formatModeName(currentModeId) }, ...availableModes],
  };
}

export function useModeSelectorState(
  sessionId: string | null,
  entry: SessionCatalogRefreshEntry | undefined,
  sourceOwner: SessionCatalogOwner | undefined,
) {
  useSettingsData(true);

  const liveModeState = useAppStore((state) =>
    sessionId ? state.sessionMode.bySessionId[sessionId] : undefined,
  );
  const configOptions = useAppStore((state) =>
    sessionId ? state.sessionModels.bySessionId[sessionId]?.configOptions : undefined,
  );
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const taskSessions = useAppStore((state) => state.taskSessions.items);
  const { items: availableAgents } = useAvailableAgents();

  const session = sessionId ? (taskSessions[sessionId] ?? null) : null;
  const isPi = session?.agent_profile_snapshot?.agent_name === "pi-acp";
  const snapshotMode = resolveSnapshotMode(session?.agent_profile_snapshot);
  const profileMode = useMemo(
    () => resolveProfileMode(session?.agent_profile_id, settingsAgents as Agent[]),
    [session?.agent_profile_id, settingsAgents],
  );
  const staticModes = useMemo(
    () => resolveStaticModes(settingsAgents as Agent[], session?.agent_profile_id, availableAgents),
    [availableAgents, session?.agent_profile_id, settingsAgents],
  );

  const hasThinkingConfig =
    isPi &&
    usableConfigOptions(configOptions).some((option) => option.category === "thought_level");
  return useMemo(() => {
    if (hasThinkingConfig) return undefined;
    const currentModeId = liveModeState ? liveModeState.currentModeId : snapshotMode || profileMode;
    const state = buildModeState(currentModeId, liveModeState?.availableModes, staticModes);
    const resolved = withRequestedMode(
      projectSessionModeChoices({
        state,
        currentModeId,
        currentModeName:
          liveModeState?.availableModes.find((mode) => mode.id === currentModeId)?.name ??
          (currentModeId ? formatModeName(currentModeId) : ""),
        liveModes: liveModeState?.availableModes,
        session,
        entry,
        sourceOwner,
      }),
      liveModeState?.requestedModeId,
    );
    return resolved ? { ...resolved, thinking: isPi } : undefined;
  }, [
    liveModeState,
    profileMode,
    snapshotMode,
    staticModes,
    session,
    entry,
    sourceOwner,
    isPi,
    hasThinkingConfig,
  ]);
}
