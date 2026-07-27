// Standalone i18n lint config — the anti-regression guard for hardcoded UI
// strings. Kept SEPARATE from eslint.config.mjs during the migration because
// the main lint runs with `--max-warnings 0`, which would fail on every
// not-yet-migrated literal. Run it non-blocking with `pnpm lint:i18n`.
//
// Close-out (docs/plans/i18n task-40) folds `lingui/no-unlocalized-strings`
// into the main config as an ERROR once every batch reports clean.
import { defineConfig, globalIgnores } from "eslint/config";
import tseslint from "typescript-eslint";
import lingui from "eslint-plugin-lingui";

// Proper nouns / brand names / code identifiers that must NOT be translated.
const ALLOWED_LITERALS = [
  "^use client$",
  "^use strict$",
  "^Kandev$",
  "^GitHub$",
  "^GitLab$",
  "^Jira$",
  "^Linear$",
  "^Slack$",
  "^Sentry$",
  "^Azure DevOps$",
  "^ACP$",
  "^MCP$",
  "^SSH$",
  "^URL$",
  "^ID$",
];

export default defineConfig([
  globalIgnores([
    "**/dist/**",
    "**/node_modules/**",
    "**/test-results/**",
    "**/playwright-report/**",
    "**/*.test.ts",
    "**/*.test.tsx",
    "e2e/**",
    "src/locales/**",
  ]),
  ...tseslint.configs.recommended,
  {
    files: ["components/**/*.{ts,tsx}", "app/**/*.{ts,tsx}"],
    plugins: { lingui },
    rules: {
      "lingui/no-unlocalized-strings": [
        "warn",
        {
          ignore: ALLOWED_LITERALS,
          // JSX attribute / object-property names that never hold display copy.
          ignoreNames: [
            "data-testid",
            "data-legacy-testid",
            "className",
            "class",
            "id",
            "key",
            "type",
            "name",
            "role",
            "href",
            "src",
            "to",
            "d",
            "fill",
            "stroke",
            "viewBox",
            "data-slot",
            "data-variant",
            "data-side",
            "testId",
            "displayName",
            "slot",
            "htmlFor",
            "aria-labelledby",
            "aria-controls",
            "aria-describedby",
          ],
          // Function/method calls whose string args are identifiers, not copy.
          ignoreFunctions: [
            "cn",
            "clsx",
            "cva",
            "tv",
            "console.*",
            "*.getAttribute",
            "*.setAttribute",
            "*.matches",
            "*.closest",
            "*.querySelector",
          ],
        },
      ],
    },
  },
]);
