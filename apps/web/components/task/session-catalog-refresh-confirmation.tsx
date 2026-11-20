"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import type { SessionCatalogRefreshTarget } from "@/hooks/domains/session/use-session-catalog-refresh";

type Props = {
  target: SessionCatalogRefreshTarget | null;
  anchorRef: RefObject<HTMLButtonElement | null>;
  onDismiss: () => void;
  onConfirm: () => Promise<void>;
};

export function SessionCatalogRefreshConfirmation({
  target,
  anchorRef,
  onDismiss,
  onConfirm,
}: Props) {
  const { t } = useTranslation();
  const title = t("task:sessionCatalogRefreshTitle");
  const description = t("task:sessionCatalogRefreshConsent");
  const confirmLabel = t("task:sessionCatalogRefreshAction");
  const onOpenChange = (open: boolean) => {
    if (!open) onDismiss();
  };
  return (
    <MobileActionConfirmation
      open={!!target}
      focusReturnRef={anchorRef}
      testId="session-catalog-refresh-confirmation"
      targetKey={target ? `${target.taskId}:${target.sessionId}:${target.noticeRevision}` : ""}
      title={title}
      description={description}
      confirmLabel={confirmLabel}
      cancelLabel={t("common:cancel")}
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
      useMobileSurfaceForCoarsePointer
      fallback={
        <ActionConfirmPopover
          open={!!target}
          anchorRef={anchorRef}
          focusReturnRef={anchorRef}
          title={title}
          description={description}
          confirmLabel={confirmLabel}
          cancelLabel={t("common:cancel")}
          onOpenChange={onOpenChange}
          onConfirm={onConfirm}
          confirmationBoundary
          testId="session-catalog-refresh-confirmation"
        />
      }
    />
  );
}
