"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Textarea } from "@kandev/ui/textarea";
import { cn } from "@/lib/utils";
import type { WorkflowTemplate } from "@/lib/types/http";

type ImportWorkflowsDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  importYaml: string;
  onImportYamlChange: (value: string) => void;
  onFileUpload: (e: React.ChangeEvent<HTMLInputElement>) => void;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  onImport: () => void;
  importLoading: boolean;
};

export function ImportWorkflowsDialog({
  open,
  onOpenChange,
  importYaml,
  onImportYamlChange,
  onFileUpload,
  fileInputRef,
  onImport,
  importLoading,
}: ImportWorkflowsDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("settings:importWorkflows")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t("settings:uploadYamlFile")}</Label>
            <input
              ref={fileInputRef}
              type="file"
              accept=".yml,.yaml"
              onChange={onFileUpload}
              className="block w-full text-sm text-muted-foreground file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-medium file:bg-primary file:text-primary-foreground file:cursor-pointer cursor-pointer"
            />
          </div>
          <div className="space-y-2">
            <Label>{t("settings:orPasteYaml")}</Label>
            <Textarea
              placeholder={t("settings:version1TypeKandevWorkflowWorkflows")}
              value={importYaml}
              onChange={(e) => onImportYamlChange(e.target.value)}
              className="font-mono text-xs max-h-96 overflow-y-auto"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button
            onClick={onImport}
            disabled={!importYaml.trim() || importLoading}
            className="cursor-pointer"
          >
            {importLoading ? t("settings:importing") : t("settings:import")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function TemplateRadioItem({
  template,
  isSelected,
}: {
  template: WorkflowTemplate;
  isSelected: boolean;
}) {
  return (
    <label
      htmlFor={template.id}
      className={cn(
        "flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors",
        isSelected ? "border-primary bg-primary/5" : "border-border hover:border-primary/50",
      )}
    >
      <RadioGroupItem value={template.id} id={template.id} className="mt-0.5" />
      <div className="flex flex-col gap-1.5 min-w-0">
        <span className="font-medium">{template.name}</span>
        {template.description && (
          <span className="text-sm text-muted-foreground">{template.description}</span>
        )}
        {template.default_steps && template.default_steps.length > 0 && (
          <div className="flex items-center gap-1.5 flex-wrap mt-0.5">
            {template.default_steps.map((step, i) => (
              <div key={i} className="flex items-center gap-1">
                {i > 0 && <span className="text-muted-foreground/40 text-xs">&rarr;</span>}
                <div className="flex items-center gap-1 text-xs text-muted-foreground">
                  <div className={cn("w-2 h-2 rounded-full", step.color ?? "bg-slate-500")} />
                  {step.name}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </label>
  );
}

type CreateWorkflowDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workflowName: string;
  onWorkflowNameChange: (value: string) => void;
  selectedTemplateId: string | null;
  onSelectedTemplateChange: (value: string | null) => void;
  workflowTemplates: WorkflowTemplate[];
  onCreate: () => void | Promise<void>;
  createLoading?: boolean;
};

/** Template radio list for the create-workflow dialog. Extracted to keep
 *  CreateWorkflowDialog under the 100-line function limit. */
function WorkflowTemplatePicker({
  workflowTemplates,
  selectedTemplateId,
  onSelectedTemplateChange,
}: Pick<
  CreateWorkflowDialogProps,
  "workflowTemplates" | "selectedTemplateId" | "onSelectedTemplateChange"
>) {
  const { t } = useTranslation();
  if (workflowTemplates.length === 0) return null;
  return (
    <div className="space-y-2">
      <Label>{t("settings:template")}</Label>
      <RadioGroup
        value={selectedTemplateId ?? "custom"}
        onValueChange={(v) => onSelectedTemplateChange(v === "custom" ? null : v)}
      >
        <div className="grid gap-3">
          {workflowTemplates.map((template) => (
            <TemplateRadioItem
              key={template.id}
              template={template}
              isSelected={selectedTemplateId === template.id}
            />
          ))}
          <label
            htmlFor="custom"
            className={cn(
              "flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-colors",
              selectedTemplateId === null
                ? "border-primary bg-primary/5"
                : "border-border hover:border-primary/50",
            )}
          >
            <RadioGroupItem value="custom" id="custom" className="mt-0.5" />
            <div className="flex flex-col gap-1.5">
              <span className="font-medium">{t("settings:custom2")}</span>
              <span className="text-sm text-muted-foreground">
                {t("settings:createYourOwnAgenticWorkflowFrom")}
              </span>
            </div>
          </label>
        </div>
      </RadioGroup>
    </div>
  );
}

export function CreateWorkflowDialog({
  open,
  onOpenChange,
  workflowName,
  onWorkflowNameChange,
  selectedTemplateId,
  onSelectedTemplateChange,
  workflowTemplates,
  onCreate,
  createLoading = false,
}: CreateWorkflowDialogProps) {
  const { t } = useTranslation();
  const handleOpenChange = (nextOpen: boolean) => {
    if (createLoading && !nextOpen) return;
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="sm:w-[900px] sm:max-w-none max-h-[90vh] flex flex-col"
        data-testid="create-workflow-dialog"
      >
        <DialogHeader>
          <DialogTitle>{t("settings:addWorkflow")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-6 overflow-y-auto">
          <div className="space-y-2">
            <Label htmlFor="workflowName">{t("settings:name")}</Label>
            <Input
              id="workflowName"
              placeholder={t("settings:myProjectWorkflow")}
              value={workflowName}
              onChange={(e) => onWorkflowNameChange(e.target.value)}
              data-testid="workflow-name-input"
            />
          </div>
          <WorkflowTemplatePicker
            workflowTemplates={workflowTemplates}
            selectedTemplateId={selectedTemplateId}
            onSelectedTemplateChange={onSelectedTemplateChange}
          />
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={createLoading}
            className="cursor-pointer"
          >
            {t("common:cancel")}
          </Button>
          <Button
            onClick={onCreate}
            disabled={createLoading}
            className="cursor-pointer"
            data-testid="confirm-create-workflow"
            data-dialog-default-action
          >
            {createLoading ? t("settings:adding") : t("settings:addWorkflow")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
