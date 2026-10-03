import { test, expect } from "../../fixtures/test-base";
/** Fragment of dnd-kit's default onDragOver accessibility announcement. */
const MOVED_OVER = "was moved over droppable area";

test.describe("Agent profile ordering", () => {
  test("sorts profiles by name and keeps the order after reload", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);
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
      const rows = testPage
        .getByTestId(`agent-profiles-${agent.name}`)
        .getByTestId("agent-profile-row");
      await expect(rows).toHaveCount(agent.profiles.length + 2, { timeout: 15_000 });
      await expect(testPage.getByTestId("sort-profiles-by-name-button")).toBeVisible();
      await testPage.getByTestId("sort-profiles-by-name-button").click();
      await expect
        .poll(
          async () => {
            const { agents: refreshed } = await apiClient.listAgents();
            const names =
              refreshed.find((item) => item.id === agent.id)?.profiles.map((p) => p.name) ?? [];
            const alpha = names.indexOf(`Alpha ${suffix}`);
            const zulu = names.indexOf(`Zulu ${suffix}`);
            return alpha >= 0 && zulu > alpha;
          },
          { timeout: 15_000 },
        )
        .toBe(true);
      await testPage.reload();
      await expect
        .poll(
          async () => {
            const names = await rows.allTextContents();
            return (
              names.findIndex((name) => name.includes(`Alpha ${suffix}`)) <
              names.findIndex((name) => name.includes(`Zulu ${suffix}`))
            );
          },
          { timeout: 15_000 },
        )
        .toBe(true);
    } finally {
      for (const id of created) await apiClient.deleteAgentProfile(id).catch(() => undefined);
    }
  });

  test("reorders by keyboard through the dedicated handle", async ({ testPage, apiClient }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("The E2E fixture must provide an installed agent");
    const suffix = Date.now();
    const created: string[] = [];
    try {
      for (const name of [`Keyboard A ${suffix}`, `Keyboard B ${suffix}`]) {
        const profile = await apiClient.createAgentProfile(agent.id, name, { model: "mock-fast" });
        created.push(profile.id);
      }
      const { agents: before } = await apiClient.listAgents();
      const initial = before.find((item) => item.id === agent.id)?.profiles ?? [];
      const activeId = initial[0]?.id;
      const overId = initial[1]?.id;
      if (!activeId || !overId) throw new Error("The keyboard reorder fixture needs two profiles");
      await testPage.goto("/settings/agents");
      const handle = testPage.getByTestId("agent-profile-drag-handle").first();
      const row = handle.locator("xpath=../../../..");
      await expect(handle).toBeVisible({ timeout: 15_000 });
      await handle.focus();
      await expect(handle).toBeFocused();
      await handle.press("Space");
      await expect(row).toHaveClass(/opacity-70/, { timeout: 5_000 });
      const announcedTarget = async () =>
        (await testPage.locator('[id^="DndLiveRegion"]').allTextContents()).find((text) =>
          text.includes(MOVED_OVER),
        ) ?? "";
      await expect.poll(announcedTarget, { timeout: 5_000 }).toContain(MOVED_OVER);
      const targetBeforeArrow = await announcedTarget();
      await handle.press("ArrowDown");
      await expect.poll(announcedTarget, { timeout: 5_000 }).not.toBe(targetBeforeArrow);
      await handle.press("Space");
      await expect(row).not.toHaveClass(/opacity-70/, { timeout: 5_000 });
      await expect
        .poll(
          async () => {
            const { agents: refreshed } = await apiClient.listAgents();
            const profiles = refreshed.find((item) => item.id === agent.id)?.profiles ?? [];
            const active = profiles.findIndex((profile) => profile.id === activeId);
            const over = profiles.findIndex((profile) => profile.id === overId);
            return active >= 0 && over >= 0 && over < active;
          },
          { timeout: 15_000 },
        )
        .toBe(true);
    } finally {
      for (const id of created) await apiClient.deleteAgentProfile(id).catch(() => undefined);
    }
  });
});
