"use client";
import { Trans, useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { ProfileFormData } from "@/components/settings/profile-form-fields";

export function CommandPrefixField({
  profile,
  baselineProfile,
  onChange,
}: {
  profile: ProfileFormData;
  baselineProfile?: ProfileFormData;
  onChange: (patch: Partial<ProfileFormData>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="space-y-2"
      data-settings-dirty={
        Boolean(baselineProfile) &&
        (profile.command_prefix ?? "") !== (baselineProfile?.command_prefix ?? "")
      }
      data-settings-dirty-level="container"
    >
      <Label htmlFor="profile-command-prefix">{t("settings:commandPrefix")}</Label>
      <Input
        id="profile-command-prefix"
        data-testid="command-prefix-input"
        value={profile.command_prefix ?? ""}
        onChange={(event) => onChange({ command_prefix: event.target.value })}
        placeholder={t("settings:eGGreywall")}
      />
      <p className="text-xs text-muted-foreground">
        <Trans i18nKey="settings:tokensPrependedToTheAgentLaunch">
          Tokens prepended to the agent launch command, so it runs under a sandbox launcher (e.g.{" "}
          <code>greywall --</code>). The value is shell-tokenised. Leave empty to run the agent
          directly. Applies to ACP sessions only — it has no effect when the profile uses TUI
          passthrough.
        </Trans>
      </p>
    </div>
  );
}
