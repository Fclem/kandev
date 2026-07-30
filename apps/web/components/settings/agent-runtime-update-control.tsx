"use client";

import { IconAlertTriangle, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AgentUpdateJob, AgentUpdatePreview, InstallJob } from "@/lib/api";
import type { RuntimeUpdate } from "@/lib/types/http";
import { useAgentUpdateDialogState } from "./use-agent-update-dialog-state";
import { Trans, useTranslation } from "react-i18next";

const ACTIVE_UPDATE_STATUSES = new Set<AgentUpdateJob["status"]>([
  "queued",
  "resolving",
  "updating",
  "refreshing",
]);

function updatePhase(status: AgentUpdateJob["status"] | undefined): string | null {
  switch (status) {
    case "queued":
      return "Update queued…";
    case "resolving":
      return "Checking latest version…";
    case "updating":
      return "Updating runtime…";
    case "refreshing":
      return "Refreshing agent capabilities…";
    default:
      return null;
  }
}

function UpdateResult({ agentName, job }: { agentName: string; job?: AgentUpdateJob }) {
  const { t } = useTranslation();
  if (!job) return null;
  if (job.status === "succeeded" && job.refresh_error) {
    return (
      <p
        className="break-words text-amber-600 dark:text-amber-400"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        <Trans
          i18nKey="settings:runtimeUpdatedButCapabilitiesCouldNot"
          values={{ refresh_error: job.refresh_error }}
        >
          Runtime updated, but capabilities could not be refreshed: {job.refresh_error}
        </Trans>
      </p>
    );
  }
  if (job.status === "succeeded") {
    return (
      <p
        className="break-words text-green-600 dark:text-green-400"
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
        className="break-words text-destructive"
        role="alert"
        data-testid={`agent-update-result-${agentName}`}
      >
        {job.error || t("settings:theRuntimeUpdateFailed")}
      </p>
    );
  }
  return null;
}

type UpdateBodyProps = {
  agentName: string;
  preview: AgentUpdatePreview | null;
  loading: boolean;
  previewError: string | null;
  approveError: string | null;
  job?: AgentUpdateJob;
  onRetryPreview: () => void;
};

function UpdateBody({
  agentName,
  preview,
  loading,
  previewError,
  approveError,
  job,
  onRetryPreview,
}: UpdateBodyProps) {
  const { t } = useTranslation();
  const phase = updatePhase(job?.status);
  return (
    <div
      className="max-h-[calc(80dvh-10rem)] min-h-0 space-y-4 overflow-y-auto overscroll-contain px-4 py-3 text-xs/relaxed"
      data-testid={`agent-update-dialog-body-${agentName}`}
    >
      {loading && (
        <p className="flex items-center gap-2 text-muted-foreground" role="status">
          <IconLoader2 className="size-4 animate-spin" />
          {t("settings:checkingTheLatestRuntimeVersion")}
        </p>
      )}
      {previewError && (
        <div
          className="space-y-2 rounded-md border border-destructive/30 bg-destructive/5 p-3"
          role="alert"
        >
          <p className="flex items-start gap-2 text-destructive">
            <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
            <span>{previewError}</span>
          </p>
          <Button type="button" variant="outline" size="sm" onClick={onRetryPreview}>
            {t("settings:retryVersionCheck")}
          </Button>
        </div>
      )}
      {approveError && (
        <div
          className="rounded-md border border-destructive/30 bg-destructive/5 p-3"
          role="alert"
          data-testid={`agent-update-approve-error-${agentName}`}
        >
          <p className="flex items-start gap-2 text-destructive">
            <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
            <span>
              <Trans i18nKey="settings:unableToStartUpdate" values={{ approveError }}>
                Unable to start update: {approveError}
              </Trans>
            </span>
          </p>
        </div>
      )}
      {preview && (
        <>
          <div className="space-y-1">
            <p className="font-medium">{t("settings:runtimeVersion")}</p>
            <p className="break-words font-mono text-sm">
              {(job?.current_version || preview.current_version || t("common:unknown")) +
                " → " +
                (job?.target_version || preview.target_version)}
            </p>
          </div>
          <div className="space-y-1 text-muted-foreground">
            <p>{t("settings:thisUpdatesTheManagedRuntimeOn")}</p>
            <p>{t("settings:activeSessionsKeepRunningOnlyLater")}</p>
          </div>
          <div className="space-y-1">
            <p className="font-medium">{t("settings:commandThatWillRun")}</p>
            <pre className="whitespace-pre-wrap break-all rounded-md bg-muted p-3 font-mono text-xs text-muted-foreground">
              {preview.command_string}
            </pre>
          </div>
        </>
      )}
      {phase && (
        <p
          className="flex items-center gap-1.5 text-muted-foreground"
          role="status"
          data-testid={`agent-update-phase-${agentName}`}
        >
          <IconLoader2 className="size-3.5 shrink-0 animate-spin" />
          {phase}
        </p>
      )}
      {job?.output && (
        <pre
          data-testid={`agent-update-log-${agentName}`}
          className="whitespace-pre-wrap break-words rounded-md bg-muted p-3 font-mono text-xs text-muted-foreground"
        >
          {job.output}
        </pre>
      )}
      <UpdateResult agentName={agentName} job={job} />
    </div>
  );
}

type UpdateFooterProps = {
  agentName: string;
  preview: AgentUpdatePreview | null;
  previewError: string | null;
  job?: AgentUpdateJob;
  loading: boolean;
  starting: boolean;
  installInFlight: boolean;
  onApprove: () => void;
  onClose: () => void;
  mobile?: boolean;
};

function canApproveUpdate({
  preview,
  previewError,
  loading,
  updateInFlight,
  starting,
  installInFlight,
}: {
  preview: AgentUpdatePreview | null;
  previewError: string | null;
  loading: boolean;
  updateInFlight: boolean;
  starting: boolean;
  installInFlight: boolean;
}) {
  return (
    Boolean(preview?.current_version) &&
    !previewError &&
    !loading &&
    !updateInFlight &&
    !starting &&
    !installInFlight
  );
}

function UpdateFooter({
  agentName,
  preview,
  previewError,
  job,
  loading,
  starting,
  installInFlight,
  onApprove,
  onClose,
  mobile,
}: UpdateFooterProps) {
  const { t } = useTranslation();
  const updateInFlight = Boolean(job && ACTIVE_UPDATE_STATUSES.has(job.status));
  const canRetry = job?.status === "failed";
  const canApprove = canApproveUpdate({
    preview,
    previewError,
    loading,
    updateInFlight,
    starting,
    installInFlight,
  });
  const showApprove = !job || canRetry;
  const content = (
    <>
      <Button type="button" variant="outline" onClick={onClose}>
        {job?.status === "succeeded" ? t("settings:done") : t("common:cancel")}
      </Button>
      {showApprove && (
        <Button
          type="button"
          disabled={!canApprove}
          onClick={onApprove}
          data-testid={`agent-update-confirm-${agentName}`}
        >
          {starting && <IconLoader2 className="mr-2 size-4 animate-spin" />}
          {canRetry ? t("settings:retryUpdate") : t("settings:approveUpdate")}
        </Button>
      )}
    </>
  );
  if (mobile) {
    return (
      <DrawerFooter className="border-t pb-[max(1rem,env(safe-area-inset-bottom))]">
        {content}
      </DrawerFooter>
    );
  }
  return <DialogFooter className="border-t px-4 py-3">{content}</DialogFooter>;
}

function UpdateTrigger({
  agentName,
  displayName,
  installInFlight,
  onOpen,
}: {
  agentName: string;
  displayName: string;
  installInFlight: boolean;
  onOpen: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={installInFlight ? 0 : -1} className="inline-flex">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-11 w-11 cursor-pointer active:scale-95 sm:h-7 sm:w-7"
            aria-label={`Update ${displayName}`}
            disabled={installInFlight}
            onClick={onOpen}
            data-testid={`agent-update-trigger-${agentName}`}
          >
            <IconRefresh className="size-4" />
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        {installInFlight ? t("settings:agentInstallationIsInProgress2") : `Update ${displayName}`}
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * The update dialog's shell: a bottom drawer on phones, a centered dialog
 * otherwise. Split from the control so that stays a state container.
 */
function UpdateSurface({
  agentName,
  displayName,
  isMobile,
  open,
  onOpenChange,
  body,
  footer,
}: {
  agentName: string;
  displayName: string;
  isMobile: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  body: React.ReactNode;
  footer: (mobile?: boolean) => React.ReactNode;
}) {
  const { t } = useTranslation();
  // "Update X" is one sentence, so it is one key rather than a concatenation.
  const title = t("settings:updateAgent", { agent: displayName });
  const description = t("settings:reviewTheUpdateBeforeKandevChanges");

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="max-h-[80dvh]" data-testid={`agent-update-drawer-${agentName}`}>
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
          {body}
          {footer(true)}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[80dvh] gap-0 p-0 sm:max-w-xl"
        data-testid={`agent-update-dialog-${agentName}`}
      >
        <DialogHeader className="px-4 pt-4">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {body}
        {footer()}
      </DialogContent>
    </Dialog>
  );
}

export function AgentRuntimeUpdateControl({
  agentName,
  displayName,
  runtimeUpdate,
  job,
  installJob,
  onPreview,
  onUpdate,
}: {
  agentName: string;
  displayName: string;
  runtimeUpdate: RuntimeUpdate;
  job?: AgentUpdateJob;
  installJob?: InstallJob;
  onPreview: (agentName: string) => Promise<AgentUpdatePreview>;
  onUpdate: (agentName: string) => Promise<AgentUpdateJob>;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const {
    activeJob,
    approve,
    approveError,
    handleOpenChange,
    loading,
    loadPreview,
    open,
    preview,
    previewError,
    starting,
  } = useAgentUpdateDialogState({ agentName, job, onPreview, onUpdate });
  const installInFlight = installJob?.status === "queued" || installJob?.status === "running";

  if (!runtimeUpdate.supported) return null;

  const body = (
    <UpdateBody
      agentName={agentName}
      preview={preview}
      loading={loading}
      previewError={previewError}
      approveError={approveError}
      job={activeJob}
      onRetryPreview={() => void loadPreview()}
    />
  );

  const footer = (mobile = false) => (
    <UpdateFooter
      agentName={agentName}
      preview={preview}
      previewError={previewError}
      job={activeJob}
      loading={loading}
      starting={starting}
      installInFlight={installInFlight}
      onApprove={() => void approve()}
      onClose={() => handleOpenChange(false)}
      mobile={mobile}
    />
  );

  return (
    <>
      <UpdateTrigger
        agentName={agentName}
        displayName={displayName}
        installInFlight={installInFlight}
        onOpen={() => handleOpenChange(true)}
      />
      <UpdateSurface
        agentName={agentName}
        displayName={displayName}
        isMobile={isMobile}
        open={open}
        onOpenChange={handleOpenChange}
        body={body}
        footer={footer}
      />
    </>
  );
}
