import { describe, expect, it } from "vitest";
import { splitMarkdownPromptMentionSegments } from "./prompt-mention-components";

const promptNames = ["daily"];

function promptValues(content: string) {
  return splitMarkdownPromptMentionSegments(content, promptNames)
    .filter((segment) => segment.kind === "prompt")
    .map((segment) => segment.value);
}

describe("splitMarkdownPromptMentionSegments round-two boundaries", () => {
  it("does not reinterpret a mismatched backtick run as a delimiter", () => {
    expect(promptValues("`foo`` @daily`")).toEqual([]);
    expect(promptValues("``` @daily")).toEqual([]);
  });

  it("treats backslashes literally while scanning an inline code span", () => {
    expect(promptValues("`foo @daily\\` tail")).toEqual([]);
  });

  it("recognizes aliases after a CRLF closing fence", () => {
    expect(promptValues("```\r\ncode\r\n```\r\n@daily")).toEqual(["@daily"]);
  });

  it("does not use a code-span bracket as an inline-link label", () => {
    expect(promptValues('`[`](/url "title @daily")')).toEqual(["@daily"]);
  });
  it("does not hide a title alias after reference-style link syntax", () => {
    expect(promptValues('[label][ref](url "title @daily")')).toEqual(["@daily"]);
  });

  it("rejects escaped whitespace in a bare link destination", () => {
    expect(promptValues('[label](url\\ bar "title @daily")')).toEqual(["@daily"]);
  });

  it("skips titles in adjacent valid inline links", () => {
    expect(promptValues('[] [label](url "title @daily")')).toEqual([]);
  });

  it("keeps formatted aliases adjacent to ordinary text as text", () => {
    expect(promptValues("x**@daily**")).toEqual([]);
    expect(promptValues("x_@daily_")).toEqual([]);
  });
});
