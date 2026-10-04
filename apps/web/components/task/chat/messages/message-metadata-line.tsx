import { useState } from "react";
import { IconHourglass } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { formatRelativeTime } from "@/lib/utils";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
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
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  if (parseStrictRfc3339Timestamp(createdAt) === null) return null;
  const date = new Date(createdAt);
  const absoluteTime = date.toLocaleString();
  const timeEl = (
    <time
      data-testid="message-metadata-timestamp"
      dateTime={createdAt}
      title={absoluteTime}
      className="text-[10px] text-muted-foreground/60 font-mono"
    >
      {formatRelativeTime(createdAt)}
    </time>
  );

  if (!usesTouchDrawer) return timeEl;
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <button
          type="button"
          data-testid="message-timestamp-trigger"
          className="cursor-pointer border-0 bg-transparent p-0 text-left"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={t("task:showFullTimestamp", { absoluteTime })}
        >
          {timeEl}
        </button>
      </DrawerTrigger>
      <DrawerContent data-testid="message-timestamp-drawer">
        <DrawerHeader>
          <DrawerTitle>{t("task:messageTime")}</DrawerTitle>
          <DrawerDescription>{absoluteTime}</DrawerDescription>
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
