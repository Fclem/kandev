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
    const horizontalOverflow = () =>
      testPage.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );

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
    // The collapsed header (toggle + count, possibly + Setup Profile) must not
    // push the page horizontally on a phone width.
    expect(await horizontalOverflow(testPage)).toBeLessThanOrEqual(0);

    // The zero-profile case renders the long "No profiles yet" sentence in the
    // header — the worst-case width. Assert it too when the fixture has such an
    // agent.
    const zeroProfileAgent = refreshed.find((item) => item.profiles.length === 0);
    if (zeroProfileAgent) {
      const zeroGroup = testPage.getByTestId(`agent-group-${zeroProfileAgent.name}`);
      const zeroToggle = zeroGroup.getByTestId(`collapse-agent-${zeroProfileAgent.name}`);
      await expect(zeroToggle).toBeVisible();
      await zeroToggle.tap();
      await expect(zeroGroup.getByTestId(`collapsed-count-${zeroProfileAgent.name}`)).toContainText(
        "No profiles yet",
      );
      expect(await horizontalOverflow(testPage)).toBeLessThanOrEqual(0);
    }

    // The choice survives a reload on the phone too.
    await testPage.reload();
    await expect(group).toBeVisible({ timeout: 15_000 });
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);
  });
});
