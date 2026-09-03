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

type PromptNameTrieNode = {
  children: Map<string, PromptNameTrieNode>;
  name?: string;
};

const promptNamePrefixCache = new WeakMap<readonly string[], PromptNameTrieNode>();

function getPromptNamePrefixIndex(promptNames: string[]) {
  const cached = promptNamePrefixCache.get(promptNames);
  if (cached) return cached;
  const root: PromptNameTrieNode = { children: new Map() };
  for (const name of promptNames) {
    let node = root;
    for (const character of name) {
      let child = node.children.get(character);
      if (!child) {
        child = { children: new Map() };
        node.children.set(character, child);
      }
      node = child;
    }
    node.name = name;
  }
  promptNamePrefixCache.set(promptNames, root);
  return root;
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
  let node = getPromptNamePrefixIndex(promptNames);
  for (let cursor = referenceStart; cursor < content.length; ) {
    const codePoint = content.codePointAt(cursor);
    if (codePoint === undefined) break;
    const character = String.fromCodePoint(codePoint);
    const child = node.children.get(character);
    if (!child) return null;
    node = child;
    cursor += character.length;
    if (node.name && (cursor >= content.length || !isMentionNameCharAt(content, cursor))) {
      return { start: index, end: cursor, name: node.name };
    }
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
