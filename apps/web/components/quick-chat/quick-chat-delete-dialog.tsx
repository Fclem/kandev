"use client";
import { useTranslation } from "react-i18next";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";

type QuickChatDeleteDialogProps = {
  sessionToDelete: string | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function QuickChatDeleteDialog({
  sessionToDelete,
  onOpenChange,
  onConfirm,
}: QuickChatDeleteDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={!!sessionToDelete} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("chat:deleteQuickChat")}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>{t("chat:thisWillPermanentlyDeleteThisQuick")}</p>
              <ul className="list-disc list-inside mt-2 space-y-1">
                <li>{t("chat:allConversationHistory")}</li>
                <li>{t("chat:theTaskAndItsData")}</li>
                <li>{t("chat:theAssociatedWorktree")}</li>
              </ul>
              <p className="mt-2">{t("common:thisActionCannotBeUndone")}</p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {t("common:delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
