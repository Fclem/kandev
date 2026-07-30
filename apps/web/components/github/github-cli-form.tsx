"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import { fetchGitHubCLIAccounts, setGitHubWorkspaceConnection } from "@/lib/api/domains/github-api";
import type { GitHubCLIAccount } from "@/lib/types/github";
import { Trans, useTranslation } from "react-i18next";

function GitHubCLIAccountNotice({
  loadError,
  loading,
  hasAccounts,
}: {
  loadError: string | null;
  loading: boolean;
  hasAccounts: boolean;
}) {
  const { t } = useTranslation();
  if (loading || hasAccounts) return null;
  if (loadError) {
    return (
      <p role="alert" className="text-xs text-destructive">
        <Trans i18nKey="github:couldNotLoadGithubCliAccounts" values={{ loadError }}>
          Could not load GitHub CLI accounts. {loadError}
        </Trans>
      </p>
    );
  }
  return (
    <p className="text-xs text-muted-foreground">
      <Trans i18nKey="github:signInWithThenReopenThis">
        {t("github:signInWith")} <code>{t("github:ghAuthLogin")}</code>, then reopen this dialog.
      </Trans>
    </p>
  );
}

function accountPlaceholder(loading: boolean, loadError: string | null) {
  if (loading) return "Loading accounts...";
  if (loadError) return "Accounts unavailable";
  return "No gh accounts found";
}

// useGitHubCLIAccounts loads the gh CLI accounts for a workspace and keeps the
// selected-account draft in sync with the fetched list.
function useGitHubCLIAccounts(workspaceId: string) {
  const [accounts, setAccounts] = useState<GitHubCLIAccount[]>([]);
  const [selected, setSelected] = useState("");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let current = true;
    setAccounts([]);
    setSelected("");
    setLoadError(null);
    setLoading(true);
    setSaving(false);
    fetchGitHubCLIAccounts(workspaceId, { cache: "no-store" })
      .then((items) => {
        if (!current) return;
        setAccounts(items);
        const preferred =
          items.find((item) => item.selected) ?? items.find((item) => item.active) ?? items[0];
        setSelected(preferred ? `${preferred.host}\n${preferred.login}` : "");
      })
      .catch((error) => {
        if (!current) return;
        setAccounts([]);
        setLoadError(
          error instanceof Error ? error.message : "Unexpected response from the server",
        );
      })
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [workspaceId]);

  return { accounts, selected, setSelected, loadError, loading, saving, setSaving };
}

export function GitHubCLIForm({
  workspaceId,
  onSaved,
}: {
  workspaceId: string;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const { accounts, selected, setSelected, loadError, loading, saving, setSaving } =
    useGitHubCLIAccounts(workspaceId);
  const { toast } = useToast();

  const account = useMemo(() => {
    const [host, login] = selected.split("\n");
    return accounts.find((item) => item.host === host && item.login === login);
  }, [accounts, selected]);

  const connect = useCallback(async () => {
    if (!account) return;
    setSaving(true);
    try {
      await setGitHubWorkspaceConnection(workspaceId, {
        source: "gh_cli",
        host: account.host,
        login: account.login,
      });
      toast({ description: `Connected ${account.login} for this workspace`, variant: "success" });
      onSaved();
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Connection failed",
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  }, [account, onSaved, setSaving, toast, workspaceId]);

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="github-cli-account">{t("github:githubCliAccount")}</Label>
        <p className="text-xs text-muted-foreground">
          {t("github:chooseTheExactAccountKandevDoes")}
        </p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
        <Select value={selected} onValueChange={setSelected} disabled={loading || !accounts.length}>
          <SelectTrigger id="github-cli-account" className="min-h-11 min-w-0 flex-1">
            <SelectValue placeholder={accountPlaceholder(loading, loadError)} />
          </SelectTrigger>
          <SelectContent>
            {accounts.map((item) => (
              <SelectItem key={`${item.host}:${item.login}`} value={`${item.host}\n${item.login}`}>
                {item.login} ({item.host}){item.active ? " - active" : ""}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button onClick={connect} disabled={!account || saving} className="h-11 cursor-pointer">
          <Trans
            i18nKey="github:useAccount"
            values={{ value0: saving && <Spinner className="mr-2 h-4 w-4" /> }}
          >
            {saving && <Spinner className="mr-2 h-4 w-4" />}
            {t("github:useAccount2")}
          </Trans>
        </Button>
      </div>
      <GitHubCLIAccountNotice
        loadError={loadError}
        loading={loading}
        hasAccounts={accounts.length > 0}
      />
    </div>
  );
}
