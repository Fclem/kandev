import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { NotConfigured } from "./azure-devops-page-client";

describe("Azure DevOps NotConfigured", () => {
  afterEach(() => cleanup());

  it("links to the workspace-scoped Azure DevOps settings when a workspace is provided", () => {
    render(<NotConfigured workspaceId="ws 1/2" />);

    expect(screen.getByRole("link").getAttribute("href")).toBe(
      "/settings/workspaces/ws%201%2F2/integrations",
    );
  });

  it("links to the global Azure DevOps settings without a workspace", () => {
    render(<NotConfigured />);

    expect(screen.getByRole("link").getAttribute("href")).toBe(
      "/settings/integrations/azure-devops",
    );
  });
});
