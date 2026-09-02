"use client";

import {
  Children,
  cloneElement,
  createElement,
  isValidElement,
  useCallback,
  useMemo,
  useState,
  type ComponentPropsWithoutRef,
  type ReactElement,
  type ReactNode,
} from "react";
import type { Components, ExtraProps } from "react-markdown";
import { useTranslation } from "react-i18next";
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

export function usePromptMentionNames() {
  const prompts = useAppStore((state) => state.prompts.items);
  return useMemo(() => prompts.map((prompt) => prompt.name), [prompts]);
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
    destinationDepth: 0,
    destinationQuote: null,
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
  destinationDepth: number;
  destinationQuote: '"' | "'" | null;
};

function skipMarkdownCode(content: string, index: number, state: MarkdownCodeState): number | null {
  if (state.destinationDepth > 0) return skipMarkdownDestination(content, index, state);
  if (state.fence) return skipMarkdownFence(content, index, state);
  if (state.delimiter) return skipMarkdownDelimiter(content, index, state);
  if (content[index] === "]" && content[index + 1] === "(") {
    if (!hasMarkdownDestinationEnd(content, index)) return null;
    state.destinationDepth = 1;
    state.destinationQuote = null;
    return index + 2;
  }
  return startMarkdownCode(content, index, state);
}

function skipMarkdownDestination(content: string, index: number, state: MarkdownCodeState): number {
  const marker = content[index];
  if (state.destinationQuote) {
    if (marker === state.destinationQuote && !isEscapedMarkdownCharacter(content, index)) {
      state.destinationQuote = null;
    }
    return index + 1;
  }
  if ((marker === '"' || marker === "'") && !isEscapedMarkdownCharacter(content, index)) {
    state.destinationQuote = marker;
    return index + 1;
  }
  if (marker === "(" && !isEscapedMarkdownCharacter(content, index)) state.destinationDepth += 1;
  if (marker === ")" && !isEscapedMarkdownCharacter(content, index)) state.destinationDepth -= 1;
  return index + 1;
}

function hasMarkdownDestinationEnd(content: string, linkEndIndex: number) {
  let depth = 1;
  let quote: '"' | "'" | null = null;
  for (let index = linkEndIndex + 2; index < content.length; index += 1) {
    const marker = content[index];
    const escaped = isEscapedMarkdownCharacter(content, index);
    if (quote) {
      if (marker === quote && !escaped) quote = null;
      continue;
    }
    if ((marker === '"' || marker === "'") && !escaped) {
      quote = marker;
      continue;
    }
    if (escaped) continue;
    if (marker === "(") depth += 1;
    if (marker !== ")" || --depth > 0) continue;
    return true;
  }
  return false;
}

function isEscapedMarkdownCharacter(content: string, index: number) {
  let slashCount = 0;
  for (let cursor = index - 1; cursor >= 0 && content[cursor] === "\\"; cursor -= 1) {
    slashCount += 1;
  }
  return slashCount % 2 === 1;
}

function skipMarkdownFence(content: string, index: number, state: MarkdownCodeState): number {
  const fence = state.fence;
  if (
    fence &&
    isMarkdownFenceAt(content, index, fence.marker, fence.length) &&
    !isEscapedMarkdownCharacter(content, index)
  ) {
    state.fence = null;
    return index + runLength(content, index, fence.marker);
  }
  return index + 1;
}

function skipMarkdownDelimiter(content: string, index: number, state: MarkdownCodeState): number {
  if (state.delimiter && content.startsWith(state.delimiter, index)) {
    const nextIndex = index + state.delimiter.length;
    state.delimiter = null;
    return nextIndex;
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
  if (length >= 3 && isMarkdownFenceAt(content, index, marker, length)) {
    state.fence = { marker, length };
  } else if (marker === "`") {
    state.delimiter = marker.repeat(length);
  }
  return index + length;
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

function isMarkdownFenceAt(content: string, index: number, marker: string, length: number) {
  const lineStart = content.lastIndexOf("\n", index - 1) + 1;
  return (
    content.slice(lineStart, index).trim() === "" && runLength(content, index, marker) >= length
  );
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

  return (
    <HoverCard open={open} onOpenChange={setOpen} openDelay={300} closeDelay={0}>
      <HoverCardTrigger asChild>
        <span
          data-testid="custom-prompt-mention"
          data-prompt-name={name}
          tabIndex={focusable ? 0 : undefined}
          role={focusable ? "button" : undefined}
          aria-expanded={focusable ? open : undefined}
          aria-label={focusable ? t("task:customPromptNamed", { name }) : undefined}
          className={cn(PROMPT_MENTION_CHIP_CLASS, "cursor-pointer")}
          onClick={(event) => {
            event.stopPropagation();
            setOpen((isOpen) => !isOpen);
          }}
          onKeyDown={(event) => {
            if (!focusable || (event.key !== "Enter" && event.key !== " ")) return;
            event.preventDefault();
            event.stopPropagation();
            setOpen((isOpen) => !isOpen);
          }}
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
