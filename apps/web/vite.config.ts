import path from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    port: readPort(process.env.PORT),
    strictPort: Boolean(process.env.PORT),
    // Vite's DNS-rebinding guard rejects non-loopback Host headers by
    // default. Dev instances served from another machine (LAN hostname,
    // tailnet name) must opt those hosts in via VITE_ALLOWED_HOSTS
    // (comma-separated); production serves the built SPA from Go and is
    // unaffected.
    allowedHosts: readAllowedHosts(process.env.VITE_ALLOWED_HOSTS),
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

function readAllowedHosts(value: string | undefined): string[] | undefined {
  if (!value) return undefined;
  const hosts = value
    .split(",")
    .map((host) => host.trim())
    .filter(Boolean);
  return hosts.length > 0 ? hosts : undefined;
}
