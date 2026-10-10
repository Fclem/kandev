import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { formatDateTime } from "@/lib/i18n/formats";
import { ActionButtons } from "./action-message-actions";
import { parseRetryAt, retryCountdownLabel } from "./transient-retry";
import type { ActionMeta } from "./action-message-details";

/**
 * A provider-limit wait notice is superseded once the session no longer names
 * its wait identity, for example after the wait resumed or was cancelled.
 */
export function isSupersededLimitWaitNotice(
  metadata: ActionMeta | undefined,
  sessionMetadata: Record<string, unknown> | null | undefined,
): boolean {
  return (
    metadata?.limit_wait === true &&
    metadata.limit_wait_identity !== sessionMetadata?.provider_limit_wait_identity
  );
}

export function ProviderLimitWaitNotice({
  metadata,
  taskId,
}: {
  metadata: ActionMeta;
  taskId?: string;
}) {
  const { t } = useTranslation();
  const [now, setNow] = useState(Date.now);
  const deadline = parseRetryAt(metadata.retry_at);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, []);
  if (deadline === undefined) return null;
  const remaining = Math.max(0, deadline - now);
  return (
    <section
      data-testid="provider-limit-wait-card"
      role="status"
      aria-live="polite"
      className="flex w-full min-w-0 items-start gap-2 rounded-md border border-amber-500/25 bg-amber-500/[0.06] p-3 sm:gap-3"
    >
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" aria-hidden="true" />
      <div className="min-w-0 flex-1 space-y-1 text-xs wrap-anywhere">
        <p className="text-amber-600 dark:text-amber-400">
          {t("chat:providerQuotaTitle", {
            provider: metadata.provider_name || t("chat:providerQuotaProviderFallback"),
          })}
        </p>
        <p>{t("chat:providerQuotaModel", { model: metadata.model_id })}</p>
        <p>
          <time dateTime={metadata.retry_at}>
            {t("chat:providerQuotaReset", { resetAt: formatDateTime(new Date(deadline)) })}
          </time>
        </p>
        <p aria-label={t("chat:transientRetryCountdownLabel")}>
          {remaining > 0
            ? t("chat:transientRetryIn", { countdown: retryCountdownLabel(remaining) })
            : t("chat:transientRetryNow")}
        </p>
      </div>
      {metadata.actions && (
        <ActionButtons
          actions={metadata.actions}
          taskId={taskId}
          labelOverride={t("common:cancel")}
          compact
        />
      )}
    </section>
  );
}
