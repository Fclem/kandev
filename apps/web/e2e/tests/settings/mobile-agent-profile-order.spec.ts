import { test, expect } from "../../fixtures/test-base";

test.describe("Agent profile ordering on mobile", () => {
  test("keeps drag handles touch-sized and sorts profile groups without overflow", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("The E2E fixture must provide an installed agent");
    const suffix = Date.now();
    const created: string[] = [];
    try {
      for (const name of [`Zulu ${suffix}`, `Alpha ${suffix}`]) {
        const profile = await apiClient.createAgentProfile(agent.id, name, { model: "mock-fast" });
        created.push(profile.id);
      }
      await testPage.goto("/settings/agents");
      const handle = testPage.getByTestId("agent-profile-drag-handle").first();
      await expect(handle).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(async () => {
          const box = await handle.boundingBox();
          return box ? Math.min(box.width, box.height) : null;
        })
        .toBeGreaterThanOrEqual(44);
      await testPage.getByTestId("sort-profiles-by-name-button").tap();
      await expect
        .poll(
          async () => {
            const { agents: refreshed } = await apiClient.listAgents();
            const profiles = refreshed.find((item) => item.id === agent.id)?.profiles ?? [];
            return (
              profiles.findIndex((profile) => profile.name === `Alpha ${suffix}`) <
              profiles.findIndex((profile) => profile.name === `Zulu ${suffix}`)
            );
          },
          { timeout: 15_000 },
        )
        .toBe(true);
      await expect
        .poll(() =>
          testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        )
        .toBe(true);
    } finally {
      for (const id of created) await apiClient.deleteAgentProfile(id).catch(() => undefined);
    }
  });
});
