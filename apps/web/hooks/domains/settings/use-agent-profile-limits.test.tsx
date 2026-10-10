import { act, cleanup, renderHook } from "@testing-library/react";
import { createElement, StrictMode } from "react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AgentProfileLimit } from "@/lib/types/http-agents";

const mocks = vi.hoisted(() => ({ listLimits: vi.fn() }));
vi.mock("@/lib/api/domains/provider-limits-api", () => ({
  listAgentProfileLimits: mocks.listLimits,
}));
import { useAgentProfileLimits } from "./use-agent-profile-limits";

const limit: AgentProfileLimit = {
  profile_id: "limited-profile",
  model: "opus",
  scope: "account",
  until: "2026-10-03T13:00:00Z",
  reset_known: true,
};

function useSnapshot() {
  useAgentProfileLimits();
  return {
    store: useAppStoreApi(),
    limit: useAppStore((state) => state.agentProfileLimits.byProfileId[limit.profile_id]),
  };
}
function wrapper({ children }: { children: ReactNode }) {
  return createElement(StrictMode, null, createElement(StateProvider, null, children));
}
beforeEach(() => mocks.listLimits.mockReset());
afterEach(() => cleanup());

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-002.8
it("keeps initial limits discoverable after StrictMode cancels the first mount while WS is offline", async () => {
  let resolve!: (limits: AgentProfileLimit[]) => void;
  mocks.listLimits.mockReturnValue(
    new Promise<AgentProfileLimit[]>((done) => {
      resolve = done;
    }),
  );
  const mounted = renderHook(useSnapshot, { wrapper, reactStrictMode: true });
  await act(async () => resolve([limit]));
  expect(mounted.result.current.limit?.scope).toBe("account");
});

it("cannot restore an already-cleared profile from a pending read", async () => {
  let resolve!: (limits: AgentProfileLimit[]) => void;
  mocks.listLimits.mockReturnValue(
    new Promise<AgentProfileLimit[]>((done) => {
      resolve = done;
    }),
  );
  const mounted = renderHook(useSnapshot, { wrapper, reactStrictMode: true });
  act(() => mounted.result.current.store.getState().setAgentProfileLimits([]));
  await act(async () => resolve([limit]));
  expect(mounted.result.current.limit).toBeUndefined();
});
