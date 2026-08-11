"use client";

import { useCallback, useRef } from "react";
import { duplicateAgentProfileAction } from "@/app/actions/agents";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { t as translate } from "@/lib/i18n";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { AppState } from "@/lib/state/store";
import type { Agent, AgentProfile } from "@/lib/types/http";

type ProfileState = Pick<AppState, "settingsAgents" | "agentProfiles">;

/**
 * Merge one duplicated profile into the latest store state. Dedupes by ID so
 * a concurrent `agent.profile.created` WS delivery cannot double-insert the
 * copy into either slice.
 *
 * The options slice is rebuilt from `settingsAgents` by ID on top of the
 * existing options: options the rebuild does not represent (e.g. profiles the
 * WS handler delivered for agents temporarily absent from `settingsAgents`)
 * are preserved, and the new copy always appears exactly once — via the
 * rebuild when its owning agent is present, otherwise via a stub so it is
 * never dropped.
 *
 * A stale duplicate HTTP response never clobbers a newer profile: when the
 * copy already exists in the store (e.g. another tab edited it and
 * `agent.profile.updated` arrived first), the newer `updatedAt` wins.
 */
export function applyProfileDuplicated(
  state: ProfileState,
  agent: Agent,
  created: AgentProfile,
): ProfileState {
  const nextAgents = state.settingsAgents.items.map((item: Agent) =>
    item.id === agent.id
      ? (() => {
          const existing = item.profiles.find((p) => p.id === created.id);
          const latest = existing && existing.updatedAt > created.updatedAt ? existing : created;
          return {
            ...item,
            profiles: [...item.profiles.filter((p) => p.id !== created.id), latest],
          };
        })()
      : item,
  );

  const rebuiltOptions = nextAgents.flatMap((item) =>
    item.profiles.map((profile) => toAgentProfileOption(item, profile)),
  );
  const rebuiltCopy = rebuiltOptions.some((option) => option.id === created.id);
  const preserved = state.agentProfiles.items.filter((option) => {
    if (option.id === created.id) {
      // Owner present: the rebuilt option wins (the store profile was already
      // resolved by the newer-updatedAt logic above). Owner absent: keep an
      // existing WS-delivered option only when it is NEWER than this —
      // possibly stale — HTTP response; otherwise let the copy option below
      // replace it with the known agent metadata.
      if (rebuiltCopy) return false;
      return (option.updatedAt ?? "") > (created.updatedAt ?? "");
    }
    return !rebuiltOptions.some((rebuilt) => rebuilt.id === option.id);
  });
  const copyOption = toAgentProfileOption({ id: agent.id, name: agent.name }, created);
  const copyAlreadyPresent = rebuiltCopy || preserved.some((option) => option.id === created.id);

  const agentProfilesItems = copyAlreadyPresent
    ? [...preserved, ...rebuiltOptions]
    : [...preserved, copyOption, ...rebuiltOptions];

  return {
    settingsAgents: { ...state.settingsAgents, items: nextAgents },
    agentProfiles: { ...state.agentProfiles, items: agentProfilesItems },
  };
}

/**
 * Duplicate a profile from the /settings/agents profile list. The returned
 * copy is merged into the store atomically so the new row appears without
 * waiting on the WebSocket round-trip.
 */
export function useProfileDuplicate() {
  const { toast } = useToast();
  const storeApi = useAppStoreApi();
  // Per-profile in-flight guard: the row button stays clickable while the
  // request runs, so a double-click must not issue two copies.
  const pendingRef = useRef(new Set<string>());

  return useCallback(
    async (agent: Agent, profile: AgentProfile) => {
      if (pendingRef.current.has(profile.id)) return;
      pendingRef.current.add(profile.id);
      try {
        const created = await duplicateAgentProfileAction(profile.id);
        storeApi.setState((state) => applyProfileDuplicated(state, agent, created));
        toast({
          title: translate("agents:duplicateProfileSuccess"),
          description: created.name,
          variant: "success",
        });
      } catch (error) {
        toast({
          title: translate("agents:failedToDuplicateProfile"),
          description: error instanceof Error ? error.message : translate("agents:requestFailed"),
          variant: "error",
        });
      } finally {
        pendingRef.current.delete(profile.id);
      }
    },
    [storeApi, toast],
  );
}
