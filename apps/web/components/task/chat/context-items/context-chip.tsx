"use client";

import { memo, useRef, useState, type ReactNode } from "react";
import {
  IconListCheck,
  IconFile,
  IconMessageDots,
  IconPhoto,
  IconAt,
  IconGitPullRequest,
  IconRoute,
  IconX,
  IconPinFilled,
} from "@tabler/icons-react";
import type { TablerIcon } from "@tabler/icons-react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { HoverCard, HoverCardTrigger, HoverCardContent } from "@kandev/ui/hover-card";
import type { ContextItemKind } from "@/lib/types/context";
import { useTranslation } from "react-i18next";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

const ICON_BY_KIND: Record<ContextItemKind, TablerIcon> = {
  plan: IconListCheck,
  file: IconFile,
  comment: IconMessageDots,
  "plan-comment": IconMessageDots,
  "walkthrough-comment": IconRoute,
  image: IconPhoto,
  "file-attachment": IconFile,
  prompt: IconAt,
  "pr-feedback": IconGitPullRequest,
  "agent-message-comment": IconMessageDots,
};

type ContextChipProps = {
  kind: ContextItemKind;
  label: string;
  pinned?: boolean;
  preview?: ReactNode;
  /** Data URL to render as a tiny thumbnail instead of the default icon */
  thumbnail?: string;
  leadingIcon?: ReactNode;
  dataTestId?: string;
  dataPath?: string;
  dataIsDirectory?: boolean;
  onClick?: () => void;
  onUnpin?: () => void;
  onRemove?: () => void;
};

export const ContextChip = memo(function ContextChip({
  kind,
  label,
  pinned,
  preview,
  thumbnail,
  leadingIcon,
  dataTestId,
  dataPath,
  dataIsDirectory,
  onClick,
  onUnpin,
  onRemove,
}: ContextChipProps) {
  const { t } = useTranslation();
  const Icon = ICON_BY_KIND[kind];
  let iconNode: ReactNode;
  if (leadingIcon) {
    iconNode = leadingIcon;
  } else if (thumbnail) {
    iconNode = <img src={thumbnail} alt="" className="h-3 w-3 shrink-0 rounded-sm object-cover" />;
  } else {
    iconNode = <Icon className="h-3 w-3 shrink-0" />;
  }

  const controls = (
    <>
      {pinned && onUnpin && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onUnpin();
          }}
          aria-label={t("task:unpinWillBeRemovedAfterSend")}
          className="ml-0.5 min-h-11 min-w-11 text-muted-foreground/70 hover:text-foreground cursor-pointer sm:min-h-0 sm:min-w-0"
        >
          <IconPinFilled className="mx-auto h-2.5 w-2.5" />
        </button>
      )}
      {onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
          aria-label={t("task:removeLabeled", { label })}
          className="ml-0.5 min-h-11 min-w-11 text-muted-foreground hover:text-foreground cursor-pointer sm:min-h-0 sm:min-w-0 sm:opacity-0 sm:group-hover:opacity-100 sm:focus-visible:opacity-100"
        >
          <IconX className="mx-auto h-2.5 w-2.5" />
        </button>
      )}
    </>
  );
  const labelContent = (
    <>
      {iconNode}
      <span className="truncate max-w-[120px]">{label}</span>
    </>
  );
  const chip = (
    <div
      data-testid={dataTestId}
      data-path={dataPath}
      data-is-directory={dataIsDirectory ? "true" : "false"}
      className={`group flex items-center gap-1 px-2 py-0.5 text-xs text-muted-foreground bg-muted/50 rounded border border-border/50 ${onClick ? "cursor-pointer hover:bg-muted/80" : ""}`}
      onClick={preview ? undefined : onClick}
    >
      {preview ? (
        <ControlledHoverChip preview={preview} label={label}>
          {(open) => (
            <button
              type="button"
              aria-haspopup="dialog"
              aria-expanded={open}
              aria-label={label}
              className="flex min-h-11 min-w-0 flex-1 items-center gap-1 text-left"
            >
              {labelContent}
            </button>
          )}
        </ControlledHoverChip>
      ) : (
        labelContent
      )}
      {controls}
    </div>
  );

  return chip;
});

function ControlledHoverChip({
  preview,
  label,
  children,
}: {
  preview: ReactNode;
  label: string;
  children: (open: boolean) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const suppressRef = useRef(false);
  const usesTouchDrawer = useTouchDrawer();

  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{children(open)}</DrawerTrigger>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{label}</DrawerTitle>
            <DrawerDescription className="sr-only">{label}</DrawerDescription>
          </DrawerHeader>
          <div className="max-h-[70dvh] overflow-y-auto px-4 pb-4">{preview}</div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <HoverCard
      open={open}
      onOpenChange={(next) => {
        if (next && suppressRef.current) return;
        setOpen(next);
      }}
      openDelay={300}
      closeDelay={0}
    >
      <HoverCardTrigger
        asChild
        onClick={() => {
          suppressRef.current = true;
          setOpen(false);
          setTimeout(() => {
            suppressRef.current = false;
          }, 300);
        }}
      >
        {children(open)}
      </HoverCardTrigger>
      <HoverCardContent side="top" align="start" className="w-80 max-h-80 overflow-y-auto">
        {preview}
      </HoverCardContent>
    </HoverCard>
  );
}
