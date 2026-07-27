import { I18nProvider as LinguiI18nProvider } from "@lingui/react";
import type { ReactNode } from "react";

import { i18n } from "./index";

/**
 * Wraps the app in Lingui's provider bound to the shared `i18n` instance.
 * Lingui re-renders consumers when `i18n.loadAndActivate` runs, so a locale
 * switch updates the whole tree without a reload. Mounted as the outermost
 * provider in app-shell so every surface (toasts, dialogs, sidebar) is covered.
 */
export function I18nProvider({ children }: { children: ReactNode }) {
  return <LinguiI18nProvider i18n={i18n}>{children}</LinguiI18nProvider>;
}
