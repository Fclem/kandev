import { createElement, type ComponentProps } from "react";
import { i18n } from "@lingui/core";
import { vi } from "vitest";
import type { Window as HappyDOMWindow } from "happy-dom";

/**
 * i18n test bootstrap.
 *
 * Lingui's React macros (`<Trans>`, `useLingui`) read a module-private context
 * and THROW when no `I18nProvider` is above them; the core `t` macro throws when
 * no locale is active. The app mounts the provider in `src/app-shell.tsx`, which
 * unit tests never render — so without this bootstrap every test touching a
 * localized component or helper fails.
 *
 * 1. Activate a locale so the core `t` macro works in plain (non-React) helpers.
 *    The empty catalog is deliberate: Lingui then falls back to each message's
 *    English source text, so assertions on English copy keep passing.
 * 2. Shim `@lingui/react` so the macros work WITHOUT a provider.
 *
 * Why shim the library rather than wrap RTL's `render` in `I18nProvider`:
 * wrapping changes the shape of every test's render tree, which breaks tests
 * that assert on tree-sensitive behavior (e.g. `use-issue-watches` renders
 * `<StrictMode>` at the root and asserts on its double-mount). It also would not
 * help the suites that render via `react-dom/server`'s `renderToStaticMarkup`,
 * which never goes through RTL. Shimming keeps every render tree byte-identical
 * and covers both paths.
 */
i18n.loadAndActivate({ locale: "en", messages: {} });

vi.mock("@lingui/react", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@lingui/react")>();

  // Lingui's context object shape. Falling back to the global `i18n` gives
  // provider-less components the same behavior they get inside the app.
  const fallbackContext = {
    i18n,
    _: i18n.t.bind(i18n),
    defaultComponent: undefined,
  };

  // `useContext` has already run by the time the real hook throws, so hook
  // order stays consistent and catching here is safe.
  const useLingui: typeof actual.useLingui = () => {
    try {
      return actual.useLingui();
    } catch {
      return fallbackContext as ReturnType<typeof actual.useLingui>;
    }
  };

  // `Trans` reads the context via a module-private helper, so the export shim
  // above can't reach it — give each instance its own provider instead. This is
  // leaf-local and does not alter the surrounding tree.
  const Trans = (props: ComponentProps<typeof actual.Trans>) =>
    createElement(actual.I18nProvider, { i18n }, createElement(actual.Trans, props));

  return { ...actual, useLingui, Trans };
});

function createLocalStorageMock(): Storage {
  const store = new Map<string, string>();

  return {
    get length() {
      return store.size;
    },
    clear() {
      store.clear();
    },
    getItem(key: string) {
      return store.has(key) ? (store.get(key) ?? null) : null;
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(key, value);
    },
  };
}

const localStorageMock = createLocalStorageMock();

Object.defineProperty(globalThis, "localStorage", {
  configurable: true,
  value: localStorageMock,
});

if (typeof window !== "undefined") {
  const happyDOMWindow = window as unknown as HappyDOMWindow;
  happyDOMWindow.happyDOM.settings.fetch.interceptor = {
    beforeAsyncRequest: ({ window: requestWindow }) =>
      Promise.resolve(new requestWindow.Response(null, { status: 404 })),
  };

  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: localStorageMock,
  });
}
