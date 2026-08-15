import { describe, expect, it, vi, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { RefObject } from "react";
import { usePreviewConsoleForwarder } from "./use-preview-console-forwarder";

// The console forwarder must only accept messages from the previewed iframe's
// ACTUAL origin (derived from its src, so dev mode works when the UI and the
// gateway run on different ports) AND from the iframe's contentWindow;
// wrong-origin or wrong-source messages are ignored.
describe("usePreviewConsoleForwarder", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  const UI_ORIGIN = window.location.origin; // jsdom: http://localhost:3000
  const GATEWAY_ORIGIN = "http://localhost:38430";

  function makeIframeRef(contentWindow: unknown, src: string) {
    return { current: { contentWindow, src } } as RefObject<HTMLIFrameElement | null>;
  }

  function dispatchMessage(data: unknown, origin: string, source: unknown) {
    window.dispatchEvent(
      new MessageEvent("message", { data, origin, source: source as MessageEventSource }),
    );
  }

  function consoleMessage(level: string, args: unknown[]) {
    return { source: "kandev-inspector", type: "console", payload: { level, args } };
  }

  it("forwards messages from the iframe's origin (prod: same as the UI)", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() =>
      usePreviewConsoleForwarder(
        makeIframeRef(contentWindow, `${UI_ORIGIN}/port-proxy/sess/5173/`),
      ),
    );

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), UI_ORIGIN, contentWindow);
    });

    expect(warn).toHaveBeenCalledWith("[preview]", "boom");
  });

  it("forwards messages in dev mode when the gateway is on a different port", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() =>
      usePreviewConsoleForwarder(
        makeIframeRef(contentWindow, `${GATEWAY_ORIGIN}/port-proxy/sess/5173/`),
      ),
    );

    // Message arrives from the gateway origin (the iframe's src), not the UI's.
    act(() => {
      dispatchMessage(consoleMessage("warn", ["dev"]), GATEWAY_ORIGIN, contentWindow);
    });

    expect(warn).toHaveBeenCalledWith("[preview]", "dev");
  });

  it("ignores messages from a different origin", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() =>
      usePreviewConsoleForwarder(
        makeIframeRef(contentWindow, `${GATEWAY_ORIGIN}/port-proxy/sess/5173/`),
      ),
    );

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), "https://evil.example", contentWindow);
    });

    expect(warn).not.toHaveBeenCalled();
  });

  it("ignores messages from a different source window", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() =>
      usePreviewConsoleForwarder(
        makeIframeRef(contentWindow, `${GATEWAY_ORIGIN}/port-proxy/sess/5173/`),
      ),
    );

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), GATEWAY_ORIGIN, { other: true });
    });

    expect(warn).not.toHaveBeenCalled();
  });

  it("ignores non-console inspector messages", () => {
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() =>
      usePreviewConsoleForwarder(
        makeIframeRef(contentWindow, `${GATEWAY_ORIGIN}/port-proxy/sess/5173/`),
      ),
    );

    act(() => {
      dispatchMessage({ source: "kandev-inspector", type: "other" }, GATEWAY_ORIGIN, contentWindow);
    });

    expect(log).not.toHaveBeenCalled();
  });

  it("ignores messages when the iframe src is not parseable", () => {
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow, "about:blank")));

    act(() => {
      dispatchMessage(consoleMessage("log", ["x"]), GATEWAY_ORIGIN, contentWindow);
    });

    expect(log).not.toHaveBeenCalled();
  });
});
