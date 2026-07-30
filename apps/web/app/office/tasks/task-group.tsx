"use client";

import { IconLayoutRows } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { TaskGroupBy } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";
import { resolveOptionLabel, type OptionLabel } from "@/lib/i18n/option-label";

const GROUP_OPTIONS: ({ value: TaskGroupBy } & OptionLabel)[] = [
  { value: "none", labelKey: "office:noGrouping" },
  { value: "status", labelKey: "office:status" },
  { value: "priority", labelKey: "office:priority" },
  { value: "assignee", labelKey: "office:assignee" },
  { value: "project", labelKey: "office:project" },
  { value: "parent", labelKey: "office:parent" },
];

type IssueGroupProps = {
  groupBy: TaskGroupBy;
  onGroupByChange: (groupBy: TaskGroupBy) => void;
};

export function TaskGroup({ groupBy, onGroupByChange }: IssueGroupProps) {
  const { t } = useTranslation();
  return (
    <Popover>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <Button
              variant={groupBy !== "none" ? "secondary" : "ghost"}
              size="icon-sm"
              className="cursor-pointer"
            >
              <IconLayoutRows className="h-4 w-4" />
            </Button>
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("office:groupBy")}</TooltipContent>
      </Tooltip>
      <PopoverContent className="w-44 p-2" align="end">
        <p className="text-xs font-medium px-2 mb-1">{t("office:groupBy")}</p>
        <div className="flex flex-col gap-0.5">
          {GROUP_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              onClick={() => onGroupByChange(opt.value)}
              className={cn(
                "flex items-center gap-2 px-2 py-1.5 text-sm rounded-md cursor-pointer text-left",
                groupBy === opt.value ? "bg-accent text-foreground" : "hover:bg-accent/50",
              )}
            >
              {resolveOptionLabel(t, opt)}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
