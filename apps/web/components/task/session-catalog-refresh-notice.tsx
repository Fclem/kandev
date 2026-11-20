"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { sameSessionCatalogOwner } from "@/lib/types/session-catalog";
import type { SessionCatalogDiscovery, SessionCatalogRecord } from "@/lib/types/session-catalog";
import { useSessionCatalogRefresh } from "@/hooks/domains/session/use-session-catalog-refresh";
import { SessionCatalogRefreshConfirmation } from "./session-catalog-refresh-confirmation";

type Props = { sessionId: string | null };

export function SessionCatalogRefreshNotice({ sessionId }: Props) {
  const { t } = useTranslation();
  const refresh = useSessionCatalogRefresh(sessionId);
  const record = refresh.entry?.record;
  const profileId = refresh.session?.execution_profile_id || refresh.session?.agent_profile_id;
  if (!record || (profileId && profileId !== record.profile_id)) return null;
  const result = sameSessionCatalogOwner(record.owner, refresh.sourceOwner)
    ? record.result
    : undefined;
  const cleared = record.disposition === "cleared";
  if (cleared && !result) return null;
  const key = catalogNoticeMessageKey(refresh, result, record.disposition);
  return (
    <aside
      aria-label={t("task:sessionCatalogNoticeLabel")}
      data-testid="session-catalog-refresh-notice"
      className="mx-2 flex min-w-0 flex-wrap items-center gap-2 py-1 text-xs"
    >
      <p
        role="status"
        aria-live="polite"
        className="min-w-0 basis-60 grow break-words text-muted-foreground"
      >
        {t(key)}
      </p>
      {!cleared && (
        <Button
          ref={refresh.anchorRef}
          type="button"
          variant="outline"
          size="sm"
          className="h-auto min-h-11 min-w-11 max-w-full whitespace-normal text-xs md:min-h-7 [@media(pointer:coarse)]:min-h-11"
          disabled={!refresh.canRefresh}
          onClick={refresh.requestRefresh}
        >
          {t("task:sessionCatalogRefreshAction")}
        </Button>
      )}
      <SessionCatalogRefreshConfirmation
        target={refresh.target}
        anchorRef={refresh.anchorRef}
        onDismiss={refresh.dismiss}
        onConfirm={refresh.confirm}
      />
    </aside>
  );
}

function catalogNoticeMessageKey(
  refresh: ReturnType<typeof useSessionCatalogRefresh>,
  result: SessionCatalogDiscovery | undefined,
  disposition: SessionCatalogRecord["disposition"],
) {
  if (refresh.isRestarting) return "task:sessionCatalogRestarting";
  if (refresh.isChecking) return "task:sessionCatalogChecking";
  if (refresh.entry?.checking?.status === "unavailable" || result?.outcome === "failed")
    return "task:sessionCatalogUnavailable";
  if (result?.outcome === "unsupported") return "task:sessionCatalogUnsupported";
  if (disposition === "cleared") return "task:sessionCatalogChecked";
  if (disposition === "host_candidates") return "task:sessionCatalogNoticeHost";
  return "task:sessionCatalogNoticeExecutor";
}
