import { test, expect } from "../../fixtures/test-base";

const COLLAPSE_STORAGE_KEY = "kandev:agents:collapsedBlocks:v1";

test.describe("Agent block collapse", () => {
  test("collapses an agent block, shows the count, and persists across reloads", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (agent.profiles.length === 0) {
      await apiClient.createAgentProfile(agent.id, "Collapse Count", { model: "mock-fast" });
    }
    const { agents: refreshed } = await apiClient.listAgents();
    const target = refreshed.find((item) => item.name === agent.name) ?? agent;
    const count = target.profiles.length;
    expect(count).toBeGreaterThan(0);
    const countLabel = count === 1 ? "1 profile" : `${count} profiles`;

    await testPage.goto("/settings/agents");
    const group = testPage.getByTestId(`agent-group-${agent.name}`);
    await expect(group).toBeVisible({ timeout: 15_000 });
    const body = group.getByTestId(`agent-profiles-${agent.name}`);
    await expect(body).toBeVisible();
    // Default is expanded: no collapse preference exists before the first toggle.
    expect(
      await testPage.evaluate((key) => localStorage.getItem(key), COLLAPSE_STORAGE_KEY),
    ).toBeNull();

    const toggle = group.getByTestId(`collapse-agent-${agent.name}`);
    await toggle.click();
    await expect(body).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);

    // The choice survives a reload.
    await testPage.reload();
    await expect(group).toBeVisible({ timeout: 15_000 });
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);

    // Expanding restores the body and moves the count back out of the header.
    await group.getByTestId(`collapse-agent-${agent.name}`).click();
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeVisible();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveCount(0);
  });
});
