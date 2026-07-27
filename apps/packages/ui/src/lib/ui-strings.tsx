import { createContext, useContext } from "react";

/**
 * Built-in user-facing strings for @kandev/ui primitives (mostly accessibility
 * labels). The package ships English defaults and has NO dependency on any i18n
 * runtime, so primitives render correctly standalone. A consuming app can wrap
 * its tree in `UIStringsProvider` with translated values to localize them.
 */
export type UIStrings = {
  close: string;
  loading: string;
  previousSlide: string;
  nextSlide: string;
  more: string;
  morePages: string;
  sidebar: string;
  sidebarDescription: string;
  toggleSidebar: string;
  previous: string;
  next: string;
  goToPreviousPage: string;
  goToNextPage: string;
};

export const DEFAULT_UI_STRINGS: UIStrings = {
  close: "Close",
  loading: "Loading",
  previousSlide: "Previous slide",
  nextSlide: "Next slide",
  more: "More",
  morePages: "More pages",
  sidebar: "Sidebar",
  sidebarDescription: "Displays the mobile sidebar.",
  toggleSidebar: "Toggle Sidebar",
  previous: "Previous",
  next: "Next",
  goToPreviousPage: "Go to previous page",
  goToNextPage: "Go to next page",
};

const UIStringsContext = createContext<UIStrings>(DEFAULT_UI_STRINGS);

export const UIStringsProvider = UIStringsContext.Provider;

/** Read the active UI strings; falls back to English defaults with no provider. */
export function useUIStrings(): UIStrings {
  return useContext(UIStringsContext);
}
