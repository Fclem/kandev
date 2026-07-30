"use client";

import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { IconTicket, IconCode } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { TaskPresetsSection } from "@/components/jira/task-presets-section";
import { JiraIssueWatchersSection } from "@/components/jira/jira-issue-watchers-section";
import { JiraEnabledControl } from "@/components/jira/jira-enabled-control";
import { IntegrationAuthStatusBanner } from "@/components/integrations/auth-status-banner";
import {
  authAllowedForInstance,
  configToForm,
  configToHealth,
  defaultAuthForInstance,
  emptyForm,
  savedSecretMatches,
  type FieldsRowProps,
  type FormState,
} from "@/components/jira/jira-settings-form-state";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  getJiraConfig,
  setJiraConfig,
  deleteJiraConfig,
  testJiraConnection,
} from "@/lib/api/domains/jira-api";
import type {
  JiraAuthMethod,
  JiraConfig,
  JiraInstanceType,
  TestJiraConnectionResult,
} from "@/lib/types/jira";
import { Trans, useTranslation } from "react-i18next";

// Session cookies are HttpOnly so document.cookie can't read them, but
// DevTools → Application → Cookies surfaces them in plain text. Users copy
// the Value cell of a single row; the backend wraps it under both
// cloud.session.token and tenant.session.token so a single paste works for
// password accounts and SSO tenants.
const COOKIE_INSTRUCTIONS = `Open DevTools (Cmd+Opt+I / Ctrl+Shift+I) on your Atlassian tab →
Application tab → Storage → Cookies → https://*.atlassian.net →
find the row named "cloud.session.token" (or "tenant.session.token"
on SSO tenants) → copy the Value cell → paste it below.
Don't include the cookie name or any "=" — just the token value.`;

type InstanceFieldsProps = FieldsRowProps & {
  setForm: Dispatch<SetStateAction<FormState>>;
};

function InstanceFields({ form, baseline, loading, setForm }: InstanceFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="jira-instance">{t("jira:instanceType")}</Label>
        <Select
          value={form.instanceType}
          onValueChange={(v) => {
            const next = v as JiraInstanceType;
            setForm((prev) => {
              // Switching instance type changes which auth methods are valid;
              // if the current one would become invalid, swap it for the
              // canonical default for the new instance.
              const auth = authAllowedForInstance(prev.authMethod, next)
                ? prev.authMethod
                : defaultAuthForInstance(next);
              return { ...prev, instanceType: next, authMethod: auth };
            });
          }}
          disabled={loading}
        >
          <SelectTrigger
            id="jira-instance"
            className="w-full cursor-pointer"
            data-settings-dirty={form.instanceType !== baseline.instanceType}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="cloud">{t("jira:atlassianCloud")}</SelectItem>
            <SelectItem value="server">{t("jira:serverDataCenter")}</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          {form.instanceType === "cloud"
            ? "Sites hosted at *.atlassian.net."
            : "Self-hosted Jira (Server or Data Center)."}
        </p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="jira-project">{t("jira:defaultProjectKeyOptional")}</Label>
        <Input
          id="jira-project"
          data-testid="jira-project-input"
          placeholder="PROJ"
          value={form.defaultProjectKey}
          data-settings-dirty={form.defaultProjectKey !== baseline.defaultProjectKey}
          onChange={(e) =>
            setForm((prev) => ({ ...prev, defaultProjectKey: e.target.value.toUpperCase() }))
          }
          disabled={loading}
        />
      </div>
    </div>
  );
}

function SiteFields({ form, baseline, loading, update }: FieldsRowProps) {
  const { t } = useTranslation();
  const placeholder =
    form.instanceType === "server" ? "https://jira.your-company.com" : "https://acme.atlassian.net";
  return (
    <div className="space-y-1.5">
      <Label htmlFor="jira-site">{t("jira:siteUrl")}</Label>
      <Input
        id="jira-site"
        data-testid="jira-site-input"
        placeholder={placeholder}
        value={form.siteUrl}
        data-settings-dirty={form.siteUrl !== baseline.siteUrl}
        onChange={(e) => update("siteUrl", e.target.value)}
        disabled={loading}
      />
    </div>
  );
}

function AuthFields({ form, baseline, loading, update }: FieldsRowProps) {
  const { t } = useTranslation();
  const showEmail = form.instanceType === "cloud" && form.authMethod === "api_token";
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="jira-auth">{t("jira:authenticationMethod")}</Label>
        <Select
          value={form.authMethod}
          onValueChange={(v) => update("authMethod", v as JiraAuthMethod)}
          disabled={loading}
        >
          <SelectTrigger
            id="jira-auth"
            className="w-full cursor-pointer"
            data-settings-dirty={form.authMethod !== baseline.authMethod}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {form.instanceType === "cloud" ? (
              <>
                <SelectItem value="api_token">{t("jira:apiTokenRecommended")}</SelectItem>
                <SelectItem value="session_cookie">{t("jira:browserSessionCookie")}</SelectItem>
              </>
            ) : (
              <SelectItem value="pat">{t("jira:personalAccessToken")}</SelectItem>
            )}
          </SelectContent>
        </Select>
      </div>
      {showEmail ? (
        <div className="space-y-1.5">
          <Label htmlFor="jira-email">{t("jira:email")}</Label>
          <Input
            id="jira-email"
            data-testid="jira-email-input"
            type="email"
            placeholder="you@example.com"
            value={form.email}
            data-settings-dirty={form.email !== baseline.email}
            onChange={(e) => update("email", e.target.value)}
            disabled={loading}
          />
        </div>
      ) : (
        // Keep the grid balanced so the auth select doesn't span both columns.
        <div aria-hidden className="hidden sm:block" />
      )}
    </div>
  );
}

function SessionSnippet() {
  const [show, setShow] = useState(false);
  return (
    <div className="text-xs text-muted-foreground space-y-2">
      <button
        type="button"
        onClick={() => setShow((v) => !v)}
        className="inline-flex items-center gap-1 underline cursor-pointer"
      >
        <Trans i18nKey="jira:howToCopyTheSessionToken" values={{ value1: show ? "Hide" : "Show" }}>
          <IconCode className="h-3 w-3" />
          {show ? "Hide" : "Show"} how to copy the session token
        </Trans>
      </button>
      {show && (
        <pre className="bg-muted rounded p-3 text-[11px] overflow-x-auto whitespace-pre-wrap">
          <code>{COOKIE_INSTRUCTIONS}</code>
        </pre>
      )}
    </div>
  );
}

type SecretFieldProps = FieldsRowProps & { hasSavedSecret: boolean };

// SECRET_COPY centralizes the field label and empty-state placeholder per
// auth method. Keyed by JiraAuthMethod so adding a new method causes the
// type system to flag the missing entry.
const SECRET_COPY: Record<JiraAuthMethod, { label: string; placeholder: string }> = {
  api_token: { label: "API token", placeholder: "paste API token here" },
  pat: { label: "Personal Access Token", placeholder: "paste personal access token here" },
  session_cookie: { label: "Session token value", placeholder: "paste cloud.session.token value" },
};

function secretPlaceholder(method: JiraAuthMethod, hasSavedSecret: boolean): string {
  return hasSavedSecret ? "••••••••" : SECRET_COPY[method].placeholder;
}

function formatExpiry(expiresAt: string): { label: string; tone: "ok" | "warn" | "danger" } {
  const diffMs = new Date(expiresAt).getTime() - Date.now();
  if (Number.isNaN(diffMs)) return { label: "Expiry unknown", tone: "warn" };
  if (diffMs <= 0) return { label: "Cookie expired — paste a fresh one", tone: "danger" };
  const hours = diffMs / (60 * 60 * 1000);
  if (hours < 24) {
    const h = Math.max(1, Math.round(hours));
    return { label: `Cookie expires in ${h}h`, tone: "danger" };
  }
  const days = Math.round(hours / 24);
  return {
    label: `Cookie expires in ${days} day${days === 1 ? "" : "s"}`,
    tone: days < 7 ? "warn" : "ok",
  };
}

const TONE_CLASSES: Record<"ok" | "warn" | "danger", string> = {
  ok: "text-muted-foreground",
  warn: "text-amber-600 dark:text-amber-400",
  danger: "text-destructive",
};

function CookieExpiry({ expiresAt }: { expiresAt: string }) {
  const { label, tone } = formatExpiry(expiresAt);
  const absolute = new Date(expiresAt).toLocaleString();
  return (
    <p className={`text-xs ${TONE_CLASSES[tone]}`} title={absolute}>
      {label}
    </p>
  );
}

type SecretFieldPropsWithExpiry = SecretFieldProps & { secretExpiresAt?: string | null };

function SecretField({
  form,
  baseline,
  loading,
  update,
  hasSavedSecret,
  secretExpiresAt,
}: SecretFieldPropsWithExpiry) {
  const { t } = useTranslation();
  const method = form.authMethod;
  const siteUrl = form.siteUrl.replace(/\/+$/, "");
  const patHref = siteUrl ? `${siteUrl}/secure/ViewProfile.jspa` : undefined;
  return (
    <div className="space-y-1.5">
      <Label htmlFor="jira-secret">
        {SECRET_COPY[method].label}
        {hasSavedSecret && (
          <span className="text-xs text-muted-foreground ml-2">
            {t("jira:savedLeaveBlankToKeepThe")}
          </span>
        )}
      </Label>
      <Input
        id="jira-secret"
        data-testid="jira-secret-input"
        type="password"
        placeholder={secretPlaceholder(method, hasSavedSecret)}
        value={form.secret}
        data-settings-dirty={form.secret !== baseline.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
      />
      {method === "session_cookie" && hasSavedSecret && secretExpiresAt && (
        <CookieExpiry expiresAt={secretExpiresAt} />
      )}
      {method === "api_token" && (
        <p className="text-xs text-muted-foreground">
          <Trans i18nKey="jira:createATokenAt">
            Create a token at{" "}
            <a
              className="underline cursor-pointer"
              href="https://id.atlassian.com/manage-profile/security/api-tokens"
              target="_blank"
              rel="noreferrer"
            >
              {t("jira:idAtlassianComManageProfileSecurity")}
            </a>
          </Trans>
        </p>
      )}
      {method === "pat" && (
        <p className="text-xs text-muted-foreground">
          {patHref ? (
            <Trans i18nKey="jira:createPatFromProfileWithLink" values={{ patHref }}>
              Create a Personal Access Token from your Jira profile (
              <a
                className="underline cursor-pointer"
                href={patHref}
                target="_blank"
                rel="noreferrer"
              >
                {patHref}
              </a>
              ) → Personal Access Tokens. Required scopes: read &amp; write.
            </Trans>
          ) : (
            t("jira:createPatFromProfile")
          )}
        </p>
      )}
      {method === "session_cookie" && <SessionSnippet />}
    </div>
  );
}

function TestResultAlert({ result }: { result: TestJiraConnectionResult | null }) {
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>
        {result.ok
          ? `Connected as ${result.displayName || result.email || result.accountId}`
          : `Failed: ${result.error}`}
      </AlertDescription>
    </Alert>
  );
}

type ActionBarProps = {
  testing: boolean;
  loading: boolean;
  hasConfig: boolean;
  disableTest: boolean;
  onTest: () => void;
  onDelete: () => void;
};

function ActionBar({ testing, loading, hasConfig, disableTest, onTest, onDelete }: ActionBarProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        onClick={onTest}
        disabled={testing || loading || disableTest}
        className="cursor-pointer"
        title={disableTest ? "Paste a token to test the connection" : undefined}
        data-testid="jira-test-button"
      >
        {testing ? "Testing..." : "Test connection"}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          className="ml-auto cursor-pointer"
          data-testid="jira-delete-button"
        >
          {t("jira:removeConfiguration")}
        </Button>
      )}
    </div>
  );
}

function useJiraConfigRefresh(workspaceId: string, setConfig: (cfg: JiraConfig | null) => void) {
  // Background refresh so the auth-health banner picks up new probe results
  // from the backend poller without requiring a page reload. We re-fetch the
  // config rather than the loud full `load()` to avoid flashing the form.
  useEffect(() => {
    const id = setInterval(() => {
      getJiraConfig({ workspaceId })
        .then((cfg) => setConfig(cfg))
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, [workspaceId, setConfig]);
}

function useJiraSettings(workspaceId: string) {
  const { toast } = useToast();
  const [config, setConfig] = useState<JiraConfig | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestJiraConnectionResult | null>(null);
  const health = configToHealth(config);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = await getJiraConfig({ workspaceId });
      setConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({ description: `Failed to load Jira config: ${String(err)}`, variant: "error" });
    } finally {
      setLoading(false);
    }
  }, [workspaceId, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  useJiraConfigRefresh(workspaceId, setConfig);

  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testJiraConnection({ ...form }, { workspaceId });
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, form]);

  const handleSave = useCallback(async () => {
    const submitted = form;
    setSaving(true);
    try {
      const saved = await setJiraConfig(
        {
          siteUrl: form.siteUrl,
          email: form.email,
          authMethod: form.authMethod,
          instanceType: form.instanceType,
          defaultProjectKey: form.defaultProjectKey,
          secret: form.secret || undefined,
        },
        { workspaceId },
      );
      setConfig(saved);
      setForm((current) =>
        JSON.stringify(current) === JSON.stringify(submitted) ? configToForm(saved) : current,
      );
      // Clear any inline test result from the previous credentials so the
      // alert reflects only the currently-saved state.
      setTestResult(null);
      toast({ description: "Jira configuration saved", variant: "success" });
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
      throw err;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, toast]);

  const handleDelete = useCallback(async () => {
    if (!confirm("Remove Jira configuration?")) return;
    try {
      await deleteJiraConfig({ workspaceId });
      setConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: "Jira configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
    }
  }, [workspaceId, toast]);
  const discard = useCallback(() => setForm(configToForm(config)), [config]);

  return {
    config,
    form,
    setForm,
    loading,
    saving,
    testing,
    testResult,
    health,
    update,
    handleTest,
    handleSave,
    handleDelete,
    discard,
  };
}

export function JiraConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const s = useJiraSettings(workspaceId);
  const baseline = configToForm(s.config);
  const savedSecretMatchesMode = savedSecretMatches(s.config, s.form);
  const missingSecret = !savedSecretMatchesMode && !s.form.secret;
  const emailRequired = s.form.instanceType === "cloud" && s.form.authMethod === "api_token";
  const disableSave =
    s.saving || !s.form.siteUrl || (emailRequired && !s.form.email) || missingSecret;
  const disableTest = missingSecret;
  const revision = JSON.stringify(s.form);
  const dirty = !s.loading && revision !== JSON.stringify(configToForm(s.config));
  let invalidReason: string | undefined;
  if (!s.form.siteUrl) invalidReason = "A Jira site URL is required.";
  else if (emailRequired && !s.form.email) invalidReason = "An email address is required.";
  else if (missingSecret) invalidReason = "A credential is required.";

  useSettingsSaveContributor({
    id: `jira-config:${workspaceId}`,
    revision,
    isDirty: dirty,
    canSave: !disableSave,
    invalidReason,
    save: s.handleSave,
    discard: s.discard,
  });

  return (
    <SettingsSection
      icon={<IconTicket className="h-5 w-5" />}
      title={t("jira:jiraIntegration")}
      description="Connect this workspace to Atlassian Cloud or a self-hosted Jira Server / Data Center instance. Credentials are stored encrypted server-side for the selected workspace."
      action={<JiraEnabledControl />}
    >
      <SettingsCard isDirty={dirty}>
        <CardContent className="space-y-4 pt-6">
          <IntegrationAuthStatusBanner health={s.health} />
          <InstanceFields
            form={s.form}
            baseline={baseline}
            loading={s.loading}
            update={s.update}
            setForm={s.setForm}
          />
          <SiteFields form={s.form} baseline={baseline} loading={s.loading} update={s.update} />
          <AuthFields form={s.form} baseline={baseline} loading={s.loading} update={s.update} />
          <SecretField
            form={s.form}
            baseline={baseline}
            loading={s.loading}
            update={s.update}
            hasSavedSecret={savedSecretMatchesMode}
            secretExpiresAt={s.config?.secretExpiresAt ?? null}
          />
          <TestResultAlert result={s.testResult} />
          <Separator />
          <ActionBar
            testing={s.testing}
            loading={s.loading}
            hasConfig={!!s.config}
            disableTest={disableTest}
            onTest={s.handleTest}
            onDelete={s.handleDelete}
          />
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}

export function JiraIntegrationPage({ workspaceId }: { workspaceId?: string } = {}) {
  return (
    <div className="space-y-8">
      <WorkspaceScopedSection workspaceId={workspaceId}>
        {(workspaceId) => <JiraConnectionSection key={workspaceId} workspaceId={workspaceId} />}
      </WorkspaceScopedSection>
      <JiraIssueWatchersSection />
      <TaskPresetsSection />
    </div>
  );
}
