import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { NotAuthenticatedNotice } from "./github-page-client";

describe("GitHub NotAuthenticatedNotice", () => {
  afterEach(() => cleanup());

  it("links to the workspace-scoped GitHub settings when a workspace is provided", () => {
    render(<NotAuthenticatedNotice workspaceId="ws 1/2" personalRequired={false} />);

    expect(screen.getByRole("link").getAttribute("href")).toBe(
      "/settings/workspaces/ws%201%2F2/integrations",
    );
  });

  it("links to the global GitHub settings without a workspace", () => {
    render(<NotAuthenticatedNotice personalRequired={false} />);

    expect(screen.getByRole("link").getAttribute("href")).toBe("/settings/integrations/github");
  });
});
