/// <reference types="vite/client" />

// Lingui `.po` catalogs are compiled to message modules by @lingui/vite-plugin
// at build time; this declares the shape of a `.po` import for TypeScript.
declare module "*.po" {
  import type { Messages } from "@lingui/core";
  export const messages: Messages;
}
