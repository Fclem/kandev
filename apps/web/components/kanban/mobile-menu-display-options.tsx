"use client";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@kandev/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";

import type { Repository } from "@/lib/types/http";
import type { WorkflowsState } from "@/lib/state/slices";
import {
  MobileTasksListOptions,
  type TasksListDisplayOptions,
} from "./mobile-menu-task-list-options";

export const mobileFieldClass = "space-y-1.5";
export const mobileFieldLabelClass = "text-xs font-medium text-muted-foreground";
export const mobileControlClass = "h-10 w-full px-3 text-sm";
export const mobileSectionTitleClass = "text-sm font-medium";

/**
 * Workflow/repository pickers and the display checkboxes for the mobile kanban
 * menu. Split out of `mobile-menu-sheet.tsx` to keep that file under the
 * 600-line limit.
 */
function getRepositoryPlaceholder(loading: boolean, empty: boolean): string {
  if (loading) return "Loading repositories...";
  if (empty) return "No repositories";
  return "Select repository";
}

export type MobileDisplayOptionsProps = {
  activeWorkflowId: string | null;
  workflows: WorkflowsState["items"];
  onWorkflowChange: (id: string | null) => void;
  repositoryValue: string;
  repositories: Repository[];
  repositoriesLoading: boolean;
  onRepositoryChange: (value: string | "all") => void;
  enablePreviewOnClick: boolean | undefined;
  onTogglePreviewOnClick: ((checked: boolean) => void) | undefined;
  tasksListShowDetails: boolean;
  onToggleTasksListShowDetails: (checked: boolean) => void;
  showTaskDetails: boolean;
  showWorkflow: boolean;
  tasksListOptions?: TasksListDisplayOptions;
};

function MobileDisplaySelects({
  activeWorkflowId,
  workflows,
  onWorkflowChange,
  repositoryValue,
  repositories,
  repositoriesLoading,
  onRepositoryChange,
  showWorkflow,
}: Omit<
  MobileDisplayOptionsProps,
  | "enablePreviewOnClick"
  | "onTogglePreviewOnClick"
  | "tasksListShowDetails"
  | "onToggleTasksListShowDetails"
  | "showTaskDetails"
  | "tasksListOptions"
>) {
  const { t } = useTranslation();
  return (
    <>
      {showWorkflow && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("common:workflow")}</label>
          <Select
            value={activeWorkflowId ?? "all"}
            onValueChange={(value) => onWorkflowChange(value === "all" ? null : value)}
          >
            <SelectTrigger className={mobileControlClass}>
              <SelectValue placeholder={t("kanban:allWorkflows")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("kanban:allWorkflows")}</SelectItem>
              {workflows.map((workflow: WorkflowsState["items"][number]) => (
                <SelectItem key={workflow.id} value={workflow.id}>
                  {workflow.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      <div className={mobileFieldClass}>
        <label className={mobileFieldLabelClass}>{t("common:repository")}</label>
        <Select
          value={repositoryValue}
          onValueChange={(value) => onRepositoryChange(value as string | "all")}
          disabled={repositories.length === 0}
        >
          <SelectTrigger className={mobileControlClass}>
            <SelectValue
              placeholder={getRepositoryPlaceholder(repositoriesLoading, repositories.length === 0)}
            />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("kanban:allRepositories")}</SelectItem>
            {repositories.map((repo: Repository) => (
              <SelectItem key={repo.id} value={repo.id}>
                {repo.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </>
  );
}

export function MobileDisplayOptions(props: MobileDisplayOptionsProps) {
  const { t } = useTranslation();
  const {
    enablePreviewOnClick,
    onTogglePreviewOnClick,
    tasksListShowDetails,
    onToggleTasksListShowDetails,
    showTaskDetails,
    tasksListOptions,
    ...selectProps
  } = props;
  return (
    <div className="space-y-4">
      <label className={mobileSectionTitleClass}>{t("kanban:displayOptions")}</label>
      <MobileDisplaySelects {...selectProps} />
      <div className={mobileFieldClass}>
        <label className={mobileFieldLabelClass}>{t("kanban:previewPanel")}</label>
        <label className="flex h-10 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
          <Checkbox
            checked={enablePreviewOnClick ?? false}
            onCheckedChange={(checked) => {
              onTogglePreviewOnClick?.(!!checked);
            }}
          />
          <span className="text-sm">{t("kanban:openPreviewOnClick")}</span>
        </label>
      </div>
      {showTaskDetails && (
        <div className={mobileFieldClass}>
          <label className={mobileFieldLabelClass}>{t("kanban:listRows")}</label>
          <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-0 text-sm font-medium">
            <Checkbox
              checked={tasksListShowDetails}
              onCheckedChange={(checked) => onToggleTasksListShowDetails(checked === true)}
            />
            <span>{t("kanban:showTaskDetails")}</span>
          </label>
          <p className="pl-6 text-xs text-muted-foreground">
            {t("kanban:addRepositoryPullRequestSessionParent")}
          </p>
        </div>
      )}
      {tasksListOptions && <MobileTasksListOptions options={tasksListOptions} />}
    </div>
  );
}
