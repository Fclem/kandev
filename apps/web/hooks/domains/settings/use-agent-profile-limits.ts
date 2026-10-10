"use client";

import { useEffect } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listAgentProfileLimits } from "@/lib/api/domains/provider-limits-api";
import { agentListAuthScopeIdentity } from "./agent-list-resource";

export function useAgentProfileLimits() {
  const store = useAppStoreApi();
  const identity = useAppStore(agentListAuthScopeIdentity);
  const connected = useAppStore((state) => state.connection.status === "connected");
  useEffect(() => {
    const abort = new AbortController();
    const version = store.getState().agentProfileLimits.version;
    void listAgentProfileLimits({ cache: "no-store", init: { signal: abort.signal } })
      .then((limits) => {
        if (!abort.signal.aborted) store.getState().setAgentProfileLimits(limits, version);
      })
      .catch((error: unknown) => {
        if (!abort.signal.aborted) console.error(error);
      });
    return () => abort.abort();
  }, [connected, identity, store]);
}
