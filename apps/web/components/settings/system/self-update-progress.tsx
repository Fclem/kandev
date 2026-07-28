"use client";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
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
  const version = target ?? t("settings:theNewVersion");
  switch (phase) {
    case "starting":
      return t("settings:startingUpdateTo", { version });
    case "installing":
      return t("settings:downloadingAndInstalling", { version });
    case "restarting":
      return t("settings:restartingKandevThisCanTakeUp");
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
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2 text-sm">
        <Spinner className="size-4" />
        <span>{activeText(phase, targetVersion)}</span>
      </div>
      <p className="text-xs text-muted-foreground">{t("settings:keepThisPageOpenItWill")}</p>
    </div>
  );
}

function DoneRow({ targetVersion }: { targetVersion: string | null }) {
  const { t } = useTranslation();
  const version = targetVersion ?? t("settings:theLatestVersion");
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-center gap-2 text-sm">
        <IconCheck className="size-4 text-emerald-500" />
        <span>{t("settings:updatedTo", { version })}</span>
      </div>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer self-start sm:self-auto"
        onClick={() => window.location.reload()}
        data-testid="system-updates-progress-reload"
      >
        {t("settings:reloadPage")}
      </Button>
    </div>
  );
}

function ErrorRow({ message, onDismiss }: { message: string | null; onDismiss: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-start gap-2 text-sm text-destructive">
        <IconAlertTriangle className="size-4 shrink-0" />
        <span>{message ?? t("settings:theUpdateFailed")}</span>
      </div>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer self-start sm:self-auto"
        onClick={onDismiss}
        data-testid="system-updates-progress-dismiss"
      >
        {t("common:dismiss")}
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
