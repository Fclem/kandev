"use client";

import { IconCheck, IconCopy } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { Trans, useTranslation } from "react-i18next";

type WebhookCreatedDialogProps = {
  open: boolean;
  webhookUrl: string;
  webhookSecret: string;
  onClose: () => void;
};

export function WebhookCreatedDialog({
  open,
  webhookUrl,
  webhookSecret,
  onClose,
}: WebhookCreatedDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-xl" data-testid="webhook-created-dialog">
        <DialogHeader>
          <DialogTitle>{t("automations:automationCreated")}</DialogTitle>
          <DialogDescription>
            <Trans i18nKey="automations:configureYourExternalSystemToPost">
              Configure your external system to POST to this URL with the secret in the{" "}
              <code className="bg-muted px-1 rounded">{t("automations:xWebhookSecret")}</code>{" "}
              header. You can come back to this automation any time to copy these values again.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <CopyableField label="Webhook URL" value={webhookUrl} />
        <CopyableField label="Webhook secret" value={webhookSecret} mono />
        <DialogFooter>
          <Button
            onClick={onClose}
            className="cursor-pointer"
            data-testid="webhook-created-dialog-close"
          >
            {t("automations:done")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CopyableField({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  const { copied, copy } = useCopyToClipboard();
  return (
    <div className="space-y-1.5">
      <Label className="text-xs">{label}</Label>
      <div className="flex gap-2">
        <Input
          readOnly
          value={value}
          className={mono ? "font-mono text-xs" : "text-xs"}
          onFocus={(e) => e.currentTarget.select()}
        />
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer shrink-0"
          onClick={() => copy(value)}
        >
          {copied ? <IconCheck className="h-3.5 w-3.5" /> : <IconCopy className="h-3.5 w-3.5" />}
        </Button>
      </div>
    </div>
  );
}
