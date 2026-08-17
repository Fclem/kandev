import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconChevronDown, IconChevronUp, IconNavigation } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useSessionMessages } from "@/hooks/domains/session/use-session-messages";
import { useSessionTurns } from "@/hooks/domains/session/use-session-turns";
import { formatRelative } from "@/lib/i18n/formats";
import {
  buildPromptHistoryEntries,
  formatPromptDuration,
  type PromptHistoryEntry,
} from "@/lib/prompt-history";
import { PanelRoot } from "./panel-primitives";

type PromptHistoryPanelContentProps = { onNavigateToPrompt?: (messageId: string) => void };

export function PromptHistoryPanelContent({ onNavigateToPrompt }: PromptHistoryPanelContentProps) {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement>(null);
  const sessionId = useAppStore((state) => state.tasks.activeSessionId);
  const session = useAppStore((state) => (sessionId ? state.taskSessions.items[sessionId] : null));
  const { messages } = useSessionMessages(sessionId);
  const turns = useSessionTurns(sessionId);
  const entries = useMemo(() => buildPromptHistoryEntries(messages, turns), [messages, turns]);
  const [expanded, setExpanded] = useState<string | null>(null);
  const maxHeight = usePanelRowMaxHeight(rootRef);

  if (session?.is_passthrough || entries.length === 0) {
    return (
      <PanelRoot
        ref={rootRef}
        data-testid="prompt-history-panel"
        className="p-4 text-sm text-muted-foreground"
      >
        {t("task:promptHistoryEmpty")}
      </PanelRoot>
    );
  }

  return (
    <PanelRoot ref={rootRef} data-testid="prompt-history-panel" className="overflow-y-auto p-2">
      {entries.map((entry, index) => (
        <PromptHistoryRow
          key={entry.messageId}
          entry={entry}
          index={index}
          expanded={expanded === entry.messageId}
          maxHeight={maxHeight}
          onToggle={() => setExpanded(expanded === entry.messageId ? null : entry.messageId)}
          onNavigate={onNavigateToPrompt}
        />
      ))}
    </PanelRoot>
  );
}

function usePanelRowMaxHeight(rootRef: RefObject<HTMLDivElement | null>) {
  const [maxHeight, setMaxHeight] = useState<string>("40vh");
  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const updateHeight = () =>
      setMaxHeight(root.clientHeight ? `${Math.round(root.clientHeight * 0.4)}px` : "40vh");
    updateHeight();
    const observer = new ResizeObserver(updateHeight);
    observer.observe(root);
    return () => observer.disconnect();
  }, [rootRef]);
  return maxHeight;
}

type PromptHistoryRowProps = {
  entry: PromptHistoryEntry;
  index: number;
  expanded: boolean;
  maxHeight: string;
  onToggle: () => void;
  onNavigate?: (messageId: string) => void;
};

function PromptHistoryRow({
  entry,
  index,
  expanded,
  maxHeight,
  onToggle,
  onNavigate,
}: PromptHistoryRowProps) {
  const { t } = useTranslation();
  const textRef = useRef<HTMLSpanElement>(null);
  const [overflow, setOverflow] = useState(false);
  useEffect(() => {
    const text = textRef.current;
    if (!text) return;
    const update = () => setOverflow(text.scrollWidth > text.clientWidth);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(text);
    return () => observer.disconnect();
  }, []);
  const showToggle = overflow || expanded;
  return (
    <div data-testid={`prompt-history-row-${index}`} className="flex items-start gap-1 py-1">
      <Button
        variant="ghost"
        size="icon"
        className="size-8 shrink-0 cursor-pointer"
        aria-label={t("task:scrollToPrompt")}
        data-testid={`prompt-history-jump-${index}`}
        onClick={() => onNavigate?.(entry.messageId)}
      >
        <IconNavigation size={16} />
      </Button>
      <div className="min-w-0 flex-1">
        {/* Same bubble as the transcript's user message (rounded-2xl
            bg-primary/30, inherited font) with lighter padding. */}
        <div className="overflow-hidden rounded-2xl bg-primary/30 px-3 py-1.5">
          <span ref={textRef} className={expanded ? "hidden" : "block truncate"}>
            {entry.content}
          </span>
          {expanded && (
            <div
              data-testid={`prompt-history-expanded-box-${index}`}
              className="overflow-y-auto whitespace-normal"
              style={{ maxHeight }}
            >
              {entry.content}
            </div>
          )}
        </div>
      </div>
      <time
        dateTime={entry.sentAt}
        title={new Date(entry.sentAt).toLocaleString()}
        className="shrink-0 text-xs text-muted-foreground"
      >
        {formatRelative(entry.sentAt)}
      </time>
      <PromptDuration durationSeconds={entry.durationSeconds} index={index} />
      {showToggle && (
        <Button
          variant="ghost"
          size="icon"
          className="size-8 shrink-0 cursor-pointer"
          aria-expanded={expanded}
          aria-label={t(expanded ? "task:collapsePrompt" : "task:expandPrompt")}
          data-testid={`prompt-history-expand-${index}`}
          onClick={onToggle}
        >
          {expanded ? <IconChevronUp size={16} /> : <IconChevronDown size={16} />}
        </Button>
      )}
    </div>
  );
}

function PromptDuration({
  durationSeconds,
  index,
}: {
  durationSeconds: number | null;
  index: number;
}) {
  const { t } = useTranslation();
  if (durationSeconds === null) return null;
  return (
    <span
      data-testid={`prompt-history-duration-${index}`}
      className="shrink-0 text-xs text-muted-foreground"
    >
      {formatPromptDuration(durationSeconds, {
        s: t("task:durationUnitSeconds"),
        m: t("task:durationUnitMinutes"),
        h: t("task:durationUnitHours"),
      })}
    </span>
  );
}
