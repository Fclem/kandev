"use client";

import { Trans, useLingui } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { IconCheck, IconAlertTriangle } from "@tabler/icons-react";
import type { SelfUpdatePhase } from "@/hooks/domains/system/use-self-update";

type SelfUpdateProgressProps = {
  phase: SelfUpdatePhase;
  targetVersion: string | null;
  errorMessage: string | null;
  onDismiss: () => void;
};

function activeText(phase: SelfUpdatePhase, target: string | null): string {
  const version = target ?? t`the new version`;
  switch (phase) {
    case "starting":
      return t`Starting update to ${version}…`;
    case "installing":
      return t`Downloading and installing ${version}…`;
    case "restarting":
      return t`Restarting Kandev — this can take up to a minute.`;
    default:
      return "";
  }
}

function ActiveRow({
  phase,
  targetVersion,
}: {
  phase: SelfUpdatePhase;
  targetVersion: string | null;
}) {
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2 text-sm">
        <Spinner className="size-4" />
        <span>{activeText(phase, targetVersion)}</span>
      </div>
      <p className="text-xs text-muted-foreground">
        <Trans>Keep this page open. It will refresh automatically when the update finishes.</Trans>
      </p>
    </div>
  );
}

function DoneRow({ targetVersion }: { targetVersion: string | null }) {
  const { t } = useLingui();
  const version = targetVersion ?? t`the latest version`;
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-center gap-2 text-sm">
        <IconCheck className="size-4 text-emerald-500" />
        <span>
          <Trans>Updated to {version}.</Trans>
        </span>
      </div>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer self-start sm:self-auto"
        onClick={() => window.location.reload()}
        data-testid="system-updates-progress-reload"
      >
        <Trans>Reload page</Trans>
      </Button>
    </div>
  );
}

function ErrorRow({ message, onDismiss }: { message: string | null; onDismiss: () => void }) {
  const { t } = useLingui();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-start gap-2 text-sm text-destructive">
        <IconAlertTriangle className="size-4 shrink-0" />
        <span>{message ?? t`The update failed.`}</span>
      </div>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer self-start sm:self-auto"
        onClick={onDismiss}
        data-testid="system-updates-progress-dismiss"
      >
        <Trans>Dismiss</Trans>
      </Button>
    </div>
  );
}

function ProgressBody({ phase, targetVersion, errorMessage, onDismiss }: SelfUpdateProgressProps) {
  if (phase === "done") return <DoneRow targetVersion={targetVersion} />;
  if (phase === "error") return <ErrorRow message={errorMessage} onDismiss={onDismiss} />;
  return <ActiveRow phase={phase} targetVersion={targetVersion} />;
}

export function SelfUpdateProgress(props: SelfUpdateProgressProps) {
  if (props.phase === "idle") return null;
  return (
    <div
      className="rounded-md border bg-muted/30 px-3 py-2"
      data-testid="system-updates-progress"
      data-phase={props.phase}
    >
      <ProgressBody {...props} />
    </div>
  );
}
