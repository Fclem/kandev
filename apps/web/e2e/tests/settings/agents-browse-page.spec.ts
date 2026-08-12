import { test, expect } from "../../fixtures/test-base";

// The default mock-agent is discovered as installed with no install_script, so
// the "Available to Install" section would not render. Intercept
// /api/v1/agents/available and return one discoverable-but-not-installed agent
// so the section has an install card to assert on.
const AVAILABLE_AGENTS = {
  agents: [
    {
      name: "codex",
      display_name: "OpenAI Codex CLI",
      available: false,
      install_script: "npm install -g @openai/codex",
      info_url: "",
      model_config: {
        default_model: "",
        available_models: [],
        modes: [],
        current_mode_id: "",
        status: "not_installed",
        error: "",
      },
      permission_settings: {},
      passthrough_config: null,
    },
  ],
  tools: [],
  total: 1,
};

test.describe("Agents browse page", () => {
  test("renders the heading and install cards statically, without a collapsible toggle", async ({
    testPage,
  }) => {
    await testPage.route("**/api/v1/agents/available**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(AVAILABLE_AGENTS),
      }),
    );

    await testPage.goto("/settings/agents/browse");

    const heading = testPage.getByRole("heading", { name: "Browse available agents" });
    await expect(heading).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("install-card-codex")).toBeVisible();

    // PR #2544 wrapped the section in a collapsible whose heading row was a
    // toggle button. Reverted, the heading must be a plain heading: no button
    // with the heading's accessible name and no button ancestor. Assert the
    // semantic shape rather than the old implementation's test ID, so any
    // future collapsible reintroduction fails even with different test IDs.
    await expect(testPage.getByRole("button", { name: "Browse available agents" })).toHaveCount(0);
    expect(await heading.evaluate((el) => el.closest("button") === null)).toBe(true);

    // A role-less clickable wrapper (e.g. <div onClick>) would not surface as
    // a button; clicking the heading must not hide the install cards.
    await heading.click();
    await expect(testPage.getByTestId("install-card-codex")).toBeVisible();

    // Compatibility guard: the exact test ID PR #2544 introduced is gone too.
    await expect(testPage.getByTestId("available-to-install-trigger")).toHaveCount(0);
  });
});
