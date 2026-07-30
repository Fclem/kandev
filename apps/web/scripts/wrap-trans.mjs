#!/usr/bin/env node
/**
 * Wrap mixed-content sentences in react-i18next's <Trans>.
 *
 * `externalize-strings.mjs` deliberately declines JSX text that is interleaved
 * with expressions or inline markup, because splitting such a sentence into
 * per-piece keys hard-codes English word order. This tool handles those: it
 * wraps the whole sentence in a single <Trans> so translators get one coherent
 * message.
 *
 * Message format (verified empirically against react-i18next, not assumed):
 *   <Trans i18nKey="k"><code>x</code> middle <code>y</code> tail</Trans>
 *     => "<0>x</0> middle <2>y</2> tail"
 * Tag indices count EVERY significant child (text nodes included) and the tag
 * body is the element's inner text. `values` is emitted whenever the message
 * interpolates, since nested Trans does not always infer them.
 *
 * Usage: node scripts/wrap-trans.mjs [--write] <dir|file> [...]
 */
import { parseSync } from "@babel/core";
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const EN = path.join(ROOT, "src", "locales", "en");
const WRITE = process.argv.includes("--write");
const TARGETS = process.argv.slice(2).filter((a) => !a.startsWith("--"));
if (!TARGETS.length) {
  console.error("usage: wrap-trans.mjs [--write] <dir|file> [...]");
  process.exit(2);
}

/**
 * Units, version prefixes, and keyboard glyphs that appear as sentence
 * fragments. Not copy: translating "ms)" or "esc" would be wrong, not helpful.
 */
const UNIT_OR_GLYPH =
  /^(v|ms\)?|s|m|h|d|K|B|KB|MB|GB|TB|esc|Page|of|for|·[\s·v]*|[+\-·|/(),.:]+|\+[A-Z]\)?)$/;

const KEEP_LITERAL =
  /^(Kandev|GitHub|GitLab|Jira|Linear|Slack|Sentry|Azure DevOps|Docker|SSH|ACP|MCP|PR|CI|AI|API|JSON|YAML|LSP|TLS|SQL|URL|ID)$/;

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

const parse = (code, filename) =>
  parseSync(code, {
    filename,
    babelrc: false,
    configFile: false,
    sourceType: "module",
    parserOpts: { plugins: ["typescript", "jsx", "topLevelAwait"] },
  });

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

/** React drops whitespace-only JSXText spanning lines; mirror that for indexing. */
const significant = (children) =>
  children.filter((c) => {
    if (c.type !== "JSXText") return true;
    return !(/^\s*$/.test(c.value) && c.value.includes("\n"));
  });

function varNameFor(expr, fallbackIndex) {
  if (expr.type === "Identifier") return expr.name;
  if (expr.type === "MemberExpression" && expr.property.type === "Identifier")
    return expr.property.name;
  return `value${fallbackIndex}`;
}

/** Flatten an element's descendant text — its tag body in the message. */
function innerText(node) {
  let out = "";
  for (const c of node.children ?? []) {
    if (c.type === "JSXText") out += c.value.replace(/\s+/g, " ");
    else if (c.type === "JSXExpressionContainer") {
      const e = c.expression;
      if (e.type === "StringLiteral") out += e.value;
      else if (e.type !== "JSXEmptyExpression") out += `{{${varNameFor(e, 0)}}}`;
    } else if (c.type === "JSXElement") out += innerText(c);
  }
  return out.trim();
}

/** Build the message plus the interpolation values a <Trans> needs. */
function buildMessage(node, code) {
  const kids = significant(node.children);
  let msg = "";
  const values = new Map(); // name -> source expression
  kids.forEach((c, i) => {
    if (c.type === "JSXText") {
      msg += c.value.replace(/\s+/g, " ");
    } else if (c.type === "JSXExpressionContainer") {
      const e = c.expression;
      if (e.type === "JSXEmptyExpression") return;
      if (e.type === "StringLiteral") {
        msg += e.value;
        return;
      }
      const name = varNameFor(e, i);
      values.set(name, code.slice(e.start, e.end));
      msg += `{{${name}}}`;
    } else if (c.type === "JSXElement" || c.type === "JSXFragment") {
      msg += `<${i}>${innerText(c)}</${i}>`;
      // Expressions nested inside the element still need values supplied.
      walk(c, (n) => {
        if (n.type === "JSXExpressionContainer") {
          const e = n.expression;
          if (
            e.type === "Identifier" ||
            (e.type === "MemberExpression" && e.property.type === "Identifier")
          ) {
            values.set(varNameFor(e, i), code.slice(e.start, e.end));
          }
        }
      });
    }
  });
  return { message: msg.replace(/\s+/g, " ").trim(), values };
}

/**
 * English plural hacks (`n !== 1 ? "s" : ""`) cannot be translated: the suffix is
 * an English morphology detail, not data. Such sentences need a real i18next
 * plural key (`x_one`/`x_other`), so decline them here rather than freezing the
 * "s" into an interpolation value.
 */
function hasPluralHack(node, code) {
  let found = false;
  walk(node, (n) => {
    if (n.type !== "ConditionalExpression") return;
    const src = code.slice(n.start, n.end);
    if (/[!=]==?\s*1\s*\?/.test(src) && /["']s["']|["']["']/.test(src)) found = true;
  });
  return found;
}

/** Worth translating? Needs real prose, not just a variable or a lone symbol. */
function worthWrapping(message) {
  const prose = message.replace(/\{\{[^}]*\}\}/g, "").replace(/<\/?\d+>/g, " ");
  const words = prose.match(/[A-Za-z]{2,}/g) ?? [];
  // One real word is still a translatable label ("Delete", "Status"); only
  // decline when there is no word at all, or it is a unit/glyph fragment.
  if (words.length < 1) return false;
  if (UNIT_OR_GLYPH.test(prose.trim())) return false;
  if (KEEP_LITERAL.test(prose.trim())) return false;
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

const report = {
  files: 0,
  wrapped: 0,
  skippedThin: 0,
  skippedNested: 0,
  skippedPluralHack: 0,
};

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

  const candidates = [];
  walk(ast, (node) => {
    if (node.type !== "JSXElement" && node.type !== "JSXFragment") return;
    const name =
      node.type === "JSXElement" && node.openingElement.name.type === "JSXIdentifier"
        ? node.openingElement.name.name
        : "";
    if (name === "Trans") return; // already wrapped
    const kids = significant(node.children);
    if (kids.length < 2) return;
    const hasText = kids.some((c) => c.type === "JSXText" && c.value.trim());
    const hasDynamic = kids.some(
      (c) =>
        (c.type === "JSXExpressionContainer" && c.expression.type !== "JSXEmptyExpression") ||
        c.type === "JSXElement" ||
        c.type === "JSXFragment",
    );
    if (!hasText || !hasDynamic) return;
    // Skip when a descendant is itself a candidate — wrap the innermost only, so
    // nested Trans elements never overlap.
    candidates.push(node);
  });

  // Keep only innermost candidates.
  const innermost = candidates.filter(
    (n) => !candidates.some((o) => o !== n && o.start >= n.start && o.end <= n.end),
  );
  report.skippedNested += candidates.length - innermost.length;

  const edits = [];
  for (const node of innermost) {
    if (hasPluralHack(node, original)) {
      report.skippedPluralHack += 1;
      continue;
    }
    const { message, values } = buildMessage(node, original);
    if (!worthWrapping(message)) {
      report.skippedThin += 1;
      continue;
    }
    const key = `${ns}:${keyFor(ns, message)}`;
    const kids = significant(node.children);
    const first = kids[0];
    const last = kids[kids.length - 1];
    const valueAttr = values.size
      ? ` values={{ ${[...values]
          .map(([n, src]) => (n === src ? n : `${n}: ${src}`))
          .join(", ")} }}`
      : "";
    edits.push({
      start: first.start,
      end: first.start,
      text: `<Trans i18nKey="${key}"${valueAttr}>`,
    });
    edits.push({ start: last.end, end: last.end, text: `</Trans>` });
    report.wrapped += 1;
  }

  if (!edits.length) return;
  edits.sort((a, b) => b.start - a.start);
  let out = original;
  for (const e of edits) out = out.slice(0, e.start) + e.text + out.slice(e.end);

  if (!/import \{[^}]*\bTrans\b[^}]*\} from "react-i18next";/.test(out)) {
    if (/from "react-i18next"/.test(out)) {
      out = out.replace(
        /import \{([^}]*)\} from "react-i18next";/,
        (m, names) => `import { Trans,${names}} from "react-i18next";`,
      );
    } else {
      // Must land BELOW any leading "use client" directive — prepending above it
      // demotes the directive to a stray expression and breaks the module.
      const lines = out.split("\n");
      const at = /^\s*["']use (client|strict)["'];?\s*$/.test(lines[0] ?? "") ? 1 : 0;
      lines.splice(at, 0, `import { Trans } from "react-i18next";`);
      out = lines.join("\n");
    }
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
console.log(JSON.stringify({ ...report, wrote: WRITE }, null, 2));
