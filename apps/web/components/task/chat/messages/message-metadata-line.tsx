import { useState } from "react";
import { IconHourglass } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { formatMessageTime } from "@/lib/i18n/message-time";
import { formatPromptDuration } from "@/lib/turn-duration";
import type { MessageMetadataField } from "@/lib/message-metadata-fields";
import type { Message, Turn } from "@/lib/types/http";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { MessageMetadataTurnItems } from "@/components/task/chat/messages/message-metadata-turn-items";

type MessageMetadataLineProps = {
  message: Message;
  turn: Turn | null;
  fields: MessageMetadataField[];
  showModel: boolean;
  sessionConfigText: string | null;
  showTimestamp: boolean;
  createdAt: string;
  durationSeconds: number | null;
  isLastQualifyingMessage: boolean;
};

function MessageTimestamp({ createdAt }: { createdAt: string }) {
  const { t } = useTranslation();
  const messageTimeDisplay = useAppStore((state) => state.userSettings.messageTimeDisplay);
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const formatted = formatMessageTime(createdAt, messageTimeDisplay);
  if (!formatted) return null;
  const timeEl = (
    <time
      data-testid="message-metadata-timestamp"
      dateTime={createdAt}
      title={formatted.counterpart}
      aria-label={t("task:messageTimestampAriaLabel", {
        label: formatted.label,
        counterpart: formatted.counterpart,
      })}
      className="min-w-0 max-w-full break-words text-[10px] text-muted-foreground/60 font-mono"
    >
      {formatted.label}
    </time>
  );

  if (!usesTouchDrawer) return timeEl;
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <button
          type="button"
          data-testid="message-timestamp-trigger"
          className="min-w-0 cursor-pointer border-0 bg-transparent p-0 text-left max-md:min-h-11 max-md:min-w-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={t("task:messageTimestampAriaLabel", {
            label: formatted.label,
            counterpart: formatted.counterpart,
          })}
        >
          {timeEl}
        </button>
      </DrawerTrigger>
      <DrawerContent data-testid="message-timestamp-drawer">
        <DrawerHeader>
          <DrawerTitle>{t("task:messageTime")}</DrawerTitle>
          <DrawerDescription>{formatted.counterpart}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

function MessageMetadataBaseItems({
  fields,
  showModel,
  sessionConfigText,
  showTimestamp,
  createdAt,
  durationSeconds,
}: Pick<
  MessageMetadataLineProps,
  "fields" | "showModel" | "sessionConfigText" | "showTimestamp" | "createdAt" | "durationSeconds"
>) {
  const { t } = useTranslation();
  return (
    <>
      {fields.includes("session_config") && showModel && sessionConfigText && (
        <span
          data-testid="message-metadata-session-config"
          className="min-w-0 truncate text-[10px] text-muted-foreground/60 font-mono"
        >
          {sessionConfigText}
        </span>
      )}
      {fields.includes("timestamp") && showTimestamp && <MessageTimestamp createdAt={createdAt} />}
      {fields.includes("duration") && durationSeconds !== null && (
        <span
          data-testid="message-turn-duration"
          className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-[10px] font-mono text-muted-foreground/60"
        >
          <IconHourglass className="h-3 w-3 shrink-0" aria-hidden="true" />
          {formatPromptDuration(durationSeconds, {
            s: t("task:durationUnitSeconds"),
            m: t("task:durationUnitMinutes"),
            h: t("task:durationUnitHours"),
          })}
        </span>
      )}
    </>
  );
}

export function MessageMetadataLine(props: MessageMetadataLineProps) {
  return (
    <>
      <MessageMetadataBaseItems {...props} />
      <MessageMetadataTurnItems
        message={props.message}
        turn={props.turn}
        fields={props.fields}
        isLastQualifyingMessage={props.isLastQualifyingMessage}
      />
    </>
  );
}
