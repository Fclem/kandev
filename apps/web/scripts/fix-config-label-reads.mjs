#!/usr/bin/env node
/**
 * Follow-up fixer for `externalize-config-labels.mjs`.
 *
 * That codemod renames a config property (`label` -> `labelKey`) so every consumer
 * fails to compile instead of silently rendering a raw key. This script closes the
 * loop by reading those compile errors and applying the mechanical half of each fix:
 *
 *   {opt.label}                     ->  {t(opt.labelKey)}
 *   type Foo = { label: string }    ->  type Foo = { labelKey: string }
 *
 * It is driven by `tsc` output rather than by re-parsing, because tsc reports the
 * exact position of the offending property access — no guessing which `.label` in
 * a file is the one that broke.
 *
 * Anything else (assignability errors, prop pass-through, tests) is left alone and
 * reported; those need a human to decide what the copy should be.
 *
 * Usage:
 *   pnpm exec tsc --noEmit 2>&1 | node scripts/fix-config-label-reads.mjs [--write]
 */
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const WRITE = process.argv.includes("--write");

const input = fs.readFileSync(0, "utf8");
/** Properties this pass knows how to redirect at their `…Key` twin. */
const RENAMED =
  /^(label|description|tooltip|hint|heading|headline|title|subtitle|helpText|placeholder|emptyLabel|errorTitle|message|note|summary|caption)$/;

const readErrors = []; // `x.label` where the type now has `labelKey`
const typeErrors = []; // object literal vs a type declared elsewhere

for (const line of input.split("\n")) {
  const m = /^(.+?)\((\d+),(\d+)\): error (TS\d+): (.+)$/.exec(line.trim());
  if (!m) continue;
  const [, file, lineNo, col, code, message] = m;
  const abs = path.isAbsolute(file) ? file : path.join(ROOT, file);
  if (!fs.existsSync(abs)) continue;
  const prop = /Property '([^']+)' does not exist/.exec(message)?.[1];
  if ((code === "TS2339" || code === "TS2551") && prop && RENAMED.test(prop)) {
    readErrors.push({ abs, line: +lineNo, col: +col, prop });
    continue;
  }
  const literalProp = /and '([^']+)' does not exist in type '([^']+)'/.exec(message);
  const literalProp2 = /but '([^']+)' does not exist in type '([^']+)'/.exec(message);
  const hit = literalProp ?? literalProp2;
  if (code === "TS2353" || code === "TS2561") {
    if (hit && RENAMED.test(hit[1].replace(/Key$/, "")) && hit[1].endsWith("Key")) {
      typeErrors.push({ typeName: hit[2], prop: hit[1] });
    }
  }
}

/** Index of the `}` closing the `{` at `start`. */
function matchingBrace(text, start) {
  let depth = 0;
  for (let i = start; i < text.length; i++) {
    if (text[i] === "{") depth++;
    else if (text[i] === "}" && --depth === 0) return i;
  }
  return text.length - 1;
}

/** Walk left from a property access to the start of its object expression. */
function objectStart(text, dotIndex) {
  let i = dotIndex - 1;
  let depth = 0;
  while (i >= 0) {
    const c = text[i];
    if (c === ")" || c === "]") depth++;
    else if (c === "(" || c === "[") {
      if (depth === 0) break;
      depth--;
    } else if (depth === 0 && !/[\w$.?]/.test(c)) break;
    i--;
  }
  return i + 1;
}

const report = { reads: 0, typeMembers: 0, skippedNoT: [], files: new Set() };

// Group by file and apply from the bottom up so earlier offsets stay valid.
const byFile = new Map();
for (const e of readErrors) {
  if (!byFile.has(e.abs)) byFile.set(e.abs, []);
  byFile.get(e.abs).push(e);
}

for (const [abs, errs] of byFile) {
  const original = fs.readFileSync(abs, "utf8");
  const lines = original.split("\n");
  // Offset of the start of each line, so (line, col) becomes an index.
  const lineStart = [0];
  for (let i = 0; i < lines.length; i++) lineStart.push(lineStart[i] + lines[i].length + 1);

  const edits = [];
  for (const e of errs) {
    const at = lineStart[e.line - 1] + (e.col - 1);
    if (original.slice(at, at + e.prop.length) !== e.prop) continue;
    // `.label` -> `.labelKey`, and wrap the whole access in t(...)
    const dot = at - 1;
    if (original[dot] !== ".") continue;
    const start = objectStart(original, dot);
    const end = at + e.prop.length;
    const expr = original.slice(start, end);
    edits.push({ start, end, text: `t(${expr}Key)` });
  }
  if (!edits.length) continue;
  edits.sort((a, b) => b.start - a.start);
  let out = original;
  let lastStart = Infinity;
  for (const e of edits) {
    if (e.end > lastStart) continue;
    out = out.slice(0, e.start) + e.text + out.slice(e.end);
    lastStart = e.start;
    report.reads += 1;
  }
  report.files.add(path.relative(ROOT, abs));
  if (WRITE) fs.writeFileSync(abs, out);
}

// Rename members on type declarations that live in another file.
const declFiles = new Set();
const walkDir = (dir) => {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (["node_modules", "dist", ".next"].includes(entry.name)) continue;
      walkDir(full);
    } else if (/\.tsx?$/.test(entry.name)) declFiles.add(full);
  }
};
// `lib/types` and `lib/api` describe the SERVER's JSON shape. A member name
// there is a wire contract, not a local convention — renaming `label` to
// `labelKey` in an API type silently stops matching the payload. Only UI-owned
// modules are eligible.
for (const dir of ["components", "app", "hooks"]) {
  const abs = path.join(ROOT, dir);
  if (fs.existsSync(abs)) walkDir(abs);
}

const wanted = new Map();
for (const { typeName, prop } of typeErrors) {
  if (!wanted.has(typeName)) wanted.set(typeName, new Set());
  wanted.get(typeName).add(prop);
}
for (const file of declFiles) {
  let text = fs.readFileSync(file, "utf8");
  let changed = false;
  for (const [typeName, props] of wanted) {
    const decl = new RegExp(`(?:type\\s+${typeName}\\s*=\\s*\\{|interface\\s+${typeName}\\s*\\{)`);
    const m = decl.exec(text);
    if (!m) continue;
    // Bound the rename to the declaration body so unrelated members are untouched.
    const bodyStart = m.index + m[0].length - 1;
    const bodyEnd = matchingBrace(text, bodyStart);
    const body = text.slice(bodyStart, bodyEnd + 1);
    const i = bodyEnd;
    let nextBody = body;
    for (const prop of props) {
      const bare = prop.replace(/Key$/, "");
      nextBody = nextBody.replace(new RegExp(`(^|\\n)(\\s*)${bare}(\\??):`, "g"), `$1$2${prop}$3:`);
    }
    if (nextBody !== body) {
      text = text.slice(0, bodyStart) + nextBody + text.slice(i + 1);
      changed = true;
      report.typeMembers += 1;
    }
  }
  if (changed) {
    report.files.add(path.relative(ROOT, file));
    if (WRITE) fs.writeFileSync(file, text);
  }
}

console.log(
  JSON.stringify(
    {
      reads: report.reads,
      typeMembers: report.typeMembers,
      files: report.files.size,
      wrote: WRITE,
    },
    null,
    2,
  ),
);
