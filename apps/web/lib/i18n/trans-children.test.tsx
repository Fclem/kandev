import { render } from "@testing-library/react";
import { afterAll, describe, expect, it } from "vitest";
import { Trans } from "react-i18next";

import { activateLocale, DEFAULT_LOCALE, i18n } from "./index";

/**
 * `<Trans>` renders the catalog message, not its children. The children exist so
 * a `<n>` tag in the message has something to substitute; any text among them is
 * only a development-time fallback.
 *
 * That distinction matters because ~340 call sites in this codebase pass a
 * `t()` call as a `<Trans>` text child:
 *
 *   <Trans i18nKey="ns:commit">
 *     <IconCheck />
 *     {t("common:commit")}
 *   </Trans>
 *
 * These read as if the inner `t()` supplies the label, and reviewers keep asking
 * whether the copy renders twice. It does not — the message wins and the inner
 * call is inert. The pattern is redundant (two keys hold the same English, which
 * can drift apart in translation) but not broken, so it is documented in
 * docs/plans/i18n/FOLLOWUPS.md rather than rewritten.
 *
 * These tests pin the behaviour so a future react-i18next upgrade that starts
 * rendering text children fails here instead of duplicating copy across the UI.
 */
afterAll(async () => {
  await activateLocale(DEFAULT_LOCALE);
});

describe("<Trans> children vs the catalog message", () => {
  it("renders the message text once and ignores a t() text child", async () => {
    await activateLocale(DEFAULT_LOCALE);
    i18n.addResource(DEFAULT_LOCALE, "transprobe", "labelled", "<0></0> Commit");
    i18n.addResource(DEFAULT_LOCALE, "transprobe", "inner", "SHOULD-NOT-RENDER");

    const { container } = render(
      <Trans i18nKey="transprobe:labelled">
        <span data-testid="icon" />
        {i18n.t("transprobe:inner")}
      </Trans>,
    );

    expect(container.textContent).toBe(" Commit");
    expect(container.textContent).not.toContain("SHOULD-NOT-RENDER");
    expect(container.querySelector("[data-testid='icon']")).not.toBeNull();
  });

  it("substitutes <n> from the children by position", async () => {
    await activateLocale(DEFAULT_LOCALE);
    i18n.addResource(DEFAULT_LOCALE, "transprobe", "wrapped", "Open <1>the docs</1> to continue");

    const { container } = render(
      <Trans i18nKey="transprobe:wrapped">
        text
        <a href="/docs">placeholder</a>
      </Trans>,
    );

    const link = container.querySelector("a");
    expect(link?.textContent).toBe("the docs");
    expect(container.textContent).toBe("Open the docs to continue");
  });
});
