"use client";

import { useState } from "react";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import { openExternalLink } from "@/lib/desktop/external-links";
import { resolveTaskReviewOpenAction } from "./task-pr-open";
import { TaskPRPickerDialog } from "./task-pr-picker-dialog";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useTranslation } from "react-i18next";

/**
 * Task-screen keybinding (default Cmd/Ctrl+Shift+G) that jumps straight to the
 * task's GitHub pull request. One linked PR opens directly; several open a
 * picker dialog; none shows a toast.
 */
export function TaskPRShortcut({ taskId }: { taskId: string | null }) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { prs } = useTaskPR(taskId);
  const mrs = useTaskMRs(taskId);
  const [pickerOpen, setPickerOpen] = useState(false);
  const overrides = useAppStore((s) => s.userSettings.keyboardShortcuts);

  useKeyboardShortcut(
    getShortcut("OPEN_TASK_PR", overrides),
    () => {
      const action = resolveTaskReviewOpenAction(prs, mrs);
      if (action.kind === "none") {
        toast({ description: t("task:noPullRequestOrMergeRequest") });
        return;
      }
      if (action.kind === "open") {
        void openExternalLink(action.url).catch(() => undefined);
        return;
      }
      setPickerOpen(true);
    },
    // Capture + stopPropagation so the binding wins over focus-trapped
    // surfaces (xterm.js, editors) — mirrors useEditorKeybinds. Disabled
    // until the task id resolves so a transient null doesn't toast.
    { capture: true, stopPropagation: true, enabled: !!taskId },
  );

  return <TaskPRPickerDialog open={pickerOpen} onOpenChange={setPickerOpen} prs={prs} mrs={mrs} />;
}
