import { describe, expect, it } from "vitest";

import {
  findElementEnd,
  findTagEnd,
  hasTextualChild,
  scanSource,
} from "./scan-icon-button-names.mjs";

const names = (src) => scanSource(src).map((f) => `${f.tag}:${f.line}`);

describe("findTagEnd", () => {
  it("stops at the closing angle bracket of the opening tag", () => {
    const src = `<Button size="icon">x</Button>`;
    expect(findTagEnd(src, 0)).toBe(src.indexOf(">"));
  });

  it("ignores angle brackets inside strings and expressions", () => {
    const src = `<Button className={cn("a>b")} onClick={() => go()} size="icon">x</Button>`;
    // The first unnested `>` is the one closing the tag, after `size="icon"`.
    expect(src[findTagEnd(src, 0)]).toBe(">");
    expect(src.slice(0, findTagEnd(src, 0))).toContain('size="icon"');
  });

  it("returns -1 when the tag never closes", () => {
    expect(findTagEnd(`<Button size="icon"`, 0)).toBe(-1);
  });
});

describe("findElementEnd", () => {
  it("skips past a nested element of the same name", () => {
    const src = `<Button><Button /></Button>`;
    const bodyStart = src.indexOf(">") + 1;
    expect(findElementEnd(src, "Button", bodyStart)).toBe(src.lastIndexOf("</Button>"));
  });

  it("treats a dotted component name literally rather than as a regex", () => {
    // `.` must not act as a wildcard, which would match `<MenuXTrigger>`.
    const src = `<Menu.Trigger>a</Menu.Trigger>`;
    const bodyStart = src.indexOf(">") + 1;
    expect(findElementEnd(src, "Menu.Trigger", bodyStart)).toBe(src.indexOf("</Menu.Trigger>"));
  });
});

describe("hasTextualChild", () => {
  it("counts literal text", () => {
    expect(hasTextualChild(`Delete`)).toBe(true);
  });

  it("counts an sr-only span", () => {
    expect(hasTextualChild(`<IconX /><span className="sr-only">Close</span>`)).toBe(true);
  });

  it("does not count icon elements alone", () => {
    expect(hasTextualChild(`<IconTrash className="h-4 w-4" />`)).toBe(false);
  });
});

describe("scanSource", () => {
  it("reports a self-closing icon button with no name", () => {
    expect(names(`<IconButton size="icon" />`)).toEqual(["IconButton:1"]);
  });

  it("reports an icon button whose only child is an icon", () => {
    expect(names(`<Button size="icon"><IconTrash /></Button>`)).toEqual(["Button:1"]);
  });

  it("does not report a button carrying aria-label, title, or sr-only text", () => {
    expect(scanSource(`<Button size="icon" aria-label="Delete"><IconX /></Button>`)).toEqual([]);
    expect(scanSource(`<Button size="icon" title="Delete"><IconX /></Button>`)).toEqual([]);
    expect(
      scanSource(`<Button size="icon"><IconX /><span className="sr-only">Delete</span></Button>`),
    ).toEqual([]);
  });

  it("still reports a button whose child is a bare expression", () => {
    // Known false positive: `{page}` renders a page number, but the scanner
    // cannot evaluate expressions. Documented in the PR; callers must eyeball.
    expect(names(`<Button size="icon">{page}</Button>`)).toEqual(["Button:1"]);
  });

  it("reports an asChild button and flags it, since the name may sit on the child", () => {
    const [finding] = scanSource(`<Button asChild size="icon"><Link href="/" /></Button>`);
    expect(finding.asChild).toBe(true);
  });

  it("covers the icon-xs, icon-sm, and icon-lg variants", () => {
    const src = [
      `<Button size="icon-xs"><IconX /></Button>`,
      `<Button size="icon-sm"><IconX /></Button>`,
      `<Button size="icon-lg"><IconX /></Button>`,
    ].join("\n");
    expect(names(src)).toEqual(["Button:1", "Button:2", "Button:3"]);
  });

  it("reports only the inner button when the same tag is nested", () => {
    const src = `<Button>\n  <Button size="icon"><IconX /></Button>\n</Button>`;
    expect(names(src)).toEqual(["Button:2"]);
  });

  it("reports the correct 1-based line for a button further down the file", () => {
    const src = `const a = 1;\n\n<Button size="icon"><IconX /></Button>`;
    expect(scanSource(src)[0].line).toBe(3);
  });
});
