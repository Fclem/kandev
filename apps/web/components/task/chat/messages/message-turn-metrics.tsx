import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { formatPromptDuration, formatTtft } from "@/lib/turn-duration";
import { useSessionUsageTurn } from "@/hooks/domains/session/use-session-usage-turns";
import type { Turn } from "@/lib/types/http";
import { deriveTurnMetrics, type TurnMetrics } from "@/lib/turn-metrics";
import { formatUsageCost, formatUsageTokens } from "@/lib/utils/conversation-usage";
import { SettingsInfo } from "@/components/settings/settings-info";

type MessageTurnMetricsProps = {
  turn: Turn;
  taskId: string;
  sessionId: string;
};

function MetricRow({
  label,
  value,
  help,
  info,
}: {
  label: string;
  value: string | null;
  help?: string;
  info?: string;
}) {
  if (value === null) return null;
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-x-3 text-sm">
      <dt className="min-w-0 flex items-center text-muted-foreground break-words">
        {label}
        {info && (
          <SettingsInfo label={label} testId="message-ttft-info">
            {info}
          </SettingsInfo>
        )}
      </dt>
      <dd className="shrink-0 font-medium tabular-nums text-right">{value}</dd>
      {help && <dd className="order-last basis-full text-xs text-muted-foreground">{help}</dd>}
    </div>
  );
}

function durationLabel(milliseconds: number, units: { s: string; m: string; h: string }): string {
  if (milliseconds < 1000) return `${Math.round(milliseconds)} ms`;
  return formatPromptDuration(milliseconds / 1000, units);
}

function costSourceLabel(source: string, t: TFunction): string {
  if (source === "models_dev_list") return t("task:conversationUsage.estimatedCost");
  if (source === "provider_reported") return t("task:conversationUsage.providerReportedCost");
  return t("task:conversationUsage.costUnavailable");
}

function MetricRows({
  metrics,
  t,
  units,
  rateFormatter,
}: {
  metrics: TurnMetrics;
  t: TFunction;
  units: { s: string; m: string; h: string };
  rateFormatter: Intl.NumberFormat;
}) {
  const cost = formatUsageCost(metrics.costSubcents ?? undefined);
  return (
    <dl className="space-y-1">
      {metrics.wallTimeMs !== null && (
        <MetricRow
          label={t("task:turnMetrics.wallTime")}
          value={durationLabel(metrics.wallTimeMs, units)}
        />
      )}
      {metrics.ttftMs !== null && (
        <MetricRow
          label={t("task:turnMetrics.ttft")}
          value={formatTtft(metrics.ttftMs, units)}
          info={t("task:turnMetrics.ttftHelp")}
        />
      )}
      {metrics.tokensPerSecond !== null && (
        <MetricRow
          label={t("task:turnMetrics.tokensPerSecond")}
          value={rateFormatter.format(metrics.tokensPerSecond)}
          help={t("task:turnMetrics.tokensPerSecondHelp")}
        />
      )}
      {metrics.inputTokens !== null && (
        <MetricRow
          label={t("task:conversationUsage.input")}
          value={formatUsageTokens(metrics.inputTokens)}
        />
      )}
      {metrics.outputTokens !== null && (
        <MetricRow
          label={t("task:conversationUsage.output")}
          value={formatUsageTokens(metrics.outputTokens)}
        />
      )}
      {metrics.cachedReadTokens !== null && (
        <MetricRow
          label={t("task:conversationUsage.cachedRead")}
          value={formatUsageTokens(metrics.cachedReadTokens)}
        />
      )}
      {metrics.cachedWriteTokens !== null && (
        <MetricRow
          label={t("task:conversationUsage.cachedWrite")}
          value={formatUsageTokens(metrics.cachedWriteTokens)}
        />
      )}
      {metrics.lastResponseReportedCacheWriteTokens !== null && (
        <MetricRow
          label={`${t("task:conversationUsage.lastResponse")}: ${t("task:conversationUsage.cachedWrite")}`}
          value={formatUsageTokens(metrics.lastResponseReportedCacheWriteTokens)}
        />
      )}
      {metrics.reasoningTokens !== null && (
        <MetricRow
          label={t("task:conversationUsage.reasoning")}
          value={formatUsageTokens(metrics.reasoningTokens)}
        />
      )}
      {cost && <MetricRow label={t("task:conversationUsage.recordedCost")} value={cost} />}
      {cost && metrics.costSources.length > 0 && (
        <MetricRow
          label={t("task:conversationUsage.priceSource")}
          value={metrics.costSources.map((source) => costSourceLabel(source, t)).join(", ")}
        />
      )}
    </dl>
  );
}

type UsageStatus = "pending" | "unavailable" | "none" | "partial" | "estimated" | null;

function getUsageStatus({
  usageTurn,
  isLoading,
  isUnavailable,
  isNotFound,
  completedAt,
}: {
  usageTurn: ReturnType<typeof useSessionUsageTurn>["usageTurn"];
  isLoading: boolean;
  isUnavailable: boolean;
  isNotFound: boolean;
  completedAt: Turn["completed_at"];
}): UsageStatus {
  if (usageTurn) {
    if (usageTurn.completeness === "partial") {
      return usageTurn.total.event_count === 0 ? "unavailable" : "partial";
    }
    if (usageTurn.completeness === "estimated") return "estimated";
    return usageTurn.total.event_count === 0 ? "none" : null;
  }
  if (isUnavailable) return "unavailable";
  if (isLoading || !completedAt) return "pending";
  return isNotFound ? "none" : null;
}

function UsageNotice({ status, t }: { status: UsageStatus; t: TFunction }) {
  if (!status) return null;
  const labels = {
    pending: t("task:conversationUsage.pending"),
    unavailable: t("task:turnMetrics.usageUnavailable"),
    none: t("task:turnMetrics.noUsage"),
    partial: t("task:conversationUsage.partial"),
    estimated: t("task:conversationUsage.estimated"),
  };
  return (
    <p className="text-xs text-muted-foreground" role="status">
      {labels[status]}
    </p>
  );
}

export function MessageTurnMetrics({ turn, taskId, sessionId }: MessageTurnMetricsProps) {
  const { t, i18n } = useTranslation();
  const usageState = useSessionUsageTurn(taskId, sessionId, turn.completed_at ? turn.id : null, {
    fallbackToSingleTurn: true,
  });
  const metrics = deriveTurnMetrics(turn, usageState.usageTurn);
  const units = {
    s: t("task:durationUnitSeconds"),
    m: t("task:durationUnitMinutes"),
    h: t("task:durationUnitHours"),
  };
  const status = getUsageStatus({
    usageTurn: usageState.usageTurn,
    isLoading: usageState.isLoading,
    isUnavailable: usageState.isUnavailable,
    isNotFound: usageState.isNotFound,
    completedAt: turn.completed_at,
  });

  return (
    <section className="space-y-2 border-b pb-3" data-testid="message-turn-metrics">
      <h3 className="text-xs font-semibold text-muted-foreground">{t("task:turnMetrics.title")}</h3>
      <MetricRows
        metrics={metrics}
        t={t}
        units={units}
        rateFormatter={new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 2 })}
      />
      <UsageNotice status={status} t={t} />
    </section>
  );
}
