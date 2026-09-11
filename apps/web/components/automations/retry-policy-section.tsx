import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { RetryPolicy } from "@/lib/types/automation";
import type { FormState } from "./automation-payload";

type Props = {
  policy: RetryPolicy;
  savedPolicy: RetryPolicy;
  updateField: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

const modes: Array<{ value: RetryPolicy["mode"]; title: string; description: string }> = [
  { value: "disabled", title: "retryDisabledTitle", description: "retryDisabledDescription" },
  { value: "finite", title: "retryFiniteTitle", description: "retryFiniteDescription" },
  { value: "infinite", title: "retryInfiniteTitle", description: "retryInfiniteDescription" },
];

export function RetryPolicySection({ policy, savedPolicy, updateField }: Props) {
  const { t } = useTranslation();
  const dirty = JSON.stringify(policy) !== JSON.stringify(savedPolicy);
  const updatePolicy = (patch: Partial<RetryPolicy>) =>
    updateField("retryPolicy", { ...policy, ...patch });
  return (
    <div className="space-y-3 border-t pt-3" data-settings-dirty={dirty}>
      <div>
        <h3 className="text-sm font-medium">{t("automations:retryTitle")}</h3>
        <p className="text-xs text-muted-foreground">{t("automations:retryDescription")}</p>
      </div>
      <RadioGroup
        value={policy.mode}
        onValueChange={(value) => updatePolicy({ mode: value as RetryPolicy["mode"] })}
        className="gap-2"
      >
        {modes.map((mode) => (
          <Label
            key={mode.value}
            htmlFor={`automation-retry-${mode.value}`}
            className="flex min-h-11 w-full cursor-pointer items-start gap-3 rounded-md border p-3"
          >
            <RadioGroupItem
              id={`automation-retry-${mode.value}`}
              value={mode.value}
              className="mt-0.5"
            />
            <span className="min-w-0 space-y-1">
              <span className="block text-sm font-medium">{t(`automations:${mode.title}`)}</span>
              <span className="block text-xs text-muted-foreground">
                {t(`automations:${mode.description}`)}
              </span>
            </span>
          </Label>
        ))}
      </RadioGroup>
      {policy.mode !== "disabled" && (
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="space-y-1.5">
            <Label htmlFor="automation-retry-max">{t("automations:retryMaxLabel")}</Label>
            <Input
              id="automation-retry-max"
              inputMode="numeric"
              value={policy.max_retries}
              disabled={policy.mode === "infinite"}
              onChange={(event) => updatePolicy({ max_retries: event.target.value })}
              aria-describedby="automation-retry-max-help"
            />
            <p id="automation-retry-max-help" className="text-xs text-muted-foreground">
              {t("automations:retryMaxHelp")}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="automation-retry-delay">{t("automations:retryDelayLabel")}</Label>
            <Input
              id="automation-retry-delay"
              inputMode="numeric"
              value={policy.delay_seconds}
              onChange={(event) => updatePolicy({ delay_seconds: event.target.value })}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="automation-retry-backoff">{t("automations:retryBackoffLabel")}</Label>
            <select
              id="automation-retry-backoff"
              className="h-9 w-full rounded-md border bg-background px-3 text-sm"
              value={policy.backoff}
              onChange={(event) =>
                updatePolicy({ backoff: event.target.value as RetryPolicy["backoff"] })
              }
            >
              <option value="fixed">{t("automations:retryBackoffFixed")}</option>
              <option value="exponential">{t("automations:retryBackoffExponential")}</option>
            </select>
          </div>
        </div>
      )}
      <p className="text-xs text-muted-foreground">{t("automations:retryHistoryHelp")}</p>
    </div>
  );
}
