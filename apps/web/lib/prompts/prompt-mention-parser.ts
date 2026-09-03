export type PromptMentionMatch = {
  start: number;
  end: number;
  name: string;
};

export function buildPromptMentionNames(promptNames: string[]) {
  return Array.from(new Set(promptNames.filter(Boolean))).sort(
    (a, b) => b.length - a.length || a.localeCompare(b),
  );
}

const promptNamePrefixCache = new WeakMap<readonly string[], Map<string, string[]>>();

function getPromptNamePrefixIndex(promptNames: string[]) {
  const cached = promptNamePrefixCache.get(promptNames);
  if (cached) return cached;
  const index = new Map<string, string[]>();
  for (const name of promptNames) {
    const prefix = name.slice(0, 2);
    const names = index.get(prefix);
    if (names) names.push(name);
    else index.set(prefix, [name]);
  }
  promptNamePrefixCache.set(promptNames, index);
  return index;
}

/**
 * Match using names ordered by buildPromptMentionNames so longer names win
 * over shorter prefixes.
 */
export function matchPromptMention(
  content: string,
  index: number,
  promptNames: string[],
): PromptMentionMatch | null {
  if (content[index] !== "@" || !isMentionStart(content, index)) return null;

  const referenceStart = index + 1;
  const candidates = getPromptNamePrefixIndex(promptNames).get(
    content.slice(referenceStart, referenceStart + 2),
  );
  for (const name of candidates ?? []) {
    if (!content.startsWith(name, referenceStart)) continue;
    const referenceEnd = referenceStart + name.length;
    if (referenceEnd < content.length && isMentionNameCharAt(content, referenceEnd)) {
      continue;
    }
    return { start: index, end: referenceEnd, name };
  }
  return null;
}

function isMentionNameCharAt(content: string, index: number) {
  const codePoint = content.codePointAt(index);
  return codePoint !== undefined && /[\p{L}\p{M}\p{N}_-]/u.test(String.fromCodePoint(codePoint));
}

function isMentionStart(content: string, index: number) {
  return index === 0 || isWhitespace(content[index - 1]);
}

function isWhitespace(value: string) {
  return value === " " || value === "\n" || value === "\t" || value === "\r";
}
