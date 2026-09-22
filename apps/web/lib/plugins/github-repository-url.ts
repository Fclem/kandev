/**
 * Strict canonical parser for public GitHub repository URLs.
 *
 * Mirrors the backend parser
 * (`apps/backend/internal/github/repository_star_url.go`) because both
 * boundaries validate the same plugin metadata (a manifest `repo_url` or an
 * untrusted catalog `index.json`). The backend stays authoritative; this parser
 * exists so the UI never offers a star control for a repository the backend
 * would reject, and so no arbitrary GitHub API path can be built from plugin
 * data.
 *
 * Grammar: HTTPS only, host exactly `github.com` (case-insensitive) with no
 * credentials and no port, no percent-encoding, no query, no fragment, and
 * exactly two non-empty path segments.
 */

const HTTPS_SCHEME = "https://";
const GITHUB_HOST = "github.com";
const GIT_SUFFIX = ".git";

/** One-character owners are valid; no leading or trailing hyphen. */
const OWNER_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$/;

/**
 * One-character names are valid; no trailing dot or hyphen. The bounded middle
 * group expresses GitHub's 100-character repository-name ceiling.
 */
const NAME_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,98}[A-Za-z0-9_])?$/;

/**
 * A single terminal `.git` belongs to the clone URL, never the repository name.
 * Stripping it a second time would let `repo.git.git` collapse onto `repo`.
 */
function canonicalRepositoryName(name: string): string | null {
  const stripped = name.toLowerCase().endsWith(GIT_SUFFIX)
    ? name.slice(0, -GIT_SUFFIX.length)
    : name;
  if (stripped.toLowerCase().endsWith(GIT_SUFFIX) || !NAME_PATTERN.test(stripped)) return null;
  return stripped.toLowerCase();
}

/**
 * The canonical lowercased `owner/repo` identity of a public GitHub repository
 * URL, or null for any URL the backend would reject.
 */
export function parseGitHubRepositoryUrl(raw: string | null | undefined): string | null {
  if (!raw || raw.includes("%")) return null;
  if (!raw.toLowerCase().startsWith(HTTPS_SCHEME)) return null;

  const afterScheme = raw.slice(HTTPS_SCHEME.length);
  const authorityEnd = afterScheme.indexOf("/");
  if (authorityEnd < 0) return null;
  // Comparing the whole authority rejects credentials and ports for free.
  if (afterScheme.slice(0, authorityEnd).toLowerCase() !== GITHUB_HOST) return null;

  const path = afterScheme.slice(authorityEnd);
  if (path.includes("?") || path.includes("#")) return null;

  const segments = path.slice(1).split("/");
  if (segments.length !== 2 || !segments[0] || !segments[1]) return null;
  if (!OWNER_PATTERN.test(segments[0])) return null;

  const name = canonicalRepositoryName(segments[1]);
  if (!name) return null;
  return `${segments[0].toLowerCase()}/${name}`;
}
