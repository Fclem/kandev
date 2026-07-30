"use client";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconDots } from "@tabler/icons-react";

import { cn } from "@/lib/utils";
import { TASK_COLOR_BAR_CLASS, type TaskColor } from "@/lib/task-colors";

/**
 * Presentational chrome for a sidebar task row — the subtree connector, the
 * selection/colour bar, the subtask collapse toggle and the overflow-menu
 * button. Split out of `task-item.tsx` to keep that file under the 600-line
 * limit; none of these carry task logic.
 */
export function RowConnector({ depth, leftPx }: { depth: number; leftPx: number }) {
  if (depth === 0) return null;
  return (
    <span
      style={{ left: leftPx }}
      className="absolute top-[10px] select-none text-[11px] text-muted-foreground/30"
    >
      ↳
    </span>
  );
}

export function SelectionBar({
  isSelected,
  color,
}: {
  isSelected: boolean;
  color: TaskColor | null;
}) {
  if (color) {
    return (
      <div
        className={cn(
          "absolute left-0 top-0 bottom-0 w-[3px] transition-opacity",
          TASK_COLOR_BAR_CLASS[color],
          isSelected ? "opacity-100" : "opacity-60",
        )}
      />
    );
  }
  return (
    <div
      className={cn(
        "absolute left-0 top-0 bottom-0 w-[2px] bg-primary transition-opacity",
        isSelected ? "opacity-100" : "opacity-0",
      )}
    />
  );
}

export function SubtaskToggle({
  taskId,
  count,
  collapsed,
  onToggle,
}: {
  taskId?: string;
  count: number;
  collapsed: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      data-testid="sidebar-subtask-toggle"
      data-task-id={taskId}
      aria-label={collapsed ? t("task:expandSubtasks") : t("task:collapseSubtasks")}
      aria-expanded={!collapsed}
      onClick={(e) => {
        e.stopPropagation();
        onToggle();
      }}
      onKeyDown={(e) => e.stopPropagation()}
      className="self-center flex items-center gap-0.5 shrink-0 cursor-pointer text-[11px] text-muted-foreground/60 hover:text-foreground"
    >
      <IconChevronDown className={cn("h-3 w-3 transition-transform", collapsed && "-rotate-90")} />
      <span>{count}</span>
    </button>
  );
}

export function TaskMenuButton({ visible, expanded }: { visible: boolean; expanded: boolean }) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(
        "mobile-task-actions self-center shrink-0 flex items-center transition-opacity duration-100",
        !visible && "[@media(hover:none)]:hidden",
        visible
          ? "opacity-100"
          : "opacity-0 pointer-events-none [@media(hover:hover)]:group-hover:opacity-100 [@media(hover:hover)]:group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto",
      )}
    >
      <button
        type="button"
        className={cn(
          "mobile-task-actions-button flex size-6 items-center justify-center rounded-md cursor-pointer touch-manipulation",
          "text-muted-foreground hover:text-foreground hover:bg-foreground/10",
          "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring transition-colors",
        )}
        onClick={(e) => {
          e.stopPropagation();
          e.preventDefault();
          e.currentTarget.dispatchEvent(
            new MouseEvent("contextmenu", {
              bubbles: true,
              clientX: e.clientX,
              clientY: e.clientY,
            }),
          );
        }}
        aria-label={t("task:taskActions")}
        aria-haspopup="menu"
        aria-expanded={expanded}
      >
        <IconDots className="h-4 w-4" />
      </button>
    </div>
  );
}
