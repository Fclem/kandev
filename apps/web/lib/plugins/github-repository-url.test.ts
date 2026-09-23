import { describe, expect, it } from "vitest";
import { parseGitHubRepositoryUrl } from "./github-repository-url";

// The accepted vectors are the backend parser's own cases
// (apps/backend/internal/github/repository_star_url_test.go), so a divergence
// between the two boundaries fails here rather than in a browser.
describe("parseGitHubRepositoryUrl canonicalization", () => {
  it.each([
    ["https://github.com/kdlbs/kandev", "kdlbs/kandev"],
    ["https://GitHub.com/Owner/Repo", "owner/repo"],
    ["https://github.com/o/r.git", "o/r"],
    ["https://github.com/A-1/R_e.p-o", "a-1/r_e.p-o"],
    ["https://github.com/a/repository-name", "a/repository-name"],
    // GitHub allows `.`, `-`, and `_` anywhere in a repository name, so a
    // trailing hyphen is a real repository and must keep its star control.
    ["https://github.com/acme/widget-", "acme/widget-"],
    ["HTTPS://github.com/owner/repo", "owner/repo"],
    // The `.git` suffix is matched case-insensitively, like the backend.
    ["https://github.com/o/r.GIT", "o/r"],
    // Boundary lengths: 39-character owner, 100-character repository.
    [
      `https://github.com/${"a".repeat(39)}/${"b".repeat(100)}`,
      `${"a".repeat(39)}/${"b".repeat(100)}`,
    ],
  ])("canonicalizes %s", (raw, expected) => {
    expect(parseGitHubRepositoryUrl(raw)).toBe(expected);
  });

  it.each([
    // Non-HTTPS, credentials, port, non-github host.
    "http://github.com/owner/repo",
    "git@github.com:owner/repo.git",
    "https://github.com:443/owner/repo",
    "https://user@github.com/owner/repo",
    "https://gitlab.com/owner/repo",
    "https://github.com.evil.test/owner/repo",
    "github.com/owner/repo",
    "/owner/repo",
    // Segment count/shape.
    "",
    "https://github.com",
    "https://github.com/",
    "https://github.com/owner",
    "https://github.com/owner/repo/extra",
    "https://github.com/owner/repo/",
    "https://github.com//owner/repo",
    "https://github.com/owner//repo",
    "https://github.com/owner/repo?ref=main",
    "https://github.com/owner/repo#readme",
    // Percent-encoding and encoded separators/dots.
    "https://github.com/owner%2Frepo",
    "https://github.com/owner/repo%2Egit",
    "https://github.com/a/%2e%2e",
    // Traversal-like and dot-only names.
    "https://github.com/..",
    "https://github.com/a/..",
    "https://github.com/./.",
    "https://github.com/a/.",
    "https://github.com/a/.git",
    "https://github.com/a/.hidden",
    // Git-suffix edge: a second suffix is not stripped.
    "https://github.com/owner/repo.git.git",
    // Out-of-grammar owner/repository names.
    "https://github.com/-owner/repo",
    "https://github.com/owner-/repo",
    "https://github.com/owner/repo.",
    "https://github.com/owner/re po",
    "https://github.com/owner/repo ",
    " https://github.com/owner/repo",
    "https://github.com/owner/repo\t",
    // Length ceilings.
    `https://github.com/${"a".repeat(40)}/repo`,
    `https://github.com/a/${"b".repeat(101)}`,
    // Unicode and confusables never reach the ASCII grammar.
    "https://github.com/ünïcode/repo",
    "https://github.com/owner/répo",
    "https://github.com/owner/r\u0435po",
    "https://github.com/\u0430dmin/repo",
  ])("rejects %s", (raw) => {
    expect(parseGitHubRepositoryUrl(raw)).toBeNull();
  });

  it("rejects absent values", () => {
    expect(parseGitHubRepositoryUrl(null)).toBeNull();
    expect(parseGitHubRepositoryUrl(undefined)).toBeNull();
  });
});
