import { describe, expect, it } from "vitest";
import type { GitHubStarConnectionStatus } from "@/lib/types/github";
import {
  PLUGINS_SETTINGS_PATH,
  buildPluginStarReturnPath,
  pluginStarKey,
  pluginStarWorkspacePrefix,
  resolvePluginStarControlMode,
} from "./plugin-star-connection";

const CANONICAL = "owner/repo";

function status(overrides: Partial<GitHubStarConnectionStatus> = {}): GitHubStarConnectionStatus {
  return {
    user_id: "user-1",
    credential_generation: 3,
    active: true,
    capable: true,
    reconnect_required: false,
    available: true,
    ...overrides,
  };
}

describe("resolvePluginStarControlMode", () => {
  it("omits the control for an unsupported deployment", () => {
    expect(resolvePluginStarControlMode(null)).toBe("omit");
    expect(resolvePluginStarControlMode(undefined)).toBe("omit");
    expect(
      resolvePluginStarControlMode(status({ available: false, active: false, capable: false })),
    ).toBe("omit");
    // available=false wins even when the row still describes a usable grant.
    expect(resolvePluginStarControlMode(status({ available: false }))).toBe("omit");
  });

  it("offers Connect GitHub without a usable grant", () => {
    expect(
      resolvePluginStarControlMode(
        status({ active: false, capable: false, reconnect_required: false }),
      ),
    ).toBe("connect");
  });

  it("hydrates actionable star controls for a valid generation", () => {
    expect(resolvePluginStarControlMode(status())).toBe("stars");
    expect(resolvePluginStarControlMode(status({ reconnect_required: true }))).toBe("stars");
  });

  it("offers Reconnect GitHub when the grant is unusable but replaceable", () => {
    expect(
      resolvePluginStarControlMode(
        status({ active: true, capable: false, reconnect_required: true }),
      ),
    ).toBe("reconnect");
    expect(
      resolvePluginStarControlMode(
        status({ active: false, capable: false, reconnect_required: true }),
      ),
    ).toBe("reconnect");
  });

  it("falls back to the non-assertive Connect CTA for unlisted tuples", () => {
    expect(
      resolvePluginStarControlMode(
        status({ active: true, capable: false, reconnect_required: false }),
      ),
    ).toBe("connect");
    expect(resolvePluginStarControlMode(status({ active: false, capable: true }))).toBe("connect");
    expect(
      resolvePluginStarControlMode(
        status({ active: false, capable: true, reconnect_required: true }),
      ),
    ).toBe("connect");
  });

  it("hydrates stars for a capable workspace or App-personal actor", () => {
    const actor = { source: "gh_cli", login: "octocat", shared: true };
    expect(resolvePluginStarControlMode(status({ actor }))).toBe("stars");
  });

  it("omits the control and CTA for an incapable non-dedicated actor", () => {
    for (const source of ["gh_cli", "pat", "legacy_shared", "github_app_user"]) {
      expect(
        resolvePluginStarControlMode(
          status({
            capable: false,
            reconnect_required: true,
            actor: { source, login: "octocat", shared: true },
            remediation: "token_scope",
          }),
        ),
      ).toBe("omit");
    }
  });

  it("offers Connect GitHub for the App personal-connection path", () => {
    expect(
      resolvePluginStarControlMode(
        status({ active: false, capable: false, actor: null, connect: "github_app_user" }),
      ),
    ).toBe("connect");
  });
});

describe("pluginStarKey", () => {
  it("scopes the key to workspace, user, generation, capability, and repository", () => {
    const key = pluginStarKey("ws-1", status(), CANONICAL);
    expect(pluginStarKey("ws-2", status(), CANONICAL)).not.toBe(key);
    expect(pluginStarKey("ws-1", status({ user_id: "user-2" }), CANONICAL)).not.toBe(key);
    expect(pluginStarKey("ws-1", status({ credential_generation: 4 }), CANONICAL)).not.toBe(key);
    expect(pluginStarKey("ws-1", status({ active: false }), CANONICAL)).not.toBe(key);
    expect(pluginStarKey("ws-1", status({ capable: false }), CANONICAL)).not.toBe(key);
    expect(pluginStarKey("ws-1", status({ reconnect_required: true }), CANONICAL)).not.toBe(key);
    expect(pluginStarKey("ws-1", status(), "owner/other")).not.toBe(key);
    const gh = status({ actor: { source: "gh_cli", login: "octocat", shared: true } });
    expect(pluginStarKey("ws-1", gh, CANONICAL)).not.toBe(key);
    expect(
      pluginStarKey(
        "ws-1",
        status({ actor: { source: "pat", login: "octocat", shared: true } }),
        CANONICAL,
      ),
    ).not.toBe(pluginStarKey("ws-1", gh, CANONICAL));
    expect(
      pluginStarKey(
        "ws-1",
        status({ actor: { source: "gh_cli", login: "hubot", shared: true } }),
        CANONICAL,
      ),
    ).not.toBe(pluginStarKey("ws-1", gh, CANONICAL));
  });

  it("keeps distinct repositories from colliding inside one scope", () => {
    const a = pluginStarKey("ws-1", status(), "owner/a");
    const b = pluginStarKey("ws-1", status(), "owner/b");
    expect(a).not.toBe(b);
  });

  it("selects one workspace by prefix without matching a longer workspace id", () => {
    const prefix = pluginStarWorkspacePrefix("ws-1");
    expect(pluginStarKey("ws-1", status(), CANONICAL).startsWith(prefix)).toBe(true);
    expect(pluginStarKey("ws-10", status(), CANONICAL).startsWith(prefix)).toBe(false);
  });
});

describe("buildPluginStarReturnPath", () => {
  it("keeps the plugins surface the user started from", () => {
    expect(buildPluginStarReturnPath(PLUGINS_SETTINGS_PATH, "")).toBe(PLUGINS_SETTINGS_PATH);
    expect(buildPluginStarReturnPath(PLUGINS_SETTINGS_PATH, "?tab=canvases")).toBe(
      `${PLUGINS_SETTINGS_PATH}?tab=canvases`,
    );
    expect(buildPluginStarReturnPath(`${PLUGINS_SETTINGS_PATH}/acme`, "")).toBe(
      `${PLUGINS_SETTINGS_PATH}/acme`,
    );
  });

  it("collapses anything outside Settings > Plugins to the plugins page", () => {
    expect(buildPluginStarReturnPath("/tasks/t-1", "")).toBe(PLUGINS_SETTINGS_PATH);
    expect(buildPluginStarReturnPath(`${PLUGINS_SETTINGS_PATH}-archive`, "")).toBe(
      PLUGINS_SETTINGS_PATH,
    );
    expect(buildPluginStarReturnPath(`${PLUGINS_SETTINGS_PATH}/../admin`, "")).toBe(
      PLUGINS_SETTINGS_PATH,
    );
    expect(buildPluginStarReturnPath("https://evil.test/", "")).toBe(PLUGINS_SETTINGS_PATH);
  });

  it("drops a replayed outcome parameter and keeps the rest of the query", () => {
    expect(
      buildPluginStarReturnPath(PLUGINS_SETTINGS_PATH, "?star_connection_outcome=x&tab=browse"),
    ).toBe(`${PLUGINS_SETTINGS_PATH}?tab=browse`);
  });
});
