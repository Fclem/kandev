"use client";

import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import type { KandevRestartPhase } from "@/hooks/domains/system/use-kandev-restart";

type RestartProgressDialogProps = {
  phase: KandevRestartPhase;
  errorMessage: string | null;
  onDismiss: () => void;
};

export function RestartProgressDialog({
  phase,
  errorMessage,
  onDismiss,
}: RestartProgressDialogProps) {
  if (phase === "idle") return null;
  const done = phase === "done";
  const failed = phase === "error";
  return (
    <Dialog open onOpenChange={(open) => !open && (done || failed) && onDismiss()}>
      <DialogContent
        className="sm:max-w-md"
        data-testid="restart-progress-dialog"
        data-phase={phase}
      >
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <RestartStatusIcon phase={phase} />
            {restartTitle(phase)}
          </DialogTitle>
          <DialogDescription>{restartDescription(phase, errorMessage)}</DialogDescription>
        </DialogHeader>
        {(done || failed) && (
          <DialogFooter>
            <Button
              variant={failed ? "outline" : "default"}
              className="w-full cursor-pointer sm:w-auto"
              onClick={done ? () => window.location.reload() : onDismiss}
            >
              {done ? <Trans>Reload page</Trans> : <Trans>Dismiss</Trans>}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}

function RestartStatusIcon({ phase }: { phase: KandevRestartPhase }) {
  if (phase === "done") return <IconCheck className="size-4 text-emerald-500" />;
  if (phase === "error") return <IconAlertTriangle className="size-4 text-destructive" />;
  return <Spinner className="size-4" />;
}

function restartTitle(phase: KandevRestartPhase): string {
  switch (phase) {
    case "starting":
      return t`Requesting restart`;
    case "restarting":
      return t`Restarting Kandev`;
    case "done":
      return t`Kandev restarted`;
    case "error":
      return t`Restart failed`;
    default:
      return t`Restarting Kandev`;
  }
}

function restartDescription(phase: KandevRestartPhase, errorMessage: string | null): string {
  switch (phase) {
    case "starting":
      return t`Preparing the local supervisor restart request.`;
    case "restarting":
      return t`Kandev is stopping and starting again. This page will detect the new process automatically.`;
    case "done":
      return t`The backend is running again. Reload the page to reconnect with the latest feature toggle state.`;
    case "error":
      return errorMessage ?? t`The restart could not be completed.`;
    default:
      return "";
  }
}
