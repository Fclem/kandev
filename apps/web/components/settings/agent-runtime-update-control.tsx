"use client";
import { t } from "@/lib/i18n";
import { useTranslation } from "react-i18next";
import { IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { AgentUpdateJob, InstallJob } from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";

const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "refreshing",
]);

function updatePhase(status: AgentUpdateJob["status"] | undefined): string | null {
  switch (status) {
    case "queued":
      return t("settings:updateQueued2");
    case "resolving":
      return t("settings:checkingLatestVersion");
    case "updating":
      return t("settings:updatingRuntime");
    case "refreshing":
      return t("settings:refreshingModels");
    default:
      return null;
  }
}

function updateButtonLabel(status: AgentUpdateJob["status"] | undefined) {
  switch (status) {
    case "queued":
      return t("settings:updateQueued");
    case "resolving":
      return t("settings:resolvingUpdate");
    case "updating":
      return t("settings:updatingAgent");
    case "refreshing":
      return t("settings:refreshingAgentCapabilities");
    case "failed":
      return t("settings:retryUpdate");
    default:
      return t("settings:updateAgent");
  }
}

function VersionSummary({
  runtimeUpdate,
  job,
}: {
  runtimeUpdate: RuntimeUpdate;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  const current = job?.current_version || runtimeUpdate.current_version || t("settings:unknown");
  if (job?.target_version) {
    return <p className="break-words font-mono text-xs">{`${current} → ${job.target_version}`}</p>;
  }
  if (job?.status === "resolving") {
    return (
      <p className="break-words font-mono text-xs">{t("settings:checkingLatest", { current })}</p>
    );
  }
  return (
    <p className="break-words text-xs text-muted-foreground">
      {t("settings:currentVersion2", { current })}
    </p>
  );
}

function UpdateResult({ agentName, job }: { agentName: string; job?: AgentUpdateJob }) {
  const { t } = useTranslation();
  if (!job) return null;
  if (job.status === "succeeded" && job.refresh_error) {
    return (
      <p
        className="break-words text-xs text-amber-600 dark:text-amber-400"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        {t("settings:runtimeUpdatedButCapabilitiesCouldNot", { refresh_error: job.refresh_error })}
      </p>
    );
  }
  if (job.status === "succeeded") {
    return (
      <p
        className="break-words text-xs text-green-600 dark:text-green-400"
        role="status"
        data-testid={`agent-update-result-${agentName}`}
      >
        {t("settings:runtimeUpdatedSuccessfully")}
      </p>
    );
  }
  if (job.status === "failed") {
    return (
      <p
        className="break-words text-xs text-destructive"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        {job.error || t("settings:theRuntimeUpdateFailed")}
      </p>
    );
  }
  return null;
}

export function AgentRuntimeUpdateControl({
  agentName,
  runtimeUpdate,
  job,
  installJob,
  onUpdate,
}: {
  agentName: string;
  runtimeUpdate: RuntimeUpdate;
  job?: AgentUpdateJob;
  installJob?: InstallJob;
  onUpdate: (agentName: string) => void;
}) {
  const { t } = useTranslation();
  if (!runtimeUpdate.supported) return null;

  const updateInFlight = Boolean(job && ACTIVE_UPDATE_STATUSES.has(job.status));
  const installInFlight = installJob?.status === "queued" || installJob?.status === "running";
  const disabled = updateInFlight || installInFlight;
  const phase = updatePhase(job?.status);
  const buttonLabel = updateButtonLabel(job?.status);

  return (
    <div className="min-w-0 space-y-2" data-testid={`agent-update-control-${agentName}`}>
      <VersionSummary runtimeUpdate={runtimeUpdate} job={job} />
      {phase && (
        <p
          className="flex items-center gap-1.5 text-xs text-muted-foreground"
          role="status"
          data-testid={`agent-update-phase-${agentName}`}
        >
          <IconLoader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
          <span className="min-w-0 break-words">{phase}</span>
        </p>
      )}
      {installInFlight && (
        <p className="break-words text-xs text-muted-foreground">
          {t("settings:agentInstallationIsInProgress")}
        </p>
      )}
      {job?.output && (
        <pre
          data-testid={`agent-update-log-${agentName}`}
          className="max-h-40 overflow-x-hidden overflow-y-auto whitespace-pre-wrap break-words rounded-md bg-muted px-2 py-1.5 font-mono text-xs text-muted-foreground"
        >
          {job.output}
        </pre>
      )}
      <UpdateResult agentName={agentName} job={job} />
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="min-h-11 w-full cursor-pointer whitespace-normal"
        disabled={disabled}
        aria-label={buttonLabel}
        onClick={() => onUpdate(agentName)}
        data-testid={`agent-update-button-${agentName}`}
      >
        {updateInFlight ? (
          <IconLoader2 className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <IconRefresh className="mr-2 h-4 w-4" />
        )}
        {buttonLabel}
      </Button>
    </div>
  );
}
