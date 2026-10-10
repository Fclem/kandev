import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import { waitForHttp } from "./causal-waits";

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.2
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.3
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.4
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-001.5
export async function exerciseLimitRecoverySettings(page: Page, api: ApiClient, mobile: boolean) {
  const { agents } = await api.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent");
  if (!agent) throw new Error("Missing mock agent");
  const profile = await api.createAgentProfile(agent.id, `Limit settings ${Date.now()}`, {
    model: "mock-fast",
    fallback_model: "mock-smart",
    auto_fallback: false,
  });
  const act = (locator: Locator) => (mobile ? locator.tap() : locator.click());
  const fallback = page.getByRole("switch", {
    name: "Use fallback model when limited",
    exact: true,
  });
  const resume = page.getByRole("switch", { name: "Resume after reset", exact: true });
  const trigger = page.getByTestId("profile-fallback-settings-trigger");
  const save = async () => {
    const response = waitForHttp(page, "PATCH", /\/api\/v1\/agent-profiles\/[^/]+$/);
    await act(page.getByRole("button", { name: /^Save( changes)?$/i }).first());
    expect((await response).ok()).toBe(true);
    await expect(page.getByText(/unsaved changes/i)).toBeHidden();
  };
  try {
    expect(profile).toMatchObject({ limitFallback: false, resumeAfterReset: false });
    await page.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
    await act(trigger);
    await expect(fallback).toHaveAttribute("aria-checked", "false");
    await expect(resume).toHaveAttribute("aria-checked", "false");
    await act(fallback);
    await act(resume);
    await expect(page.getByRole("button", { name: /^Save( changes)?$/i }).first()).toBeEnabled();
    await save();
    await expect
      .poll(() => api.getAgentProfile(profile.id))
      .toMatchObject({ limitFallback: true, resumeAfterReset: true });
    await page.reload();
    await act(trigger);
    await expect(fallback).toHaveAttribute("aria-checked", "true");
    await expect(resume).toHaveAttribute("aria-checked", "true");
    await act(page.getByRole("switch", { name: "Require exact model", exact: true }));
    await expect(fallback).toBeDisabled();
    await expect(fallback).toHaveAttribute("aria-checked", "true");
    await expect(resume).toBeEnabled();
    await save();
    await expect
      .poll(() => api.getAgentProfile(profile.id))
      .toMatchObject({ requireExactModel: true, limitFallback: true, resumeAfterReset: true });
    await page.reload();
    await act(trigger);
    await act(page.getByRole("switch", { name: "Require exact model", exact: true }));
    await expect(fallback).toBeEnabled();
    await expect(fallback).toHaveAttribute("aria-checked", "true");
    await save();
    await api.updateAgentProfile(profile.id, { auto_fallback: true });
    await page.reload();
    await act(trigger);
    await expect(fallback).toBeDisabled();
    await expect(fallback).toHaveAttribute("aria-checked", "true");
    await expect(resume).toBeEnabled();
    await api.updateAgentProfile(profile.id, { auto_fallback: false, fallback_model: "" });
    await page.reload();
    await act(trigger);
    await expect(fallback).toBeDisabled();
    await expect(resume).toBeEnabled();
    await assertNoDocumentHorizontalOverflow(page, "limit recovery settings");
    const help = page.getByRole("button", { name: "About automatic reset recovery", exact: true });
    if (mobile) {
      const box = await help.boundingBox();
      expect(box?.width).toBeGreaterThanOrEqual(44);
      expect(box?.height).toBeGreaterThanOrEqual(44);
      await help.tap();
      await expect(page.locator('[data-slot="drawer-content"][data-state="open"]')).toBeVisible();
      await assertNoDocumentHorizontalOverflow(page, "limit reset help drawer");
      await page.keyboard.press("Escape");
    } else {
      await help.focus();
      await expect(page.getByRole("tooltip")).toBeVisible();
    }
  } finally {
    await api.deleteAgentProfile(profile.id, true);
  }
}
