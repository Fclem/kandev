"use client";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings:gitIdentity")}</CardTitle>
        <CardDescription>{t("settings:optionalAuthorIdentityAppliedInRemote")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="git-user-name">{t("settings:gitUserName")}</Label>
          <Input
            id="git-user-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            placeholder={t("settings:janeDeveloper")}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="git-user-email">{t("settings:gitUserEmail")}</Label>
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
