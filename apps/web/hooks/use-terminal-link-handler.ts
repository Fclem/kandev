import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { openExternalLink } from "@/lib/desktop/external-links";
import { useTranslation } from "react-i18next";

/**
 * Returns a stable callback for handling terminal link clicks.
 * Reads the user's `terminalLinkBehavior` setting to decide whether
 * to open URLs in a new browser tab or the built-in browser panel.
 */
export function useTerminalLinkHandler(): (event: MouseEvent, uri: string) => void {
  const { t } = useTranslation();
  const behaviorRef = useRef<"new_tab" | "browser_panel">("new_tab");
  const behavior = useAppStore((s) => s.userSettings.terminalLinkBehavior);

  useEffect(() => {
    behaviorRef.current = behavior;
  }, [behavior]);

  return useCallback((_event: MouseEvent, uri: string) => {
    if (behaviorRef.current === "browser_panel") {
      const api = useDockviewStore.getState().api;
      if (api) {
        const browserId = `browser:${Date.now()}`;
        api.addPanel({
          id: browserId,
          component: "browser",
          title: t("common:browser"),
          params: { url: uri },
        });
        return;
      }
    }
    void openExternalLink(uri).catch(() => undefined);
  }, []);
}
