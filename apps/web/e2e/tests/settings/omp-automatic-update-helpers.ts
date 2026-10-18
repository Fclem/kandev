import { expect, type Locator, type Page } from "@playwright/test";
import type { AgentUpdateStatus } from "../../../lib/api/domains/agent-update-api";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { waitForHttp } from "../../helpers/causal-waits";
import { expectTouchControl, expectTouchSquareControl } from "../../helpers/control-sizing";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { installRuntimeUpdateFixture } from "./agent-runtime-update-helpers";

const POLICY_URL = "/settings/agents#runtime-update-omp-acp";
const AUTOMATIC_PATH = /\/agent-update\/omp-acp\/automatic$/;

type OMPStatus = AgentUpdateStatus & { update_mode: "self_update" };

async function installOMPPolicyFixture(page: Page, consent = false) {
  let status: OMPStatus = {
    agent_name: "omp-acp",
    display_name: "Oh My Pi",
    runtime_id: "omp-acp",
    update_mode: "self_update",
    owner: "external",
    mechanism: "native",
    management: "manual",
    source: "@oh-my-pi/pi-coding-agent",
    guidance_url: "https://github.com/can1357/oh-my-pi",
    current_version: "1.2.0",
    effective_version: "1.2.0",
    latest_version: "1.1.0",
    available: true,
    enabled: true,
    auto_update_supported: true,
    auto_update: consent,
    auto_update_paused: false,
    package: "@oh-my-pi/pi-coding-agent",
    default_version: "",
    check_state: "unknown",
  };
  const runtime = await installRuntimeUpdateFixture(page, {
    mode: "self_update",
    displayName: "Oh My Pi",
    currentVersion: "1.2.0",
    defaultVersion: "",
    latestVersion: "1.1.0",
    statusResponse: [status],
  });
  let failSave = false;
  let writes = 0;
  let paused = false;
  await page.route("**/api/v1/agent-update/omp-acp/automatic", async (route) => {
    if (route.request().method() !== "PATCH") return route.fulfill({ status: 405 });
    if (failSave) {
      failSave = false;
      return route.fulfill({ status: 503, json: { error: "fixture policy write unavailable" } });
    }
    const { enabled } = route.request().postDataJSON() as { enabled: boolean };
    writes++;
    if (enabled && !status.auto_update) paused = false;
    status = { ...status, auto_update: enabled, auto_update_paused: enabled && paused };
    runtime.setStatusResponse([status]);
    return route.fulfill({ json: { enabled } });
  });
  return {
    runtime,
    update(patch: Partial<OMPStatus>) {
      paused = patch.auto_update_paused ?? paused;
      status = { ...status, ...patch };
      status.auto_update_paused = status.auto_update && paused;
      runtime.setStatusResponse([status]);
    },
    failNextSave() {
      failSave = true;
    },
    writes: () => writes,
  };
}

async function activate(control: Locator, mobile: boolean) {
  if (mobile) {
    await expectTouchControl(control);
    await control.tap();
  } else await control.click();
}

async function savePolicy(page: Page, mobile: boolean, expectedStatus = 200) {
  const save = page.getByTestId("settings-floating-save").getByRole("button", { name: /Save/i });
  await expect(save).toBeVisible();
  const [response] = await Promise.all([
    waitForHttp(page, "PATCH", AUTOMATIC_PATH),
    activate(save, mobile),
  ]);
  expect(response.status()).toBe(expectedStatus);
}

async function inspectOMPControls(page: Page, mobile: boolean) {
  const row = page.getByTestId("runtime-policy-omp-acp");
  const automatic = row.getByRole("switch", { name: "Automatic updates" });
  await expect(automatic).toBeVisible();
  const descriptionId = await automatic.getAttribute("aria-describedby");
  expect(descriptionId).toBeTruthy();
  const warning = page.locator(`[id="${descriptionId}"]`);
  await expect(warning).toBeVisible();
  await expect(warning).toContainText(/configured channel/i);
  await expect(warning).toContainText(/15 minutes/);
  await expect(warning).toContainText(/in place/);
  await expect(warning).toContainText(/cannot roll back/i);
  await expect(row).not.toContainText(/does not run an unverified/i);
  await expect(page.locator("#runtime-automatic-help")).toHaveCount(0);
  const manage = row.getByRole("link", { name: "Manage update", exact: true });
  await expect(manage).toHaveAttribute("href", "#installed-agent-omp-acp");
  await expect(row.getByRole("link", { name: "Manual update guidance" })).toHaveAttribute(
    "href",
    /oh-my-pi/,
  );
  const touchTarget = row.locator('[data-settings-touch-target="true"]');
  if (mobile) {
    await expectTouchSquareControl(touchTarget);
    await expectTouchControl(manage);
    await expectTouchControl(row.getByRole("link", { name: "Manual update guidance" }));
  }
  return { row, automatic, touchTarget, manage };
}

async function inspectOMPScroll(page: Page, row: Locator, mobile: boolean) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
  if (!mobile) return;
  const owners = await row.evaluate((element) => {
    let count = 0;
    for (let ancestor = element.parentElement; ancestor; ancestor = ancestor.parentElement) {
      if (["auto", "scroll"].includes(getComputedStyle(ancestor).overflowY)) count++;
    }
    return count;
  });
  expect(owners, "OMP retains one page scroll owner").toBe(1);
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-004.1, AC-AGENTS-RUNTIME-NOTIFY-004.11, AC-AGENTS-RUNTIME-NOTIFY-004.12
export async function ompAutomaticConsent(page: Page, mobile: boolean, capture?: PrAssetCapture) {
  const fixture = await installOMPPolicyFixture(page);
  try {
    await page.goto(POLICY_URL);
    const { row, automatic, touchTarget, manage } = await inspectOMPControls(page, mobile);
    await expect(automatic).not.toBeChecked();
    await row.scrollIntoViewIfNeeded();
    await capture?.screenshot("omp-before-consent", {
      caption: "OMP channel cadence and in-place risk are visible before consent.",
    });
    await activate(touchTarget, mobile);
    const summary = page.locator("#runtime-updates > details > summary");
    await activate(summary, mobile);
    await expect(automatic).not.toBeVisible();
    await activate(summary, mobile);
    await expect(automatic).toBeChecked();
    expect(fixture.writes()).toBe(0);
    await savePolicy(page, mobile);
    await page.reload();
    await expect(automatic).toBeChecked();
    await activate(touchTarget, mobile);
    await activate(
      page.getByTestId("settings-floating-save").getByRole("button", { name: /Reset/i }),
      mobile,
    );
    await expect(automatic).toBeChecked();
    expect(fixture.writes()).toBe(1);
    fixture.failNextSave();
    await activate(touchTarget, mobile);
    await savePolicy(page, mobile, 503);
    await expect(automatic).not.toBeChecked();
    await expect(page.getByTestId("settings-floating-save")).toBeVisible();
    await activate(
      page.getByTestId("settings-floating-save").getByRole("button", { name: /Reset/i }),
      mobile,
    );
    await page.reload();
    await expect(automatic).toBeChecked();
    expect(fixture.writes()).toBe(1);
    await activate(manage, mobile);
    await expect(page).toHaveURL(/#installed-agent-omp-acp$/);
    await expect(page.locator("#installed-agent-omp-acp")).toBeInViewport();
    fixture.update({ available: false, auto_update_supported: false });
    const unavailableStatus = page.waitForResponse(
      (response) => response.url().includes("/agent-update/status") && response.ok(),
    );
    await page.goto("about:blank");
    await page.goto(POLICY_URL);
    expect(JSON.stringify(await (await unavailableStatus).json())).toMatch(/"available":\s*false/);
    await openOtherRegistrations(page);
    await expect(automatic).toBeChecked();
    await expect(automatic).toBeEnabled();
    await activate(touchTarget, mobile);
    await savePolicy(page, mobile);
    await page.reload();
    await openOtherRegistrations(page);
    await expect(row).toBeVisible();
    await expect(automatic).toHaveCount(0);
    expect(fixture.writes()).toBe(2);
    expect(fixture.runtime.postCount()).toBe(0);
    await inspectOMPScroll(page, row, mobile);
  } finally {
    if (!page.isClosed()) await page.unrouteAll({ behavior: "wait" });
  }
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-004.10, AC-AGENTS-RUNTIME-NOTIFY-004.11, AC-AGENTS-RUNTIME-NOTIFY-004.12
export async function ompAutomaticOutcomeRecovery(
  page: Page,
  mobile: boolean,
  capture?: PrAssetCapture,
) {
  const fixture = await installOMPPolicyFixture(page, true);
  const finishedAt = "2026-10-10T12:00:00Z";
  fixture.update({
    last_change: {
      id: "omp-changed-earlier",
      previous_version: "1.0.0",
      observed_version: "1.2.0",
      finished_at: finishedAt,
    },
    last_outcome: {
      id: "omp-no-change",
      status: "succeeded",
      update_mode: "self_update",
      operation: "up_to_date",
      previous_version: "1.2.0",
      observed_version: "1.2.0",
      target_version: "",
      finished_at: finishedAt,
      output_excerpt: `configured channel unchanged\n<script>never execute</script>\n${"long-output".repeat(40)}`,
    },
  });
  try {
    await page.goto(POLICY_URL);
    const { row, automatic, touchTarget } = await inspectOMPControls(page, mobile);
    const latest = row.getByTestId("runtime-omp-result-omp-acp");
    await expect(latest).toContainText(/reported no version change/i);
    await expect(latest).not.toContainText(/up to date|installed|activate|rollback|default/i);
    await expect(row.getByText(/Last change:/)).toContainText("1.0.0");
    await expect(row.getByText(/Last change:/)).toContainText("1.2.0");
    await expect(row.getByText(/Stable latest.*1.1.0/)).toBeVisible();
    const output = row.locator("details");
    await expect(output).not.toHaveAttribute("open");
    const outputSummary = output.locator("summary");
    if (mobile) await expectTouchControl(outputSummary);
    await row.scrollIntoViewIfNeeded();
    await capture?.screenshot("omp-retained-no-change", {
      caption:
        "OMP keeps the previous change separately from a neutral no-change result and collapsed updater output.",
    });
    await activate(outputSummary, mobile);
    await expect(output.locator("pre")).toContainText("<script>never execute</script>");
    await expect(output.locator("script")).toHaveCount(0);
    await inspectOMPScroll(page, row, mobile);
    await activate(outputSummary, mobile);
    fixture.update({
      last_outcome: {
        id: "omp-running",
        status: "running",
        update_mode: "self_update",
        previous_version: "1.2.0",
        target_version: "",
      },
    });
    let refreshed = waitForHttp(page, "GET", /\/agent-update\/status$/);
    await fixture.runtime.emit("agent.update.started", {
      job_id: "omp-running",
      agent_name: "omp-acp",
      update_mode: "self_update",
      automatic: true,
      status: "updating",
    });
    await refreshed;
    await expect(latest).toContainText(/checking.*channel/i);
    fixture.update({
      current_version: "1.3.0",
      effective_version: "1.3.0",
      last_change: {
        id: "omp-changed",
        previous_version: "1.2.0",
        observed_version: "1.3.0",
        finished_at: finishedAt,
      },
      last_outcome: {
        id: "omp-changed",
        status: "succeeded",
        update_mode: "self_update",
        operation: "update",
        previous_version: "1.2.0",
        observed_version: "1.3.0",
        target_version: "",
        finished_at: finishedAt,
      },
    });
    refreshed = waitForHttp(page, "GET", /\/agent-update\/status$/);
    await fixture.runtime.emit("agent.update.finished", {
      job_id: "omp-changed",
      agent_name: "omp-acp",
      update_mode: "self_update",
      automatic: true,
      status: "succeeded",
      operation: "update",
      effective_version: "1.3.0",
    });
    await refreshed;
    await expect(latest).toContainText("1.2.0");
    await expect(latest).toContainText("1.3.0");
    await expect(latest).toContainText(/reported a version change/i);
    await fixture.runtime.emit("system.update_available", {
      agent_name: "omp-acp",
      runtime_id: "omp-acp",
      display_name: "Oh My Pi",
      update_mode: "self_update",
      runtime_update_status: "succeeded",
      previous_version: "1.2.0",
      version: "1.3.0",
      occurrence_id: "omp-changed",
    });
    const changedNotice = page
      .getByTestId("toast-message")
      .filter({ hasText: /Oh My Pi version changed/i });
    await expect(changedNotice).toContainText("1.3.0");
    await expect(changedNotice).not.toContainText(/select.*version|older.*version|Kandev default/i);
    const changedReview = changedNotice.getByRole("link", { name: "Review runtime" });
    if (mobile) await expectTouchControl(changedReview);
    await activate(changedReview, mobile);
    await expect(changedNotice).toHaveCount(0);
    fixture.update({
      auto_update_paused: true,
      last_outcome: {
        id: "omp-validation-failed",
        status: "failed",
        update_mode: "self_update",
        previous_version: "1.3.0",
        target_version: "",
        finished_at: finishedAt,
        output_excerpt: "updater complete\nvalidation requires authentication",
      },
    });
    refreshed = waitForHttp(page, "GET", /\/agent-update\/status$/);
    await fixture.runtime.emit("agent.update.finished", {
      job_id: "omp-validation-failed",
      agent_name: "omp-acp",
      update_mode: "self_update",
      automatic: true,
      status: "failed",
    });
    await refreshed;
    await expect(latest).toContainText(/installation may (?:already )?have changed/i);
    await expect(row.getByTestId("runtime-omp-paused-omp-acp")).toContainText(
      /off.*save.*on.*save/i,
    );
    await expect(output).not.toHaveAttribute("open");
    await expect(row.getByText(/Last change:/)).toContainText("1.3.0");
    await row.scrollIntoViewIfNeeded();
    await capture?.screenshot("omp-paused-recovery", {
      caption:
        "An OMP validation failure retains the last change and explains saved opt-out then opt-in recovery without rollback claims.",
    });
    await fixture.runtime.emit("system.update_available", {
      agent_name: "omp-acp",
      runtime_id: "omp-acp",
      display_name: "Oh My Pi",
      update_mode: "self_update",
      runtime_update_status: "failed",
      occurrence_id: "omp-validation-failed",
    });
    const pausedNotice = page
      .getByTestId("toast-message")
      .filter({ hasText: /Oh My Pi automatic checks paused/i });
    await expect(pausedNotice).toContainText(/off.*save.*on.*save/i);
    await expect(pausedNotice).not.toContainText(/select.*version|older.*version|Kandev default/i);
    await activate(pausedNotice.getByRole("link", { name: "Review runtime" }), mobile);
    await expect(pausedNotice).toHaveCount(0);
    await activate(touchTarget, mobile);
    await savePolicy(page, mobile);
    await expect(automatic).not.toBeChecked();
    await expect(row.getByTestId("runtime-omp-paused-omp-acp")).toHaveCount(0);
    await activate(touchTarget, mobile);
    await savePolicy(page, mobile);
    await page.reload();
    await expect(automatic).toBeChecked();
    await expect(row.getByTestId("runtime-omp-paused-omp-acp")).toHaveCount(0);
    await expect(latest).toContainText(/next eligible interval/i);
    await inspectOMPScroll(page, row, mobile);
    expect(fixture.writes()).toBe(2);
    expect(fixture.runtime.postCount()).toBe(0);
    await waitForFiniteAnimations(row);
  } finally {
    if (!page.isClosed()) await page.unrouteAll({ behavior: "wait" });
  }
}

// An unavailable runtime is listed under the collapsed "Other registered runtimes" disclosure.
async function openOtherRegistrations(page: Page) {
  const summary = page.getByText(/^Other registered runtimes? \(\d+\)$/);
  const details = page.locator("details", { has: summary }).last();
  // The group can remount while hydrating, which closes an already opened disclosure.
  await expect(async () => {
    if (!(await details.evaluate((element) => (element as HTMLDetailsElement).open))) {
      await summary.click({ timeout: 2000 });
    }
    await expect(details).toHaveJSProperty("open", true, { timeout: 1000 });
  }).toPass();
}
