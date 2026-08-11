import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { DisabledNotice, NotConfiguredNotice } from "./linear-page-client";

describe("Linear NotConfiguredNotice", () => {
  afterEach(() => cleanup());

  it("links to the workspace-scoped Linear settings when a workspace is provided", () => {
    render(<NotConfiguredNotice workspaceId="ws 1/2" />);

    expect(screen.getByRole("link").getAttribute("href")).toBe(
      "/settings/workspaces/ws%201%2F2/integrations",
    );
  });

  it("links to the global Linear settings without a workspace", () => {
    render(<NotConfiguredNotice />);

    expect(screen.getByRole("link").getAttribute("href")).toBe("/settings/integrations/linear");
  });
});

describe("Linear DisabledNotice", () => {
  afterEach(() => cleanup());

  it("links to the workspace-scoped Linear settings when a workspace is provided", () => {
    render(<DisabledNotice workspaceId="ws 1/2" />);

    expect(screen.getByRole("link").getAttribute("href")).toBe(
      "/settings/workspaces/ws%201%2F2/integrations",
    );
  });

  it("links to the global Linear settings without a workspace", () => {
    render(<DisabledNotice />);

    expect(screen.getByRole("link").getAttribute("href")).toBe("/settings/integrations/linear");
  });
});
