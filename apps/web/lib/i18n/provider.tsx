import { I18nextProvider } from "react-i18next";
import type { ReactNode } from "react";

import { i18n } from "./index";

/**
 * Binds react-i18next to the shared i18next instance. `changeLanguage` notifies
 * subscribers, so a locale switch re-renders the whole tree without a reload.
 * Mounted as the outermost provider in app-shell so every surface (toasts,
 * dialogs, sidebar) is covered.
 */
export function I18nProvider({ children }: { children: ReactNode }) {
  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>;
}
