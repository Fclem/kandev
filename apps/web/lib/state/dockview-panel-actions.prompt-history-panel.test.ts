import { describe, expect, it } from "vitest";
import { buildExtraPanelActions } from "./dockview-panel-actions";
import { makeApi, makeStore } from "./dockview-panel-actions.test-utils";

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

    actions.scrollTranscriptToMessage("session-1", "message-1", "Agent");

    expect(api.getPanel("session:session-1")).toMatchObject({ api: { component: "chat" } });
    expect(store.state.scrollTarget).toMatchObject({
      sessionId: "session-1",
      messageId: "message-1",
      hostPanelId: "session:session-1",
    });
  });
});
