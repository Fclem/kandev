import { type CDPSession, type Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { settledBoundingBox } from "../../helpers/settled-box";

async function getScrollTop(anchor: Locator) {
  return anchor.evaluate((element) => {
    let current = element.parentElement;
    while (current && current !== document.documentElement) {
      const style = getComputedStyle(current);
      if (
        current.scrollHeight > current.clientHeight &&
        (style.overflowY === "auto" || style.overflowY === "scroll")
      ) {
        return current.scrollTop;
      }
      current = current.parentElement;
    }
    return document.scrollingElement?.scrollTop ?? 0;
  });
}

async function touchSwipeOutsideHandle(cdp: CDPSession, row: Locator) {
  const box = await settledBoundingBox(row);
  const startX = box.x + Math.min(20, box.width / 3);
  const startY = box.y + box.height / 2;
  const before = await getScrollTop(row);
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: startX, y: startY }],
  });
  for (let i = 1; i <= 8; i++) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ x: startX, y: startY - (120 * i) / 8 }],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  return before;
}

async function touchDragProfile(
  cdp: CDPSession,
  sourceHandle: Locator,
  sourceRow: Locator,
  targetRow: Locator,
) {
  await targetRow.scrollIntoViewIfNeeded();
  await sourceHandle.scrollIntoViewIfNeeded();
  const from = await settledBoundingBox(sourceHandle);
  const to = await settledBoundingBox(targetRow);
  const startX = from.x + from.width / 2;
  const startY = from.y + from.height / 2;
  const endX = to.x + to.width / 2;
  const endY = to.y + to.height / 2;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: startX, y: startY }],
  });
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchMove",
    touchPoints: [{ x: startX + 14, y: startY }],
  });
  await expect(sourceRow.locator("xpath=..")).toHaveClass(/opacity-70/);
  for (let i = 1; i <= 10; i++) {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [
        {
          x: startX + ((endX - startX) * i) / 10,
          y: startY + ((endY - startY) * i) / 10,
        },
      ],
    });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
}

async function visibleProfileNames(group: Locator) {
  return group
    .getByTestId("agent-profile-row")
    .evaluateAll((rows) =>
      rows.map(
        (row) =>
          row
            .querySelector<HTMLElement>('[data-testid="agent-profile-row-link"]')
            ?.getAttribute("aria-label") ?? "",
      ),
    );
}

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

  test("touch-drags a profile, persists its order, and scrolls outside the handle", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent) throw new Error("The E2E fixture must provide an installed agent");
    const suffix = Date.now();
    const created: string[] = [];
    let cdp: CDPSession | undefined;
    try {
      for (let index = 0; index < 8; index++) {
        const profile = await apiClient.createAgentProfile(agent.id, `Touch ${index} ${suffix}`, {
          model: "mock-fast",
        });
        created.push(profile.id);
      }
      const { agents: refreshed } = await apiClient.listAgents();
      const currentAgent = refreshed.find((item) => item.id === agent.id);
      if (!currentAgent) throw new Error("The E2E agent disappeared after profile creation");
      const createdIds = new Set(created);
      const createdProfiles = currentAgent.profiles.filter((profile) => createdIds.has(profile.id));
      expect(createdProfiles).toHaveLength(created.length);
      const sourceProfile = createdProfiles[0];
      const targetProfile = createdProfiles[1];
      if (!sourceProfile || !targetProfile) {
        throw new Error("The E2E profiles must include a source and target row");
      }
      const expectedProfiles = [...currentAgent.profiles];
      const sourceIndex = expectedProfiles.findIndex((profile) => profile.id === sourceProfile.id);
      const targetIndex = expectedProfiles.findIndex((profile) => profile.id === targetProfile.id);
      const source = expectedProfiles[sourceIndex];
      const target = expectedProfiles[targetIndex];
      if (!source || !target) throw new Error("The E2E profile order omitted a created profile");
      expectedProfiles[sourceIndex] = target;
      expectedProfiles[targetIndex] = source;

      await testPage.goto("/settings/agents");
      const group = testPage.getByTestId(`agent-profiles-${agent.name}`);
      const profileName = new Map(
        currentAgent.profiles.map((profile) => [profile.id, profile.name]),
      );
      const rowFor = (id: string) =>
        group.getByTestId("agent-profile-row").filter({ hasText: profileName.get(id)! });
      const sourceRow = rowFor(sourceProfile.id);
      const targetRow = rowFor(targetProfile.id);
      await expect(sourceRow).toBeVisible({ timeout: 15_000 });
      cdp = await testPage.context().newCDPSession(testPage);

      await sourceRow.scrollIntoViewIfNeeded();
      const scrollBefore = await touchSwipeOutsideHandle(cdp, sourceRow);
      await expect.poll(() => getScrollTop(sourceRow)).toBeGreaterThan(scrollBefore);
      expect(new URL(testPage.url()).pathname).toBe("/settings/agents");

      await touchDragProfile(
        cdp,
        sourceRow.getByTestId("agent-profile-drag-handle"),
        sourceRow,
        targetRow,
      );
      const expectedNames = expectedProfiles.map((profile) => profile.name);
      await expect.poll(() => visibleProfileNames(group)).toEqual(expectedNames);
      await expect
        .poll(
          async () => {
            const { agents: latest } = await apiClient.listAgents();
            return (
              latest.find((item) => item.id === agent.id)?.profiles.map((profile) => profile.id) ??
              []
            );
          },
          { timeout: 15_000 },
        )
        .toEqual(expectedProfiles.map((profile) => profile.id));

      await testPage.reload();
      const reloadedGroup = testPage.getByTestId(`agent-profiles-${agent.name}`);
      await expect(reloadedGroup.getByTestId("agent-profile-row").first()).toBeVisible({
        timeout: 15_000,
      });
      await expect.poll(() => visibleProfileNames(reloadedGroup)).toEqual(expectedNames);
    } finally {
      await cdp?.detach().catch(() => undefined);
      for (const id of created) await apiClient.deleteAgentProfile(id).catch(() => undefined);
    }
  });
});
