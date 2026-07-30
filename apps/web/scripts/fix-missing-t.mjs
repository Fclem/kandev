#!/usr/bin/env node
/**
 * Bind `t` wherever a rewrite introduced a call to it but nothing provides it.
 *
 * Driven by `tsc`'s "Cannot find name 't'" errors, so it only touches positions
 * the compiler has already proven are broken. For each one it walks out to the
 * enclosing function and picks the binding that is legal there:
 *
 *   - a component or hook (`Foo`, `useBar`) gets `const { t } = useTranslation()`
 *   - anything else gets the module-level `t` from `@/lib/i18n`, which resolves at
 *     CALL time and so still follows a locale switch
 *
 * Usage:
 *   pnpm exec tsc --noEmit 2>&1 | node scripts/fix-missing-t.mjs [--write]
 */
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(import.meta.dirname, "..");
const WRITE = process.argv.includes("--write");

const targets = new Map(); // file -> Set of line numbers
for (const line of fs.readFileSync(0, "utf8").split("\n")) {
  const m = /^(.+?)\((\d+),(\d+)\): error TS2304: Cannot find name 't'\.$/.exec(line.trim());
  if (!m) continue;
  const abs = path.isAbsolute(m[1]) ? m[1] : path.join(ROOT, m[1]);
  if (!fs.existsSync(abs)) continue;
  if (!targets.has(abs)) targets.set(abs, new Set());
  targets.get(abs).add(+m[2]);
}

const FN_START =
  /^(\s*)(?:export\s+)?(?:async\s+)?(?:function\s+([A-Za-z_$][\w$]*)|(?:const|let)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*(?::[^=]+)?=>)/;

const canUseHooks = (name) => !!name && (/^[A-Z]/.test(name) || /^use[A-Z]/.test(name));

/**
 * The line whose trailing `{` opens the function BODY — not the one that opens a
 * destructured parameter or its inline type. Tracks paren depth from the
 * signature, so the body brace is the first one seen after the params close.
 */
function findBodyLine(lines, fnLine) {
  let parens = 0;
  let seenParams = false;
  for (let i = fnLine; i < Math.min(fnLine + 60, lines.length); i++) {
    for (const ch of lines[i]) {
      if (ch === "(") {
        parens++;
        seenParams = true;
      } else if (ch === ")") parens--;
    }
    if (seenParams && parens === 0 && /\)\s*(:[^=]*)?\s*(=>\s*)?\{\s*$/.test(lines[i])) return i;
  }
  return -1;
}

/** Insert a statement after the last complete import (multi-line aware). */
function addImport(lines, statement) {
  if (lines.some((l) => l.includes(statement))) return lines;
  let last = -1;
  let open = false;
  for (let i = 0; i < lines.length; i++) {
    if (open) {
      if (/^\}\s*from/.test(lines[i])) {
        open = false;
        last = i;
      }
      continue;
    }
    if (/^import\s/.test(lines[i])) {
      if (/;\s*$/.test(lines[i])) last = i;
      else open = true;
    }
  }
  if (last === -1) {
    let at = 0;
    while (at < lines.length && /^\s*$/.test(lines[at])) at++;
    if (/^\s*["']use (client|strict)["'];?\s*$/.test(lines[at] ?? "")) at++;
    lines.splice(at, 0, statement);
    return lines;
  }
  lines.splice(last + 1, 0, statement);
  return lines;
}

const report = { hooks: 0, moduleT: 0, files: 0, unresolved: [] };

for (const [abs, lineNos] of targets) {
  let lines = fs.readFileSync(abs, "utf8").split("\n");
  const rel = path.relative(ROOT, abs);
  // Highest line first so inserting a binding cannot shift a later target.
  const sorted = [...lineNos].sort((a, b) => b - a);
  let needsHookImport = false;
  let needsModuleT = false;
  const handled = new Set();

  for (const lineNo of sorted) {
    let fnLine = -1;
    let name = null;
    for (let i = lineNo - 1; i >= 0; i--) {
      const m = FN_START.exec(lines[i]);
      if (m) {
        fnLine = i;
        name = m[2] ?? m[3];
        break;
      }
    }
    if (fnLine === -1) {
      report.unresolved.push(`${rel}:${lineNo}`);
      continue;
    }
    if (handled.has(fnLine)) continue;
    handled.add(fnLine);

    if (!canUseHooks(name)) {
      // A plain helper cannot hold a hook; the module-level `t` is call-time.
      needsModuleT = true;
      continue;
    }
    // Find the brace that opens the BODY, not the one that opens a destructured
    // parameter or its inline type. Track paren depth from the function start:
    // the body brace is the first `{` at the end of a line once the parameter
    // list has closed.
    const bodyLine = findBodyLine(lines, fnLine);
    if (bodyLine === -1) {
      report.unresolved.push(`${rel}:${lineNo} (${name})`);
      continue;
    }
    const indent = (/^(\s*)/.exec(lines[fnLine]) ?? ["", ""])[1] + "  ";
    lines.splice(bodyLine + 1, 0, `${indent}const { t } = useTranslation();`);
    needsHookImport = true;
    report.hooks += 1;
  }

  if (needsHookImport && !lines.some((l) => l.includes('from "react-i18next"'))) {
    lines = addImport(lines, 'import { useTranslation } from "react-i18next";');
  } else if (needsHookImport) {
    const idx = lines.findIndex((l) => l.includes('from "react-i18next"'));
    if (!/\buseTranslation\b/.test(lines[idx])) {
      lines[idx] = lines[idx].replace(
        /import \{([^}]*)\}/,
        (m, names) => `import {${names.replace(/\s*$/, "")}, useTranslation }`,
      );
    }
  }
  if (
    needsModuleT &&
    !lines.some((l) => /import \{[^}]*\bt\b[^}]*\} from "@\/lib\/i18n"/.test(l))
  ) {
    lines = addImport(lines, 'import { t } from "@/lib/i18n";');
    report.moduleT += 1;
  }
  report.files += 1;
  if (WRITE) fs.writeFileSync(abs, lines.join("\n"));
}

console.log(JSON.stringify(report, null, 2));
