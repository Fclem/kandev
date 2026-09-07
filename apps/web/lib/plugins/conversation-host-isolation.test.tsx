import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RawSessionEvent } from "@/lib/ws/client";
import { PluginConversationScopeProvider, pluginConversationApi } from "./conversation-host";

const transport = vi.hoisted(() => ({
  request: vi.fn(),
}));

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://host" }) }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    request: transport.request,
    onRawSessionEvent(_listener: (event: RawSessionEvent) => void) {
      return () => undefined;
    },
    onConnectionStatus(_listener: (status: "connected" | "disconnected") => void) {
      return () => undefined;
    },
  }),
}));

function ScopedHarness({ sessionId, testId }: { sessionId: string; testId: string }) {
  const state = pluginConversationApi.useSessionMessages({ sessionId });
  return <span data-testid={testId}>{String(state.hydrated)}</span>;
}

function response(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function renderScope(taskId: string, sessionId: string, testId: string) {
  return (
    <PluginConversationScopeProvider
      pluginId="plugin-history"
      taskId={taskId}
      sessionId={sessionId}
    >
      <ScopedHarness sessionId={sessionId} testId={testId} />
    </PluginConversationScopeProvider>
  );
}

describe("plugin conversation panel isolation", () => {
  beforeEach(() => {
    transport.request.mockReset();
    transport.request.mockResolvedValue({
      success: true,
      snapshot_token: "snapshot-1",
      resume_token: "resume-1",
      consumer_id: "consumer-1",
      event_watermark: 0,
      expires_at: "2099-01-01T00:00:00Z",
      result: "fresh",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        if (String(input).endsWith("/conversation/binding")) {
          return Promise.resolve(
            response({
              bindingToken: "binding-1",
              generation: 7,
              expiresAt: "2099-01-01T00:00:00Z",
            }),
          );
        }
        return Promise.resolve(response({ messages: [], hasMore: false, cursor: null }));
      }),
    );
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("gives concurrent panels independent consumer identities and snapshots", async () => {
    render(
      <>
        {renderScope("task-1", "session-1", "panel-one")}
        {renderScope("task-2", "session-2", "panel-two")}
      </>,
    );

    await waitFor(() => expect(screen.getByTestId("panel-one").textContent).toBe("true"));
    await waitFor(() => expect(screen.getByTestId("panel-two").textContent).toBe("true"));
    const subscriptions = transport.request.mock.calls.filter(
      ([action]) => action === "session.subscribe",
    );
    expect(subscriptions).toHaveLength(2);
    expect(subscriptions[0][1].consumer_id).not.toBe(subscriptions[1][1].consumer_id);
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("session-1"))).toBe(true);
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("session-2"))).toBe(true);
  });
});
