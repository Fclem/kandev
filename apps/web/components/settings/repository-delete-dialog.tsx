"use client";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  const hasActiveSessions = activeSessionCount > 0;
  const description = hasActiveSessions
    ? t("settings:repositoryUsedByActiveSessions", { count: activeSessionCount })
    : t("settings:thisWillRemoveTheRepositoryAnd");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("settings:deleteRepository")}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            {hasActiveSessions ? t("common:close") : t("common:cancel")}
          </Button>
          {!hasActiveSessions && (
            <Button
              type="button"
              variant="destructive"
              className="cursor-pointer"
              onClick={onDelete}
              disabled={deleteLoading}
            >
              {t("settings:deleteRepository2")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
