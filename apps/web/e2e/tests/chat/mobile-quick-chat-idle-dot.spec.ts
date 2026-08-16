import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { sendQuickChatMessage, startQuickChatFromSetup } from "./quick-chat-helpers";

test.describe("quick chat idle dot", () => {
  test("marks the mobile header after a closed quick chat turn completes", async ({ testPage }) => {
    const ws = watchWs(testPage);
    await testPage.goto("/");
    await testPage.getByTestId("mobile-quick-chat-button").tap();
    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    const created = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") && response.request().method() === "POST",
    );
    await startQuickChatFromSetup(dialog, testPage);
    await expect(
      testPage.getByRole("status", { name: /Agent is (starting|running)/ }),
    ).not.toBeVisible();
    const { session_id: sessionId } = (await (await created).json()) as { session_id: string };
    const button = testPage.getByTestId("mobile-quick-chat-button");
    const completed = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    await sendQuickChatMessage(dialog, testPage, "/slow 8s");
    await expect(dialog.getByText("Running slow response", { exact: false })).toBeVisible();
    await dialog.getByTestId("quick-chat-close").tap();
    await completed;
    await expect(button.getByTestId("quick-chat-unseen-dot")).toBeVisible();
  });
});
