"use client";

import {
  Children,
  cloneElement,
  createElement,
  isValidElement,
  useCallback,
  useMemo,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type KeyboardEvent,
  type MouseEvent,
  type ReactElement,
  type ReactNode,
} from "react";
import type { Components, ExtraProps } from "react-markdown";
import { useTranslation } from "react-i18next";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { cn } from "@/lib/utils";
import { PromptPreview } from "@/components/task/chat/context-items/prompt-preview";
import { useAppStore } from "@/components/state-provider";
import {
  buildPromptMentionNames,
  type PromptMentionSegment,
} from "@/lib/prompts/prompt-mention-segments";
import { matchPromptMention, type PromptMentionMatch } from "@/lib/prompts/prompt-mention-parser";
import type { EntityReference } from "@/lib/types/entity-reference";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { buildEntityReferenceMarkdownComponents } from "./entity-reference-chip";

type PromptMentionMarkdownTag =
  | "a"
  | "p"
  | "li"
  | "h1"
  | "h2"
  | "h3"
  | "h4"
  | "h5"
  | "h6"
  | "blockquote"
  | "td"
  | "th";
const PROMPT_MENTION_NESTED_TAGS: Record<string, true> = {
  a: true,
  del: true,
  em: true,
  mark: true,
  s: true,
  small: true,
  strong: true,
  sub: true,
  sup: true,
  u: true,
};

type MarkdownChildrenProps<T extends PromptMentionMarkdownTag> = ComponentPropsWithoutRef<T> & {
  children?: ReactNode;
  node?: ExtraProps["node"];
};

export const PROMPT_MENTION_CHIP_CLASS =
  "inline rounded-md border border-emerald-300/35 bg-emerald-400/20 px-1.5 py-0.5 font-mono text-[0.88em] font-semibold text-emerald-950 box-decoration-clone break-all dark:text-emerald-100";

export function useStablePromptMentionNames(promptNames: string[]) {
  const stableNamesRef = useRef<string[]>([]);
  if (
    stableNamesRef.current.length !== promptNames.length ||
    stableNamesRef.current.some((name, index) => name !== promptNames[index])
  ) {
    stableNamesRef.current = promptNames;
  }
  return stableNamesRef.current;
}

export function usePromptMentionNames() {
  const prompts = useAppStore((state) => state.prompts.items);
  return useStablePromptMentionNames(prompts.map((prompt) => prompt.name));
}

export function usePromptMentionMarkdownComponents(
  promptNames: string[],
  entityReferences: readonly EntityReference[],
): Components | undefined {
  return useMemo(() => {
    const mentionNames = buildPromptMentionNames(promptNames);
    const baseComponents = buildEntityReferenceMarkdownComponents(entityReferences);
    if (mentionNames.length === 0) {
      return entityReferences.length === 0 ? undefined : baseComponents;
    }
    const renderChildren = (children: ReactNode, keyPrefix: string, interactive = true) =>
      renderChildrenWithPromptMentions(children, mentionNames, keyPrefix, interactive);
    return {
      ...baseComponents,
      a: ({ children, node, ...props }: MarkdownChildrenProps<"a">) => {
        const BaseLink = baseComponents.a;
        return BaseLink ? (
          createElement(BaseLink, { ...props, node }, renderChildren(children, "a", false))
        ) : (
          <a {...props}>{renderChildren(children, "a", false)}</a>
        );
      },
      p: ({ children, node, ...props }: MarkdownChildrenProps<"p">) => {
        void node;
        return <p {...props}>{renderChildren(children, "p")}</p>;
      },
      li: ({ children, node, ...props }: MarkdownChildrenProps<"li">) => {
        void node;
        return <li {...props}>{renderChildren(children, "li")}</li>;
      },
      h1: ({ children, node, ...props }: MarkdownChildrenProps<"h1">) => {
        void node;
        return <h1 {...props}>{renderChildren(children, "h1")}</h1>;
      },
      h2: ({ children, node, ...props }: MarkdownChildrenProps<"h2">) => {
        void node;
        return <h2 {...props}>{renderChildren(children, "h2")}</h2>;
      },
      h3: ({ children, node, ...props }: MarkdownChildrenProps<"h3">) => {
        void node;
        return <h3 {...props}>{renderChildren(children, "h3")}</h3>;
      },
      h4: ({ children, node, ...props }: MarkdownChildrenProps<"h4">) => {
        void node;
        return <h4 {...props}>{renderChildren(children, "h4")}</h4>;
      },
      h5: ({ children, node, ...props }: MarkdownChildrenProps<"h5">) => {
        void node;
        return <h5 {...props}>{renderChildren(children, "h5")}</h5>;
      },
      h6: ({ children, node, ...props }: MarkdownChildrenProps<"h6">) => {
        void node;
        return <h6 {...props}>{renderChildren(children, "h6")}</h6>;
      },
      blockquote: ({ children, node, ...props }: MarkdownChildrenProps<"blockquote">) => {
        void node;
        return <blockquote {...props}>{renderChildren(children, "blockquote")}</blockquote>;
      },
      td: ({ children, node, ...props }: MarkdownChildrenProps<"td">) => {
        void node;
        return <td {...props}>{renderChildren(children, "td")}</td>;
      },
      th: ({ children, node, ...props }: MarkdownChildrenProps<"th">) => {
        void node;
        return <th {...props}>{renderChildren(children, "th")}</th>;
      },
    };
  }, [promptNames, entityReferences]);
}

function renderChildrenWithPromptMentions(
  children: ReactNode,
  promptNames: string[],
  keyPrefix: string,
  interactive = true,
): ReactNode[] {
  return Children.toArray(children).flatMap((child, index) => {
    const childKeyPrefix = `${keyPrefix}-${index}`;
    if (typeof child === "string") {
      return renderTextWithPromptMentions(child, promptNames, childKeyPrefix, interactive);
    }
    if (!isValidElement(child)) return child;
    const element = child as ReactElement<{ children?: ReactNode }>;
    if (
      typeof element.type !== "string" ||
      PROMPT_MENTION_NESTED_TAGS[element.type] !== true ||
      element.props.children === undefined
    ) {
      return element;
    }
    return cloneElement(element, {
      children: renderChildrenWithPromptMentions(
        element.props.children,
        promptNames,
        childKeyPrefix,
        interactive,
      ),
    });
  });
}

export function splitMarkdownPromptMentionSegments(
  content: string,
  promptNames: string[],
): PromptMentionSegment[] {
  if (content.length === 0 || promptNames.length === 0) {
    return [{ kind: "text", value: content }];
  }

  const segments: PromptMentionSegment[] = [];
  let lastIndex = 0;
  const codeState: MarkdownCodeState = {
    delimiter: null,
    fence: null,
  };

  for (let index = 0; index < content.length; ) {
    const codeIndex = skipMarkdownCode(content, index, codeState);
    if (codeIndex !== null) {
      index = codeIndex;
      continue;
    }

    if (content[index] !== "@") {
      index += 1;
      continue;
    }

    const match = matchMarkdownPromptMention(content, index, promptNames);
    if (!match) {
      index += 1;
      continue;
    }
    if (match.start > lastIndex) {
      segments.push({ kind: "text", value: content.slice(lastIndex, match.start) });
    }
    segments.push({
      kind: "prompt",
      value: content.slice(match.start, match.end),
      name: match.name,
    });
    index = match.end;
    lastIndex = match.end;
  }

  if (lastIndex < content.length) {
    segments.push({ kind: "text", value: content.slice(lastIndex) });
  }
  return segments;
}

type MarkdownCodeState = {
  delimiter: string | null;
  fence: { marker: string; length: number } | null;
};

function skipMarkdownCode(content: string, index: number, state: MarkdownCodeState): number | null {
  if (state.fence) return skipMarkdownFence(content, index, state);
  if (state.delimiter) return skipMarkdownDelimiter(content, index, state);
  if (content[index] === "]" && !isEscapedMarkdownCharacter(content, index)) {
    const destinationEnd = findMarkdownDestinationEnd(content, index);
    if (destinationEnd !== null) return destinationEnd + 1;
  }
  return startMarkdownCode(content, index, state);
}

function skipMarkdownFence(content: string, index: number, state: MarkdownCodeState): number {
  const fence = state.fence;
  if (
    fence &&
    isMarkdownFenceCloserAt(content, index, fence.marker, fence.length) &&
    !isEscapedMarkdownCharacter(content, index)
  ) {
    state.fence = null;
    return index + runLength(content, index, fence.marker);
  }
  return index + 1;
}

function skipMarkdownDelimiter(content: string, index: number, state: MarkdownCodeState): number {
  const delimiter = state.delimiter;
  if (
    delimiter &&
    runLength(content, index, delimiter[0]) === delimiter.length &&
    content.startsWith(delimiter, index) &&
    !isEscapedMarkdownCharacter(content, index)
  ) {
    state.delimiter = null;
    return index + delimiter.length;
  }
  return index + 1;
}

function startMarkdownCode(
  content: string,
  index: number,
  state: MarkdownCodeState,
): number | null {
  const marker = content[index];
  if (isEscapedMarkdownCharacter(content, index)) return null;
  const length = marker === "`" || marker === "~" ? runLength(content, index, marker) : 0;
  if (length === 0) return null;
  if (length >= 3 && isMarkdownFenceOpenerAt(content, index, marker, length)) {
    state.fence = { marker, length };
    return index + length;
  }
  if (marker === "`" && hasMarkdownDelimiterEnd(content, index, length)) {
    state.delimiter = marker.repeat(length);
    return index + length;
  }
  return null;
}

function findMarkdownDestinationEnd(content: string, linkEndIndex: number): number | null {
  if (!hasMarkdownLinkLabelAt(content, linkEndIndex)) return null;
  const destinationStart = linkEndIndex + 2;
  if (content[destinationStart] === ")") return destinationStart;

  const destinationEnd =
    content[destinationStart] === "<"
      ? findAngleMarkdownDestinationEnd(content, destinationStart)
      : findBareMarkdownDestinationEnd(content, destinationStart);
  if (destinationEnd === null) return null;
  return findMarkdownLinkSuffixEnd(content, destinationEnd);
}

function findAngleMarkdownDestinationEnd(content: string, start: number): number | null {
  for (let index = start + 1; index < content.length; index += 1) {
    if (isEscapedMarkdownCharacter(content, index)) {
      index += 1;
      continue;
    }
    if (content[index] === ">") return index + 1;
    if (content[index] === "\n" || content[index] === "\r") return null;
  }
  return null;
}

function findBareMarkdownDestinationEnd(content: string, start: number): number | null {
  let depth = 0;
  for (let index = start; index < content.length; index += 1) {
    if (isEscapedMarkdownCharacter(content, index)) {
      index += 1;
      continue;
    }
    const marker = content[index];
    if (isMarkdownWhitespace(marker)) return depth === 0 ? index : null;
    if (marker === "(") depth += 1;
    if (marker === ")" && depth === 0) return index;
    if (marker === ")") depth -= 1;
  }
  return depth === 0 ? content.length : null;
}

function findMarkdownLinkSuffixEnd(content: string, destinationEnd: number): number | null {
  let index = destinationEnd;
  if (content[index] === ")") return index;
  if (!isMarkdownWhitespace(content[index])) return null;
  while (isMarkdownWhitespace(content[index])) index += 1;
  if (content[index] === ")") return index;

  const titleMarker = content[index];
  if (titleMarker !== '"' && titleMarker !== "'" && titleMarker !== "(") return null;
  const titleEnd =
    titleMarker === "("
      ? findParenthesizedMarkdownTitleEnd(content, index)
      : findQuotedMarkdownTitleEnd(content, index, titleMarker);
  if (titleEnd === null) return null;
  index = titleEnd + 1;
  while (isMarkdownWhitespace(content[index])) index += 1;
  return content[index] === ")" ? index : null;
}

function hasMarkdownLinkLabelAt(content: string, linkEndIndex: number) {
  if (content[linkEndIndex] !== "]" || isEscapedMarkdownCharacter(content, linkEndIndex)) {
    return false;
  }
  let depth = 1;
  for (let index = linkEndIndex - 1; index >= 0; index -= 1) {
    if (isEscapedMarkdownCharacter(content, index)) continue;
    if (content[index] === "]") {
      depth += 1;
    } else if (content[index] === "[") {
      depth -= 1;
      if (depth === 0) return true;
    }
  }
  return false;
}

function findQuotedMarkdownTitleEnd(
  content: string,
  start: number,
  marker: '"' | "'",
): number | null {
  for (let index = start + 1; index < content.length; index += 1) {
    if (content[index] === marker && !isEscapedMarkdownCharacter(content, index)) return index;
  }
  return null;
}

function findParenthesizedMarkdownTitleEnd(content: string, start: number): number | null {
  let depth = 1;
  for (let index = start + 1; index < content.length; index += 1) {
    if (isEscapedMarkdownCharacter(content, index)) {
      index += 1;
      continue;
    }
    if (content[index] === "(") depth += 1;
    if (content[index] === ")" && --depth === 0) return index;
  }
  return null;
}

function hasMarkdownDelimiterEnd(content: string, start: number, length: number) {
  for (let index = start + length; index < content.length; index += 1) {
    if (content[index] !== "`" || isEscapedMarkdownCharacter(content, index)) continue;
    const run = runLength(content, index, "`");
    if (run === length) return true;
    index += run - 1;
  }
  return false;
}

function isMarkdownFenceOpenerAt(content: string, index: number, marker: string, length: number) {
  const lineStart = content.lastIndexOf("\n", index - 1) + 1;
  const indentation = content.slice(lineStart, index);
  if (!/^ {0,3}$/.test(indentation) || runLength(content, index, marker) < length) {
    return false;
  }
  if (marker !== "`") return true;
  const lineEnd = content.indexOf("\n", index);
  const info = content.slice(index + length, lineEnd === -1 ? content.length : lineEnd);
  return !info.includes("`");
}

function isMarkdownFenceCloserAt(content: string, index: number, marker: string, length: number) {
  const lineStart = content.lastIndexOf("\n", index - 1) + 1;
  const indentation = content.slice(lineStart, index);
  if (!/^ {0,3}$/.test(indentation) || runLength(content, index, marker) < length) {
    return false;
  }
  const lineEnd = content.indexOf("\n", index);
  const suffix = content.slice(
    index + runLength(content, index, marker),
    lineEnd === -1 ? content.length : lineEnd,
  );
  return /^[ \t]*$/.test(suffix);
}

function isMarkdownWhitespace(value: string | undefined) {
  return value === " " || value === "\t" || value === "\n" || value === "\r";
}
function isEscapedMarkdownCharacter(content: string, index: number) {
  let slashCount = 0;
  for (let cursor = index - 1; cursor >= 0 && content[cursor] === "\\"; cursor -= 1) {
    slashCount += 1;
  }
  return slashCount % 2 === 1;
}

function matchMarkdownPromptMention(
  content: string,
  index: number,
  promptNames: string[],
): PromptMentionMatch | null {
  const direct = matchPromptMention(content, index, promptNames);
  if (direct) return direct;

  const marker = content[index - 1];
  if (!marker || !"*_~[".includes(marker)) return null;
  const closingMarker = marker === "[" ? "]" : marker;
  const candidatePrefix = `${content.slice(0, index - 1)} ${content.slice(index)}`;
  for (let closingIndex = index + 1; closingIndex < content.length; closingIndex += 1) {
    if (content[closingIndex] !== closingMarker) continue;
    const candidate =
      candidatePrefix.slice(0, closingIndex) + " " + candidatePrefix.slice(closingIndex + 1);
    const match = matchPromptMention(candidate, index, promptNames);
    if (match?.end === closingIndex) return match;
  }
  return null;
}

function runLength(content: string, index: number, marker: string) {
  let end = index;
  while (content[end] === marker) end += 1;
  return end - index;
}

export function renderTextWithPromptMentions(
  text: string,
  promptNames: string[],
  keyPrefix: string,
  interactive = true,
) {
  return splitMarkdownPromptMentionSegments(text, promptNames).map((segment, index) => {
    if (segment.kind === "text") return segment.value;
    return (
      <PromptMentionChip
        key={`${keyPrefix}-prompt-${segment.name}-${index}`}
        name={segment.name}
        value={segment.value}
        interactive={interactive}
      />
    );
  });
}
export function PromptMentionText({
  text,
  promptNames,
  keyPrefix = "prompt-text",
  focusable = true,
}: {
  text: string;
  promptNames: string[];
  keyPrefix?: string;
  focusable?: boolean;
}) {
  const mentionNames = useMemo(() => buildPromptMentionNames(promptNames), [promptNames]);
  return (
    <>
      {splitMarkdownPromptMentionSegments(text, mentionNames).map((segment, index) =>
        segment.kind === "text" ? (
          segment.value
        ) : (
          <PromptMentionChip
            key={`${keyPrefix}-prompt-${segment.name}-${index}`}
            name={segment.name}
            value={segment.value}
            focusable={focusable}
          />
        ),
      )}
    </>
  );
}
export function PromptMentionChip({
  name,
  value,
  focusable = true,
  interactive = true,
}: {
  name: string;
  value: string;
  focusable?: boolean;
  interactive?: boolean;
}) {
  const { t } = useTranslation();
  const content = useAppStore(
    useCallback(
      (state) => state.prompts.items.find((prompt) => prompt.name === name)?.content ?? null,
      [name],
    ),
  );
  const [open, setOpen] = useState(false);
  const usesTouchDrawer = useTouchDrawer();

  if (!content || !interactive) {
    return (
      <span
        data-testid="custom-prompt-mention"
        data-prompt-name={name}
        title={t("task:customPromptNamed", { name })}
        className={PROMPT_MENTION_CHIP_CLASS}
      >
        {value}
      </span>
    );
  }

  const label = t("task:customPromptNamed", { name });
  const handleToggle = () => setOpen((isOpen) => !isOpen);
  const handleClick = (event: MouseEvent) => {
    event.stopPropagation();
    handleToggle();
  };
  const handleKeyDown = (event: KeyboardEvent) => {
    if (!focusable || (event.key !== "Enter" && event.key !== " ")) return;
    event.preventDefault();
    event.stopPropagation();
    handleToggle();
  };

  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>
          <button
            type="button"
            data-testid="custom-prompt-mention"
            data-prompt-name={name}
            aria-haspopup="dialog"
            aria-expanded={open}
            aria-label={label}
            className={cn(PROMPT_MENTION_CHIP_CLASS, "h-11 min-w-11 cursor-pointer")}
            onClick={(event) => event.stopPropagation()}
          >
            {value}
          </button>
        </DrawerTrigger>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{label}</DrawerTitle>
            <DrawerDescription className="sr-only">{label}</DrawerDescription>
          </DrawerHeader>
          <div className="max-h-[70dvh] overflow-y-auto px-4 pb-4">
            <PromptPreview content={content} />
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <HoverCard open={open} onOpenChange={setOpen} openDelay={300} closeDelay={0}>
      <HoverCardTrigger asChild>
        <span
          data-testid="custom-prompt-mention"
          data-prompt-name={name}
          tabIndex={focusable ? 0 : undefined}
          role={focusable ? "button" : undefined}
          aria-expanded={focusable ? open : undefined}
          aria-label={focusable ? label : undefined}
          className={cn(PROMPT_MENTION_CHIP_CLASS, "cursor-pointer")}
          onClick={handleClick}
          onKeyDown={handleKeyDown}
        >
          {value}
        </span>
      </HoverCardTrigger>
      <HoverCardContent side="top" align="start" className="w-80 max-h-80 overflow-y-auto">
        <PromptPreview content={content} />
      </HoverCardContent>
    </HoverCard>
  );
}
