"use client";

import { useState, useEffect, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@kandev/ui/dialog";
import { useAppStore } from "@/components/state-provider";
import { listSentryInstances, listSentryOrganizations } from "@/lib/api/domains/sentry-api";
import { clearWorkspaceScopedForm } from "@/lib/watcher-repository-default";
import {
  AutomationFields,
  FilterFields,
  InstancePicker,
  PromptField,
  SettingsFields,
  WorkspacePicker,
  type FormSetter,
} from "./sentry-issue-watch-fields";
import {
  type FormState,
  parseMaxInflightTasks,
  isWatchFormReady,
  buildWatchPayload,
  formStateFromWatch,
  makeEmptyForm,
} from "./sentry-issue-watch-form";
import type {
  CreateSentryIssueWatchRequest,
  SentryConfig,
  SentryIssueWatch,
  UpdateSentryIssueWatchRequest,
} from "@/lib/types/sentry";
import { useTranslation } from "react-i18next";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: SentryIssueWatch | null;
  workspaceId?: string;
  onCreate: (req: CreateSentryIssueWatchRequest) => Promise<unknown>;
  onUpdate: (
    id: string,
    workspaceId: string,
    req: UpdateSentryIssueWatchRequest,
  ) => Promise<unknown>;
};

// useWorkspaceInstances loads the workspace's Sentry instances for the required
// instance selector and auto-selects the sole instance on a fresh create.
function useWorkspaceInstances(
  open: boolean,
  workspaceId: string,
  hasWatch: boolean,
  setForm: FormSetter,
) {
  const [instances, setInstances] = useState<SentryConfig[]>([]);
  useEffect(() => {
    if (!open || !workspaceId) {
      setInstances([]);
      return;
    }
    let cancelled = false;
    listSentryInstances(workspaceId)
      .then((list) => {
        if (!cancelled) setInstances(list);
      })
      .catch(() => {
        if (!cancelled) setInstances([]);
      });
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId]);
  useEffect(() => {
    if (hasWatch || instances.length !== 1) return;
    setForm((p) => (p.sentryInstanceId ? p : { ...p, sentryInstanceId: instances[0].id }));
  }, [hasWatch, instances, setForm]);
  return instances;
}

function savingLabel(saving: boolean, isEdit: boolean): string {
  if (saving) return "Saving…";
  return isEdit ? "Update" : "Create";
}

// useWatchOrgs loads the org list for the org dropdown and auto-selects the
// sole org on a fresh create (with one choice there is nothing to pick).
function useWatchOrgs(
  open: boolean,
  workspaceId: string,
  instanceId: string,
  hasWatch: boolean,
  setForm: FormSetter,
) {
  const [orgs, setOrgs] = useState<string[]>([]);
  useEffect(() => {
    if (!open || !instanceId) {
      setOrgs([]);
      return;
    }
    setOrgs([]);
    let cancelled = false;
    listSentryOrganizations(workspaceId, instanceId)
      .then((res) => {
        if (!cancelled) setOrgs((res.organizations ?? []).map((o) => o.slug));
      })
      .catch(() => {
        if (!cancelled) setOrgs([]);
      });
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, instanceId]);
  useEffect(() => {
    if (hasWatch || orgs.length !== 1) return;
    setForm((p) => (p.orgSlug ? p : { ...p, orgSlug: orgs[0] }));
  }, [hasWatch, orgs, setForm]);
  return orgs;
}

export function SentryIssueWatchDialog({
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: Props) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState<FormState>(() => makeEmptyForm(workspaceId ?? ""));

  useEffect(() => {
    if (watch) {
      setForm(formStateFromWatch(watch));
    } else {
      setForm(makeEmptyForm(workspaceId ?? activeWorkspaceId ?? ""));
    }
  }, [watch, open, workspaceId, activeWorkspaceId]);

  const instances = useWorkspaceInstances(open, form.workspaceId, !!watch, setForm);
  const orgs = useWatchOrgs(open, form.workspaceId, form.sentryInstanceId, !!watch, setForm);

  const workspaceLocked = true;

  const canSave = isWatchFormReady(form, { requiresInstance: !watch });

  const handleSave = useCallback(async () => {
    const maxInflight = parseMaxInflightTasks(form.maxInflightTasks);
    if (maxInflight === "invalid") return;
    setSaving(true);
    try {
      const payload = buildWatchPayload(form, maxInflight);
      if (watch) {
        await onUpdate(watch.id, watch.workspaceId, payload);
      } else {
        await onCreate({
          ...payload,
          workspaceId: form.workspaceId,
          sentryInstanceId: form.sentryInstanceId,
        });
      }
      onOpenChange(false);
    } catch {
      // Error surfaced by caller's toast.
    } finally {
      setSaving(false);
    }
  }, [form, watch, onCreate, onUpdate, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full max-w-full sm:w-[800px] sm:max-w-none max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{watch ? "Edit Sentry Watcher" : "Create Sentry Watcher"}</DialogTitle>
          <DialogDescription>{t("sentry:pollSentryWithAStructuredFilter")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <WorkspacePicker
            value={form.workspaceId}
            onChange={(v) => setForm((p) => clearWorkspaceScopedForm(p, v))}
            disabled={workspaceLocked}
          />
          <InstancePicker
            instances={instances}
            value={form.sentryInstanceId}
            onChange={(v) =>
              setForm((p) => ({ ...p, sentryInstanceId: v, orgSlug: "", projectSlug: "" }))
            }
            disabled={!!watch}
          />
          <Separator />
          <FilterFields form={form} setForm={setForm} orgs={orgs} />
          <Separator />
          <AutomationFields form={form} setForm={setForm} />
          <Separator />
          <PromptField
            value={form.prompt}
            onChange={(v) => setForm((p) => ({ ...p, prompt: v }))}
          />
          <Separator />
          <SettingsFields form={form} setForm={setForm} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button onClick={handleSave} disabled={saving || !canSave} className="cursor-pointer">
            {savingLabel(saving, !!watch)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
