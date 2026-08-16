import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const state = {
  tasks: { activeSessionId: "session-1" },
  taskSessions: { items: { "session-1": { name: "Agent" } } },
};
const messages = [
  {
    id: "prompt-1",
    session_id: "session-1",
    task_id: "task-1",
    author_type: "user",
    content: "A prompt that is rendered in history",
    type: "message",
    created_at: "2026-01-01T00:00:00.000Z",
  },
];
const navigate = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));
vi.mock("@/hooks/domains/session/use-session-messages", () => ({
  useSessionMessages: () => ({ messages }),
}));
vi.mock("@/hooks/domains/session/use-session-turns", () => ({
  useSessionTurns: () => [],
}));
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: () => navigate,
}));

import { PromptHistoryPanelContent } from "./prompt-history-panel-content";

beforeEach(() => {
  navigate.mockClear();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
});

describe("PromptHistoryPanelContent", () => {
  it("renders an active-session prompt and calls its navigation seam", () => {
    const onNavigateToPrompt = vi.fn();
    render(<PromptHistoryPanelContent onNavigateToPrompt={onNavigateToPrompt} />);

    expect(screen.getByTestId("prompt-history-panel")).toBeTruthy();
    expect(screen.getByTestId("prompt-history-row-0").textContent).toContain(messages[0].content);
    fireEvent.click(screen.getByTestId("prompt-history-jump-0"));
    expect(onNavigateToPrompt).toHaveBeenCalledWith("prompt-1");
  });
});
