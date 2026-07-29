#!/usr/bin/env node
/**
 * Externalize hardcoded user-facing strings to i18next keys.
 *
 * Handles JSX text, display attributes (placeholder/aria-label/title/alt), and
 * toast argument strings. Applies surgical range edits to the ORIGINAL source so
 * diffs stay scoped, emits/merges `src/locales/en/<namespace>.json`, and inserts
 * `const { t } = useTranslation()` (or a module-level `t` import) where needed.
 *
 * SAFETY: a literal is skipped when it is ALSO used as a logic sentinel anywhere
 * in the same file (compared with ==/===/!==/switch-case, or used as an object
 * key / Map key). Translating those silently breaks behavior — see
 * docs/plans/i18n/FOLLOWUPS.md for the type-to-confirm incident this prevents.
 *
 * Usage: node scripts/externalize-strings.mjs [--write] <dir|file> [...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const WRITE = process.argv.includes("--write");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
if (!TARGETS.length) {
  console.error("usage: externalize-strings.mjs [--write] <dir|file> [...]");
  process.exit(2);
}

const DISPLAY_ATTRS = new Set(["placeholder", "aria-label", "aria-description", "title", "alt"]);
const TOAST_CALLEES = /^(toast|toast\.(success|error|info|warning|message|loading))$/;

/** Brand/proper nouns and acronyms that must stay untranslated. */
const KEEP_LITERAL =
  /^(Kandev|GitHub|GitLab|Jira|Linear|Slack|Sentry|Azure DevOps|Docker|SSH|ACP|MCP|PR|CI|AI|API|JSON|YAML|LSP|TLS|SQL|URL|ID|PostgreSQL|SQLite|Claude|Codex|OpenCode|Copilot|Amp)$/;

/**
 * Path prefix -> catalog namespace. Ordered: the first matching prefix wins, so
 * more specific paths must come first.
 */
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

function namespaceFor(rel) {
  const p = rel.replace(/\\/g, "/");
  return NAMESPACE_RULES.find(([re]) => re.test(p))?.[1] ?? "common";
}

function keyFromText(text) {
  const words = text
    .replace(/\{\{[^}]*\}\}/g, " ")
    .replace(/<\/?\d+>/g, " ")
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

/** Heuristic: is this literal display copy rather than code/data? */
function looksLikeCopy(raw) {
  const s = raw.trim();
  if (s.length < 2) return false;
  if (!/[A-Za-z]/.test(s)) return false;
  if (KEEP_LITERAL.test(s)) return false;
  if (/^https?:\/\//.test(s) || s.startsWith("/") || s.startsWith("~/")) return false; // urls/paths
  if (/^[a-z0-9]+([-_][a-z0-9]+)+$/.test(s)) return false; // kebab/snake identifiers
  if (/^[a-z]+([A-Z][a-z0-9]*)+$/.test(s)) return false; // camelCase identifier
  if (/^[A-Z][A-Z0-9_]*$/.test(s)) return false; // SCREAMING_CASE sentinel
  if (/[{}<>$]/.test(s) && !/\s/.test(s)) return false; // template/code fragment
  if (/^\.?[\w-]+\.(tsx?|jsx?|json|md|ya?ml|css)$/.test(s)) return false; // filenames
  if (/^[\w.-]+@[\w.-]+$/.test(s)) return false; // emails
  if (!/[A-Za-z]{2}/.test(s)) return false;
  return true;
}

function parse(code, filename) {
  return parseSync(code, {
    filename,
    babelrc: false,
    configFile: false,
    sourceType: "module",
    parserOpts: { plugins: ["typescript", "jsx", "topLevelAwait"] },
  });
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

/**
 * Every string in this file that participates in logic: comparisons, switch
 * cases, object/member keys, or array-membership checks. These must never be
 * translated.
 */
function sentinelsIn(ast) {
  const out = new Set();
  const add = (n) => {
    if (n?.type === "StringLiteral") out.add(n.value);
  };
  walk(ast, (node) => {
    if (node.type === "BinaryExpression" && /^([=!]==?|in)$/.test(node.operator)) {
      add(node.left);
      add(node.right);
    }
    if (node.type === "SwitchCase") add(node.test);
    if (node.type === "ObjectProperty" && node.computed) add(node.key);
    if (node.type === "MemberExpression" && node.computed) add(node.property);
    // `["a","b"].includes(x)` / `new Set(["a"])` style membership tables
    if (
      node.type === "ArrayExpression" &&
      node.elements.every((e) => e?.type === "StringLiteral")
    ) {
      for (const e of node.elements) add(e);
    }
    // Object keys are frequently enum maps keyed by sentinel.
    if (node.type === "ObjectProperty" && node.key?.type === "StringLiteral") add(node.key);
  });
  return out;
}

/**
 * True when this text node is only part of a sentence — i.e. its parent element
 * also contains expression containers or inline elements. Splitting such a
 * sentence into per-fragment keys bakes in English word order and cannot be
 * translated correctly, so the codemod declines and reports it instead.
 */
function isSentenceFragment(node, parent) {
  if (!parent || !Array.isArray(parent.children)) return false;
  const siblings = parent.children.filter((c) => {
    if (c === node) return false;
    if (c.type === "JSXText") return c.value.trim().length > 0;
    if (c.type === "JSXExpressionContainer")
      return (
        c.expression?.type !== "JSXEmptyExpression" &&
        !(c.expression?.type === "StringLiteral" && !c.expression.value.trim())
      );
    return c.type === "JSXElement" || c.type === "JSXFragment";
  });
  return siblings.length > 0;
}

const FUNCTION_TYPES = new Set([
  "FunctionDeclaration",
  "FunctionExpression",
  "ArrowFunctionExpression",
  "ObjectMethod",
  "ClassMethod",
]);
function fnName(node, parent) {
  if (node.id?.name) return node.id.name;
  if (parent?.type === "VariableDeclarator" && parent.id?.type === "Identifier")
    return parent.id.name;
  if (parent?.type === "ObjectProperty" && parent.key?.type === "Identifier")
    return parent.key.name;
  return "";
}
const canUseHooks = (name) => /^[A-Z]/.test(name) || /^use[A-Z]/.test(name);

// ---------------------------------------------------------------- collect files
function listFiles() {
  const out = [];
  const walkDir = (dir) => {
    for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, e.name);
      if (e.isDirectory()) {
        if (["node_modules", "dist", "e2e", "locales", "__tests__"].includes(e.name)) continue;
        walkDir(full);
      } else if (/\.tsx$/.test(e.name) && !/\.test\.tsx$/.test(e.name)) out.push(full);
    }
  };
  for (const t of TARGETS) {
    const abs = path.isAbsolute(t) ? t : path.join(ROOT, t);
    if (!fs.existsSync(abs)) continue;
    if (fs.statSync(abs).isDirectory()) walkDir(abs);
    else if (/\.tsx$/.test(abs)) out.push(abs);
  }
  return out;
}

// ------------------------------------------------------------------- catalogs
const catalogs = {};
function loadCatalog(ns) {
  if (catalogs[ns]) return catalogs[ns];
  const file = path.join(EN, `${ns}.json`);
  catalogs[ns] = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : {};
  return catalogs[ns];
}
/** Reuse an existing key when the same English text is already in the catalog. */
function keyFor(ns, message) {
  const cat = loadCatalog(ns);
  for (const [k, v] of Object.entries(cat)) if (v === message) return k;
  const common = loadCatalog("common");
  for (const [k, v] of Object.entries(common)) if (v === message) return `common:${k}`;
  const base = keyFromText(message);
  let key = base;
  let n = 2;
  while (key in cat) key = `${base}${n++}`;
  cat[key] = message;
  return key;
}
const qualify = (ns, key) => (key.includes(":") ? key : `${ns}:${key}`);

// ------------------------------------------------------------------- transform
const report = {
  files: 0,
  jsxText: 0,
  attrs: 0,
  toasts: 0,
  skippedSentinel: 0,
  skippedFragment: 0,
  bindings: 0,
};

/** JSX text child -> {t("ns:key")}, unless it is a sentinel or a fragment. */
function handleJsxText(node, parent, { ns, sentinels, edits, noteHost }) {
  if (node.type !== "JSXText") return;
  const raw = node.value;
  const trimmed = raw.trim();
  if (!trimmed || !looksLikeCopy(trimmed)) return;
  if (sentinels.has(trimmed)) {
    report.skippedSentinel += 1;
    return;
  }
  if (isSentenceFragment(node, parent)) {
    // Part of a sentence interleaved with expressions or inline markup.
    // Externalizing each piece separately would hard-code English word order;
    // these need one <Trans> for the whole sentence, so decline and report.
    report.skippedFragment += 1;
    return;
  }
  const key = qualify(ns, keyFor(ns, trimmed.replace(/\s+/g, " ")));
  const lead = raw.slice(0, raw.indexOf(trimmed));
  const tail = raw.slice(raw.indexOf(trimmed) + trimmed.length);
  edits.push({ start: node.start, end: node.end, text: `${lead}{t("${key}")}${tail}` });
  report.jsxText += 1;
  noteHost();
}

/** placeholder / aria-label / title / alt with a literal value. */
function handleDisplayAttribute(node, { ns, sentinels, edits, noteHost }) {
  if (node.type !== "JSXAttribute" || !DISPLAY_ATTRS.has(String(node.name?.name))) return;
  const v = node.value;
  if (v?.type !== "StringLiteral" || !looksLikeCopy(v.value)) return;
  if (sentinels.has(v.value)) {
    report.skippedSentinel += 1;
    return;
  }
  const key = qualify(ns, keyFor(ns, v.value));
  edits.push({ start: v.start, end: v.end, text: `{t("${key}")}` });
  report.attrs += 1;
  noteHost();
}

/** toast("...") / toast.error("...") first argument. */
function handleToastCall(node, { ns, sentinels, edits, noteHost }) {
  if (node.type !== "CallExpression") return;
  const callee = node.callee;
  let name = "";
  if (callee.type === "Identifier") name = callee.name;
  else if (
    callee.type === "MemberExpression" &&
    callee.object.type === "Identifier" &&
    callee.property.type === "Identifier"
  )
    name = `${callee.object.name}.${callee.property.name}`;
  if (!TOAST_CALLEES.test(name)) return;
  const arg = node.arguments[0];
  if (arg?.type !== "StringLiteral" || !looksLikeCopy(arg.value)) return;
  if (sentinels.has(arg.value)) {
    report.skippedSentinel += 1;
    return;
  }
  const key = qualify(ns, keyFor(ns, arg.value));
  edits.push({ start: arg.start, end: arg.end, text: `t("${key}")` });
  report.toasts += 1;
  noteHost();
}

/** Insert a statement after the last COMPLETE import (multi-line aware). */
function addImport(src, statement) {
  if (src.includes(statement)) return src;
  const lines = src.split("\n");
  let last = -1;
  let open = false;
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i];
    if (open) {
      if (/^\}\s*from/.test(l)) {
        open = false;
        last = i;
      }
      continue;
    }
    if (/^import\s/.test(l)) {
      if (/;\s*$/.test(l)) last = i;
      else open = true;
    }
  }
  if (last === -1) return statement + "\n" + src;
  lines.splice(last + 1, 0, statement);
  return lines.join("\n");
}

/** Ensure `useTranslation` and/or the module-level `t` are imported. */
function wireImports(src, { needModuleT, addedBindings }) {
  let out = src;
  if (addedBindings || /\buseTranslation\(/.test(out)) {
    if (!/from "react-i18next"/.test(out)) {
      out = addImport(out, `import { useTranslation } from "react-i18next";`);
    } else if (!/\buseTranslation\b[^}]*\} from "react-i18next"/.test(out)) {
      out = out.replace(
        /import \{([^}]*)\} from "react-i18next";/,
        (m, names) =>
          `import {${names.replace(/\s*$/, "")}, useTranslation } from "react-i18next";`,
      );
    }
  }
  if (needModuleT && !/import \{[^}]*\bt\b[^}]*\} from "@\/lib\/i18n"/.test(out)) {
    out = addImport(out, `import { t } from "@/lib/i18n";`);
  }
  return out;
}

function transform(file) {
  const original = fs.readFileSync(file, "utf8");
  const rel = path.relative(ROOT, file);
  const ns = namespaceFor(rel);
  let ast;
  try {
    ast = parse(original, file);
  } catch {
    return;
  }
  const sentinels = sentinelsIn(ast);
  const edits = [];
  const needT = []; // function nodes that must gain a hook binding
  let needModuleT = false;
  const stack = [];

  const noteHost = () => {
    const host = [...stack].reverse().find((f) => canUseHooks(f.name));
    if (host) {
      if (!needT.includes(host.node)) needT.push(host.node);
    } else {
      needModuleT = true;
    }
  };

  const ctx = { ns, sentinels, edits, noteHost };
  const visit = (node, parent) => {
    const isFn = FUNCTION_TYPES.has(node.type);
    if (isFn) stack.push({ node, name: fnName(node, parent) });

    handleJsxText(node, parent, ctx);
    handleDisplayAttribute(node, ctx);
    handleToastCall(node, ctx);

    for (const key of Object.keys(node)) {
      if (key === "loc" || key.endsWith("Comments")) continue;
      const child = node[key];
      if (Array.isArray(child)) {
        for (const c of child) if (c && typeof c.type === "string") visit(c, node);
      } else if (child && typeof child.type === "string") visit(child, node);
    }
    if (isFn) stack.pop();
  };
  visit(ast, null);

  if (!edits.length) return;

  // Hook bindings are folded into the SAME edit list: applying them separately
  // would use AST offsets already invalidated by the text edits above.
  const alreadyBound = (fn) => {
    let found = false;
    walk(fn.body ?? fn, (n) => {
      if (
        n.type === "VariableDeclarator" &&
        n.init?.type === "CallExpression" &&
        n.init.callee.type === "Identifier" &&
        n.init.callee.name === "useTranslation"
      )
        found = true;
    });
    return found;
  };
  let bindingCount = 0;
  for (const fn of needT) {
    if (alreadyBound(fn)) continue;
    const body = fn.body;
    if (body?.type !== "BlockStatement") continue;
    edits.push({
      start: body.start + 1,
      end: body.start + 1,
      text: `\n  const { t } = useTranslation();`,
    });
    bindingCount += 1;
  }

  // Apply from the end so earlier offsets stay valid; a zero-width insert at the
  // same position as a replacement must land after it, hence the length tiebreak.
  edits.sort((a, b) => b.start - a.start || a.end - a.start - (b.end - b.start));
  let out = original;
  let lastStart = Infinity;
  for (const e of edits) {
    if (e.end > lastStart) continue;
    out = out.slice(0, e.start) + e.text + out.slice(e.end);
    lastStart = e.start;
  }
  report.bindings += bindingCount;

  out = wireImports(out, { needModuleT, addedBindings: bindingCount });

  if (out !== original) {
    report.files += 1;
    if (WRITE) fs.writeFileSync(file, out);
  }
}

listFiles().forEach(transform);

if (WRITE) {
  fs.mkdirSync(EN, { recursive: true });
  for (const [ns, entries] of Object.entries(catalogs)) {
    const sorted = Object.fromEntries(
      Object.entries(entries).sort(([a], [b]) => a.localeCompare(b)),
    );
    fs.writeFileSync(path.join(EN, `${ns}.json`), JSON.stringify(sorted, null, 2) + "\n");
  }
}
console.log(JSON.stringify({ ...report, wrote: WRITE }, null, 2));
