"use client";

import { Trans, useLingui } from "@lingui/react/macro";
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
  const { t } = useLingui();
  const pluginName = target?.display_name ?? t`this plugin`;
  return (
    <Dialog
      open={Boolean(target)}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans>Uninstall plugin</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              This will permanently remove{" "}
              <span className="font-medium text-foreground">{pluginName}</span> and revoke its API
              key. This action cannot be undone.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose} className="cursor-pointer">
            <Trans>Cancel</Trans>
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={onConfirm}
            disabled={busy}
            className="cursor-pointer"
          >
            <Trans>Confirm uninstall</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
