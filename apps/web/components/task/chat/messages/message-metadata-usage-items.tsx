import { useTranslation } from "react-i18next";
import { useSessionUsageTurn } from "@/hooks/domains/session/use-session-usage-turns";
import { deriveTurnMetrics } from "@/lib/turn-metrics";
import { formatUsageCost, formatUsageTokens } from "@/lib/utils/conversation-usage";
import type { MessageMetadataField } from "@/lib/message-metadata-fields";
import type { Turn } from "@/lib/types/http";

const USAGE_TEXT = "min-w-0 max-w-full break-words text-[10px] text-muted-foreground/60 font-mono";

export function MessageMetadataUsageItems({
  taskId,
  sessionId,
  turn,
  fields,
}: {
  taskId: string;
  sessionId: string;
  turn: Turn;
  fields: readonly MessageMetadataField[];
}) {
  const { t, i18n } = useTranslation();
  const { usageTurn } = useSessionUsageTurn(taskId, sessionId, turn.id, {
    fallbackToSingleTurn: true,
  });
  const metrics = deriveTurnMetrics(turn, usageTurn);
  const tokenBreakdown = [
    metrics.inputTokens === null
      ? null
      : t("task:turnMetrics.tokenInput", { value: formatUsageTokens(metrics.inputTokens) }),
    metrics.outputTokens === null
      ? null
      : t("task:turnMetrics.tokenOutput", { value: formatUsageTokens(metrics.outputTokens) }),
    metrics.cachedReadTokens === null
      ? null
      : t("task:turnMetrics.tokenCached", { value: formatUsageTokens(metrics.cachedReadTokens) }),
  ]
    .filter((value): value is string => value !== null)
    .join(" · ");
  const cost = formatUsageCost(metrics.costSubcents ?? undefined);
  const source = metrics.costSources
    .map((value) => {
      if (value === "models_dev_list") return t("task:conversationUsage.estimatedCost");
      if (value === "provider_reported") return t("task:conversationUsage.providerReportedCost");
      return null;
    })
    .filter((value): value is string => value !== null)
    .join(", ");
  const rateFormatter = new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 });

  return (
    <>
      {fields.includes("tps") && metrics.tokensPerSecond !== null && (
        <span
          className={USAGE_TEXT}
          data-testid="message-metadata-tps"
          title={t("task:turnMetrics.tokensPerSecondHelp")}
        >
          {t("task:turnMetrics.rateValue", {
            value: rateFormatter.format(metrics.tokensPerSecond),
          })}
        </span>
      )}
      {fields.includes("tokens") && tokenBreakdown && (
        <span className={USAGE_TEXT} data-testid="message-metadata-tokens">
          {tokenBreakdown}
        </span>
      )}
      {fields.includes("cost") && cost && (
        <span className={USAGE_TEXT} data-testid="message-metadata-cost">
          {source ? t("task:turnMetrics.costWithSource", { cost, source }) : cost}
        </span>
      )}
    </>
  );
}
