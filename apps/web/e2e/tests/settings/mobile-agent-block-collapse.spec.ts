import { test, expect } from "../../fixtures/test-base";

test.describe("Agent block collapse on mobile", () => {
  test("collapses an agent block from a touch-sized control and keeps it collapsed after reload", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (agent.profiles.length === 0) {
      await apiClient.createAgentProfile(agent.id, "Mobile Collapse Count", {
        model: "mock-fast",
      });
    }
    const { agents: refreshed } = await apiClient.listAgents();
    const target = refreshed.find((item) => item.name === agent.name) ?? agent;
    const count = target.profiles.length;
    expect(count).toBeGreaterThan(0);
    const countLabel = count === 1 ? "1 profile" : `${count} profiles`;

    await testPage.goto("/settings/agents");
    const group = testPage.getByTestId(`agent-group-${agent.name}`);
    await expect(group).toBeVisible({ timeout: 15_000 });

    const toggle = group.getByTestId(`collapse-agent-${agent.name}`);
    // Wait for the card chrome to settle before touching the toggle — the
    // discovery/availability resolution re-renders the header and can detach
    // a locator resolved a moment earlier.
    await expect(toggle).toBeVisible();
    await toggle.scrollIntoViewIfNeeded();
    const box = await toggle.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);

    await toggle.tap();
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);

    // The choice survives a reload on the phone too.
    await testPage.reload();
    await expect(group).toBeVisible({ timeout: 15_000 });
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);
  });
});
