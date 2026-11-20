import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import type { StoreApi } from "zustand";
import { createAppStore } from "@/lib/state/store";
import type { AppState } from "@/lib/state/store";
import { agentProfileId, sessionId, taskId } from "@/lib/types/http";
import { sessionCatalogRecordSchema } from "@/lib/types/session-catalog";
import { SessionCatalogRefreshNotice } from "./session-catalog-refresh-notice";

const mocks = vi.hoisted(() => ({ request: vi.fn(), toast: vi.fn(), updateToast: vi.fn() }));
let store: StoreApi<AppState>;
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast, updateToast: mocks.updateToast }),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request: mocks.request }) }));

const NOTICE_TASK_ID = "notice-task";
const CATALOG_CONFIRMATION_ID = "session-catalog-refresh-confirmation";

const id = "notice-session";
const owner = { execution_id: "execution", attempt_id: "attempt", source_generation: 1 };
const record = sessionCatalogRecordSchema.parse({
  version: 1,
  revision: 1,
  profile_id: "profile",
  owner,
  disposition: "rediscovery_required",
  reason: "remote_executor",
});
beforeEach(() => {
  vi.clearAllMocks();
  store = createAppStore();
  store.getState().upsertTaskSessionFromEvent(NOTICE_TASK_ID, {
    id: sessionId(id),
    task_id: taskId(NOTICE_TASK_ID),
    state: "WAITING_FOR_INPUT",
    agent_execution_id: owner.execution_id,
    agent_profile_id: agentProfileId("profile"),
    started_at: "2026-11-16T00:00:00Z",
    updated_at: "2026-11-16T00:00:00Z",
    metadata: {
      runtime_config: { model: "kept", mode: "plan", config_options: { effort: "high" } },
      composer_draft: "Unsent work",
    },
  });
  store.getState().applySessionCatalogRefresh(NOTICE_TASK_ID, id, record);
});
afterEach(cleanup);

it("invalidates open consent when a newer notice arrives without dispatching a restart", () => {
  render(<SessionCatalogRefreshNotice sessionId={id} />);
  fireEvent.click(screen.getByRole("button", { name: "Refresh session" }));
  expect(screen.getByRole("dialog").contains(screen.getByTestId(CATALOG_CONFIRMATION_ID))).toBe(
    true,
  );
  act(() =>
    store.getState().applySessionCatalogRefresh(NOTICE_TASK_ID, id, {
      ...record,
      revision: 2,
      reason: "new_context",
    }),
  );
  expect(screen.queryByTestId(CATALOG_CONFIRMATION_ID)).toBeNull();
  expect(mocks.request).not.toHaveBeenCalled();
});

it("recovers a durable notice on reload without depending on model hydration or recreating checking", () => {
  const session = store.getState().taskSessions.items[id];
  store = createAppStore({
    taskSessions: {
      items: {
        [id]: {
          ...session,
          metadata: { ...session.metadata, profile_catalog_refresh: record },
        },
      },
    },
  });
  render(<SessionCatalogRefreshNotice sessionId={id} />);
  const action = screen.getByRole("button", { name: "Refresh session" });
  expect(action.getAttribute("disabled")).toBeNull();
  expect(store.getState().sessionCatalogRefresh.bySessionId[id]?.checking).toBeUndefined();
  expect(store.getState().taskSessions.items[id].metadata?.composer_draft).toBe("Unsent work");
  expect(mocks.request).not.toHaveBeenCalled();
});

it.each(["unsupported", "failed"] as const)(
  "retires %s presentation when a different executor owns the session",
  (outcome) => {
    const view = render(<SessionCatalogRefreshNotice sessionId={id} />);
    const pendingStatus = screen.getByRole("status").textContent;
    const updateOwner = (executionId: string, updatedAt: string) => {
      const session = store.getState().taskSessions.items[id];
      store.getState().upsertTaskSessionFromEvent(NOTICE_TASK_ID, {
        ...session,
        agent_execution_id: executionId,
        updated_at: updatedAt,
        metadata: {
          ...session.metadata,
          acp_model_state: {
            settings_source_execution_id: executionId,
            settings_attempt_id: owner.attempt_id,
            settings_source_generation: owner.source_generation,
          },
        },
      });
    };
    act(() => {
      updateOwner(owner.execution_id, "2026-11-16T00:00:01Z");
      store.getState().applySessionCatalogRefresh(NOTICE_TASK_ID, id, {
        ...record,
        revision: 2,
        result: {
          outcome,
          models: { present: false, supported: false, choices: null },
          modes: { present: false, supported: false, choices: null },
        },
      });
    });
    expect(screen.getByRole("status").textContent).not.toBe(pendingStatus);
    act(() => updateOwner("replacement-executor", "2026-11-16T00:00:02Z"));
    expect(screen.getByRole("status").textContent).toBe(pendingStatus);
    expect(
      screen.getByRole("button", { name: "Refresh session" }).getAttribute("disabled"),
    ).toBeNull();
    expect(mocks.request).not.toHaveBeenCalled();
    view.unmount();
  },
);

it("does not expose another concrete profile's retained notice", () => {
  render(<SessionCatalogRefreshNotice sessionId={id} />);
  act(() => {
    const session = store.getState().taskSessions.items[id];
    store.getState().upsertTaskSessionFromEvent(NOTICE_TASK_ID, {
      ...session,
      execution_profile_id: agentProfileId("other-concrete-profile"),
      updated_at: "2026-11-16T00:00:01Z",
    });
  });
  expect(screen.queryByTestId("session-catalog-refresh-notice")).toBeNull();
  expect(mocks.request).not.toHaveBeenCalled();
});
