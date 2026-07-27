"use client";

import { t } from "@lingui/core/macro";
import { Trans, useLingui } from "@lingui/react/macro";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { IconFlask, IconLock, IconRefresh } from "@tabler/icons-react";
import type { RuntimeFlagState } from "@/lib/types/runtime-flags";
import { SettingsCard } from "@/components/settings/settings-card";

type FeatureToggleCardProps = {
  flag: RuntimeFlagState;
  isDirty?: boolean;
  saving: boolean;
  onChange: (next: boolean) => void;
  onReset: () => void;
};

export function FeatureToggleCard({
  flag,
  isDirty = false,
  saving,
  onChange,
  onReset,
}: FeatureToggleCardProps) {
  const { t } = useLingui();
  const disabled = saving || flag.env_locked || !flag.mutable;
  return (
    <SettingsCard isDirty={isDirty} data-testid={`feature-toggle-${flag.key}`}>
      <CardHeader className="gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-2">
          <CardTitle className="flex flex-wrap items-center gap-2 text-base">
            {flag.label}
            <FlagBadges flag={flag} />
          </CardTitle>
          <p className="text-sm text-muted-foreground">{flag.description}</p>
        </div>
        <Switch
          checked={flag.effective_value}
          data-settings-dirty={isDirty}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={t`Toggle ${flag.label}`}
          className="cursor-pointer disabled:cursor-not-allowed"
        />
      </CardHeader>
      <CardContent className="space-y-3">
        {flag.risk_description && (
          <p className="text-sm leading-6 text-muted-foreground">{flag.risk_description}</p>
        )}
        <FlagMetadata flag={flag} />
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <Button
            variant="outline"
            size="sm"
            disabled={saving || flag.env_locked || !flag.mutable || flag.override_value == null}
            onClick={onReset}
            className="cursor-pointer disabled:cursor-not-allowed"
          >
            <IconRefresh className="mr-1 h-3.5 w-3.5" />
            <Trans>Use default</Trans>
          </Button>
          {flag.env_locked && (
            <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
              <IconLock className="h-3.5 w-3.5" />
              <Trans>Controlled by launch environment</Trans>
            </span>
          )}
        </div>
      </CardContent>
    </SettingsCard>
  );
}

function FlagBadges({ flag }: { flag: RuntimeFlagState }) {
  return (
    <>
      {flag.stability === "experimental" && (
        <Badge variant="secondary" className="gap-1">
          <IconFlask className="h-3 w-3" />
          <Trans>Experimental</Trans>
        </Badge>
      )}
      {flag.kind === "debug" && (
        <Badge variant="outline">
          <Trans>Debug</Trans>
        </Badge>
      )}
    </>
  );
}

function FlagMetadata({ flag }: { flag: RuntimeFlagState }) {
  const source = sourceLabel(flag);
  return (
    <div className="flex flex-col gap-2 text-xs text-muted-foreground sm:flex-row sm:flex-wrap sm:items-center">
      <span>
        <Trans>Source: {source}</Trans>
      </span>
      <span>
        <Trans>Env: {flag.env_var}</Trans>
      </span>
      {flag.restart_required && (
        <span>
          <Trans>Requires restart</Trans>
        </span>
      )}
      {flag.requires_restart_to_apply && (
        <span className="font-medium text-amber-700">
          <Trans>Pending restart</Trans>
        </span>
      )}
    </div>
  );
}

function sourceLabel(flag: RuntimeFlagState): string {
  if (flag.source === "env") return t`Environment`;
  if (flag.source === "override") return t`Saved override`;
  return t`Default`;
}
