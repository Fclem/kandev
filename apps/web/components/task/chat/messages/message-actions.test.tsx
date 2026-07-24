import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { StateProvider } from "@/components/state-provider";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { MessageActions } from "./message-actions";

const TOUCH_DRAWER = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => TOUCH_DRAWER.enabled,
}));

const MESSAGE_TIMESTAMP = "2026-07-20T10:15:00Z";

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "message-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "user",
    content: "Keep the remaining actions",
    type: "message",
    created_at: MESSAGE_TIMESTAMP,
    ...overrides,
  };
}

afterEach(() => {
  TOUCH_DRAWER.enabled = false;
  cleanup();
});

describe("MessageActions", () => {
  it("renders user navigation alongside the existing actions", () => {
    const onToggleRaw = vi.fn();
    const onPrevious = vi.fn();
    const onNext = vi.fn();
    render(
      <StateProvider>
        <MessageActions
          message={message()}
          onToggleRaw={onToggleRaw}
          navigation={{
            canNavigatePrevious: true,
            canNavigateNext: false,
            isBusy: false,
            onPrevious,
            onNext,
          }}
        />
      </StateProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Show raw text" }));
    const previous = screen.getByRole("button", { name: "Previous user message" });
    previous.focus();
    fireEvent.click(previous);

    expect(onToggleRaw).toHaveBeenCalledOnce();
    expect(onPrevious).toHaveBeenCalledOnce();
    expect(onNext).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(previous);
    expect(screen.getByRole("button", { name: "Copy message to clipboard" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Next user message" }).hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("does not render navigation for an agent message", () => {
    render(
      <StateProvider>
        <MessageActions
          message={message({ author_type: "agent" })}
          navigation={{
            canNavigatePrevious: true,
            canNavigateNext: true,
            isBusy: false,
            onPrevious: vi.fn(),
            onNext: vi.fn(),
          }}
        />
      </StateProvider>,
    );

    expect(screen.queryByRole("button", { name: "Previous user message" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Next user message" })).toBeNull();
  });
});

describe("MessageActions timestamp tooltip", () => {
  it("renders the relative timestamp as a <time> element with the full absolute time as its title", () => {
    const { container } = render(
      <StateProvider>
        <MessageActions message={message({ author_type: "agent" })} />
      </StateProvider>,
    );

    const timeEl = container.querySelector("time");
    expect(timeEl).not.toBeNull();
    expect(timeEl?.getAttribute("dateTime")).toBe(MESSAGE_TIMESTAMP);
    expect(timeEl?.getAttribute("title")).toBe(new Date(MESSAGE_TIMESTAMP).toLocaleString());
  });

  it("omits the timestamp element entirely when showTimestamp is false", () => {
    const { container } = render(
      <StateProvider>
        <MessageActions message={message({ author_type: "agent" })} showTimestamp={false} />
      </StateProvider>,
    );

    expect(container.querySelector("time")).toBeNull();
  });
});

describe("MessageActions timestamp tooltip on touch devices", () => {
  it("exposes the full absolute time via a tap-to-open drawer instead of relying on hover-only title", () => {
    TOUCH_DRAWER.enabled = true;
    const expectedAbsoluteTime = new Date(MESSAGE_TIMESTAMP).toLocaleString();

    render(
      <StateProvider>
        <MessageActions message={message({ author_type: "agent" })} />
      </StateProvider>,
    );

    const trigger = screen.getByTestId("message-timestamp-trigger");
    expect(trigger.querySelector("time")).not.toBeNull();
    expect(screen.queryByText(expectedAbsoluteTime)).toBeNull();

    fireEvent.click(trigger);

    expect(screen.getByText(expectedAbsoluteTime)).not.toBeNull();
  });
});
