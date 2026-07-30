import { Trans, useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

export function WorkflowSyncedBadge({ sourcePath }: { sourcePath?: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          tabIndex={0}
          className="text-xs cursor-default"
          data-testid="workflow-synced-badge"
        >
          {t("settings:synced")}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        <Trans
          i18nKey="settings:readOnlyManagedByWorkflowSync"
          values={{ value1: sourcePath || "a configured repository" }}
        >
          Read-only - managed by workflow sync from {sourcePath || "a configured repository"}. Edit
          or remove it in the synced repository.
        </Trans>
      </TooltipContent>
    </Tooltip>
  );
}
