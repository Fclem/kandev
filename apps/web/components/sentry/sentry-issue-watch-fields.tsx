"use client";

// Field groups rendered inside the Sentry issue-watch dialog: the shared
// SelectField primitive plus the filter, prompt, automation and settings
// sections. The dialog itself owns form state, loading and submission.

import { useCallback, useEffect, useMemo, useState } from "react";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconInfoCircle } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflows } from "@/hooks/use-workflows";
import { useWorkflowSteps, stepPlaceholder } from "@/hooks/use-workflow-steps";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import { listSentryProjects } from "@/lib/api/domains/sentry-api";
import { WatcherRepositoryFields } from "@/components/watcher-repository-fields";
import { SENTRY_ISSUE_WATCH_PLACEHOLDERS } from "./sentry-issue-watch-placeholders";
import { LevelMultiSelect, StatusMultiSelect } from "./sentry-issue-watch-multiselect";
import { MaxInflightTasksField } from "./sentry-issue-watch-throttle-field";
import {
  STATS_PERIOD_OPTIONS,
  type FormState,
  orgSelectItems,
  projectSelectItems,
} from "./sentry-issue-watch-form";
import type { SentryConfig, SentryLevel, SentryProject, SentryStatus } from "@/lib/types/sentry";
import { Trans, useTranslation } from "react-i18next";

export type FormSetter = React.Dispatch<React.SetStateAction<FormState>>;

function useFormData(workspaceId: string) {
  useSettingsData(true);
  useWorkflows(workspaceId, true);
  const allWorkflows = useAppStore((s) => s.workflows.items);
  const workflows = useMemo(() => allWorkflows.filter((w) => !w.hidden), [allWorkflows]);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const executors = useAppStore((s) => s.executors.items);
  const allExecutorProfiles = useMemo(
    () =>
      executors
        .filter((e) => e.type !== "local" && e.type !== "local_pc")
        .flatMap((e) => e.profiles ?? []),
    [executors],
  );
  const filteredAgentProfiles = useMemo(
    () => agentProfiles.filter((p) => !p.cli_passthrough),
    [agentProfiles],
  );
  return { workflows, agentProfiles: filteredAgentProfiles, allExecutorProfiles };
}

function useSentryProjects(workspaceId: string, instanceId: string, orgSlug: string) {
  const [projects, setProjects] = useState<SentryProject[]>([]);
  useEffect(() => {
    if (!workspaceId || !instanceId) {
      setProjects([]);
      return;
    }
    setProjects([]);
    let cancelled = false;
    listSentryProjects(workspaceId, instanceId)
      .then((res) => {
        if (!cancelled) setProjects(res.projects ?? []);
      })
      .catch(() => {
        if (!cancelled) setProjects([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, instanceId]);
  // Sentry's auth-token endpoint already filters to the user's accessible orgs;
  // if an orgSlug is set, restrict to projects that match.
  return useMemo(
    () => (orgSlug ? projects.filter((p) => p.orgSlug === orgSlug) : projects),
    [projects, orgSlug],
  );
}

/** Labelled single-select used by every field group in this dialog. */
export function SelectField(props: {
  label: string;
  description?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  items: { id: string; label: string }[];
  disabled?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <Label>{props.label}</Label>
      {props.description && <p className="text-xs text-muted-foreground">{props.description}</p>}
      <Select
        value={props.value || undefined}
        onValueChange={props.onChange}
        disabled={props.disabled}
      >
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={props.placeholder} />
        </SelectTrigger>
        <SelectContent>
          {props.items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function OrgProjectRow({
  form,
  setForm,
  projects,
  orgs,
}: {
  form: FormState;
  setForm: FormSetter;
  projects: SentryProject[];
  orgs: string[];
}) {
  const { t } = useTranslation();
  const onOrgChange = (v: string) =>
    // The selected project may belong to a different org — clear it so the
    // project dropdown re-picks within the new org.
    setForm((p) => ({ ...p, orgSlug: v, projectSlug: "" }));
  const onProjectChange = (v: string) => setForm((p) => ({ ...p, projectSlug: v }));
  const orgItems = orgSelectItems(orgs, form.orgSlug);
  const projectItems = projectSelectItems(projects, form.projectSlug);
  return (
    <div className="grid grid-cols-2 gap-4">
      <SelectField
        label={t("sentry:organizationSlug")}
        description={t("sentry:theSentryOrgToPoll")}
        value={form.orgSlug}
        onChange={onOrgChange}
        placeholder={orgItems.length === 0 ? t("sentry:noOrganizationsAvailable") : t("sentry:selectOrganization")}
        items={orgItems}
        disabled={orgItems.length === 0}
      />
      <SelectField
        label={t("sentry:projectSlug")}
        description={t("sentry:theSentryProjectToPoll")}
        value={form.projectSlug}
        onChange={onProjectChange}
        placeholder={projectItems.length === 0 ? t("sentry:noProjectsAvailable") : t("sentry:selectProject")}
        items={projectItems}
        disabled={projectItems.length === 0}
      />
    </div>
  );
}

export function FilterFields({
  form,
  setForm,
  orgs,
}: {
  form: FormState;
  setForm: FormSetter;
  orgs: string[];
}) {
  const { t } = useTranslation();
  const projects = useSentryProjects(form.workspaceId, form.sentryInstanceId, form.orgSlug);
  const toggleLevel = useCallback(
    (level: SentryLevel) =>
      setForm((p) => ({
        ...p,
        levels: p.levels.includes(level)
          ? p.levels.filter((l) => l !== level)
          : [...p.levels, level],
      })),
    [setForm],
  );
  const toggleStatus = useCallback(
    (status: SentryStatus) =>
      setForm((p) => ({
        ...p,
        statuses: p.statuses.includes(status)
          ? p.statuses.filter((s) => s !== status)
          : [...p.statuses, status],
      })),
    [setForm],
  );
  return (
    <>
      <OrgProjectRow form={form} setForm={setForm} projects={projects} orgs={orgs} />
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <Label>{t("sentry:environment")}</Label>
          <p className="text-xs text-muted-foreground">
            {t("sentry:optionalRestrictToOneEnvironment")}
          </p>
          <Input
            value={form.environment}
            onChange={(e) => setForm((p) => ({ ...p, environment: e.target.value }))}
            placeholder={t("sentry:production")}
          />
        </div>
        <SelectField
          label={t("sentry:statsPeriod")}
          description={t("sentry:howFarBackToLookFor")}
          value={form.statsPeriod}
          onChange={(v) => setForm((p) => ({ ...p, statsPeriod: v }))}
          placeholder={t("sentry:any")}
          items={STATS_PERIOD_OPTIONS.map((o) => ({ id: o.value, label: o.label }))}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("sentry:levels")}</Label>
        <p className="text-xs text-muted-foreground">{t("sentry:clickToToggleMatchesIssuesAt")}</p>
        <LevelMultiSelect selected={form.levels} onToggle={toggleLevel} />
      </div>
      <div className="space-y-1.5">
        <Label>{t("sentry:statuses")}</Label>
        <p className="text-xs text-muted-foreground">{t("sentry:clickToToggleMatchesIssuesAt2")}</p>
        <StatusMultiSelect selected={form.statuses} onToggle={toggleStatus} />
      </div>
      <div className="space-y-1.5">
        <Label>{t("sentry:query")}</Label>
        <p className="text-xs text-muted-foreground">
          {t("sentry:freeTextSentrySearchQueryOptional")}
        </p>
        <Input
          value={form.query}
          onChange={(e) => setForm((p) => ({ ...p, query: e.target.value }))}
          placeholder={t("sentry:isUnresolvedTransactionApiCheckout")}
        />
      </div>
    </>
  );
}

function PlaceholdersHelp() {
  const { t } = useTranslation();
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs" align="start">
          <p className="text-xs font-medium mb-1">{t("sentry:availablePlaceholders")}</p>
          <ul className="text-xs space-y-0.5">
            {SENTRY_ISSUE_WATCH_PLACEHOLDERS.map((p) => (
              <li key={p.key}>
                <code className="text-[10px] bg-white/15 px-1 rounded">{`{{${p.key}}}`}</code>{" "}
                <span className="opacity-70">{p.description}</span>
              </li>
            ))}
          </ul>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export function PromptField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label>{t("sentry:taskPrompt")}</Label>
        <PlaceholdersHelp />
      </div>
      <p className="text-xs text-muted-foreground">
        <Trans i18nKey="sentry:thePromptSentToTheAgent">
          The prompt sent to the agent for each new issue. Type {"{{"} to insert placeholders.
        </Trans>
      </p>
      <div className="rounded-md border border-border overflow-hidden">
        <ScriptEditor
          value={value}
          onChange={onChange}
          language="markdown"
          height={computeEditorHeight(value)}
          lineNumbers="off"
          placeholders={SENTRY_ISSUE_WATCH_PLACEHOLDERS}
        />
      </div>
    </div>
  );
}

export function WorkspacePicker({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const workspaces = useAppStore((s) => s.workspaces.items);
  return (
    <SelectField
      label={t("common:workspace")}
      description={t("sentry:tasksCreatedByThisWatcherLand")}
      value={value}
      onChange={onChange}
      placeholder={t("sentry:selectWorkspace")}
      items={workspaces.map((w) => ({ id: w.id, label: w.name }))}
      disabled={disabled}
    />
  );
}

export function InstancePicker({
  instances,
  value,
  onChange,
  disabled,
}: {
  instances: SentryConfig[];
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const noInstances = instances.length === 0;
  return (
    <SelectField
      label={t("sentry:sentryInstance")}
      description={t("sentry:whichSentryInstanceThisWatcherPolls")}
      value={value}
      onChange={onChange}
      placeholder={noInstances ? t("sentry:noSentryInstancesInThisWorkspace") : t("sentry:selectAnInstance")}
      items={instances.map((i) => ({ id: i.id, label: i.name }))}
      disabled={disabled || noInstances}
    />
  );
}

export function AutomationFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  const { workflows, agentProfiles, allExecutorProfiles } = useFormData(form.workspaceId);
  const { steps, loading: stepsLoading } = useWorkflowSteps(form.workflowId);
  return (
    <>
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label={t("common:workflow")}
          description={t("sentry:tasksAreCreatedInThisWorkflow")}
          value={form.workflowId}
          onChange={(v) => setForm((p) => ({ ...p, workflowId: v, workflowStepId: "" }))}
          placeholder={t("common:selectWorkflow")}
          items={workflows.map((w) => ({ id: w.id, label: w.name }))}
        />
        <SelectField
          label={t("sentry:workflowStep")}
          description={t("sentry:initialStepForNewTasks")}
          value={form.workflowStepId}
          onChange={(v) => setForm((p) => ({ ...p, workflowStepId: v }))}
          placeholder={stepPlaceholder(form.workflowId, stepsLoading, steps.length)}
          items={steps.map((s) => ({ id: s.id, label: s.name }))}
          disabled={!form.workflowId || stepsLoading || steps.length === 0}
        />
      </div>
      <WatcherRepositoryFields
        workspaceId={form.workspaceId}
        repositoryId={form.repositoryId}
        baseBranch={form.baseBranch}
        onRepositoryChange={(repositoryId) =>
          setForm((p) => ({ ...p, repositoryId, baseBranch: "" }))
        }
        onBaseBranchChange={(baseBranch) => setForm((p) => ({ ...p, baseBranch }))}
      />
      <div className="grid grid-cols-2 gap-4">
        <SelectField
          label={t("sentry:agentProfile")}
          description={t("sentry:optionalFallsBackToStepDefault")}
          value={form.agentProfileId}
          onChange={(v) => setForm((p) => ({ ...p, agentProfileId: v }))}
          placeholder={t("sentry:useStepDefault")}
          items={agentProfiles.map((p) => ({ id: p.id, label: p.label }))}
        />
        <SelectField
          label={t("sentry:executorProfile")}
          description={t("sentry:optionalFallsBackToStepDefault")}
          value={form.executorProfileId}
          onChange={(v) => setForm((p) => ({ ...p, executorProfileId: v }))}
          placeholder={t("sentry:useStepDefault")}
          items={allExecutorProfiles.map((p) => ({ id: p.id, label: p.name }))}
        />
      </div>
    </>
  );
}

export function SettingsFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-1.5">
        <Label>{t("sentry:pollIntervalSeconds")}</Label>
        <p className="text-xs text-muted-foreground">{t("sentry:howOftenToReRunThe")}</p>
        <Input
          type="number"
          value={form.pollInterval}
          onChange={(e) => setForm((p) => ({ ...p, pollInterval: Number(e.target.value) }))}
          min={60}
          max={3600}
        />
      </div>
      <MaxInflightTasksField form={form} setForm={setForm} />
      <div className="flex items-center justify-between">
        <div>
          <Label>{t("common:enabled")}</Label>
          <p className="text-xs text-muted-foreground">{t("sentry:pauseOrResumePolling")}</p>
        </div>
        <Switch
          checked={form.enabled}
          onCheckedChange={(v) => setForm((p) => ({ ...p, enabled: v }))}
          className="cursor-pointer"
        />
      </div>
    </>
  );
}
