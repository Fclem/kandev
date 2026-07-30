"use client";

import { useEffect, useState } from "react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "sonner";
import { listActivity } from "@/lib/api/domains/office-api";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import { ActivityRow } from "./activity-row";
import { EmptyState } from "../../components/shared/empty-state";
import { PageHeader } from "../../components/shared/page-header";
import { useTranslation } from "react-i18next";
import { resolveOptionLabel } from "@/lib/i18n/option-label";

const FILTER_OPTIONS = [
  { value: "all", labelKey: "office:allTypes" },
  { value: "agent", labelKey: "office:agent" },
  { value: "task", labelKey: "office:task2" },
  { value: "project", labelKey: "office:project" },
  { value: "budget", labelKey: "office:budget2" },
  { value: "approval", labelKey: "office:approval" },
  { value: "system", labelKey: "office:system" },
];

export function ActivityFeed({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [filterType, setFilterType] = useState("all");

  useEffect(() => {
    listActivity(workspaceId, filterType)
      .then((res) => setEntries(res.activity ?? []))
      .catch((err) => {
        toast.error(err instanceof Error ? err.message : "Failed to load activity");
      });
  }, [workspaceId, filterType]);

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office:activity")}
        action={
          <Select value={filterType} onValueChange={setFilterType}>
            <SelectTrigger className="w-[140px] h-8 text-xs cursor-pointer">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FILTER_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value} className="cursor-pointer">
                  {resolveOptionLabel(t, opt)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />

      {entries.length === 0 ? (
        <EmptyState
          message={t("office:noActivityYet")}
          description={t("office:actionsByAgentsAndUsersAre")}
        />
      ) : (
        <div className="border border-border rounded-lg divide-y divide-border">
          {entries.map((entry) => (
            <ActivityRow key={entry.id} entry={entry} />
          ))}
        </div>
      )}
    </div>
  );
}
