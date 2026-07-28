#!/usr/bin/env node
// Heuristic scan for icon-only buttons that have no accessible name.
//
// Finds JSX elements using the `icon*` Button size variants and reports the
// ones with no `aria-label` / `aria-labelledby` / `title` attribute and no
// textual (or `sr-only`) child. Output is a starting point for review, not a
// verdict: `asChild` wrappers and custom components can supply a name
// indirectly, so every hit still needs eyeballing.
//
// Usage: node scripts/scan-icon-button-names.mjs [--json]

import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const ROOTS = ["app", "components", "hooks", "lib"];
const CWD = process.cwd();

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === "generated") continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) walk(full, out);
    else if (full.endsWith(".tsx")) out.push(full);
  }
  return out;
}

// Walk forward from the `<` of an opening tag to its closing `>`, skipping
// over strings, template literals, and balanced braces (JSX expressions).
function findTagEnd(src, start) {
  let i = start;
  let depth = 0;
  while (i < src.length) {
    const ch = src[i];
    if (ch === '"' || ch === "'" || ch === "`") {
      const quote = ch;
      i += 1;
      while (i < src.length && src[i] !== quote) {
        if (src[i] === "\\") i += 1;
        i += 1;
      }
    } else if (ch === "{") depth += 1;
    else if (ch === "}") depth -= 1;
    else if (ch === ">" && depth === 0) return i;
    i += 1;
  }
  return -1;
}

// Find the end of the element body for a non-self-closing tag, tracking
// nesting of same-named elements.
function findElementEnd(src, tagName, bodyStart) {
  const open = new RegExp(`<${tagName}[\\s/>]`, "g");
  const close = new RegExp(`</${tagName}\\s*>`, "g");
  let depth = 1;
  let i = bodyStart;
  while (i < src.length) {
    open.lastIndex = i;
    close.lastIndex = i;
    const o = open.exec(src);
    const c = close.exec(src);
    if (!c) return src.length;
    if (o && o.index < c.index) {
      depth += 1;
      i = o.index + 1;
      continue;
    }
    depth -= 1;
    if (depth === 0) return c.index;
    i = c.index + 1;
  }
  return src.length;
}

const NAME_ATTRS = /\b(aria-label|aria-labelledby|title)\s*=/;
const SPREAD = /\{\.\.\./;

// Children that count as an accessible name: literal text, or an `sr-only`
// span. Icon components (<Trash2 />) and bare expressions do not.
function hasTextualChild(body) {
  if (/\bsr-only\b/.test(body)) return true;
  const stripped = body
    .replace(/<[^>]*>/g, " ") // elements
    .replace(/\{[^{}]*\}/g, " ") // simple expressions
    .replace(/\{\{[\s\S]*?\}\}/g, " ");
  return /[A-Za-z0-9]/.test(stripped.replace(/ /g, ""));
}

const findings = [];

for (const root of ROOTS) {
  for (const file of walk(join(CWD, root))) {
    const src = readFileSync(file, "utf8");
    const sizeRe = /size="icon(?:-xs|-sm|-lg)?"/g;
    let m;
    while ((m = sizeRe.exec(src)) !== null) {
      // Scan back to the `<` that opens this element.
      const tagStart = src.lastIndexOf("<", m.index);
      if (tagStart === -1) continue;
      const tagName = /^<([A-Za-z][\w.]*)/.exec(src.slice(tagStart))?.[1];
      if (!tagName) continue;
      const tagEnd = findTagEnd(src, tagStart);
      if (tagEnd === -1) continue;
      const openTag = src.slice(tagStart, tagEnd + 1);
      const selfClosing = openTag.trimEnd().endsWith("/>");

      const body = selfClosing
        ? ""
        : src.slice(tagEnd + 1, findElementEnd(src, tagName, tagEnd + 1));

      const named = NAME_ATTRS.test(openTag);
      const spread = SPREAD.test(openTag);
      const asChild = /\basChild\b/.test(openTag);
      const textChild = !selfClosing && hasTextualChild(body);

      if (named || textChild) continue;

      findings.push({
        file: relative(CWD, file),
        line: src.slice(0, tagStart).split("\n").length,
        tag: tagName,
        size: m[0],
        asChild,
        spread,
        snippet: openTag.replace(/\s+/g, " ").slice(0, 120),
      });
    }
  }
}

if (process.argv.includes("--json")) {
  console.log(JSON.stringify(findings, null, 2));
} else {
  for (const f of findings) {
    const flags = [f.asChild && "asChild", f.spread && "spread"].filter(Boolean).join(",");
    console.log(`${f.file}:${f.line}\t<${f.tag}> ${f.size}${flags ? ` [${flags}]` : ""}`);
  }
  console.log(`\n${findings.length} icon button(s) with no obvious accessible name`);
}
