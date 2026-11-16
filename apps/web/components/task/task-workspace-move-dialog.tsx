"use client";

import { useId, type RefObject } from "react";
import { IconLoader2, IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { usePathname } from "@/lib/routing/client-router";
import { isTaskDetailPath } from "@/lib/links";
import {
  WorkspaceMoveCurrentPlacement,
  WorkspaceMoveNotice,
  WorkspaceMoveOutcome,
  WorkspaceMoveSelectors,
} from "@/components/task/task-workspace-move-form-sections";
import {
  useTaskWorkspaceMove,
  type TaskWorkspaceMoveState,
} from "@/hooks/domains/kanban/use-task-workspace-move";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { createFocusReturnHandler } from "@/lib/dialog-focus-return";
import { cn } from "@/lib/utils";

type TaskWorkspaceMoveDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Captured when the form opened; never retargeted to another task. */
  taskId: string | null;
  workspaceId: string | null;
  focusReturnRef?: RefObject<HTMLElement | null>;
};

function MoveFooter({
  state,
  isTouchSurface,
  onClose,
}: {
  state: TaskWorkspaceMoveState;
  isTouchSurface: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const committed = state.operation?.status === "committed";
  const submitting = state.operation?.status === "submitting";
  return (
    <div
      className={cn(
        "flex shrink-0 items-center justify-end gap-2 border-t border-border/70 px-5 py-3",
        isTouchSurface &&
          "justify-stretch px-4 pb-[max(12px,env(safe-area-inset-bottom,0px))] pt-3",
      )}
    >
      <Button
        type="button"
        variant="outline"
        className={cn("cursor-pointer", isTouchSurface && "min-h-12 flex-1")}
        onClick={onClose}
        data-testid="workspace-move-cancel"
      >
        {committed ? t("task:workspaceMoveClose") : t("task:workspaceMoveCancel")}
      </Button>
      {!committed && (
        <Button
          type="submit"
          className={cn("cursor-pointer", isTouchSurface && "min-h-12 flex-1")}
          disabled={!state.canSubmit}
          data-testid="workspace-move-submit"
        >
          {submitting && <IconLoader2 className="size-4 animate-spin" aria-hidden="true" />}
          {t("task:workspaceMoveSubmit")}
        </Button>
      )}
    </div>
  );
}

function MoveForm({
  state,
  taskId,
  isTouchSurface,
  onClose,
}: {
  state: TaskWorkspaceMoveState;
  taskId: string | null;
  isTouchSurface: boolean;
  onClose: () => void;
}) {
  const ids: [string, string, string] = [useId(), useId(), useId()];
  // A listing stays where it is and offers the moved task; an open task
  // already follows its canonical workspace context.
  const pathname = usePathname();
  const taskIsOpen = taskId !== null && isTaskDetailPath(pathname, taskId);
  return (
    <form
      className="flex min-h-0 flex-1 flex-col"
      data-testid="workspace-move-form"
      onSubmit={(event) => {
        event.preventDefault();
        void state.submit();
      }}
    >
      <div
        data-testid="workspace-move-scroll"
        className={cn(
          "min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-4",
          isTouchSurface && "px-4 py-3",
        )}
      >
        <div
          className={cn("grid min-w-0 grid-cols-[minmax(0,1fr)] gap-3", isTouchSurface && "gap-4")}
        >
          <WorkspaceMoveCurrentPlacement state={state} isTouchSurface={isTouchSurface} />
          <WorkspaceMoveSelectors state={state} isTouchSurface={isTouchSurface} ids={ids} />
          <WorkspaceMoveNotice />
          <WorkspaceMoveOutcome
            state={state}
            isTouchSurface={isTouchSurface}
            showOpenTask={!taskIsOpen}
            onOpenTask={onClose}
          />
        </div>
      </div>
      <MoveFooter state={state} isTouchSurface={isTouchSurface} onClose={onClose} />
    </form>
  );
}

/**
 * Moves one task to another workspace. Desktop uses a compact dialog; phones
 * (below the 768px boundary) use an inset bottom drawer capped to the dynamic
 * viewport, with one scrolling body and a fixed, safe-area-aware footer.
 */
export function TaskWorkspaceMoveDialog({
  open,
  onOpenChange,
  taskId,
  workspaceId,
  focusReturnRef,
}: TaskWorkspaceMoveDialogProps) {
  const { t } = useTranslation();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  const isTouchSurface = isMobile || !isFinePointer;
  const closeFocus = createFocusReturnHandler(focusReturnRef);
  const close = () => onOpenChange(false);
  // Owned above the responsive branch so a 767/768px resize keeps the draft.
  const state = useTaskWorkspaceMove({ open, taskId, workspaceId });
  const header = (
    <div className="flex shrink-0 items-start justify-between gap-3 border-b border-border/70 px-5 py-4">
      <div className="min-w-0">
        {isMobile ? (
          <DrawerHeader className="p-0 text-left">
            <DrawerTitle className="text-base">{t("task:workspaceMoveTitle")}</DrawerTitle>
            <DrawerDescription>{t("task:workspaceMoveDescription")}</DrawerDescription>
          </DrawerHeader>
        ) : (
          <DialogHeader className="text-left">
            <DialogTitle className="text-base">{t("task:workspaceMoveTitle")}</DialogTitle>
            <DialogDescription>{t("task:workspaceMoveDescription")}</DialogDescription>
          </DialogHeader>
        )}
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className={cn("shrink-0 cursor-pointer", isTouchSurface && "size-11")}
        aria-label={t("common:close")}
        onClick={close}
        data-testid="workspace-move-close"
      >
        <IconX className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
  const body = (
    <MoveForm state={state} taskId={taskId} isTouchSurface={isTouchSurface} onClose={close} />
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="max-h-[calc(100dvh-24px-env(safe-area-inset-top,0px))] min-h-0 overflow-hidden p-0 outline-none"
          data-testid="workspace-move-drawer"
          onCloseAutoFocus={closeFocus}
        >
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
            {header}
            {body}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[min(90dvh,720px)] min-h-0 flex-col gap-0 overflow-hidden p-0 sm:max-w-lg"
        data-testid="workspace-move-dialog"
        showCloseButton={false}
        onCloseAutoFocus={closeFocus}
      >
        {header}
        {body}
      </DialogContent>
    </Dialog>
  );
}
