"use client";

import { useEffect, useState } from "react";
import { IconExternalLink, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import { useGitHubAppRegistrations } from "@/hooks/domains/github/use-github-app-registrations";
import { GitHubAppCreateForm } from "./github-app-create-form";
import { GitHubAppImportForm } from "./github-app-import-form";
import { GitHubAppRegistrationList } from "./github-app-registration-list";
import { Trans, useTranslation } from "react-i18next";

type AppView = "choose" | "import" | "create";
type RegistrationHook = ReturnType<typeof useGitHubAppRegistrations>;

export function GitHubAppConnectionPanel({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const registrations = useGitHubAppRegistrations(workspaceId);
  const [view, setView] = useState<AppView>("choose");
  const { selectedId, setSelectedId, selectedRegistration } = useAppRegistrationSelection(
    workspaceId,
    registrations,
  );
  const { toast } = useToast();

  useEffect(() => {
    setView("choose");
  }, [workspaceId]);

  async function install() {
    if (!selectedRegistration || selectedRegistration.status !== "active") return;
    try {
      const response = await registrations.startInstall(selectedRegistration.id);
      const url = response.url ?? response.URL;
      if (!url) throw new Error("GitHub did not return an installation URL");
      window.location.assign(url);
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "App installation failed",
        variant: "error",
      });
    }
  }

  if (view === "import") {
    return (
      <div className="space-y-4">
        <BackButton onClick={() => setView("choose")} />
        <GitHubAppImportForm
          workspaceId={workspaceId}
          registrations={registrations}
          onImported={(registrationId) => {
            setSelectedId(registrationId);
            setView("choose");
          }}
        />
      </div>
    );
  }
  if (view === "create") {
    return (
      <div className="space-y-4">
        <BackButton onClick={() => setView("choose")} />
        <GitHubAppCreateForm workspaceId={workspaceId} registrations={registrations} />
      </div>
    );
  }
  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">{t("github:chooseAGithubApp")}</h3>
        <p className="text-xs leading-5 text-muted-foreground">
          {t("github:useAnAppWhenAutomationNeeds")}
        </p>
      </div>
      {registrations.loading ? (
        <div className="flex min-h-11 items-center gap-2 text-sm text-muted-foreground">
          <Trans i18nKey="github:loadingRegisteredApps">
            <Spinner className="h-4 w-4" /> Loading registered Apps...
          </Trans>
        </div>
      ) : (
        <GitHubAppRegistrationList
          registrations={registrations.registrations}
          value={selectedId}
          onChange={setSelectedId}
        />
      )}
      {registrations.error && <p className="text-xs text-destructive">{registrations.error}</p>}
      <AppActionButtons
        installDisabled={selectedRegistration?.status !== "active" || registrations.mutating}
        mutating={registrations.mutating}
        onInstall={() => void install()}
        onImport={() => setView("import")}
        onCreate={() => setView("create")}
      />
      <p className="text-xs leading-5 text-muted-foreground">
        {t("github:aRegistrationCanBeReusedAcross")}
      </p>
    </div>
  );
}

// AppActionButtons is the install / add-existing / create-new button row of the
// GitHub App chooser view.
function AppActionButtons({
  installDisabled,
  mutating,
  onInstall,
  onImport,
  onCreate,
}: {
  installDisabled: boolean;
  mutating: boolean;
  onInstall: () => void;
  onImport: () => void;
  onCreate: () => void;
}) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
      <Button
        disabled={installDisabled}
        onClick={onInstall}
        className="h-11 cursor-pointer"
        data-testid="github-app-install-button"
      >
        <Trans
          i18nKey="github:installForThisWorkspace"
          values={{ value0: mutating && <Spinner className="mr-2 h-4 w-4" /> }}
        >
          {mutating && <Spinner className="mr-2 h-4 w-4" />}
          Install for this workspace
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Trans>
      </Button>
      <Button variant="outline" className="h-11 cursor-pointer" onClick={onImport}>
        <Trans i18nKey="github:addExistingApp">
          <IconPlus className="mr-2 h-4 w-4" /> Add existing App
        </Trans>
      </Button>
      <Button variant="outline" className="h-11 cursor-pointer" onClick={onCreate}>
        <Trans i18nKey="github:createNewApp">
          <IconPlus className="mr-2 h-4 w-4" /> Create new App
        </Trans>
      </Button>
    </div>
  );
}

function useAppRegistrationSelection(workspaceId: string, registrations: RegistrationHook) {
  const [selectedId, setSelectedId] = useState("");
  useEffect(() => setSelectedId(""), [workspaceId]);
  useEffect(() => {
    if (!registrations.loaded) return;
    setSelectedId((current) => {
      const currentRegistration = registrations.registrations.find(({ id }) => id === current);
      if (currentRegistration?.status === "active") return current;
      if (registrations.selected?.status === "active") return registrations.selected.id;
      return registrations.registrations.find(({ status }) => status === "active")?.id ?? "";
    });
  }, [registrations.loaded, registrations.registrations, registrations.selected]);
  const selectedRegistration = registrations.registrations.find(({ id }) => id === selectedId);
  return { selectedId, setSelectedId, selectedRegistration };
}

function BackButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <Button variant="ghost" className="h-11 cursor-pointer px-2" onClick={onClick}>
      {t("github:backToRegisteredApps")}
    </Button>
  );
}
