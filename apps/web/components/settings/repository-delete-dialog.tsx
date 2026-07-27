"use client";

import { Trans, useLingui } from "@lingui/react/macro";
import { plural } from "@lingui/core/macro";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";

type DeleteRepositoryDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: () => void;
  activeSessionCount: number;
  deleteLoading: boolean;
};

export function DeleteRepositoryDialog({
  open,
  onOpenChange,
  onDelete,
  activeSessionCount,
  deleteLoading,
}: DeleteRepositoryDialogProps) {
  const { t } = useLingui();
  const hasActiveSessions = activeSessionCount > 0;
  const description = hasActiveSessions
    ? plural(activeSessionCount, {
        one: "This repository is used by # active agent session. Stop or finish it before deleting the repository.",
        other:
          "This repository is used by # active agent sessions. Stop or finish them before deleting the repository.",
      })
    : t`This will remove the repository and its scripts. This action cannot be undone.`;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans>Delete repository</Trans>
          </DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            {hasActiveSessions ? t`Close` : t`Cancel`}
          </Button>
          {!hasActiveSessions && (
            <Button
              type="button"
              variant="destructive"
              className="cursor-pointer"
              onClick={onDelete}
              disabled={deleteLoading}
            >
              <Trans>Delete Repository</Trans>
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
