import path from "node:path";
import react from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";
import { lingui, linguiTransformerBabelPreset } from "@lingui/vite-plugin";
import { defineConfig } from "vite";

export default defineConfig({
  // Vite 8 (Rolldown) + @vitejs/plugin-react v6 transform via oxc, not Babel, so
  // Lingui macros are transformed by @rolldown/plugin-babel with the Lingui
  // preset (its filter targets only files using Lingui macros). `lingui()`
  // compiles `.po` catalog imports on the fly.
  plugins: [react(), lingui(), babel({ presets: [linguiTransformerBabelPreset()] })],
  server: {
    port: readPort(process.env.PORT),
    strictPort: Boolean(process.env.PORT),
  },
  preview: {
    port: readPort(process.env.PORT),
    strictPort: Boolean(process.env.PORT),
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "."),
      "@kandev/ui": path.resolve(__dirname, "../packages/ui/src"),
      "@kandev/theme": path.resolve(__dirname, "../packages/theme/src"),
      "@kandev/types": path.resolve(__dirname, "../packages/types/src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});

function readPort(value: string | undefined): number | undefined {
  if (!value) return undefined;
  const port = Number(value);
  return Number.isInteger(port) && port > 0 ? port : undefined;
}
