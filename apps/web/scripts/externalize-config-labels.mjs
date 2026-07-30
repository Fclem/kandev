#!/usr/bin/env node
/**
 * Externalize display copy held in MODULE-SCOPE config objects.
 *
 *   const STATUS_OPTIONS = [{ value: "todo", label: "Todo" }];   // before
 *   const STATUS_OPTIONS = [{ value: "todo", labelKey: "task:todo" }];  // after
 *
 * `externalize-strings.mjs` deliberately declines these: wrapping the literal as
 * `label: t("task:todo")` evaluates at import time, freezing the copy to whatever
 * locale was active at boot and never updating on a switch. The fix is to store
 * the KEY as data and translate at the render site.
 *
 * The property is RENAMED (`label` -> `labelKey`) on purpose. A consumer that
 * still reads `.label` then fails to compile, instead of silently rendering the
 * raw key to the user — which is what makes this safe to run across files whose
 * consumers this script cannot see. Local `type`/`interface` members of the same
 * name are renamed too, so the file itself stays consistent; everything else
 * surfaces as a tsc error to fix by hand.
 *
 * Only objects whose shape is declared INLINE are rewritten. When the enclosing
 * declaration is annotated with a named type (`const X: SelectFieldItem[] = …`),
 * that type is very often also satisfied by runtime data — a user's agent-profile
 * names alongside our own static options — and those values must not become
 * catalog keys. Such objects are reported for hand treatment (give the type both
 * a `labelKey` and a raw `label`, and resolve at render).
 *
 * Usage: node scripts/externalize-config-labels.mjs [--write] <dir|file> [...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const WRITE = process.argv.includes("--write");
/**
 * Rewrite objects annotated with a named type too. Off by default because such a
 * type is often ALSO satisfied by runtime data, which must not become a key —
 * pass it per-file once you have checked that shape has only static sources.
 */
const FORCE_NAMED = process.argv.includes("--include-named-types");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
if (!TARGETS.length) {
  console.error("usage: externalize-config-labels.mjs [--write] <dir|file> [...]");
  process.exit(2);
}

/** Config keys that hold display copy. Mirrors DISPLAY_PROPS in the sibling script. */
const DISPLAY_KEYS = new Set([
  "label",
  "description",
  "tooltip",
  "hint",
  "heading",
  "headline",
  "title",
  "subtitle",
  "helpText",
  "placeholder",
  "emptyLabel",
  "errorTitle",
  "message",
  "note",
  "summary",
  "caption",
]);

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

function keyFromText(text) {
  const words = text
    .replace(/[^A-Za-z0-9 ]+/g, " ")
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 6);
  if (!words.length) return "text";
  const [first, ...rest] = words;
  return (
    first.toLowerCase() + rest.map((w) => w[0].toUpperCase() + w.slice(1).toLowerCase()).join("")
  );
}

/**
 * Keyboard key names are printed on physical keys and are not translated copy —
 * the spec lists keyboard glyphs as out of scope. They show up as `label` on
 * keybar/shortcut config objects, which otherwise look exactly like display copy.
 */
const KEY_NAMES =
  /^(Esc|Escape|Tab|Home|End|PgUp|PgDn|Page Up|Page Down|Enter|Return|Space|Backspace|Delete|Del|Ins|Insert|Shift|Ctrl|Control|Alt|Option|Cmd|Command|Meta|Fn|F\d{1,2}|Up|Down|Left|Right)$/;

/**
 * Fixture modules hold sample domain data (task titles, repo names) for demos and
 * storybook-style previews. That is user data by nature, never translated.
 */
const FIXTURE_FILE = /(^|\/)(.*-)?(mock|mocks|fixture|fixtures|demo)(-.*)?(\.|\/)/i;

/** Same copy heuristic as externalize-strings.mjs. */
const KEEP_LITERAL =
  /^(Kandev|GitHub|GitLab|Jira|Linear|Slack|Sentry|Azure DevOps|Docker|SSH|ACP|MCP|PR|CI|AI|API|JSON|YAML|LSP|TLS|SQL|URL|ID|PostgreSQL|SQLite|Claude|Codex|OpenCode|Copilot|Amp)$/;
function looksLikeCopy(raw) {
  const s = raw.trim();
  if (s.length < 2 || !/[A-Za-z]{2}/.test(s)) return false;
  if (KEEP_LITERAL.test(s) || KEY_NAMES.test(s)) return false;
  if (/^https?:\/\//.test(s) || s.startsWith("/") || s.startsWith("~/")) return false;
  if (/^[a-z0-9]+([-_][a-z0-9]+)+$/.test(s)) return false;
  if (/^[a-z]+([A-Z][a-z0-9]*)+$/.test(s)) return false;
  if (/^[A-Z][A-Z0-9_]*$/.test(s)) return false;
  if (/^[a-zA-Z]+:[a-zA-Z0-9_]+$/.test(s)) return false; // already a catalog key
  if (/[{}<>$]/.test(s) && !/\s/.test(s)) return false;
  if (/^[\w.-]+@[\w.-]+$/.test(s)) return false;
  return true;
}

const catalogs = {};
function loadCatalog(ns) {
  if (catalogs[ns]) return catalogs[ns];
  const file = path.join(EN, `${ns}.json`);
  catalogs[ns] = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : {};
  return catalogs[ns];
}
function keyFor(ns, message) {
  const cat = loadCatalog(ns);
  for (const [k, v] of Object.entries(cat)) if (v === message) return k;
  const base = keyFromText(message);
  let key = base;
  let n = 2;
  while (key in cat) key = `${base}${n++}`;
  cat[key] = message;
  return key;
}

function walk(node, visit, parent = null) {
  if (!node || typeof node.type !== "string") return;
  visit(node, parent);
  for (const key of Object.keys(node)) {
    if (key === "loc" || key.endsWith("Comments")) continue;
    const child = node[key];
    if (Array.isArray(child)) {
      for (const c of child) if (c && typeof c.type === "string") walk(c, visit, node);
    } else if (child && typeof child.type === "string") walk(child, visit, node);
  }
}

const FUNCTION_TYPES = new Set([
  "FunctionDeclaration",
  "FunctionExpression",
  "ArrowFunctionExpression",
  "ObjectMethod",
  "ClassMethod",
]);

/** Literals used in comparisons or as keys must never become catalog keys. */
function sentinelsIn(ast) {
  const out = new Set();
  const add = (n) => n?.type === "StringLiteral" && out.add(n.value);
  walk(ast, (node) => {
    if (node.type === "BinaryExpression" && /^([=!]==?|in)$/.test(node.operator)) {
      add(node.left);
      add(node.right);
    }
    if (node.type === "SwitchCase") add(node.test);
    if (node.type === "ObjectProperty" && node.computed) add(node.key);
    if (node.type === "MemberExpression" && node.computed) add(node.property);
  });
  return out;
}

function listFiles() {
  const out = [];
  const walkDir = (dir) => {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, e.name);
      if (e.isDirectory()) {
        if (["node_modules", "dist", "e2e", "locales", "__tests__"].includes(e.name)) continue;
        walkDir(full);
      } else if (/\.tsx?$/.test(e.name) && !/\.test\.tsx?$/.test(e.name)) out.push(full);
    }
  };
  for (const t of TARGETS) {
    const abs = path.isAbsolute(t) ? t : path.join(ROOT, t);
    if (!fs.existsSync(abs)) continue;
    if (fs.statSync(abs).isDirectory()) walkDir(abs);
    else out.push(abs);
  }
  return out;
}

const report = {
  files: 0,
  properties: 0,
  renamedTypeMembers: 0,
  skippedSentinel: 0,
  skippedNamedType: 0,
};
const namedTypeSkips = [];

/**
 * The named type this annotation refers to, or null when the shape is inline.
 * `Record<K, {inline}>` and `{inline}[]` count as inline: the members are written
 * right there, so nothing else can be feeding them.
 */
function describeAnnotation(node) {
  if (!node) return null;
  switch (node.type) {
    case "TSTypeReference":
      if (node.typeName?.name === "Record" || node.typeName?.name === "Array") {
        const args = node.typeParameters?.params ?? [];
        return describeAnnotation(args[args.length - 1]);
      }
      return node.typeName?.name ?? "unknown";
    case "TSArrayType":
      return describeAnnotation(node.elementType);
    case "TSTypeLiteral":
    case "TSTypeOperator":
      return null;
    default:
      return null;
  }
}

/** Rewrite one module-scope display property to hold its catalog key. */
function collectProperty(node, { ns, sentinels, edits, renamed }) {
  const name = node.key?.name ?? node.key?.value;
  if (!DISPLAY_KEYS.has(String(name))) return;
  if (node.value?.type !== "StringLiteral" || !looksLikeCopy(node.value.value)) return;
  if (sentinels.has(node.value.value)) {
    report.skippedSentinel += 1;
    return;
  }
  const key = `${ns}:${keyFor(ns, node.value.value)}`;
  // Rename the property so a consumer still reading `.label` fails to compile
  // rather than rendering the raw key to a user.
  edits.push({ start: node.start, end: node.end, text: `${name}Key: "${key}"` });
  renamed.add(String(name));
  report.properties += 1;
}

function transform(file) {
  const original = fs.readFileSync(file, "utf8");
  const rel = path.relative(ROOT, file);
  if (FIXTURE_FILE.test(rel.replace(/\\/g, "/"))) return;
  const ns = namespaceFor(rel);
  let ast;
  try {
    ast = parseSync(original, {
      filename: file,
      babelrc: false,
      configFile: false,
      sourceType: "module",
      parserOpts: { plugins: ["typescript", "jsx", "topLevelAwait"] },
    });
  } catch {
    return;
  }
  const sentinels = sentinelsIn(ast);
  const edits = [];
  const renamed = new Set();

  // Depth-tracked walk: a property inside any function is NOT module scope, and
  // externalize-strings.mjs already handles those (it can safely emit `t()`
  // there, because the call happens per render rather than at import).
  const ancestors = [];
  /** The named type annotating this property's declaration, if any. */
  const annotatingType = () => {
    const declarator = ancestors.find(
      (a) => a.type === "VariableDeclarator" && a.id?.typeAnnotation,
    );
    if (!declarator) return null;
    return describeAnnotation(declarator.id.typeAnnotation.typeAnnotation);
  };

  const considerProperty = (node) => {
    if (ancestors.some((a) => FUNCTION_TYPES.has(a.type))) return;
    const named = annotatingType();
    if (named && !FORCE_NAMED) {
      report.skippedNamedType += 1;
      namedTypeSkips.push(`${rel}\t${named}`);
      return;
    }
    collectProperty(node, { ns, sentinels, edits, renamed });
  };

  const visit = (node) => {
    if (node.type === "ObjectProperty") considerProperty(node);
    ancestors.push(node);
    for (const key of Object.keys(node)) {
      if (key === "loc" || key.endsWith("Comments")) continue;
      const child = node[key];
      if (Array.isArray(child)) {
        for (const c of child) if (c && typeof c.type === "string") visit(c);
      } else if (child && typeof child.type === "string") visit(child);
    }
    ancestors.pop();
  };
  visit(ast);

  if (!edits.length) return;

  // Keep the file self-consistent: rename matching members on local type aliases
  // and interfaces so the declaration still describes the literal.
  walk(ast, (node) => {
    if (node.type !== "TSPropertySignature") return;
    const name = node.key?.name;
    if (!renamed.has(String(name))) return;
    edits.push({ start: node.key.start, end: node.key.end, text: `${name}Key` });
    report.renamedTypeMembers += 1;
  });

  edits.sort((a, b) => b.start - a.start);
  let out = original;
  let lastStart = Infinity;
  for (const e of edits) {
    if (e.end > lastStart) continue;
    out = out.slice(0, e.start) + e.text + out.slice(e.end);
    lastStart = e.start;
  }
  report.files += 1;
  if (WRITE) fs.writeFileSync(file, out);
}

listFiles().forEach(transform);

if (WRITE) {
  for (const [ns, entries] of Object.entries(catalogs)) {
    const sorted = Object.fromEntries(
      Object.entries(entries).sort(([a], [b]) => a.localeCompare(b)),
    );
    fs.writeFileSync(path.join(EN, `${ns}.json`), JSON.stringify(sorted, null, 2) + "\n");
  }
}
if (process.env.DUMP_NAMED_TYPE_SKIPS) {
  for (const line of [...new Set(namedTypeSkips)].sort()) console.error(`NAMED_TYPE\t${line}`);
}
console.log(JSON.stringify({ ...report, wrote: WRITE }, null, 2));
