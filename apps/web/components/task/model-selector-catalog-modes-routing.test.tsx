import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useStore } from "zustand";
import type { StoreApi } from "zustand";
import { createAppStore } from "@/lib/state/store";
import type { AppState } from "@/lib/state/store";
import { agentProfileId, sessionId, taskId } from "@/lib/types/http";
import {
  sessionCatalogRecordSchema,
  sessionProviderCatalogSchema,
} from "@/lib/types/session-catalog";
import * as sessionApi from "@/lib/api/domains/session-api";
import { ModelSelector } from "./model-selector";
import { ModeSelector } from "./mode-selector";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  toast: vi.fn(),
  updateToast: vi.fn(),
  availableAgents: [],
  mobile: false,
}));
let store: StoreApi<AppState>;
vi.mock("@/components/state-provider", () => {
  return {
    useAppStoreApi: () => store,
    useOptionalAppStoreApi: () => store,
    useAppStore: (selector: (state: AppState) => unknown) => useStore(store, selector),
  };
});
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast, updateToast: mocks.updateToast }),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/hooks/domains/settings/use-available-agents", () => ({
  useAvailableAgents: () => ({ items: mocks.availableAgents }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: mocks.mobile ? "mobile" : "desktop",
    isMobile: mocks.mobile,
    isTablet: false,
    isDesktop: !mocks.mobile,
    isCompactDesktop: false,
    isFullDesktop: !mocks.mobile,
    isFinePointer: !mocks.mobile,
    usesDesktopWorkbench: !mocks.mobile,
  }),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request: mocks.request }) }));
vi.mock("@/lib/api/domains/session-api", async (importOriginal) => {
  const actual = await importOriginal<typeof sessionApi>();
  return {
    ...actual,
    setSessionModel: vi.fn(async () => ({ ok: true })),
    setSessionConfigOption: vi.fn(async () => ({ ok: true })),
    setSessionMode: vi.fn(async () => ({ ok: true })),
    resolveSessionModelConfig: vi.fn(async () => ({ status: "ok", config_options: [] })),
  };
});

const CATALOG_TASK_ID = "catalog-task";
const PROVIDER_MODEL_ID = "provider-only";
const PROVIDER_MODEL_LABEL = "Provider only";
const MODEL_SETTINGS_LABEL = "Session model settings";
const CATALOG_CONFIRMATION_ID = "session-catalog-refresh-confirmation";
const ARIA_DISABLED_ATTRIBUTE = "aria-disabled";
const MODE_SELECTOR_TEST_ID = "session-mode-selector";

const id = "catalog-session";
const owner = { execution_id: "execution-1", attempt_id: "attempt-1", source_generation: 1 };
type ProbeChoice = { id: string; name?: string };
// The target's own complete task probe; only this provenance is selectable.
function taskProbe(models?: ProbeChoice[], modes?: ProbeChoice[]) {
  const unsupported = { present: false, supported: false, choices: null };
  return {
    source: "task_probe_v1",
    probe: {
      outcome: "complete",
      models: models ? { present: true, supported: true, choices: models } : unsupported,
      modes: modes ? { present: true, supported: true, choices: modes } : unsupported,
    },
  };
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.request.mockReset();
  mocks.mobile = false;
  store = createAppStore();
  store.getState().upsertTaskSessionFromEvent(CATALOG_TASK_ID, {
    id: sessionId(id),
    task_id: taskId(CATALOG_TASK_ID),
    state: "WAITING_FOR_INPUT",
    agent_execution_id: owner.execution_id,
    agent_profile_id: agentProfileId("profile"),
    agent_profile_snapshot: { agent_name: "mock-agent", model: "old-model" },
    started_at: "2026-11-16T00:00:00Z",
    updated_at: "2026-11-16T00:00:00Z",
    metadata: {
      acp_model_state: {
        settings_source_execution_id: owner.execution_id,
        settings_attempt_id: owner.attempt_id,
        settings_source_generation: 1,
      },
      runtime_config: { model: "old-model", mode: "default", config_options: { effort: "high" } },
      acp_config_baseline: { model: "old-model", effort: "medium" },
      composer_draft: "Unsent work",
    },
  });
  store.getState().setSessionModels(id, {
    currentModelId: "old-model",
    models: [
      { modelId: "old-model", name: "Old model" },
      { modelId: PROVIDER_MODEL_ID, name: PROVIDER_MODEL_LABEL },
    ],
    configOptionsSettled: true,
    configOptions: [
      {
        type: "select",
        id: "model",
        category: "model",
        name: "Model",
        currentValue: "old-model",
        options: [
          { value: "old-model", name: "Old model" },
          { value: PROVIDER_MODEL_ID, name: PROVIDER_MODEL_LABEL },
        ],
      },
    ],
  });
  store.getState().applySessionCatalogRefresh(
    CATALOG_TASK_ID,
    id,
    sessionCatalogRecordSchema.parse({
      version: 1,
      revision: 1,
      owner,
      profile_id: "profile",
      digest: "host",
      disposition: "host_candidates",
      reason: "new_candidates",
      models: [
        { id: "new-model", name: "New model" },
        { id: "old-model", name: "Old model" },
      ],
      modes: [],
      ...taskProbe(
        [
          { id: "new-model", name: "New model" },
          { id: "old-model", name: "Old model" },
        ],
        [],
      ),
    }),
  );
});
afterEach(cleanup);
function prepareModeChoices() {
  store.getState().setSessionMode(id, "default", [
    { id: "default", name: "Default" },
    { id: "provider-mode", name: "Provider mode" },
  ]);
  store.getState().applySessionCatalogRefresh(
    CATALOG_TASK_ID,
    id,
    sessionCatalogRecordSchema.parse({
      version: 1,
      revision: 2,
      owner,
      profile_id: "profile",
      disposition: "host_candidates",
      reason: "new_candidates",
      modes: [
        { id: "host-mode", name: "Host mode" },
        { id: "default", name: "Default" },
      ],
      ...taskProbe(undefined, [
        { id: "host-mode", name: "Host mode" },
        { id: "default", name: "Default" },
      ]),
    }),
  );
}

function openModePicker() {
  const trigger = screen.getByTestId(MODE_SELECTOR_TEST_ID);
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
  fireEvent.click(trigger);
}

it("offers refresh for a host-only mode without sending SetMode or restarting", () => {
  prepareModeChoices();
  render(
    <TooltipProvider>
      <ModeSelector sessionId={id} />
    </TooltipProvider>,
  );
  openModePicker();
  fireEvent.click(screen.getByRole("menuitem", { name: /Host mode/ }));
  expect(sessionApi.setSessionMode).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
  expect(screen.getByRole("dialog").contains(screen.getByTestId(CATALOG_CONFIRMATION_ID))).toBe(
    true,
  );
  expect(store.getState().sessionMode.bySessionId[id].currentModeId).toBe("default");
});

it.each(["empty", "models-only", "unsupported", "failed"] as const)(
  "retires stale selectable modes after explicit %s discovery without changing the current mode",
  (kind) => {
    prepareModeChoices();
    const absent = { present: false, supported: false, choices: null };
    const result = {
      outcome: kind === "empty" || kind === "models-only" ? "complete" : kind,
      models:
        kind === "models-only"
          ? { present: true, supported: true, choices: [{ id: "model", name: "Model" }] }
          : absent,
      modes: kind === "empty" ? { present: true, supported: true, choices: [] } : absent,
    };
    store.getState().applySessionProviderCatalog(
      CATALOG_TASK_ID,
      id,
      sessionProviderCatalogSchema.parse({
        revision: 1,
        profile_id: "profile",
        owner,
        result,
      }),
    );
    store.getState().applySessionCatalogRefresh(
      CATALOG_TASK_ID,
      id,
      sessionCatalogRecordSchema.parse({
        version: 1,
        revision: 3,
        profile_id: "profile",
        owner,
        result,
        disposition:
          kind === "unsupported" || kind === "failed" ? "rediscovery_required" : "cleared",
        reason: kind,
        models: [],
        modes: [],
      }),
    );
    render(
      <TooltipProvider>
        <ModeSelector sessionId={id} />
      </TooltipProvider>,
    );
    expect(screen.getByTestId(MODE_SELECTOR_TEST_ID).textContent).toContain("Default");
    openModePicker();
    const stale = screen.queryByRole("menuitem", { name: "Default" });
    expect(!stale || stale.getAttribute(ARIA_DISABLED_ATTRIBUTE) === "true").toBe(true);
    if (stale) fireEvent.click(stale);
    expect(screen.queryByRole("menuitem", { name: /Host mode/ })).toBeNull();
    expect(sessionApi.setSessionMode).not.toHaveBeenCalled();
    expect(store.getState().sessionMode.bySessionId[id].currentModeId).toBe("default");
  },
);

it("keeps phone mode refresh consent in the picker and cancels without SetMode or restart", async () => {
  mocks.mobile = true;
  prepareModeChoices();
  render(
    <TooltipProvider>
      <ModeSelector sessionId={id} />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByTestId(MODE_SELECTOR_TEST_ID));
  fireEvent.click(screen.getByRole("button", { name: "Host mode" }));
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1);
  const dialog = screen.getByRole("dialog");
  fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByTestId(CATALOG_CONFIRMATION_ID)).toBeNull());
  expect(screen.getByRole("button", { name: "Host mode" }).getAttribute("disabled")).toBeNull();
  expect(sessionApi.setSessionMode).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
});

it("uses newly advertised mode-only results through the ordinary SetMode path", async () => {
  prepareModeChoices();
  const result = {
    outcome: "complete",
    models: { present: false, supported: false, choices: null },
    modes: { present: true, supported: true, choices: [{ id: "fresh-mode", name: "Fresh mode" }] },
  };
  store.getState().applySessionProviderCatalog(
    CATALOG_TASK_ID,
    id,
    sessionProviderCatalogSchema.parse({
      revision: 1,
      owner,
      profile_id: "profile",
      result,
    }),
  );
  store.getState().applySessionCatalogRefresh(
    CATALOG_TASK_ID,
    id,
    sessionCatalogRecordSchema.parse({
      version: 1,
      revision: 3,
      owner,
      profile_id: "profile",
      disposition: "cleared",
      reason: "complete",
      result,
    }),
  );
  render(
    <TooltipProvider>
      <ModeSelector sessionId={id} />
    </TooltipProvider>,
  );
  openModePicker();
  fireEvent.click(screen.getByRole("menuitem", { name: "Fresh mode" }));
  await waitFor(() => expect(sessionApi.setSessionMode).toHaveBeenCalledWith(id, "fresh-mode"));
  expect(screen.queryByTestId(CATALOG_CONFIRMATION_ID)).toBeNull();
  expect(mocks.request).not.toHaveBeenCalled();
  expect(store.getState().sessionMode.bySessionId[id].currentModeId).toBe("default");
});

it("offers concrete-profile candidates and confirms restart for a routed session", async () => {
  const session = store.getState().taskSessions.items[id];
  store.getState().setTaskSession({
    ...session,
    agent_profile_id: agentProfileId("routing-parent"),
    execution_profile_id: agentProfileId("profile"),
  });
  store.getState().applySessionProviderCatalog(
    CATALOG_TASK_ID,
    id,
    sessionProviderCatalogSchema.parse({
      revision: 1,
      profile_id: "profile",
      owner,
      result: {
        outcome: "complete",
        models: {
          present: true,
          supported: true,
          choices: [
            { id: "old-model", name: "Old model" },
            { id: PROVIDER_MODEL_ID, name: PROVIDER_MODEL_LABEL },
          ],
        },
        modes: { present: false, supported: false, choices: null },
      },
    }),
  );
  mocks.request.mockResolvedValue({});
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  expect(
    screen
      .getByRole("option", { name: PROVIDER_MODEL_LABEL })
      .getAttribute(ARIA_DISABLED_ATTRIBUTE),
  ).not.toBe("true");
  fireEvent.click(screen.getByRole("option", { name: "New model" }));
  const confirmation = screen.getByTestId(CATALOG_CONFIRMATION_ID);
  expect(screen.getByRole("dialog").contains(confirmation)).toBe(true);
  fireEvent.click(within(confirmation).getByRole("button", { name: "Refresh session" }));
  await waitFor(() =>
    expect(mocks.request.mock.calls.map(([action]) => action)).toEqual([
      "session.stop",
      "session.launch",
    ]),
  );
  expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
  expect(store.getState().taskSessions.items[id].metadata?.runtime_config).toEqual(
    session.metadata?.runtime_config,
  );
});

it("offers concrete-profile mode candidates for a routed session", () => {
  prepareModeChoices();
  const session = store.getState().taskSessions.items[id];
  store.getState().setTaskSession({
    ...session,
    agent_profile_id: agentProfileId("routing-parent"),
    execution_profile_id: agentProfileId("profile"),
  });
  store.getState().applySessionProviderCatalog(
    CATALOG_TASK_ID,
    id,
    sessionProviderCatalogSchema.parse({
      revision: 1,
      profile_id: "profile",
      owner,
      result: {
        outcome: "complete",
        models: { present: false, supported: false, choices: null },
        modes: { present: true, supported: true, choices: [{ id: "default", name: "Default" }] },
      },
    }),
  );
  render(
    <TooltipProvider>
      <ModeSelector sessionId={id} />
    </TooltipProvider>,
  );
  openModePicker();
  fireEvent.click(screen.getByRole("menuitem", { name: /Host mode/ }));
  expect(screen.getByRole("dialog").contains(screen.getByTestId(CATALOG_CONFIRMATION_ID))).toBe(
    true,
  );
  expect(sessionApi.setSessionMode).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
});

it("retires open consent when the concrete execution profile changes under the same routing parent", async () => {
  const session = store.getState().taskSessions.items[id];
  store.getState().setTaskSession({ ...session, execution_profile_id: agentProfileId("profile") });
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("option", { name: "New model" }));
  const confirmation = screen.getByTestId(CATALOG_CONFIRMATION_ID);
  expect(screen.getByRole("dialog").contains(confirmation)).toBe(true);
  act(() =>
    store
      .getState()
      .setTaskSession({ ...session, execution_profile_id: agentProfileId("replacement-profile") }),
  );
  await waitFor(() => expect(screen.queryByRole("button", { name: "Refresh session" })).toBeNull());
  expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
});

const UNSUPPORTED_SURFACE = { present: false, supported: false, choices: null };

function applyProviderCatalog(providerOwner: typeof owner, result: Record<string, unknown>) {
  store.getState().applySessionProviderCatalog(
    CATALOG_TASK_ID,
    id,
    sessionProviderCatalogSchema.parse({
      revision: 1,
      profile_id: "profile",
      owner: providerOwner,
      result,
    }),
  );
}

function renderModeSelector() {
  render(
    <TooltipProvider>
      <ModeSelector sessionId={id} />
    </TooltipProvider>,
  );
}

it.each(["mock-agent", "codex-app-server"])(
  "hides the mode picker for a %s provider with models only and no current mode",
  (agentName) => {
    const session = store.getState().taskSessions.items[id];
    store.getState().setTaskSession({
      ...session,
      agent_profile_snapshot: { agent_name: agentName, model: "old-model" },
    });
    applyProviderCatalog(owner, {
      outcome: "complete",
      models: { present: true, supported: true, choices: [{ id: "old-model", name: "Old model" }] },
      modes: UNSUPPORTED_SURFACE,
    });
    renderModeSelector();
    expect(screen.queryByTestId(MODE_SELECTOR_TEST_ID)).toBeNull();
  },
);

it("keeps the mode picker for a models-only provider when the session has a current mode", () => {
  store.getState().setSessionMode(id, "default", [
    { id: "default", name: "Default" },
    { id: "plan", name: "Plan" },
  ]);
  applyProviderCatalog(owner, {
    outcome: "complete",
    models: { present: true, supported: true, choices: [{ id: "old-model", name: "Old model" }] },
    modes: UNSUPPORTED_SURFACE,
  });
  renderModeSelector();
  expect(screen.getByTestId(MODE_SELECTOR_TEST_ID).textContent).toContain("Default");
});

// A provider record left by an earlier owner (a failed start, or a previous
// launch whose transport never publishes discovery) is not evidence about the
// current owner and must not disable its live choices.
it("ignores a previous owner's provider record when the current owner never publishes", async () => {
  const previousOwner = {
    execution_id: "execution-0",
    attempt_id: "attempt-0",
    source_generation: 1,
  };
  store.getState().setSessionMode(id, "default", [
    { id: "default", name: "Default" },
    { id: "provider-mode", name: "Provider mode" },
  ]);
  applyProviderCatalog(previousOwner, {
    outcome: "complete",
    models: {
      present: true,
      supported: true,
      choices: [{ id: "stale-model", name: "Stale model" }],
    },
    modes: { present: true, supported: true, choices: [{ id: "stale-mode", name: "Stale mode" }] },
  });
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
      <ModeSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  expect(
    screen
      .getByRole("option", { name: PROVIDER_MODEL_LABEL })
      .getAttribute(ARIA_DISABLED_ATTRIBUTE),
  ).not.toBe("true");
  expect(screen.queryByRole("option", { name: "Stale model" })).toBeNull();
  fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
  cleanup();
  renderModeSelector();
  openModePicker();
  const providerMode = screen.getByRole("menuitem", { name: "Provider mode" });
  expect(providerMode.getAttribute(ARIA_DISABLED_ATTRIBUTE)).not.toBe("true");
  expect(screen.queryByRole("menuitem", { name: "Stale mode" })).toBeNull();
  fireEvent.click(providerMode);
  await waitFor(() => expect(sessionApi.setSessionMode).toHaveBeenCalledWith(id, "provider-mode"));
});
