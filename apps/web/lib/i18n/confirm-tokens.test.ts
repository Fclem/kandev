import fs from "node:fs";
import path from "node:path";

import { afterAll, describe, expect, it } from "vitest";

import { activateLocale, DEFAULT_LOCALE, i18n } from "./index";

/**
 * Type-to-confirm dialogs gate a destructive action on the user typing an exact
 * token (`RESET`, `RESTORE`, `DELETE`, `delete`, …) that is also compared with
 * `===` and, in some cases, sent to the backend.
 *
 * If that token is baked into translatable copy, a translated locale tells the
 * user to type a word the gate will never accept — the dialog becomes
 * impossible to satisfy, and the bug is invisible until a second language ships.
 * The fix is to interpolate the sentinel, so these tests assert the invariant:
 *
 *   1. the token survives verbatim in every locale (including `pseudo`, which
 *      accents everything translatable), and
 *   2. no catalog message hardcodes a sentinel token.
 *
 * See docs/plans/i18n/FOLLOWUPS.md.
 */

/** Messages that must interpolate the sentinel rather than contain it. */
const CONFIRM_MESSAGES = [
  "settings:typeToConfirm",
  "settings:typeToEnableTheConfirmButton",
  "settings:typeTokenToEnableConfirmAndRelaunch",
  "settings:typeTokenToConfirmDeletion",
] as const;

/** Sentinels used by the confirm dialogs in the app. */
const TOKENS = ["RESET", "RESTORE", "DEDICATED", "ADOPT", "DELETE", "delete"] as const;

const LOCALES_DIR = path.resolve(import.meta.dirname, "..", "..", "src", "locales");

function catalogEntries(locale: string): [string, string][] {
  const dir = path.join(LOCALES_DIR, locale);
  const out: [string, string][] = [];
  for (const file of fs.readdirSync(dir).filter((f) => f.endsWith(".json"))) {
    const ns = file.replace(/\.json$/, "");
    const parsed = JSON.parse(fs.readFileSync(path.join(dir, file), "utf8")) as Record<
      string,
      unknown
    >;
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof value === "string") out.push([`${ns}:${key}`, value]);
    }
  }
  return out;
}

afterAll(async () => {
  await activateLocale(DEFAULT_LOCALE);
});

describe("type-to-confirm sentinels survive translation", () => {
  for (const locale of ["en", "pseudo"] as const) {
    it(`renders the token verbatim under ${locale}`, async () => {
      await activateLocale(locale);
      for (const key of CONFIRM_MESSAGES) {
        for (const token of TOKENS) {
          const rendered = i18n.t(key, { token });
          expect(
            rendered,
            `${key} under ${locale} dropped or altered the "${token}" sentinel`,
          ).toContain(token);
        }
      }
    });
  }

  it("keeps confirm copy free of hardcoded sentinels", () => {
    // A sentinel written into the message text (rather than interpolated) is the
    // exact defect this guards: translators would translate it.
    const offenders = CONFIRM_MESSAGES.flatMap((key) => {
      const message = i18n.getResource("en", key.split(":")[0], key.split(":")[1]) as
        | string
        | undefined;
      if (!message) return [];
      const hits = TOKENS.filter((token) => message.includes(token));
      return hits.length ? [`${key}: contains ${hits.join(", ")}`] : [];
    });
    expect(offenders).toEqual([]);
  });

  it("has no orphaned pre-fix message keys", () => {
    // These keys baked the sentinel into the copy; they must stay deleted so the
    // broken wording cannot be reintroduced by a stale reference.
    const removed = ["typeResetToConfirm", "typeResetToEnableTheConfirm", "typeToConfirm2"];
    const keys = new Set(catalogEntries("en").map(([key]) => key.split(":")[1]));
    for (const key of removed) {
      expect(keys.has(key), `${key} should have been removed`).toBe(false);
    }
  });
});
