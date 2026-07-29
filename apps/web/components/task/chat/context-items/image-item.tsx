"use client";

import { memo, useState, useCallback } from "react";
import { IconFile, IconPhoto } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent } from "@kandev/ui/dialog";
import type { ImageContextItem } from "@/lib/types/context";
import {
  IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME,
  ImagePreviewContent,
} from "@/components/task/chat/image-preview-dialog";
import { ContextChip } from "./context-chip";
import { useTranslation } from "react-i18next";

/** Hover preview for an image chip: the thumbnail plus the prompt/file delivery toggle. */
function ImageChipPreview({
  previewSrc,
  deliveryMode,
  onDeliveryModeChange,
}: {
  previewSrc: string;
  deliveryMode: ImageContextItem["attachment"]["deliveryMode"];
  onDeliveryModeChange?: (mode: "prompt" | "path") => void;
}) {
  const { t } = useTranslation();
  const deliveryDescription =
    deliveryMode === "path"
      ? "Upload into the workspace so the agent can read or edit the file."
      : "Send as prompt context for visual understanding. The agent will not get a file path.";
  return (
    <div className="space-y-1.5">
      <img
        src={previewSrc}
        alt={t("task:preview")}
        className="max-w-full max-h-48 rounded object-contain"
      />
      {onDeliveryModeChange && (
        <div className="space-y-1.5">
          <div
            className="flex items-center gap-1"
            role="group"
            aria-label={t("task:attachmentDeliveryMode")}
          >
            <Button
              type="button"
              size="sm"
              variant={deliveryMode === "prompt" ? "default" : "outline"}
              className="h-6 px-2 text-xs"
              data-testid="attachment-delivery-prompt"
              data-selected={deliveryMode === "prompt" ? "true" : "false"}
              aria-pressed={deliveryMode === "prompt"}
              onClick={(event) => {
                event.stopPropagation();
                onDeliveryModeChange("prompt");
              }}
            >
              {t("task:prompt")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant={deliveryMode === "path" ? "default" : "outline"}
              className="h-6 px-2 text-xs"
              data-testid="attachment-delivery-path"
              data-selected={deliveryMode === "path" ? "true" : "false"}
              aria-pressed={deliveryMode === "path"}
              onClick={(event) => {
                event.stopPropagation();
                onDeliveryModeChange("path");
              }}
            >
              {t("task:file")}
            </Button>
          </div>
          <p className="text-[11px] leading-snug text-muted-foreground">{deliveryDescription}</p>
        </div>
      )}
    </div>
  );
}

export const ImageItem = memo(function ImageItem({ item }: { item: ImageContextItem }) {
  const { t } = useTranslation();
  const [dialogOpen, setDialogOpen] = useState(false);
  const previewSrc = item.attachment.preview;
  const deliveryMode = item.attachment.deliveryMode;
  let leadingIcon;

  const handleClick = useCallback(() => {
    setDialogOpen(true);
  }, []);

  const preview = previewSrc ? (
    <ImageChipPreview
      previewSrc={previewSrc}
      deliveryMode={deliveryMode}
      onDeliveryModeChange={item.onDeliveryModeChange}
    />
  ) : undefined;
  if (deliveryMode === "path") {
    leadingIcon = (
      <span className="relative h-3 w-3 shrink-0" aria-hidden="true">
        <IconFile className="h-3 w-3 text-muted-foreground" />
        {previewSrc && (
          <img
            src={previewSrc}
            alt=""
            className="absolute -right-1 -bottom-1 h-2 w-2 rounded-[2px] border border-background object-cover"
          />
        )}
      </span>
    );
  } else if (!previewSrc) {
    leadingIcon = <IconPhoto className="h-3 w-3 shrink-0" />;
  }

  return (
    <>
      <ContextChip
        kind="image"
        label={item.label}
        thumbnail={deliveryMode === "prompt" ? previewSrc : undefined}
        leadingIcon={leadingIcon}
        preview={preview}
        onClick={previewSrc ? handleClick : undefined}
        onRemove={item.onRemove}
      />
      {previewSrc && (
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogContent
            aria-describedby={undefined}
            className={IMAGE_PREVIEW_DIALOG_CONTENT_CLASSNAME}
          >
            <ImagePreviewContent src={previewSrc} alt={t("task:fullSizePreview")} />
          </DialogContent>
        </Dialog>
      )}
    </>
  );
});
