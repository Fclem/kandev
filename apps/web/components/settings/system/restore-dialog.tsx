"use client";
import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconCircleCheck } from "@tabler/icons-react";
import { restoreBackup } from "@/lib/api/domains/system-api";
import { useSystemJob } from "@/hooks/domains/system/use-system-jobs";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  name: string;
};

const CONFIRM_TOKEN = "RESTORE";

function ConfirmView({
  name,
  typed,
  onTyped,
  submitting,
  error,
  enabled,
  onCancel,
  onConfirm,
}: {
  name: string;
  typed: string;
  onTyped: (v: string) => void;
  submitting: boolean;
  error: string | null;
  enabled: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconAlertTriangle className="h-5 w-5 text-destructive" />
          {t("settings:restoreSnapshot")}
        </DialogTitle>
        <DialogDescription className="space-y-2">
          <span>
            <Trans i18nKey="settings:restoreOverTheCurrentDatabaseAfter" values={{ name }}>
              {t("settings:restore")} <code className="font-mono">{name}</code> over the current
              database. After the staged copy is in place you will be asked to quit and relaunch
              Kandev so the new data is loaded fresh - the backend does not auto-restart.
            </Trans>
          </span>
          <span className="block font-medium text-foreground">
            <Trans
              i18nKey="settings:typeToEnableTheConfirmButton"
              values={{ token: CONFIRM_TOKEN }}
            >
              {t("settings:type")} <code>{CONFIRM_TOKEN}</code> to enable the confirm button.
            </Trans>
          </span>
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-3">
        <Input
          autoFocus
          placeholder={t("settings:typeToConfirm", { token: CONFIRM_TOKEN })}
          value={typed}
          onChange={(e) => onTyped(e.target.value)}
          disabled={submitting}
          data-testid="system-restore-input"
        />
        {error && (
          <p className="text-xs text-destructive" data-testid="system-restore-error">
            {error}
          </p>
        )}
        {submitting && (
          <div
            className="flex items-center gap-2 text-sm text-muted-foreground"
            data-testid="system-restore-pending"
          >
            <Spinner className="size-4" /> {t("settings:writingTheSnapshotOverTheLive")}
          </div>
        )}
      </div>

      <DialogFooter>
        <Button
          variant="outline"
          onClick={onCancel}
          disabled={submitting}
          className="cursor-pointer"
          data-testid="system-restore-cancel"
        >
          {t("common:cancel")}
        </Button>
        <Button
          variant="destructive"
          onClick={onConfirm}
          disabled={!enabled}
          className="cursor-pointer"
          data-testid="system-restore-confirm"
        >
          {t("settings:restore")}
        </Button>
      </DialogFooter>
    </>
  );
}

function SuccessView({ name, onClose }: { name: string; onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <IconCircleCheck className="h-5 w-5 text-emerald-500" />
          {t("settings:restoreComplete")}
        </DialogTitle>
        <DialogDescription>
          <span>
            <Trans i18nKey="settings:hasBeenWrittenOverTheCurrent" values={{ name }}>
              <code className="font-mono">{name}</code> has been written over the current database.
              Quit and relaunch Kandev to load the restored data - the running backend still holds
              connections to the previous version and will serve stale rows until you restart.
            </Trans>
          </span>
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button
          variant="outline"
          onClick={onClose}
          className="cursor-pointer"
          data-testid="system-restore-close"
        >
          {t("common:close")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function RestoreDialog({ open, onOpenChange, name }: Props) {
  const { t } = useTranslation();
  const [typed, setTyped] = useState("");
  const [jobId, setJobId] = useState<string | null>(null);
  const [requestPending, setRequestPending] = useState(false);
  const [requestError, setRequestError] = useState<string | null>(null);

  const job = useSystemJob(jobId);
  const succeeded = job?.state === "succeeded";
  const failed = job?.state === "failed";
  // submitting spans both the HTTP roundtrip and the in-flight backend job.
  const submitting = requestPending || (jobId !== null && !succeeded && !failed);
  const error = requestError ?? (failed ? (job?.message ?? t("settings:restoreFailed")) : null);
  const enabled = typed === CONFIRM_TOKEN && !submitting && !succeeded;

  const handleClose = (next: boolean) => {
    if (submitting) return;
    if (!next) {
      setTyped("");
      setRequestError(null);
      setJobId(null);
    }
    onOpenChange(next);
  };

  const onConfirm = async () => {
    setRequestPending(true);
    setRequestError(null);
    setJobId(null);
    try {
      const res = await restoreBackup(name, CONFIRM_TOKEN);
      setJobId(res.job_id);
    } catch (err) {
      setRequestError(err instanceof Error ? err.message : t("settings:restoreRequestFailed"));
    } finally {
      setRequestPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent data-testid="system-restore-dialog">
        {succeeded ? (
          <SuccessView name={name} onClose={() => handleClose(false)} />
        ) : (
          <ConfirmView
            name={name}
            typed={typed}
            onTyped={setTyped}
            submitting={submitting}
            error={error}
            enabled={enabled}
            onCancel={() => handleClose(false)}
            onConfirm={() => void onConfirm()}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
