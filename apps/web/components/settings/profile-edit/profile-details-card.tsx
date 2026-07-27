"use client";

import { Trans } from "@lingui/react/macro";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { SettingsCard } from "@/components/settings/settings-card";

type ProfileDetailsCardProps = {
  name: string;
  baselineName?: string;
  onNameChange: (v: string) => void;
};

export function ProfileDetailsCard({ name, baselineName, onNameChange }: ProfileDetailsCardProps) {
  const isDirty = baselineName !== undefined && name.trim() !== baselineName.trim();
  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle>
          <Trans>Profile Details</Trans>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="profile-name">
            <Trans>Name</Trans>
          </Label>
          <Input
            id="profile-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            data-settings-dirty={isDirty}
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}
