import { describe, it, expect, vi, beforeEach } from "vitest";

import { buildImproveKandevDescription } from "./improve-kandev-dialog-helpers";
import type { DiagnosticBundleJob } from "@/lib/types/system";
import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";

const bundleJob = (id: string, status: DiagnosticBundleJob["status"]): DiagnosticBundleJob =>
  ({
    id,
    status,
    sources: ["backend", "frontend", "runtime"],
    build_deadline: "",
    expires_at: null,
    browser_profiles: 0,
    frontend_entry_count: 0,
    frontend_bytes: 0,
    warnings: [],
  }) as DiagnosticBundleJob;

const leaseDiagnosticBundle = vi.fn();
const createDiagnosticBundle = vi.fn();
const fetchDiagnosticBundle = vi.fn();
vi.mock("@/lib/api/domains/improve-kandev-api", async (orig) => {
  const actual = await orig<typeof import("@/lib/api/domains/improve-kandev-api")>();
  return {
    ...actual,
    leaseDiagnosticBundle: (...args: unknown[]) => leaseDiagnosticBundle(...args),
  };
});

vi.mock("@/lib/api/domains/system-api", () => ({
  createDiagnosticBundle: (...args: unknown[]) => createDiagnosticBundle(...args),
  fetchDiagnosticBundle: (...args: unknown[]) => fetchDiagnosticBundle(...args),
}));

const bootstrap: ImproveKandevBootstrapResponse = {
  workspace_id: "ws-1",
  repository_id: "r1",
  workflow_id: "w1",
  issue_workflow_id: "w2",
  branch: "main",
  bundle_dir: "/tmp/kandev-improve-abc",
  bundle_file: "/tmp/kandev-improve-abc/diagnostic-bundle.zip",
  github_login: "octocat",
  has_write_access: false,
  fork_status: "unknown",
};

describe("buildImproveKandevDescription", () => {
  beforeEach(() => {
    leaseDiagnosticBundle.mockReset();
    createDiagnosticBundle.mockReset();
    fetchDiagnosticBundle.mockReset();
    createDiagnosticBundle.mockResolvedValue(bundleJob("bundle-1", "ready"));
    leaseDiagnosticBundle.mockResolvedValue({
      path: "/leased/task-context/diagnostic-bundle.zip",
      status: "ready",
      sources: ["backend", "frontend", "runtime"],
    });
  });

  it("returns description unchanged when bootstrap is null", async () => {
    const out = await buildImproveKandevDescription("desc", null, true);
    expect(out).toBe("desc");
    expect(createDiagnosticBundle).not.toHaveBeenCalled();
  });

  it("returns description unchanged when captureLogs is false", async () => {
    const out = await buildImproveKandevDescription("desc", bootstrap, false);
    expect(out).toBe("desc");
    expect(createDiagnosticBundle).not.toHaveBeenCalled();
  });

  it("leases the ready standard-source bundle path into the description", async () => {
    const out = await buildImproveKandevDescription("Original prompt", bootstrap, true);
    expect(out).toContain("Original prompt");
    expect(out).toContain("/leased/task-context/diagnostic-bundle.zip");
    expect(out).not.toContain(bootstrap.bundle_file);
    expect(createDiagnosticBundle).toHaveBeenCalledWith(["backend", "frontend", "runtime"]);
    expect(leaseDiagnosticBundle).toHaveBeenCalledWith(bootstrap.bundle_dir, "bundle-1");
    expect(fetchDiagnosticBundle).not.toHaveBeenCalled();
  });

  it("polls a building job to partial before leasing its archive", async () => {
    createDiagnosticBundle.mockResolvedValueOnce(bundleJob("bundle-2", "building"));
    fetchDiagnosticBundle.mockResolvedValueOnce(bundleJob("bundle-2", "partial"));

    const out = await buildImproveKandevDescription("desc", bootstrap, true);

    expect(fetchDiagnosticBundle).toHaveBeenCalledWith("bundle-2");
    expect(leaseDiagnosticBundle).toHaveBeenCalledWith(bootstrap.bundle_dir, "bundle-2");
    expect(out).toContain("/leased/task-context/diagnostic-bundle.zip");
  });

  it.each(["failed", "expired"] as const)("does not attach a %s bundle", async (status) => {
    createDiagnosticBundle.mockResolvedValueOnce(bundleJob("bundle-3", status));

    const out = await buildImproveKandevDescription("desc", bootstrap, true);

    expect(out).toBe("desc");
    expect(leaseDiagnosticBundle).not.toHaveBeenCalled();
  });

  it("does not attach a path when leasing is rejected", async () => {
    leaseDiagnosticBundle.mockRejectedValueOnce(new Error("lease rejected"));

    const out = await buildImproveKandevDescription("desc", bootstrap, true);

    expect(out).toBe("desc");
    expect(out).not.toContain(bootstrap.bundle_file);
  });
});
