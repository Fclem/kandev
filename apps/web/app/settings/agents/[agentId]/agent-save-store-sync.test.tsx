import { act, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { Agent } from "@/lib/types/http";
import { registerAgentsHandlers } from "@/lib/ws/handlers/agents";
import { syncSavedAgentToStore } from "./agent-save-store-sync";

const AGENT_ID = "agent";
const LIVE_PROFILE_ID = "live";
const CREATED_PROFILE_ID = "created";
const DELETED_PROFILE_ID = "deleted";

function Capture({ onStore }: { onStore: (store: StoreApi<AppState>) => void }) {
  onStore(useAppStoreApi());
  return null;
}

function profile(id: string, name = id) {
  return { id, name, model: "mock-fast", enabled: true } as Agent["profiles"][number];
}

function option(id: string) {
  return {
    id,
    label: id,
    agent_id: AGENT_ID,
    agent_name: "mock-agent",
    cli_passthrough: false,
  };
}

function profileEvent(id: string, name: string, timestamp: string) {
  return {
    id,
    type: "notification",
    action: "agent.profile.created",
    timestamp,
    payload: {
      profile: {
        id,
        agent_id: AGENT_ID,
        name,
        model: "mock-fast",
        enabled: true,
        created_at: "2026-07-26T10:00:00Z",
        updated_at: timestamp,
      },
    },
  };
}

describe("saved agent store synchronization", () => {
  it("keeps profile create/delete events that arrive while the save response is pending", async () => {
    let store!: StoreApi<AppState>;
    render(
      <StateProvider>
        <Capture onStore={(value) => (store = value)} />
      </StateProvider>,
    );
    const existing = {
      id: AGENT_ID,
      name: "mock-agent",
      profiles: [profile(LIVE_PROFILE_ID), profile(DELETED_PROFILE_ID)],
    } as Agent;
    act(() => {
      store.getState().setSettingsAgents([existing]);
      store.getState().setAgentProfiles([option(LIVE_PROFILE_ID), option(DELETED_PROFILE_ID)]);
    });
    const versionAtSaveStart = store.getState().agentProfiles.version;
    const handlers = registerAgentsHandlers(store);
    let resolveSave!: (agent: Agent) => void;
    const response = new Promise<Agent>((resolve) => {
      resolveSave = resolve;
    });
    const save = response.then((agent) => syncSavedAgentToStore(store, agent, versionAtSaveStart));

    act(() => {
      handlers["agent.profile.created"]!(
        profileEvent(CREATED_PROFILE_ID, "Created during save", "2026-07-26T11:00:00Z") as never,
      );
      handlers["agent.profile.deleted"]!({
        id: "deleted-event",
        type: "notification",
        action: "agent.profile.deleted",
        timestamp: "2026-07-26T12:00:00Z",
        payload: {
          profile: {
            id: DELETED_PROFILE_ID,
            agent_id: AGENT_ID,
            name: "Deleted during save",
            model: "mock-fast",
            enabled: true,
            created_at: "2026-07-26T10:00:00Z",
            updated_at: "2026-07-26T10:00:00Z",
          },
        },
      } as never);
    });

    await act(async () => {
      resolveSave({
        ...existing,
        name: "saved-agent-name",
        profiles: [profile(LIVE_PROFILE_ID), profile(DELETED_PROFILE_ID)],
      });
      await save;
    });

    const savedAgent = store.getState().settingsAgents.items[0];
    expect(savedAgent.name).toBe("saved-agent-name");
    expect(savedAgent.profiles.map((item) => item.id)).toEqual([
      CREATED_PROFILE_ID,
      LIVE_PROFILE_ID,
    ]);
    expect(store.getState().agentProfiles.items.map((item) => item.id)).toEqual([
      CREATED_PROFILE_ID,
      LIVE_PROFILE_ID,
    ]);
  });
});
