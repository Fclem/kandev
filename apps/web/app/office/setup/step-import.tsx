"use client";

import { Button } from "@kandev/ui/button";
import type { OnboardingFSWorkspace } from "@/lib/api/domains/office-api";
import { CloseButton } from "./close-button";
import { Trans, useTranslation } from "react-i18next";

type StepImportProps = {
  fsWorkspaces: OnboardingFSWorkspace[];
  submitting: boolean;
  onImport: () => void;
  onSkip: () => void;
  closeHref: string;
};

export function StepImport({
  fsWorkspaces,
  submitting,
  onImport,
  onSkip,
  closeHref,
}: StepImportProps) {
  const { t } = useTranslation();
  return (
    <div className="fixed inset-0 z-50 bg-background flex items-center justify-center">
      <div className="relative w-full max-w-2xl mx-auto px-6 text-center">
        <CloseButton href={closeHref} />
        <h1 className="text-2xl font-semibold tracking-tight">
          {t("office:existingConfigurationFound")}
        </h1>
        <p className="mt-2 text-muted-foreground">
          <Trans
            i18nKey="office:foundOnTheFilesystemImportSettings"
            values={{ value1: t("office:workspaces", { count: fsWorkspaces.length }) }}
          >
            {t("office:found")} {t("office:workspaces", { count: fsWorkspaces.length })} on the
            filesystem. Import settings to get started?
          </Trans>
        </p>
        <div className="mt-6 rounded-lg border bg-muted/50 p-4">
          <ul className="space-y-1 text-sm text-left">
            {fsWorkspaces.map((ws) => (
              <li key={ws.name} className="flex items-center gap-2">
                <span className="h-1.5 w-1.5 rounded-full bg-primary" />
                {ws.name}
              </li>
            ))}
          </ul>
        </div>
        <div className="mt-8 flex items-center justify-center gap-3">
          <Button
            variant="outline"
            onClick={onSkip}
            disabled={submitting}
            className="cursor-pointer"
          >
            {t("office:startFresh")}
          </Button>
          <Button onClick={onImport} disabled={submitting} className="cursor-pointer">
            {submitting ? "Importing..." : "Import & Continue"}
          </Button>
        </div>
      </div>
    </div>
  );
}
