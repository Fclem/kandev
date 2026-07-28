import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { UIStringsProvider, type UIStrings } from "@kandev/ui/lib/ui-strings";

/**
 * Feeds translated values into the @kandev/ui `UIStringsProvider` so the shared
 * primitives' built-in accessibility labels ("Close", "Toggle Sidebar", …) are
 * localized. `useTranslation` makes this reactive: switching locale
 * re-renders and updates the strings. The package itself has no i18n dependency
 * — this bridge lives in the app.
 */
export function TranslatedUIStrings({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const strings: UIStrings = {
    close: t("common:close"),
    loading: t("common:loading"),
    previousSlide: t("common:previousSlide"),
    nextSlide: t("common:nextSlide"),
    more: t("common:more2"),
    morePages: t("common:morePages"),
    sidebar: t("common:sidebar"),
    sidebarDescription: t("common:displaysTheMobileSidebar"),
    toggleSidebar: t("common:toggleSidebar"),
    previous: t("common:previous"),
    next: t("common:next"),
    goToPreviousPage: t("common:goToPreviousPage"),
    goToNextPage: t("common:goToNextPage"),
  };
  return <UIStringsProvider value={strings}>{children}</UIStringsProvider>;
}
