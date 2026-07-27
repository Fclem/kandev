"use client";

import { Trans, useLingui } from "@lingui/react/macro";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";

type GitIdentityCardProps = {
  name: string;
  email: string;
  onNameChange: (value: string) => void;
  onEmailChange: (value: string) => void;
};

export function GitIdentityCard({
  name,
  email,
  onNameChange,
  onEmailChange,
}: GitIdentityCardProps) {
  const { t } = useLingui();
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Trans>Git Identity</Trans>
        </CardTitle>
        <CardDescription>
          <Trans>Optional author identity applied in remote executor environments.</Trans>
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="git-user-name">
            <Trans>Git User Name</Trans>
          </Label>
          <Input
            id="git-user-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            placeholder={t`Jane Developer`}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="git-user-email">
            <Trans>Git User Email</Trans>
          </Label>
          <Input
            id="git-user-email"
            value={email}
            onChange={(e) => onEmailChange(e.target.value)}
            placeholder="jane@example.com"
          />
        </div>
      </CardContent>
    </Card>
  );
}
