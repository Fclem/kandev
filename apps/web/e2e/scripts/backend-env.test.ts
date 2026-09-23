import { describe, expect, it } from "vitest";
import { BackendFixtureEnvOverrides, sanitizeInheritedEnv } from "../fixtures/backend-env";

describe("sanitizeInheritedEnv", () => {
  it("drops the deployment facts a host shell could otherwise impose", () => {
    // Both of these decide whether the spawned backend may use the root-owned
    // deployment-secret supervisor. A host value must not reach it, because
    // profiles.yaml only fills variables that are still unset.
    const cleaned = sanitizeInheritedEnv({
      KANDEV_SERVICE_MODE: "user",
      KANDEV_DEPLOYMENT_SECRET_TARGET: "supported",
      KANDEV_DEPLOYMENT_SECRET_TARGET_SUFFIX: "kept",
    });

    expect(cleaned.KANDEV_SERVICE_MODE).toBeUndefined();
    expect(cleaned.KANDEV_DEPLOYMENT_SECRET_TARGET).toBeUndefined();
    expect(cleaned.KANDEV_DEPLOYMENT_SECRET_TARGET_SUFFIX).toBe("kept");
  });

  it("drops GitHub tokens, profile-managed flags and star OAuth settings", () => {
    const cleaned = sanitizeInheritedEnv({
      GH_TOKEN: "secret",
      GITHUB_TOKEN: "secret",
      KANDEV_FEATURES_OFFICE: "false",
      KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CLIENT_ID: "cid",
      KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CLIENT_SECRET_REF: "ref",
      KANDEV_WEB_TITLE_PREFIX: "Debug",
      KANDEV_LOG_LEVEL: "debug",
    });

    expect(cleaned.GH_TOKEN).toBeUndefined();
    expect(cleaned.GITHUB_TOKEN).toBeUndefined();
    expect(cleaned.KANDEV_FEATURES_OFFICE).toBeUndefined();
    expect(cleaned.KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CLIENT_ID).toBeUndefined();
    expect(cleaned.KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CLIENT_SECRET_REF).toBeUndefined();
    expect(cleaned.KANDEV_WEB_TITLE_PREFIX).toBeUndefined();
    expect(cleaned.KANDEV_LOG_LEVEL).toBe("debug");
  });

  it("drops PATH casing aliases and keeps unrelated variables", () => {
    const cleaned = sanitizeInheritedEnv({
      Path: "/usr/bin",
      HOME: "/home/dev",
      E2E_PORT_OFFSET: "2",
    });

    expect(cleaned.Path).toBeUndefined();
    expect(cleaned.HOME).toBe("/home/dev");
    expect(cleaned.E2E_PORT_OFFSET).toBe("2");
  });

  it("lets an explicit restart override win over the sanitized baseline", () => {
    // Specs that want the rejection set it deliberately; the sanitizer only
    // clears what the developer's shell happened to export.
    const scoped = new BackendFixtureEnvOverrides();
    scoped.add({ KANDEV_SERVICE_MODE: "user" });

    const applied = sanitizeInheritedEnv({ KANDEV_SERVICE_MODE: "user", HOME: "/home/dev" });
    const merged = scoped.apply(applied);

    expect(merged.KANDEV_SERVICE_MODE).toBe("user");
    expect(applied.KANDEV_SERVICE_MODE).toBeUndefined();
    expect(merged.HOME).toBe("/home/dev");
  });
});
