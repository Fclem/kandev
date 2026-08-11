import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { JiraErrorMessage } from "./jira-ticket-common";

describe("JiraErrorMessage reconnect link", () => {
  afterEach(() => cleanup());

  it("links to the workspace-scoped Jira settings when a workspace is provided", () => {
    render(<JiraErrorMessage error="status 401" workspaceId="ws 1/2" />);

    expect(screen.getByRole("link", { name: /Reconnect/ }).getAttribute("href")).toBe(
      "/settings/workspaces/ws%201%2F2/integrations",
    );
  });

  it("links to the global Jira settings without a workspace", () => {
    render(<JiraErrorMessage error="status 401" />);

    expect(screen.getByRole("link", { name: /Reconnect/ }).getAttribute("href")).toBe(
      "/settings/integrations/jira",
    );
  });
});
