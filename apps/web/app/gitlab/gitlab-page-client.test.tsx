import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { NotConnectedNotice } from "./gitlab-page-client";

describe("GitLab NotConnectedNotice", () => {
  afterEach(() => cleanup());

  it("links to the workspace-scoped GitLab settings when a workspace is provided", () => {
    render(<NotConnectedNotice workspaceId="ws 1/2" />);

    expect(screen.getByRole("link").getAttribute("href")).toBe(
      "/settings/workspaces/ws%201%2F2/integrations",
    );
  });

  it("links to the global GitLab settings without a workspace", () => {
    render(<NotConnectedNotice />);

    expect(screen.getByRole("link").getAttribute("href")).toBe("/settings/integrations/gitlab");
  });
});
