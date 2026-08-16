import { describe, expect, it } from "vitest";
import { buildExtraPanelActions } from "./dockview-panel-actions";
import { makeApi, makeStore } from "./dockview-panel-actions.test-utils";
import { CENTER_GROUP } from "./layout-manager";

const SESSION_ID = "session-1";
const MESSAGE_ID = "message-1";

describe("addPromptHistoryPanel", () => {
  it("adds the registered panel in the invoking group", () => {
    const api = makeApi({ extraGroupIds: ["group-invoking"] });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.addPromptHistoryPanel({ groupId: "group-invoking", inCenter: true });

    const panel = api.getPanel("prompt-history");
    expect(panel).toMatchObject({
      id: "prompt-history",
      group: { id: "group-invoking" },
      api: { component: "prompt-history" },
    });
  });
});

describe("scrollTranscriptToMessage", () => {
  it("opens a session chat target and records its exact owner", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    expect(api.getPanel(`session:${SESSION_ID}`)).toMatchObject({ api: { component: "chat" } });
    expect(store.state.scrollTarget).toMatchObject({
      sessionId: SESSION_ID,
      messageId: MESSAGE_ID,
      hostPanelId: `session:${SESSION_ID}`,
    });
  });

  it("focuses an existing session panel instead of adding a second tab", () => {
    const api = makeApi();
    api.addPanel({
      id: "session:session-1",
      component: "chat",
      title: "Agent",
      position: { referenceGroup: CENTER_GROUP },
    });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    const panelCount = api.panels.length;

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    expect(api.panels).toHaveLength(panelCount);
    expect(
      (api.getPanel(`session:${SESSION_ID}`) as unknown as { isActive: boolean }).isActive,
    ).toBe(true);
    expect(store.state.scrollTarget?.hostPanelId).toBe("session:session-1");
  });

  it("focuses the canonical chat panel without adding a session tab when it is the only chat target", () => {
    const api = makeApi();
    api.addPanel({
      id: "chat",
      component: "chat",
      title: "Agent",
      position: { referenceGroup: CENTER_GROUP },
    });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    expect(api.getPanel(`session:${SESSION_ID}`)).toBeUndefined();
    expect((api.getPanel("chat") as unknown as { isActive: boolean }).isActive).toBe(true);
    expect(store.state.scrollTarget?.hostPanelId).toBe("chat");
  });

  it("activates the target panel before recording the target", () => {
    const api = makeApi();
    api.addPanel({
      id: "chat",
      component: "chat",
      title: "Agent",
      position: { referenceGroup: CENTER_GROUP },
    });
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    const chatPanel = api.getPanel("chat");
    if (!chatPanel) throw new Error("chat panel did not seed");

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    expect((chatPanel as unknown as { isActive: boolean }).isActive).toBe(true);
    expect(store.state.scrollTarget).not.toBeNull();
  });

  it("monotonically increases the token across requests", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);

    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");
    const firstToken = store.state.scrollTarget?.token ?? -1;
    actions.scrollTranscriptToMessage(SESSION_ID, "message-2", "Agent");
    const secondToken = store.state.scrollTarget?.token ?? -1;

    expect(secondToken).toBeGreaterThan(firstToken);
  });

  it("clearScrollTarget clears only on the exact token", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");
    const token = store.state.scrollTarget?.token ?? -1;

    actions.clearScrollTarget(token - 1);
    expect(store.state.scrollTarget).not.toBeNull();

    actions.clearScrollTarget(token);
    expect(store.state.scrollTarget).toBeNull();
  });

  it("clearScrollTargetForSession clears only the matching session", () => {
    const api = makeApi();
    const store = makeStore(api);
    const actions = buildExtraPanelActions(store.set, store.get);
    actions.scrollTranscriptToMessage(SESSION_ID, MESSAGE_ID, "Agent");

    actions.clearScrollTargetForSession("session-2");
    expect(store.state.scrollTarget).not.toBeNull();

    actions.clearScrollTargetForSession(SESSION_ID);
    expect(store.state.scrollTarget).toBeNull();
  });
});
