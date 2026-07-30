"use client";
import { useEffect, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
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
import { Input } from "@kandev/ui/input";
import type { StorageQuarantineEntry, StorageQuarantinePurgeScope } from "@/lib/types/system";

type ConfirmationDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  phrase: "DEDICATED" | "ADOPT" | "DELETE" | "DELETE ELIGIBLE" | "DELETE ALL NOW";
  actionLabel: string;
  actionTestId: string;
  destructive?: boolean;
  onConfirm: () => void;
};

function ConfirmationDialog(props: ConfirmationDialogProps) {
  const { t } = useTranslation();
  const [confirmation, setConfirmation] = useState("");
  useEffect(() => {
    if (!props.open) setConfirmation("");
  }, [props.open]);
  return (
    <AlertDialog open={props.open} onOpenChange={props.onOpenChange}>
      <AlertDialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
        <AlertDialogHeader>
          <AlertDialogTitle>{props.title}</AlertDialogTitle>
          <AlertDialogDescription className="text-left">
            <Trans
              i18nKey="settings:typeToContinue"
              values={{ description: props.description, token: props.phrase }}
            >
              {props.description} {t("settings:type")} <strong>{props.phrase}</strong> to continue.
            </Trans>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Input
          value={confirmation}
          onChange={(event) => setConfirmation(event.target.value)}
          className="h-11"
          aria-label={t("settings:typeToConfirm", { token: props.phrase })}
          data-testid={`${props.actionTestId}-confirmation`}
        />
        <AlertDialogFooter>
          <AlertDialogCancel className="min-h-11 cursor-pointer">
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            variant={props.destructive ? "destructive" : "default"}
            disabled={confirmation !== props.phrase}
            onClick={props.onConfirm}
            className="min-h-11 cursor-pointer"
            data-testid={props.actionTestId}
          >
            {props.actionLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function DedicatedDockerDialog(
  props: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm">,
) {
  const { t } = useTranslation();
  return (
    <ConfirmationDialog
      {...props}
      title={t("settings:useThisDedicatedDockerDaemon")}
      description={t("settings:buildCacheAndUnusedImageCleanup")}
      phrase="DEDICATED"
      actionLabel={t("settings:acknowledgeDaemon")}
      actionTestId="storage-docker-confirm"
    />
  );
}

export function ExternalGoCacheDialog({
  path,
  ...props
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm"> & { path: string }) {
  const { t } = useTranslation();
  const target = path || t("settings:theSelectedPath");
  return (
    <ConfirmationDialog
      {...props}
      title={t("settings:adoptAnExternalGoBuildCache")}
      description={t("settings:kandevWillBeAllowedToRotate", { target })}
      phrase="ADOPT"
      actionLabel={t("settings:adoptCache")}
      actionTestId="storage-go-cache-adopt-confirm"
    />
  );
}

export function PermanentDeleteDialog({
  entry,
  ...props
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm"> & {
  entry: StorageQuarantineEntry | null;
}) {
  const { t } = useTranslation();
  const target = entry?.quarantine_path ?? t("settings:theSelectedQuarantineEntry");
  return (
    <ConfirmationDialog
      {...props}
      title={t("settings:permanentlyDeleteQuarantinedData")}
      description={t("settings:thisCannotBeUndoneKandevWill", { target })}
      phrase="DELETE"
      actionLabel={t("settings:deletePermanently")}
      actionTestId="storage-quarantine-delete-confirm"
      destructive
    />
  );
}

export function QuarantinePurgeDialog({
  scope,
  eligibleCount,
  protectedCount,
  ...props
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm"> & {
  scope: StorageQuarantinePurgeScope;
  eligibleCount: number;
  protectedCount: number;
}) {
  const { t } = useTranslation();
  const eligible = scope === "eligible";
  return (
    <ConfirmationDialog
      {...props}
      title={
        eligible ? t("settings:clearEligibleQuarantine") : t("settings:forceClearAllQuarantine")
      }
      description={
        eligible
          ? // Two sentences, each with its own plural: English agreement on
            // "item/items" and "remains/remain" cannot be assembled from
            // fragments in other languages.
            `${t("settings:quarantineRemoveEligible", { count: eligibleCount })} ${t(
              "settings:quarantineProtectedRemain",
              { count: protectedCount },
            )}`
          : t("settings:thisAttemptsToPermanentlyRemoveEvery")
      }
      phrase={eligible ? "DELETE ELIGIBLE" : "DELETE ALL NOW"}
      actionLabel={eligible ? t("settings:clearEligible") : t("settings:forceClearAll")}
      actionTestId={
        eligible
          ? "storage-quarantine-clear-eligible-confirm"
          : "storage-quarantine-force-clear-confirm"
      }
      destructive
    />
  );
}
