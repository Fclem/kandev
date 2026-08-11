import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { NotConfiguredNotice } from "./jira-page-client";

describe("Jira NotConfiguredNotice", () => {
  afterEach(() => cleanup());

  it("links to the workspace-scoped Jira settings when a workspace is provided", () => {
    render(<NotConfiguredNotice workspaceId="ws 1/2" />);

    expect(screen.getByRole("link").getAttribute("href")).toBe(
      "/settings/workspaces/ws%201%2F2/integrations",
    );
  });

  it("links to the global Jira settings without a workspace", () => {
    render(<NotConfiguredNotice />);

    expect(screen.getByRole("link").getAttribute("href")).toBe("/settings/integrations/jira");
  });
});
