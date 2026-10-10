"use client";

import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { FallbackOptionHelp } from "./model-fallback-settings-shell";

function RecoverySwitch({
  kind,
  checked,
  disabled,
  onChange,
}: {
  kind: "limitFallback" | "resumeAfterReset";
  checked: boolean;
  disabled: boolean;
  onChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <div className="min-w-0 space-y-1 rounded-md border border-border/70 bg-muted/10 p-3">
      <div className="flex min-h-11 items-center justify-between gap-4 md:min-h-7 [@media(pointer:coarse)]:min-h-11">
        <div className="flex min-w-0 items-center gap-1">
          <Label htmlFor={id}>{t(`settings:${kind}`)}</Label>
          <FallbackOptionHelp kind={kind} />
        </div>
        <Switch
          id={id}
          checked={checked}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={t(`settings:${kind}`)}
          className="after:-inset-y-[14px] md:after:-inset-y-2 [@media(pointer:coarse)]:after:-inset-y-[14px]"
        />
      </div>
      <p className="text-xs text-muted-foreground">{t(`settings:${kind}Helper`)}</p>
    </div>
  );
}

export function LimitRecoveryFields({
  limitFallback,
  resumeAfterReset,
  fallbackDisabled,
  onLimitFallbackChange,
  onResumeAfterResetChange,
}: {
  limitFallback: boolean;
  resumeAfterReset: boolean;
  fallbackDisabled: boolean;
  onLimitFallbackChange: (checked: boolean) => void;
  onResumeAfterResetChange: (checked: boolean) => void;
}) {
  return (
    <>
      <RecoverySwitch
        kind="limitFallback"
        checked={limitFallback}
        disabled={fallbackDisabled}
        onChange={onLimitFallbackChange}
      />
      <RecoverySwitch
        kind="resumeAfterReset"
        checked={resumeAfterReset}
        disabled={false}
        onChange={onResumeAfterResetChange}
      />
    </>
  );
}
