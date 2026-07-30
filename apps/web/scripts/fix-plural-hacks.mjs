#!/usr/bin/env node
/**
 * Convert English plural hacks into real i18next plural keys.
 *
 * `{n} comment{n !== 1 ? "s" : ""}` bakes English morphology into the markup: the
 * trailing "s" is not data, and no other language forms plurals that way. i18next
 * expresses this with suffixed keys and a `count` value:
 *
 *   {t("task:comments", { count: n })}
 *   { "comments_one": "{{count}} comment", "comments_other": "{{count}} comments" }
 *
 * Only the exact `<count> <word>{<count> OP 1 ? "s" : ""}` shape is rewritten —
 * anything else is left alone and reported, because guessing at irregular plurals
 * ("entry"/"entries") from a regex would silently corrupt copy.
 *
 * Usage: node scripts/fix-plural-hacks.mjs [--write] <dir|file> [...]
 */
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const WRITE = process.argv.includes("--write");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
if (!TARGETS.length) {
  console.error("usage: fix-plural-hacks.mjs [--write] <dir|file> [...]");
  process.exit(2);
}

const NAMESPACE_RULES = [
  [/^(app|components)\/settings\//, "settings"],
  [/^app\/office\//, "office"],
  [/^components\/app-sidebar\//, "sidebar"],
  [/^components\/app-status-bar\//, "statusBar"],
  [/^components\/(quick-chat|config-chat)\//, "chat"],
  [/^components\/(kanban\/|kanban-)/, "kanban"],
  [/^components\/(task\/|task-)/, "task"],
  [/^components\/review\//, "review"],
  [/^components\/diff\//, "diff"],
  [/^components\/editors\//, "editors"],
  [/^(app|components)\/github\//, "github"],
  [/^(app|components)\/gitlab\//, "gitlab"],
  [/^(app|components)\/jira\//, "jira"],
  [/^(app|components)\/linear\//, "linear"],
  [/^components\/sentry\//, "sentry"],
  [/^(app|components)\/azure-devops\//, "azureDevops"],
  [/^components\/automations\//, "automations"],
  [/^components\/plugins\//, "plugins"],
  [/^components\/(integrations|vcs)\//, "integrations"],
  [/^app\/stats\//, "stats"],
  [/^app\/auth\//, "auth"],
];
const namespaceFor = (rel) =>
  NAMESPACE_RULES.find(([re]) => re.test(rel.replace(/\\/g, "/")))?.[1] ?? "common";

const catalogs = {};
function loadCatalog(ns) {
  if (catalogs[ns]) return catalogs[ns];
  const file = path.join(EN, `${ns}.json`);
  catalogs[ns] = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : {};
  return catalogs[ns];
}

/** Reuse an existing plural pair when the same wording is already present. */
function pluralKeyFor(ns, word) {
  const cat = loadCatalog(ns);
  const one = `{{count}} ${word}`;
  const other = `{{count}} ${word}s`;
  for (const [k, v] of Object.entries(cat)) {
    if (k.endsWith("_one") && v === one) return k.slice(0, -4);
  }
  const base = `${word}s`;
  let key = base;
  let n = 2;
  while (`${key}_one` in cat) key = `${base}${n++}`;
  cat[`${key}_one`] = one;
  cat[`${key}_other`] = other;
  return key;
}

function listFiles() {
  const out = [];
  const walk = (dir) => {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, e.name);
      if (e.isDirectory()) {
        if (["node_modules", "dist", "e2e", "locales", "__tests__"].includes(e.name)) continue;
        walk(full);
      } else if (/\.tsx$/.test(e.name) && !/\.test\.tsx$/.test(e.name)) out.push(full);
    }
  };
  for (const t of TARGETS) {
    const abs = path.isAbsolute(t) ? t : path.join(ROOT, t);
    if (!fs.existsSync(abs)) continue;
    if (fs.statSync(abs).isDirectory()) walk(abs);
    else if (/\.tsx$/.test(abs)) out.push(abs);
  }
  return out;
}

// `{count} word{count OP 1 ? "s" : ""}` — the count expression must match on both
// sides, otherwise this is not a simple plural and we leave it alone.
// Both suffix orders occur: `n !== 1 ? "s" : ""` and `n === 1 ? "" : "s"`.
const HACK =
  /\{\s*([A-Za-z_$][\w$.?[\]]*)\s*\}(\s+)([A-Za-z][A-Za-z-]*)\s*\{\s*([A-Za-z_$][\w$.?[\]]*)\s*(?:!==|===|>|<|!=|==)\s*1\s*\?\s*(?:"s"\s*:\s*""|""\s*:\s*"s")\s*\}/gs;

const report = { files: 0, converted: 0, skippedMismatch: 0, skippedIrregular: 0 };
const IRREGULAR = /^(entr|categor|repositor|propert|activit|director|famil)/i;

for (const file of listFiles()) {
  const original = fs.readFileSync(file, "utf8");
  if (!/\?\s*(?:"s"\s*:\s*""|""\s*:\s*"s")/.test(original)) continue;
  const rel = path.relative(ROOT, file);
  const ns = namespaceFor(rel);
  let changed = false;
  let needsHook = false;

  const out = original.replace(HACK, (match, count, _gap, word, count2) => {
    if (count !== count2) {
      report.skippedMismatch += 1;
      return match;
    }
    if (IRREGULAR.test(word)) {
      // "entry" -> "entries", not "entrys"; needs a human.
      report.skippedIrregular += 1;
      return match;
    }
    changed = true;
    needsHook = true;
    report.converted += 1;
    const key = pluralKeyFor(ns, word);
    return `{t("${ns}:${key}", { count: ${count} })}`;
  });

  if (!changed) continue;
  let final = out;
  if (needsHook && !/from "react-i18next"/.test(final)) {
    const lines = final.split("\n");
    const at = /^\s*["']use (client|strict)["'];?\s*$/.test(lines[0] ?? "") ? 1 : 0;
    lines.splice(at, 0, `import { useTranslation } from "react-i18next";`);
    final = lines.join("\n");
  } else if (needsHook && !/\buseTranslation\b[^}]*\} from "react-i18next"/.test(final)) {
    final = final.replace(
      /import \{([^}]*)\} from "react-i18next";/,
      (m, names) => `import {${names.replace(/\s*$/, "")}, useTranslation } from "react-i18next";`,
    );
  }

  report.files += 1;
  if (WRITE) fs.writeFileSync(file, final);
}

if (WRITE) {
  for (const [ns, entries] of Object.entries(catalogs)) {
    const sorted = Object.fromEntries(
      Object.entries(entries).sort(([a], [b]) => a.localeCompare(b)),
    );
    fs.writeFileSync(path.join(EN, `${ns}.json`), JSON.stringify(sorted, null, 2) + "\n");
  }
}
console.log(JSON.stringify({ ...report, wrote: WRITE }, null, 2));
