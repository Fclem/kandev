import { focusOrAddPanel } from "./dockview-layout-builders";
import {
  addSessionPanel,
  addSidePanel,
  buildReviewPanelActions,
  type SidePanelOpts,
  type StoreGet,
  type StoreSet,
} from "./dockview-panel-actions";
import { buildTerminalPanelActions } from "./dockview-terminal-panel-actions";
import { panelTitle } from "./layout-manager/panel-title";
import {
  parsePluginPanelId,
  pluginPanelId,
  PLUGIN_PANEL_COMPONENT,
  PLUGIN_PANEL_TAB_COMPONENT,
} from "./layout-manager/plugin-panels";

let scrollTargetToken = 0;

function buildTranscriptActions(set: StoreSet, get: StoreGet) {
  return {
    scrollTranscriptToMessage: (sessionId: string, messageId: string, title: string) => {
      const { api: dockviewApi, centerGroupId } = get();
      if (!dockviewApi) return;
      const sessionPanelId = `session:${sessionId}`;
      const targetPanel = dockviewApi.getPanel(sessionPanelId) ?? dockviewApi.getPanel("chat");
      if (targetPanel) {
        targetPanel.api.setActive();
      } else {
        addSessionPanel(dockviewApi, centerGroupId, sessionId, title);
      }
      scrollTargetToken += 1;
      set({
        scrollTarget: {
          sessionId,
          messageId,
          token: scrollTargetToken,
          hostPanelId: targetPanel?.id ?? sessionPanelId,
        },
      });
    },
    clearScrollTarget: (token: number) => {
      if (get().scrollTarget?.token === token) set({ scrollTarget: null });
    },
    clearScrollTargetForSession: (sessionId: string) => {
      if (get().scrollTarget?.sessionId === sessionId) set({ scrollTarget: null });
    },
    addVscodePanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: panelTitle("vscode"),
        position: { referenceGroup: centerGroupId },
      });
    },
    openInternalVscode: (_goto: { file: string; line: number; col: number } | null) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const existing = api.getPanel("vscode");
      if (existing) {
        existing.api.setActive();
        return;
      }
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: panelTitle("vscode"),
        position: { referenceGroup: centerGroupId },
      });
    },
  };
}

function buildSidePanelActions(get: StoreGet) {
  return {
    addPlanPanel: (opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        { id: "plan", component: "plan", title: panelTitle("plan"), tabComponent: "planTab" },
        opts,
      );
    },
    addPluginPanel: (pluginId: string, panelKey: string, title: string, opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        {
          id: pluginPanelId(pluginId, panelKey),
          component: PLUGIN_PANEL_COMPONENT,
          title,
          tabComponent: PLUGIN_PANEL_TAB_COMPONENT,
          params: { pluginId, panelKey },
        },
        opts,
      );
    },
    closePluginPanels: (pluginId: string) => {
      const { api } = get();
      if (!api) return;
      api.panels
        .filter((panel) => parsePluginPanelId(panel.id)?.pluginId === pluginId)
        .forEach((panel) => api.removePanel(panel));
    },
    addTodosPanel: (opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        { id: "todos", component: "todos", title: panelTitle("todos") },
        opts,
      );
    },
    addPromptHistoryPanel: (opts?: SidePanelOpts) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSidePanel(
        api,
        centerGroupId,
        { id: "prompt-history", component: "prompt-history", title: panelTitle("prompt-history") },
        opts,
      );
    },
  };
}

export function buildExtraPanelActions(setOrGet: StoreSet | StoreGet, maybeGet?: StoreGet) {
  const set: StoreSet = maybeGet ? (setOrGet as StoreSet) : () => {};
  const get: StoreGet = maybeGet ?? (setOrGet as StoreGet);
  return {
    ...buildTranscriptActions(set, get),
    ...buildSidePanelActions(get),
    ...buildReviewPanelActions(get),
    ...buildTerminalPanelActions(get),
  };
}
