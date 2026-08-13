"use client";

import { useTranslation } from "react-i18next";

/**
 * The composer's "a message will auto-start the agent" hint for
 * recovered-idle (resume-skipped) sessions, shown while the agent is
 * stopped and no other affordance owns the surface. Informational only:
 * no click action.
 */
export function ComposerAgentStartHint({ show }: { show: boolean }) {
  const { t } = useTranslation();
  if (!show) return null;
  return (
    <p data-testid="composer-agent-start-hint" className="px-1 pb-1 text-xs text-muted-foreground">
      {t("task:composerStartAgentHint")}
    </p>
  );
}
