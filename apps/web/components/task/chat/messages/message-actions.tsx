"use client";

import { useState } from "react";
import { useShallow } from "zustand/react/shallow";
import {
  IconArrowDown,
  IconArrowUp,
  IconCheck,
  IconCopy,
  IconCode,
  IconEyeCode,
  IconInfoCircle,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useAppStore } from "@/components/state-provider";
import type { Message, Turn } from "@/lib/types/http";
import {
  buildMessageDebugEntries,
  hasMessageDebugMetadata,
} from "@/components/task/chat/messages/message-debug-metadata";
import { formatMessageSessionConfig } from "@/components/task/chat/messages/message-session-config";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

const ACTION_BUTTON_SIZE = "h-5 w-5 p-1";
const ACTION_BUTTON_HOVER = "hover:bg-muted rounded";

function shouldCompactNavigation(isFinePointer: boolean, isMobile: boolean): boolean {
  return isFinePointer && !isMobile;
}
const ACTION_BUTTON_TRANSITION = "transition-colors duration-200";

type MessageActionsProps = {
  message: Message;
  showCopy?: boolean;
  showTimestamp?: boolean;
  showRawToggle?: boolean;
  hasHiddenPrompts?: boolean;
  showModel?: boolean;
  isRawView?: boolean;
  onToggleRaw?: () => void;
  navigation?: MessageNavigationActions;
};

export type MessageNavigationActions = {
  canNavigatePrevious: boolean;
  canNavigateNext: boolean;
  isBusy: boolean;
  onPrevious: () => void;
  onNext: () => void;
};

function NavigationButton({
  direction,
  disabled,
  isBusy,
  compact,
  onClick,
}: {
  direction: "previous" | "next";
  disabled: boolean;
  isBusy: boolean;
  compact: boolean;
  onClick: () => void;
}) {
  const isPrevious = direction === "previous";
  const label = isPrevious ? "Previous user message" : "Next user message";
  const Icon = isPrevious ? IconArrowUp : IconArrowDown;
  return (
    <button
      type="button"
      onClick={(event) => {
        event.currentTarget.blur();
        onClick();
      }}
      disabled={disabled}
      aria-label={label}
      aria-busy={isBusy || undefined}
      title={label}
      className={cn(
        "h-11 w-11 shrink-0 cursor-pointer rounded p-3 transition-colors duration-200 hover:bg-muted disabled:cursor-default disabled:opacity-35",
        compact && "h-5 w-5 p-1",
      )}
    >
      <Icon className="h-full w-full" />
    </button>
  );
}

function CopyButton({ copied, onCopy }: { copied: boolean; onCopy: () => void }) {
  return (
    <button
      onClick={onCopy}
      className={cn(
        ACTION_BUTTON_SIZE,
        ACTION_BUTTON_HOVER,
        ACTION_BUTTON_TRANSITION,
        copied && "text-green-400",
      )}
      title="Copy message"
      aria-label="Copy message to clipboard"
    >
      {copied ? <IconCheck className="h-full w-full" /> : <IconCopy className="h-full w-full" />}
    </button>
  );
}

function RawToggleButton({
  isRawView,
  onToggleRaw,
  hasHiddenPrompts,
}: {
  isRawView: boolean;
  onToggleRaw: () => void;
  hasHiddenPrompts?: boolean;
}) {
  return (
    <button
      onClick={onToggleRaw}
      className={cn(
        "flex items-center gap-0.5 rounded",
        ACTION_BUTTON_HOVER,
        ACTION_BUTTON_TRANSITION,
        hasHiddenPrompts ? "h-5 px-1 py-1" : ACTION_BUTTON_SIZE,
        isRawView && "bg-muted text-foreground",
      )}
      title={isRawView ? "Show formatted" : "Show raw text"}
      aria-label={isRawView ? "Show formatted message" : "Show raw text"}
    >
      <IconCode className="h-3 w-3" />
      {hasHiddenPrompts && <IconEyeCode className="h-3 w-3" />}
    </button>
  );
}

function MetadataValue({ value }: { value: unknown }) {
  if (value == null) return <span className="text-muted-foreground">null</span>;
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return <span className="font-mono text-muted-foreground">{String(value)}</span>;
  }
  return (
    <pre className="max-h-[48vh] overflow-auto rounded border bg-background p-3 text-[11px] leading-relaxed">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

function MessageDebugDialog({
  message,
  turn,
  usageMultiplier,
}: {
  message: Message;
  turn: Turn | null;
  usageMultiplier?: string | null;
}) {
  const [open, setOpen] = useState(false);
  const context = { usageMultiplier };
  if (!hasMessageDebugMetadata(message, turn, context)) return null;
  const entries = buildMessageDebugEntries(message, turn, context);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button
          className={cn(ACTION_BUTTON_SIZE, ACTION_BUTTON_HOVER, ACTION_BUTTON_TRANSITION)}
          title="Message metadata"
          aria-label="Show message metadata"
        >
          <IconInfoCircle className="h-full w-full" />
        </button>
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Message Metadata</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3 overflow-auto pr-1">
          {Object.entries(entries).map(([key, value]) => (
            <div key={key} className="grid gap-1">
              <div className="font-mono text-[10px] uppercase text-muted-foreground">{key}</div>
              <MetadataValue value={value} />
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function MessageTimestamp({ createdAt }: { createdAt: string }) {
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const absoluteTime = new Date(createdAt).toLocaleString();
  const timeEl = (
    <time
      dateTime={createdAt}
      title={absoluteTime}
      className="text-[10px] text-muted-foreground/60 font-mono"
    >
      {formatRelativeTime(createdAt)}
    </time>
  );

  if (!usesTouchDrawer) return timeEl;

  // Native `title` tooltips never fire on touch (no hover event), so coarse
  // pointers get a tap-to-open Drawer surfacing the same absolute time.
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <button
          type="button"
          data-testid="message-timestamp-trigger"
          className="cursor-pointer border-0 bg-transparent p-0 text-left"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={`Show full timestamp: ${absoluteTime}`}
        >
          {timeEl}
        </button>
      </DrawerTrigger>
      <DrawerContent data-testid="message-timestamp-drawer">
        <DrawerHeader>
          <DrawerTitle>Message time</DrawerTitle>
          <DrawerDescription>{absoluteTime}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

function MessageMetaInfo({
  showModel,
  sessionConfigText,
  showTimestamp,
  createdAt,
}: {
  showModel: boolean;
  sessionConfigText: string | null;
  showTimestamp: boolean;
  createdAt: string;
}) {
  return (
    <>
      {showModel && sessionConfigText && (
        <span className="min-w-0 truncate text-[10px] text-muted-foreground/60 font-mono">
          {sessionConfigText}
        </span>
      )}
      {showTimestamp && <MessageTimestamp createdAt={createdAt} />}
    </>
  );
}

export function MessageActions({
  message,
  showCopy = true,
  showTimestamp = true,
  showRawToggle = true,
  hasHiddenPrompts = false,
  showModel = false,
  isRawView = false,
  onToggleRaw,
  navigation,
}: MessageActionsProps) {
  const { copied, copy } = useCopyToClipboard();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const compactNavigation = shouldCompactNavigation(isFinePointer, isMobile);
  const { turn, usageMultiplier } = useAppStore(
    useShallow((state) => {
      const turnId = message.turn_id;
      const turn =
        turnId && message.session_id
          ? (state.turns.bySession[message.session_id]?.find((item) => item.id === turnId) ?? null)
          : null;
      if (!message.session_id) return { turn, usageMultiplier: null };
      const sessionModels = state.sessionModels.bySessionId[message.session_id];
      const metadataModel = (message.metadata?.model ?? turn?.metadata?.model) as
        | string
        | undefined;
      const modelId = metadataModel ?? sessionModels?.currentModelId;
      const usageMultiplier =
        sessionModels?.models.find((model) => model.modelId === modelId)?.usageMultiplier ?? null;
      return { turn, usageMultiplier };
    }),
  );
  const sessionConfigText = formatMessageSessionConfig(message.metadata, turn?.metadata);
  const handleCopy = async () => {
    await copy(message.content);
  };

  return (
    <div
      data-testid="message-actions"
      className={cn(
        "mt-2 flex items-center gap-2 opacity-100 transition-opacity focus-within:opacity-100",
        { "opacity-0 group-hover:opacity-100": compactNavigation },
      )}
    >
      {showCopy && <CopyButton copied={copied} onCopy={handleCopy} />}
      {showRawToggle && onToggleRaw && (
        <RawToggleButton
          isRawView={isRawView}
          onToggleRaw={onToggleRaw}
          hasHiddenPrompts={hasHiddenPrompts}
        />
      )}
      <MessageDebugDialog message={message} turn={turn} usageMultiplier={usageMultiplier} />
      {message.author_type === "user" && navigation && (
        <>
          <NavigationButton
            direction="previous"
            disabled={navigation.isBusy || !navigation.canNavigatePrevious}
            isBusy={navigation.isBusy}
            compact={compactNavigation}
            onClick={navigation.onPrevious}
          />
          <NavigationButton
            direction="next"
            disabled={navigation.isBusy || !navigation.canNavigateNext}
            isBusy={navigation.isBusy}
            compact={compactNavigation}
            onClick={navigation.onNext}
          />
        </>
      )}
      <MessageMetaInfo
        showModel={showModel}
        sessionConfigText={sessionConfigText}
        showTimestamp={showTimestamp}
        createdAt={message.created_at}
      />
    </div>
  );
}
