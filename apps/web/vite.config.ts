import path from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    port: readPort(process.env.PORT),
    strictPort: Boolean(process.env.PORT),
    // Remote dev hosts (cloud VMs, LAN hostnames) are otherwise blocked by
    // Vite's DNS-rebinding host check. Opt in per environment with an
    // explicit comma-separated host list, e.g.
    // KANDEV_VITE_ALLOWED_HOSTS="a.example.com,b.example.com".
    allowedHosts: readAllowedHosts(process.env.KANDEV_VITE_ALLOWED_HOSTS),
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

/**
 * Reads the comma-separated KANDEV_VITE_ALLOWED_HOSTS override. Returns
 * undefined (Vite's default host policy) when unset, or the explicit host
 * list. Wildcards are deliberately unsupported: allowing every host would
 * expose the dev server to DNS-rebinding attacks, so only named hosts can
 * be opted in.
 */
function readAllowedHosts(value: string | undefined): string[] | undefined {
  if (!value) return undefined;
  return value
    .split(",")
    .map((host) => host.trim())
    .filter(Boolean);
}
