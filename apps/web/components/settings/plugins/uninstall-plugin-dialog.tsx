"use client";
import { Trans, useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import type { PluginRecord } from "@/lib/types/plugins";

type UninstallPluginDialogProps = {
  target: PluginRecord | null;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
};

export function UninstallPluginDialog({
  target,
  busy,
  onClose,
  onConfirm,
}: UninstallPluginDialogProps) {
  const { t } = useTranslation();
  const pluginName = target?.display_name ?? t("settings:thisPlugin");
  return (
    <Dialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("settings:uninstallPlugin")}</DialogTitle>
          <DialogDescription>
            <Trans i18nKey="settings:thisWillPermanentlyRemoveAndRevoke" values={{ pluginName }}>
              This will permanently remove{" "}
              <span className="font-medium text-foreground">{pluginName}</span> and revoke its API
              key. This action cannot be undone.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={busy}
            className="cursor-pointer"
          >
            {t("settings:confirmUninstall")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
