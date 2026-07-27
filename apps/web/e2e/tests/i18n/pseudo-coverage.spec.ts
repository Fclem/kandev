import { type Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";

/**
 * Pseudo-locale coverage oracle.
 *
 * Under the pseudo-locale every EXTRACTED message is accented
 * (`Language` → `Ĺàńĝũàĝē`). So any user-facing text that is still plain ASCII
 * is, by definition, a string that was never wrapped in a Lingui macro. This
 * spec crawls chrome-heavy screens and reports those leftovers.
 *
 * Gated on `KANDEV_I18N_COVERAGE=1` while the full-sweep migration is in
 * flight (docs/plans/i18n): it is a diagnostic during the sweep and becomes a
 * hard gate at close-out (task-40), when the env guard is removed.
 *
 *   KANDEV_I18N_COVERAGE=1 pnpm e2e -- e2e/tests/i18n/pseudo-coverage.spec.ts
 */

const COVERAGE_ENABLED = process.env.KANDEV_I18N_COVERAGE === "1";

/** Screens whose visible text is overwhelmingly UI chrome, not user data. */
const SCREENS = [
  { name: "settings — appearance", url: "/settings/general/appearance" },
  { name: "settings — executors", url: "/settings/executors" },
  { name: "settings — agents", url: "/settings/agents" },
  { name: "settings — integrations", url: "/settings/integrations" },
];

/**
 * Text that is legitimately un-accented under pseudo: brand/proper nouns, code
 * identifiers, and units/symbols. Kept in sync with the eslint guard allowlist
 * in apps/web/eslint.i18n.config.mjs.
 */
const ALLOWED = [
  "Kandev",
  "GitHub",
  "GitLab",
  "Jira",
  "Linear",
  "Slack",
  "Sentry",
  "Azure DevOps",
  "ACP",
  "MCP",
  "SSH",
  "URL",
  "ID",
  "English",
  "Pseudo",
  "QA",
];

async function activatePseudo(page: Page, url: string) {
  await page.goto(url);
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 15_000 });
}

/**
 * Collect visible text nodes that look like un-externalized English copy.
 * Nodes inside script/style, and nodes that are wholly allowlisted, are ignored.
 */
async function findUnlocalizedText(page: Page, allowed: string[]): Promise<string[]> {
  return page.evaluate((allowedList) => {
    const skipTags = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "CODE", "PRE", "SVG"]);
    const wordlike = /[A-Za-z]{4,}/;
    const accented = /[À-ɏ]/;
    const found = new Set<string>();

    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    let node: Node | null;
    while ((node = walker.nextNode())) {
      const text = (node.textContent ?? "").trim();
      if (!text || !wordlike.test(text) || accented.test(text)) continue;

      const el = node.parentElement;
      if (!el || skipTags.has(el.tagName)) continue;
      // Ignore hidden nodes — not user-visible copy.
      const rect = el.getBoundingClientRect();
      if (rect.width === 0 && rect.height === 0) continue;

      // Strip allowlisted tokens; if nothing word-like remains, it's fine.
      let residue = text;
      for (const token of allowedList) residue = residue.split(token).join(" ");
      if (!wordlike.test(residue)) continue;

      found.add(text.slice(0, 120));
    }
    return [...found];
  }, allowed);
}

test.describe("i18n pseudo-locale coverage", () => {
  test.skip(
    !COVERAGE_ENABLED,
    "Set KANDEV_I18N_COVERAGE=1 to run the string-externalization oracle (hard gate at task-40).",
  );

  for (const screen of SCREENS) {
    test(`no un-externalized copy on ${screen.name}`, async ({ testPage }) => {
      await activatePseudo(testPage, screen.url);
      // Let lazy panels settle before scanning.
      await testPage.waitForTimeout(1_000);

      const leftovers = await findUnlocalizedText(testPage, ALLOWED);
      expect(
        leftovers,
        `Un-externalized strings on ${screen.name}:\n${leftovers.map((s) => `  - ${s}`).join("\n")}`,
      ).toEqual([]);
    });
  }
});
