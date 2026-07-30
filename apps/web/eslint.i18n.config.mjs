// Standalone i18n lint config — the anti-regression guard for hardcoded UI
// strings. Kept SEPARATE from eslint.config.mjs while the full-sweep migration
// is in flight, because the main lint runs with `--max-warnings 0` and would
// fail on every not-yet-migrated literal. Run it non-blocking: `pnpm lint:i18n`.
//
// Close-out (docs/plans/i18n task-40) folds `i18next/no-literal-string` into the
// main config as an ERROR once every batch reports clean.
import { defineConfig, globalIgnores } from "eslint/config";
import tseslint from "typescript-eslint";
import i18next from "eslint-plugin-i18next";

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
    "scripts/**",
  ]),
  ...tseslint.configs.recommended,
  {
    files: ["components/**/*.{ts,tsx}", "app/**/*.{ts,tsx}"],
    plugins: { i18next },
    rules: {
      "i18next/no-literal-string": [
        "warn",
        {
          // Only flag text that actually reaches the user.
          mode: "jsx-text-only",
          "should-validate-template": false,
          // Brand/proper nouns and symbol-only strings are not translatable copy.
          words: {
            exclude: [
              "^\\s*$",
              "^[^A-Za-z]*$",
              "^(Kandev|GitHub|GitLab|Jira|Linear|Slack|Sentry|Azure DevOps)$",
              "^(ACP|MCP|SSH|URL|ID|PR|CI|AI|API|JSON|YAML|LSP|TLS|SQL|JQL)$",
              // Units, version prefixes, and keyboard glyphs — not translatable
              // copy; these show up as fragments beside an interpolated value.
              "^(v|ms\\)?|s|m|h|d|K|B|KB|MB|GB|TB|esc)$",
              "^\\+[A-Z]\\)?$",
              "^[·+\\-|/(),.:\\s]+$",
            ],
          },
          "jsx-attributes": {
            // Attributes that carry display copy and must be translated.
            include: ["placeholder", "aria-label", "aria-description", "title", "alt"],
            exclude: [
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
              "htmlFor",
              "data-.*",
              "aria-labelledby",
              "aria-controls",
              "aria-describedby",
            ],
          },
          callees: {
            // String args to these are identifiers/classnames, not copy.
            exclude: [
              "cn",
              "clsx",
              "cva",
              "tv",
              "t",
              "i18n(ext)?.*",
              "require",
              "console\\.\\w+",
              ".*\\.(getAttribute|setAttribute|matches|closest|querySelector)",
            ],
          },
        },
      ],
    },
  },
]);
