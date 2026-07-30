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
          // `jsx-only` (not `jsx-text-only`) so the guard also sees copy that
          // never appears as a JSX text node: ternary button labels
          // (`{saving ? "Saving..." : "Save"}`) and display props on internal
          // components (`label=`, `description=`, `tooltip=`). Those are the
          // majority of user-facing strings in this codebase, and the narrower
          // mode reported them as clean. The cost is that every attribute is
          // now checked, so the `words.exclude` list below has to carry the
          // weight of separating copy from prop enum values.
          mode: "jsx-only",
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
              // All-caps acronym badges (ENTRY, KAN, MTD, WIQL, JQL) label a
              // field or entity; they are identifiers, not prose.
              "^[A-Z][A-Z0-9_]{1,9}$",
              // Terminal control glyphs (^C, ^D) and repeat counts (3x) are
              // symbols, and "id · vN.N" is a version line, not prose.
              "^\\^[A-Z]$",
              "^·?\\s*v?$",
              // Single lowercase/camel/kebab tokens are prop enum values,
              // classnames, and identifiers (variant="ghost", side="top",
              // value="work-items") — never display copy, which is capitalized
              // or multi-word.
              "^[a-z][a-zA-Z0-9]*$",
              "^[a-z0-9]+(-[a-z0-9]+)+$",
              // CSS lengths, colors, Tailwind class lists, link rel/target
              // keywords, route paths, and `__sentinel__` option values.
              "^\\d+(\\.\\d+)?(px|rem|em|%|vh|vw|ch|fr|s|ms)$",
              "^#[0-9a-fA-F]{3,8}$",
              "^_(blank|self|parent|top)$",
              "^(noopener|noreferrer)( (noopener|noreferrer))*$",
              "^__[a-z_]+__$",
              "^/[\\w/\\-\\[\\]:.]*$",
              "^(?:-?[a-z0-9]+(?:[:/-][a-z0-9.]+)*\\s+)+-?[a-z0-9]+(?:[:/-][a-z0-9.]+)*$",
            ],
          },
          "jsx-attributes": {
            // Attributes that carry display copy and must be translated.
            include: ["placeholder", "aria-label", "aria-description", "title", "alt"],
            exclude: [
              ".*[Cc]lassName$",
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
