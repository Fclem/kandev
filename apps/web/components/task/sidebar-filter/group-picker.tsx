"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import { useTranslation } from "react-i18next";

const GROUP_OPTIONS: Array<{ key: GroupKey; labelKey: string }> = [
  { key: "none", labelKey: "task:none2" },
  { key: "repository", labelKey: "task:repositoryFallback" },
  { key: "workflow", labelKey: "task:workflow3" },
  { key: "workflowStep", labelKey: "task:workflowStep" },
  { key: "executorType", labelKey: "task:executorType" },
  { key: "state", labelKey: "task:state3" },
];

type Props = {
  value: GroupKey;
  onChange: (next: GroupKey) => void;
};

export function GroupPicker({ value, onChange }: Props) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={(v) => onChange(v as GroupKey)}>
      <SelectTrigger size="sm" className="h-7 w-full text-xs" data-testid="group-key-select">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {GROUP_OPTIONS.map((opt) => (
          <SelectItem key={opt.key} value={opt.key} className="text-xs">
            {t(opt.labelKey)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
