import { act, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { Agent } from "@/lib/types/http";

const AGENT_ID = "agent";
const AGENT_NAME = "mock-agent";
const OFFICE_AGENT_ID = "office-agent";
const PROFILE_IDS = {
  created: "created",
  deleted: "deleted",
  live: "live",
  office: "office-profile",
  stale: "stale",
};

function Capture({ onStore }: { onStore: (store: StoreApi<AppState>) => void }) {
  onStore(useAppStoreApi());
  return null;
}

function option(id: string, agentId = AGENT_ID, agentName = AGENT_NAME) {
  return { id, label: id, agent_id: agentId, agent_name: agentName, cli_passthrough: false };
}

describe("agent page list snapshot membership fence", () => {
  it("rejects a delayed pre-create/delete GET without changing either mirrored list", () => {
    let store!: StoreApi<AppState>;
    render(
      <StateProvider>
        <Capture
          onStore={(value) => {
            store = value;
          }}
        />
      </StateProvider>,
    );
    const api = store;
    const existing = {
      id: AGENT_ID,
      name: AGENT_NAME,
      profiles: [
        { id: PROFILE_IDS.live, name: "Live" },
        { id: PROFILE_IDS.deleted, name: "Deleted" },
      ],
    } as Agent;
    act(() => {
      api.getState().setSettingsAgents([existing]);
      api.getState().setAgentProfiles([option(PROFILE_IDS.live), option(PROFILE_IDS.deleted)]);
    });
    const epoch = api.getState().agentProfiles.version;
    const created = { id: PROFILE_IDS.created, name: "Created" };
    act(() => {
      api.getState().bumpAgentProfilesVersion();
      api.getState().setSettingsAgents([
        {
          ...existing,
          profiles: [created, ...existing.profiles],
        } as Agent,
      ]);
      api
        .getState()
        .setAgentProfiles([
          option(PROFILE_IDS.created),
          option(PROFILE_IDS.live),
          option(PROFILE_IDS.deleted),
        ]);
      api.getState().bumpAgentProfilesVersion();
      api.getState().setSettingsAgents([
        {
          ...existing,
          profiles: [created, existing.profiles[0]],
        } as Agent,
      ]);
      api.getState().setAgentProfiles([option(PROFILE_IDS.created), option(PROFILE_IDS.live)]);
    });
    const accepted = api.getState().applyAgentListSnapshot(
      [
        {
          ...existing,
          profiles: [existing.profiles[0], existing.profiles[1]],
        } as Agent,
      ],
      epoch,
    );
    expect(accepted).toBe(false);
    expect(api.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      PROFILE_IDS.created,
      PROFILE_IDS.live,
    ]);
    expect(api.getState().agentProfiles.items.map((profile) => profile.id)).toEqual([
      PROFILE_IDS.created,
      PROFILE_IDS.live,
    ]);
  });
});

describe("accepted agent-list snapshots", () => {
  it("retains Office profile options for agents absent from an accepted list snapshot", () => {
    let store!: StoreApi<AppState>;
    render(
      <StateProvider>
        <Capture
          onStore={(value) => {
            store = value;
          }}
        />
      </StateProvider>,
    );
    const api = store;
    const existing = {
      id: AGENT_ID,
      name: AGENT_NAME,
      profiles: [
        { id: PROFILE_IDS.live, name: "Live" },
        { id: PROFILE_IDS.stale, name: "Stale" },
      ],
    } as Agent;
    act(() => {
      api.getState().setSettingsAgents([existing]);
      api
        .getState()
        .setAgentProfiles([
          option(PROFILE_IDS.live),
          option(PROFILE_IDS.stale),
          option(PROFILE_IDS.office, OFFICE_AGENT_ID, "Office Agent"),
        ]);
    });

    const accepted = api.getState().applyAgentListSnapshot(
      [
        {
          ...existing,
          profiles: [existing.profiles[0]],
        } as Agent,
      ],
      api.getState().agentProfiles.version,
    );

    expect(accepted).toBe(true);
    expect(api.getState().settingsAgents.items[0].profiles.map((profile) => profile.id)).toEqual([
      PROFILE_IDS.live,
    ]);
    expect(api.getState().agentProfiles.items.map(({ agent_id, id }) => [agent_id, id])).toEqual([
      [AGENT_ID, PROFILE_IDS.live],
      [OFFICE_AGENT_ID, PROFILE_IDS.office],
    ]);
  });
});
