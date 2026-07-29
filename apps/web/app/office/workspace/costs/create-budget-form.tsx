"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Card, CardContent } from "@kandev/ui/card";
import { toast } from "sonner";
import { createBudget } from "@/lib/api/domains/office-api";
import { useTranslation } from "react-i18next";

type Props = {
  workspaceId: string;
  onCreated: () => void;
  onCancel: () => void;
};

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="text-xs text-muted-foreground">{label}</label>
      {children}
    </div>
  );
}

type FormState = {
  scopeType: string;
  scopeId: string;
  limitDollars: string;
  period: string;
  alertPct: string;
  action: string;
};

function FormFields({
  state,
  onChange,
}: {
  state: FormState;
  onChange: (patch: Partial<FormState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-3">
      <FormField label="Scope Type">
        <Select value={state.scopeType} onValueChange={(v) => onChange({ scopeType: v })}>
          <SelectTrigger className="h-8 text-sm cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="workspace" className="cursor-pointer">
              {t("common:workspace")}
            </SelectItem>
            <SelectItem value="agent" className="cursor-pointer">
              {t("office:agent")}
            </SelectItem>
            <SelectItem value="project" className="cursor-pointer">
              {t("office:project")}
            </SelectItem>
          </SelectContent>
        </Select>
      </FormField>
      <FormField label="Scope ID">
        <Input
          className="h-8 text-sm"
          value={state.scopeId}
          onChange={(e) => onChange({ scopeId: e.target.value })}
          placeholder={t("office:entityId")}
        />
      </FormField>
      <FormField label="Limit ($)">
        <Input
          className="h-8 text-sm"
          type="number"
          value={state.limitDollars}
          onChange={(e) => onChange({ limitDollars: e.target.value })}
          placeholder="10.00"
        />
      </FormField>
      <FormField label="Period">
        <Select value={state.period} onValueChange={(v) => onChange({ period: v })}>
          <SelectTrigger className="h-8 text-sm cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="monthly" className="cursor-pointer">
              {t("office:monthly")}
            </SelectItem>
            <SelectItem value="total" className="cursor-pointer">
              {t("office:total")}
            </SelectItem>
          </SelectContent>
        </Select>
      </FormField>
      <FormField label="Alert Threshold (%)">
        <Input
          className="h-8 text-sm"
          type="number"
          value={state.alertPct}
          onChange={(e) => onChange({ alertPct: e.target.value })}
        />
      </FormField>
      <FormField label="Action on Exceed">
        <Select value={state.action} onValueChange={(v) => onChange({ action: v })}>
          <SelectTrigger className="h-8 text-sm cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="notify_only" className="cursor-pointer">
              {t("office:notifyOnly")}
            </SelectItem>
            <SelectItem value="pause_agent" className="cursor-pointer">
              {t("office:pauseAgent")}
            </SelectItem>
            <SelectItem value="block_new_tasks" className="cursor-pointer">
              {t("office:blockNewTasks")}
            </SelectItem>
          </SelectContent>
        </Select>
      </FormField>
    </div>
  );
}

export function CreateBudgetForm({ workspaceId, onCreated, onCancel }: Props) {
  const { t } = useTranslation();
  const [state, setState] = useState<FormState>({
    scopeType: "workspace",
    scopeId: workspaceId,
    limitDollars: "",
    period: "monthly",
    alertPct: "80",
    action: "notify_only",
  });
  const [saving, setSaving] = useState(false);

  const handleChange = (patch: Partial<FormState>) => setState((prev) => ({ ...prev, ...patch }));

  const handleSubmit = async () => {
    setSaving(true);
    try {
      await createBudget(workspaceId, {
        scopeType: state.scopeType as "agent" | "project" | "workspace",
        scopeId: state.scopeId,
        // Budgets are stored as hundredths of a cent (subcents) to
        // match cost_subcents semantics — see apps/web/lib/utils.ts:formatDollars.
        limitSubcents: Math.round(parseFloat(state.limitDollars || "0") * 10000),
        period: state.period as "monthly" | "total",
        alertThresholdPct: parseInt(state.alertPct, 10) || 80,
        actionOnExceed: state.action as "notify_only" | "pause_agent" | "block_new_tasks",
      });
      onCreated();
      toast.success(t("office:budgetPolicyCreated"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create budget policy");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardContent className="pt-4 space-y-3">
        <FormFields state={state} onChange={handleChange} />
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" size="sm" className="cursor-pointer" onClick={onCancel}>
            {t("common:cancel")}
          </Button>
          <Button size="sm" className="cursor-pointer" disabled={saving} onClick={handleSubmit}>
            {saving ? "Creating..." : "Create Policy"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
