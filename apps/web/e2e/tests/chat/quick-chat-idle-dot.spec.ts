import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { openQuickChatWithAgent, sendQuickChatMessage } from "./quick-chat-helpers";

test.describe("quick chat idle dot", () => {
  test("marks a closed quick chat after a turn completes and re-arms after opening", async ({
    testPage,
  }) => {
    const ws = watchWs(testPage);
    const created = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    const dialog = await openQuickChatWithAgent(testPage);
    const createdBody = (await created).json() as Promise<{ session_id: string }>;
    const { session_id: sessionId } = await createdBody;
    const shortcut = testPage.getByTestId("sidebar-quick-chat-shortcut");

    await expect(shortcut.getByTestId("quick-chat-unseen-dot")).toHaveCount(0);
    const firstCompletion = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await testPage.keyboard.press("Escape");
    await firstCompletion;
    await expect(shortcut.getByTestId("quick-chat-unseen-dot")).toBeVisible();

    await shortcut.click();
    await expect(shortcut.getByTestId("quick-chat-unseen-dot")).toHaveCount(0);
    const secondCompletion = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await testPage.keyboard.press("Escape");
    await secondCompletion;
    await expect(shortcut.getByTestId("quick-chat-unseen-dot")).toBeVisible();
  });
});
