import { useLingui } from "@lingui/react/macro";
import type { ReactNode } from "react";

import { UIStringsProvider, type UIStrings } from "@kandev/ui/lib/ui-strings";

/**
 * Feeds translated values into the @kandev/ui `UIStringsProvider` so the shared
 * primitives' built-in accessibility labels ("Close", "Toggle Sidebar", …) are
 * localized. `useLingui` (macro) makes this reactive: switching locale
 * re-renders and updates the strings. The package itself has no i18n dependency
 * — this bridge lives in the app.
 */
export function TranslatedUIStrings({ children }: { children: ReactNode }) {
  const { t } = useLingui();
  const strings: UIStrings = {
    close: t`Close`,
    loading: t`Loading`,
    previousSlide: t`Previous slide`,
    nextSlide: t`Next slide`,
    more: t`More`,
    morePages: t`More pages`,
    sidebar: t`Sidebar`,
    sidebarDescription: t`Displays the mobile sidebar.`,
    toggleSidebar: t`Toggle Sidebar`,
    previous: t`Previous`,
    next: t`Next`,
    goToPreviousPage: t`Go to previous page`,
    goToNextPage: t`Go to next page`,
  };
  return <UIStringsProvider value={strings}>{children}</UIStringsProvider>;
}
