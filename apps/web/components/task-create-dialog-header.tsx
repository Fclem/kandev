"use client";

import { Trans, useLingui } from "@lingui/react/macro";
import { DialogTitle } from "@kandev/ui/dialog";

export type DialogHeaderContentProps = {
  isCreateMode: boolean;
  isEditMode: boolean;
  sessionRepoName?: string;
  initialTitle?: string;
};

/**
 * Header for the task-create dialog. In create/edit mode the header is
 * intentionally minimal — the dialog itself signals "create" and the
 * repo + branch chips and task-name input live in the body. Session mode
 * shows a breadcrumb (repo / task / new session).
 */
export function DialogHeaderContent({
  isCreateMode,
  isEditMode,
  sessionRepoName,
  initialTitle,
}: DialogHeaderContentProps) {
  const { t } = useLingui();
  if (isCreateMode || isEditMode) {
    return <DialogTitle className="sr-only">{isEditMode ? t`Edit task` : t`New task`}</DialogTitle>;
  }
  return (
    <DialogTitle asChild>
      <div className="flex items-center gap-1 min-w-0 text-sm font-medium">
        {sessionRepoName && (
          <>
            <span className="truncate text-muted-foreground">{sessionRepoName}</span>
            <span className="text-muted-foreground mx-0.5">/</span>
          </>
        )}
        <span className="truncate">{initialTitle || t`Task`}</span>
        <span className="text-muted-foreground mx-0.5">/</span>
        <span className="text-muted-foreground whitespace-nowrap">
          <Trans>new session</Trans>
        </span>
      </div>
    </DialogTitle>
  );
}
