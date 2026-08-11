import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { SentryErrorMessage } from "./sentry-issue-common";

describe("SentryErrorMessage reconnect link", () => {
  afterEach(() => cleanup());

  it("links to the workspace-scoped Sentry settings when a workspace is provided", () => {
    render(<SentryErrorMessage error="status 401" workspaceId="ws 1/2" />);

    expect(screen.getByRole("link", { name: /Reconnect/ }).getAttribute("href")).toBe(
      "/settings/workspaces/ws%201%2F2/integrations",
    );
  });

  it("links to the global Sentry settings without a workspace", () => {
    render(<SentryErrorMessage error="status 401" />);

    expect(screen.getByRole("link", { name: /Reconnect/ }).getAttribute("href")).toBe(
      "/settings/integrations/sentry",
    );
  });
});
