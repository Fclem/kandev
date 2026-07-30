"use client";
import { Trans, useTranslation } from "react-i18next";

import { IconDownload, IconPlus, IconUpload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { WorkflowSyncButton } from "@/components/settings/workflow-sync-section";

type WorkflowSectionActionsProps = {
  onExport: () => void;
  onImport: () => void;
  onAdd: () => void;
  onGitHubSync: () => void;
};

// WorkflowSectionActions is the toolbar of the Workflows settings section:
// GitHub Sync, Export All, Import, and Add Workflow.
export function WorkflowSectionActions({
  onExport,
  onImport,
  onAdd,
  onGitHubSync,
}: WorkflowSectionActionsProps) {
  const { t } = useTranslation();
  return (
    // sm:justify-end keeps wrapped rows right-aligned next to the section
    // title; below sm the toolbar sits under the title, left-aligned.
    <div className="flex flex-wrap gap-2 sm:justify-end">
      <WorkflowSyncButton onClick={onGitHubSync} />
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={onExport}
        className="cursor-pointer"
      >
        <Trans i18nKey="settings:exportAll">
          <IconDownload className="h-4 w-4 mr-2" />
          {t("settings:exportAll2")}
        </Trans>
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={onImport}
        className="cursor-pointer"
      >
        <Trans i18nKey="settings:import2">
          <IconUpload className="h-4 w-4 mr-2" />
          {t("settings:import")}
        </Trans>
      </Button>
      <Button
        type="button"
        size="sm"
        onClick={onAdd}
        className="cursor-pointer"
        data-testid="add-workflow-button"
      >
        <Trans i18nKey="settings:addWorkflow2">
          <IconPlus className="h-4 w-4 mr-2" />
          {t("settings:addWorkflow")}
        </Trans>
      </Button>
    </div>
  );
}
