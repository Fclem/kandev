import { useTranslation } from "react-i18next";
import { deriveTurnMetrics } from "@/lib/turn-metrics";
import type { MessageMetadataField } from "@/lib/message-metadata-fields";
import type { Message, Turn } from "@/lib/types/http";
import { MessageMetadataUsageItems } from "@/components/task/chat/messages/message-metadata-usage-items";
import { formatTtft } from "@/lib/turn-duration";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

const USAGE_FIELDS: readonly MessageMetadataField[] = ["tps", "tokens", "cost"];
const TURN_FIELDS: readonly MessageMetadataField[] = ["ttft", ...USAGE_FIELDS];
const METADATA_TEXT =
  "min-w-0 max-w-full break-words text-[10px] text-muted-foreground/60 font-mono";

function hasSelectedField(
  fields: readonly MessageMetadataField[],
  candidates: readonly MessageMetadataField[],
) {
  return candidates.some((field) => fields.includes(field));
}

export function MessageMetadataTurnItems({
  message,
  turn,
  fields,
  isLastQualifyingMessage,
}: {
  message: Message;
  turn: Turn | null;
  fields: readonly MessageMetadataField[];
  isLastQualifyingMessage: boolean;
}) {
  const { t } = useTranslation();
  const showTurnItems = Boolean(
    isLastQualifyingMessage && turn?.completed_at && hasSelectedField(fields, TURN_FIELDS),
  );
  const showUsageItems = showTurnItems && hasSelectedField(fields, USAGE_FIELDS);
  const ttftMs = turn ? deriveTurnMetrics(turn, null).ttftMs : null;

  return (
    <>
      {showTurnItems && fields.includes("ttft") && turn && ttftMs !== null && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span tabIndex={0} className={METADATA_TEXT} data-testid="message-metadata-ttft">
              {t("task:turnMetrics.ttft")}:{" "}
              {formatTtft(ttftMs, {
                s: t("task:durationUnitSeconds"),
                m: t("task:durationUnitMinutes"),
                h: t("task:durationUnitHours"),
              })}
            </span>
          </TooltipTrigger>
          <TooltipContent>{t("task:turnMetrics.ttftHelp")}</TooltipContent>
        </Tooltip>
      )}
      {showUsageItems && message.task_id && message.session_id && turn && (
        <MessageMetadataUsageItems
          taskId={message.task_id}
          sessionId={message.session_id}
          turn={turn}
          fields={fields}
        />
      )}
    </>
  );
}
