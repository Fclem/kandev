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

const MAX_VISIBLE_FILES = 20;

export type DiscardLocalChangesDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dirtyFiles: string[];
  repoPath?: string;
  onConfirm: () => void;
  onCancel: () => void;
};

export function DiscardLocalChangesDialog({
  open,
  onOpenChange,
  dirtyFiles,
  repoPath,
  onConfirm,
  onCancel,
}: DiscardLocalChangesDialogProps) {
  const { t } = useTranslation();
  const visible = dirtyFiles.slice(0, MAX_VISIBLE_FILES);
  const overflow = dirtyFiles.length - visible.length;
  const target = repoPath
    ? t("common:inYourLocalCloneAt", { repoPath })
    : t("common:inYourLocalClone");

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent
        data-testid="discard-local-changes-dialog"
        onClick={(e) => e.stopPropagation()}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{t("common:discardLocalChanges")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("common:startingThisTaskWillPermanentlyDiscard", { target })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <ul
          className="max-h-48 overflow-auto rounded-md border border-border bg-muted/40 p-2 text-xs font-mono text-muted-foreground space-y-0.5"
          data-testid="discard-local-changes-files"
        >
          {visible.map((path) => (
            <li key={path} className="truncate" title={path}>
              {path}
            </li>
          ))}
          {overflow > 0 && (
            <li
              className="pt-1 text-[11px] italic text-muted-foreground/80"
              data-testid="discard-local-changes-overflow"
            >
              {t("common:more", { overflow })}
            </li>
          )}
        </ul>
        <AlertDialogFooter>
          <AlertDialogCancel
            className="cursor-pointer"
            data-testid="discard-local-changes-cancel"
            onClick={onCancel}
          >
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            data-testid="discard-local-changes-confirm"
            onClick={onConfirm}
          >
            {t("common:discardAndContinue")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
