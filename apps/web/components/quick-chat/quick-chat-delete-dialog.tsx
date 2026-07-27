"use client";

import { Trans } from "@lingui/react/macro";
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
  return (
    <AlertDialog open={!!sessionToDelete} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            <Trans>Delete Quick Chat?</Trans>
          </AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>
                <Trans>This will permanently delete this quick chat session, including:</Trans>
              </p>
              <ul className="list-disc list-inside mt-2 space-y-1">
                <li>
                  <Trans>All conversation history</Trans>
                </li>
                <li>
                  <Trans>The task and its data</Trans>
                </li>
                <li>
                  <Trans>The associated worktree</Trans>
                </li>
              </ul>
              <p className="mt-2">
                <Trans>This action cannot be undone.</Trans>
              </p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">
            <Trans>Cancel</Trans>
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            <Trans>Delete</Trans>
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
