import { describe, expect, it, vi, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type { RefObject } from "react";
import { usePreviewConsoleForwarder } from "./use-preview-console-forwarder";

// The console forwarder must only accept messages from the gateway origin
// (matching the shim's scoped postMessage target) AND from the previewed
// iframe's contentWindow; wrong-origin or wrong-source messages are ignored.
describe("usePreviewConsoleForwarder", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  function makeIframeRef(contentWindow: unknown) {
    return { current: { contentWindow } } as RefObject<HTMLIFrameElement | null>;
  }

  function dispatchMessage(data: unknown, origin: string, source: unknown) {
    window.dispatchEvent(
      new MessageEvent("message", { data, origin, source: source as MessageEventSource }),
    );
  }

  function consoleMessage(level: string, args: unknown[]) {
    return { source: "kandev-inspector", type: "console", payload: { level, args } };
  }

  it("forwards same-origin console messages from the preview iframe", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow)));

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), window.location.origin, contentWindow);
    });

    expect(warn).toHaveBeenCalledWith("[preview]", "boom");
  });

  it("ignores messages from a different origin", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow)));

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), "https://evil.example", contentWindow);
    });

    expect(warn).not.toHaveBeenCalled();
  });

  it("ignores messages from a different source window", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow)));

    act(() => {
      dispatchMessage(consoleMessage("warn", ["boom"]), window.location.origin, { other: true });
    });

    expect(warn).not.toHaveBeenCalled();
  });

  it("ignores non-console inspector messages", () => {
    const log = vi.spyOn(console, "log").mockImplementation(() => {});
    const contentWindow = {};
    renderHook(() => usePreviewConsoleForwarder(makeIframeRef(contentWindow)));

    act(() => {
      dispatchMessage(
        { source: "kandev-inspector", type: "other" },
        window.location.origin,
        contentWindow,
      );
    });

    expect(log).not.toHaveBeenCalled();
  });
});
