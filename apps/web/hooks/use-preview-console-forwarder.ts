"use client";

import { useEffect } from "react";
import { isInspectorMessage, isPreviewConsoleMessage } from "@/lib/preview-inspect-bridge";

const PREFIX = "[preview]";

// Pipes iframe `console.log/warn/error/info/debug` calls — forwarded by the
// runtime shim injected by the gateway port-proxy — into the parent window's
// console with a `[preview]` prefix. Lets developers see iframe diagnostics
// without manually switching DevTools' execution context to the iframe.
//
// The `iframeRef` argument is used to verify that incoming messages came from
// the previewed iframe and not from another frame or extension; the origin is
// additionally pinned to the iframe's ACTUAL origin, derived from its src
// (the gateway that serves the preview). Deriving instead of assuming
// same-origin keeps forwarding correct in dev mode, where the UI (Vite) and
// the gateway run on different ports and therefore different origins.
export function usePreviewConsoleForwarder(
  iframeRef: React.RefObject<HTMLIFrameElement | null>,
): void {
  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      const frame = iframeRef.current;
      if (!frame) return;
      let expectedOrigin: string;
      try {
        expectedOrigin = new URL(frame.src).origin;
      } catch {
        return; // about:blank or unparseable src: nothing to forward
      }
      if (event.origin !== expectedOrigin) return;
      if (event.source !== frame.contentWindow) return;
      if (!isInspectorMessage(event.data)) return;
      if (!isPreviewConsoleMessage(event.data)) return;
      const { level, args } = event.data.payload;
      const fn = console[level] ?? console.log;
      fn.call(console, PREFIX, ...args);
    }
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [iframeRef]);
}
