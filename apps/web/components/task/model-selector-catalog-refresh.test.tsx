import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

const id = "catalog-session";
const owner = { execution_id: "execution-1", attempt_id: "attempt-1", source_generation: 1 };
type ProbeChoice = { id: string; name?: string };
// The target's own complete task probe; only this provenance is selectable.
function taskProbe(models: ProbeChoice[], modes?: ProbeChoice[]) {
  return {
    source: "task_probe_v1",
    probe: {
      outcome: "complete",
      models: { present: true, supported: true, choices: models },
      modes: modes
        ? { present: true, supported: true, choices: modes }
        : { present: false, supported: false, choices: null },
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

it("opens refresh consent for a host-only candidate without model/config/resolve or lifecycle requests", () => {
  const before = store.getState().taskSessions.items[id].metadata;
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("option", { name: "New model" }));
  expect(screen.getByRole("dialog").contains(screen.getByTestId(CATALOG_CONFIRMATION_ID))).toBe(
    true,
  );
  expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
  expect(sessionApi.setSessionConfigOption).not.toHaveBeenCalled();
  expect(sessionApi.setSessionMode).not.toHaveBeenCalled();
  expect(sessionApi.resolveSessionModelConfig).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
  expect(store.getState().taskSessions.items[id].metadata).toEqual(before);
  expect(store.getState().sessionModels.bySessionId[id].currentModelId).toBe("old-model");
});

it("does not reuse host candidates after the same execution rotates its settings owner", () => {
  const session = store.getState().taskSessions.items[id];
  store.getState().setTaskSession({
    ...session,
    metadata: {
      ...session.metadata,
      acp_model_state: {
        settings_source_execution_id: owner.execution_id,
        settings_attempt_id: "next-attempt",
        settings_source_generation: 2,
      },
    },
  });
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  expect(screen.queryByRole("option", { name: "New model" })).toBeNull();
  expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
});

it("offers candidates for an initial owner whose empty attempt is omitted from the persisted snapshot", () => {
  const session = store.getState().taskSessions.items[id];
  store.getState().setTaskSession({
    ...session,
    metadata: {
      ...session.metadata,
      acp_model_state: {
        settings_source_execution_id: owner.execution_id,
        settings_source_generation: 1,
      },
    },
  });
  store.getState().applySessionCatalogRefresh(
    CATALOG_TASK_ID,
    id,
    sessionCatalogRecordSchema.parse({
      version: 1,
      revision: 2,
      owner: { ...owner, attempt_id: "" },
      profile_id: "profile",
      digest: "initial-host",
      disposition: "host_candidates",
      reason: "new_candidates",
      models: [{ id: "new-model", name: "New model" }],
      modes: [],
      ...taskProbe([{ id: "new-model", name: "New model" }], []),
    }),
  );
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("option", { name: "New model" }));
  expect(screen.getByRole("dialog").contains(screen.getByTestId(CATALOG_CONFIRMATION_ID))).toBe(
    true,
  );
  expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
});

it.each(["empty", "mode-only", "unsupported", "failed"] as const)(
  "retires stale selectable models after explicit %s discovery while retaining the selected label",
  (kind) => {
    const absent = { present: false, supported: false, choices: null };
    const result = {
      outcome: kind === "empty" || kind === "mode-only" ? "complete" : kind,
      models: kind === "empty" ? { present: true, supported: true, choices: [] } : absent,
      modes:
        kind === "mode-only"
          ? { present: true, supported: true, choices: [{ id: "plan", name: "Plan" }] }
          : absent,
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
        revision: 2,
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
        <ModelSelector sessionId={id} />
      </TooltipProvider>,
    );
    const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
    expect(trigger.textContent).toContain("Old model");
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    const stale = screen.queryByRole("option", { name: "Old model" });
    expect(stale?.getAttribute(ARIA_DISABLED_ATTRIBUTE) ?? "true").toBe("true");
    if (stale) fireEvent.click(stale);
    expect(screen.queryByRole("option", { name: "New model" })).toBeNull();
    expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
    expect(sessionApi.setSessionConfigOption).not.toHaveBeenCalled();
    expect(mocks.request).not.toHaveBeenCalled();
  },
);

it("keeps live-provider-only choices on the ordinary configuration path", async () => {
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("option", { name: PROVIDER_MODEL_LABEL }));
  await waitFor(() =>
    expect(sessionApi.setSessionConfigOption).toHaveBeenCalledWith(id, "model", PROVIDER_MODEL_ID),
  );
  expect(screen.queryByTestId(CATALOG_CONFIRMATION_ID)).toBeNull();
  expect(mocks.request).not.toHaveBeenCalled();
});

it.each([
  {
    nextOwner: { execution_id: "execution-2", attempt_id: "resume-2", source_generation: 1 },
    staleExecution: false,
  },
  {
    nextOwner: {
      execution_id: owner.execution_id,
      attempt_id: "rediscovery-2",
      source_generation: 2,
    },
    staleExecution: false,
  },
  {
    nextOwner: {
      execution_id: "execution-3",
      attempt_id: "live-before-http",
      source_generation: 1,
    },
    staleExecution: true,
  },
])(
  "uses fresh provider choices before HTTP metadata catches up to $nextOwner.attempt_id",
  async ({ nextOwner, staleExecution }) => {
    const session = store.getState().taskSessions.items[id];
    if (!staleExecution)
      store.getState().setTaskSession({ ...session, agent_execution_id: nextOwner.execution_id });
    store
      .getState()
      .setSessionAgentctlStatus(id, { status: "ready", agentExecutionId: nextOwner.execution_id });
    store.getState().applySessionProviderCatalog(
      CATALOG_TASK_ID,
      id,
      sessionProviderCatalogSchema.parse({
        revision: 2,
        profile_id: "profile",
        owner: nextOwner,
        result: {
          outcome: "complete",
          models: {
            present: true,
            supported: true,
            choices: [{ id: PROVIDER_MODEL_ID, name: PROVIDER_MODEL_LABEL }],
          },
          modes: { present: false, supported: false, choices: null },
        },
      }),
    );
    store.getState().applySessionCatalogRefresh(
      CATALOG_TASK_ID,
      id,
      sessionCatalogRecordSchema.parse({
        version: 1,
        revision: 2,
        profile_id: "profile",
        owner: nextOwner,
        disposition: "cleared",
        reason: "discovery_complete",
      }),
    );
    render(
      <TooltipProvider>
        <ModelSelector sessionId={id} />
      </TooltipProvider>,
    );
    const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    const choice = screen.getByRole("option", { name: PROVIDER_MODEL_LABEL });
    expect(choice.getAttribute(ARIA_DISABLED_ATTRIBUTE)).not.toBe("true");
    expect(store.getState().taskSessions.items[id].metadata).toEqual(session.metadata);
    fireEvent.click(choice);
    await waitFor(() =>
      expect(sessionApi.setSessionConfigOption).toHaveBeenCalledWith(
        id,
        "model",
        PROVIDER_MODEL_ID,
      ),
    );
    expect(screen.queryByTestId(CATALOG_CONFIRMATION_ID)).toBeNull();
  },
);

it("does not revive a previous provider's choices after the live execution advances", () => {
  store
    .getState()
    .setSessionAgentctlStatus(id, { status: "ready", agentExecutionId: "execution-2" });
  store.getState().applySessionProviderCatalog(
    CATALOG_TASK_ID,
    id,
    sessionProviderCatalogSchema.parse({
      revision: 2,
      profile_id: "profile",
      owner,
      result: {
        outcome: "complete",
        models: {
          present: true,
          supported: true,
          choices: [{ id: PROVIDER_MODEL_ID, name: PROVIDER_MODEL_LABEL }],
        },
        modes: { present: false, supported: false, choices: null },
      },
    }),
  );
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  expect(screen.queryByRole("option", { name: PROVIDER_MODEL_LABEL })).toBeNull();
  expect(
    screen.getByRole("option", { name: "Old model" }).getAttribute(ARIA_DISABLED_ATTRIBUTE),
  ).toBe("true");
  expect(sessionApi.setSessionConfigOption).not.toHaveBeenCalled();
});

function publishGroupedHostCatalog(composed: boolean, mixed = false) {
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
      ...taskProbe([
        { id: "new-model", name: "New model" },
        { id: "old-model", name: "Old model" },
      ]),
      models: [
        { id: "new-model", name: "New model" },
        { id: "old-model", name: "Old model" },
      ],
      catalog: {
        version: 1,
        nodes: [
          {
            kind: "group",
            id: "host-group",
            label: "Fresh group",
            children: [
              { kind: "choice", value: "new-model", label: "New model" },
              { kind: "choice", value: "old-model", label: "Old model" },
            ],
          },
        ],
        compositions: composed
          ? [
              {
                id: "host-pairing",
                group_id: "host-group",
                label: "Fresh group",
                dimensions: [
                  {
                    id: "pairing",
                    label: "Pairing",
                    choices: mixed
                      ? [
                          { value: "x", label: "Pairing X" },
                          { value: "y", label: "Pairing Y" },
                        ]
                      : [{ value: "x", label: "Pairing X" }],
                  },
                ],
                bindings: mixed
                  ? [
                      { values: { pairing: "x" }, model: "old-model" },
                      { values: { pairing: "y" }, model: "new-model" },
                    ]
                  : [{ values: { pairing: "x" }, model: "new-model" }],
              },
            ]
          : undefined,
      },
    }),
  );
}

it("preserves host group and leaf order and appends provider-only choices in their own group", () => {
  publishGroupedHostCatalog(false);
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  const host = screen.getByRole("group", { name: "Fresh group" });
  expect(
    within(host)
      .getAllByRole("option")
      .map((row) => row.getAttribute("aria-label")),
  ).toEqual(["New model", "Old model"]);
  const live = screen.getByRole("option", { name: PROVIDER_MODEL_LABEL });
  // Ordinary singleton groups retain their existing accessible breadcrumb.
  const description = (live.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ");
  expect(description).toContain("Current session choices");
});

it("offers refresh before opening a host-only composed draft or resolving dependent provider options", () => {
  publishGroupedHostCatalog(true);
  const before = store.getState().sessionModels.bySessionId[id];
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  fireEvent.click(screen.getByTestId("model-catalog-composition-row-host-group"));
  expect(screen.getByRole("dialog").contains(screen.getByTestId(CATALOG_CONFIRMATION_ID))).toBe(
    true,
  );
  expect(sessionApi.resolveSessionModelConfig).not.toHaveBeenCalled();
  expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
  expect(sessionApi.setSessionConfigOption).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
  expect(store.getState().sessionModels.bySessionId[id]).toEqual(before);
});

it("blocks a host-only binding in a mixed composition before changing its preview draft or resolving options", async () => {
  publishGroupedHostCatalog(true, true);
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  const trigger = screen.getByRole("button", { name: MODEL_SETTINGS_LABEL });
  fireEvent.pointerDown(trigger);
  fireEvent.click(trigger);
  fireEvent.click(screen.getByTestId("model-catalog-composition-row-host-group"));
  await waitFor(() =>
    expect(screen.getByTestId("model-composition-apply").getAttribute("disabled")).toBeNull(),
  );
  fireEvent.click(screen.getByTestId("model-composition-dimension-pairing"));
  fireEvent.click(screen.getByTestId("model-composition-choice-pairing-y"));
  expect(sessionApi.resolveSessionModelConfig).not.toHaveBeenCalledWith(
    id,
    expect.objectContaining({ model: "new-model" }),
  );
  expect(screen.getByRole("dialog").contains(screen.getByTestId(CATALOG_CONFIRMATION_ID))).toBe(
    true,
  );
  expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
  expect(sessionApi.setSessionConfigOption).not.toHaveBeenCalled();
  expect(mocks.request).not.toHaveBeenCalled();
  expect(store.getState().sessionModels.bySessionId[id].currentModelId).toBe("old-model");
});

it("uses one phone picker modal for refresh consent and restores choices on Cancel", async () => {
  mocks.mobile = true;
  render(
    <TooltipProvider>
      <ModelSelector sessionId={id} />
    </TooltipProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: MODEL_SETTINGS_LABEL }));
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1);
  fireEvent.click(screen.getByRole("option", { name: "New model" }));
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(1);
  const dialog = screen.getByRole("dialog");
  fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByTestId(CATALOG_CONFIRMATION_ID)).toBeNull());
  expect(
    screen.getByRole("option", { name: "New model" }).getAttribute(ARIA_DISABLED_ATTRIBUTE),
  ).toBe("false");
  expect(mocks.request).not.toHaveBeenCalled();
  expect(sessionApi.setSessionModel).not.toHaveBeenCalled();
});
