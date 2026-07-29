// Pure form-state helpers for the Jira connection settings form: config <-> form
// mapping, instance/auth compatibility rules, and saved-secret reuse checks.

import type { IntegrationAuthHealth } from "@/components/integrations/auth-status-banner";
import type { JiraAuthMethod, JiraConfig, JiraInstanceType } from "@/lib/types/jira";

export type FormState = {
  siteUrl: string;
  email: string;
  authMethod: JiraAuthMethod;
  instanceType: JiraInstanceType;
  defaultProjectKey: string;
  secret: string;
};

export const emptyForm: FormState = {
  siteUrl: "",
  email: "",
  authMethod: "api_token",
  instanceType: "cloud",
  defaultProjectKey: "",
  secret: "",
};

export type FieldsRowProps = {
  form: FormState;
  baseline: FormState;
  loading: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

export function configToForm(cfg: JiraConfig | null): FormState {
  if (!cfg) return emptyForm;
  return {
    siteUrl: cfg.siteUrl,
    email: cfg.email,
    authMethod: cfg.authMethod,
    // Legacy rows written before Server/DC support carry an empty instanceType;
    // default to cloud so the dropdown has a valid selection.
    instanceType: cfg.instanceType || "cloud",
    defaultProjectKey: cfg.defaultProjectKey,
    secret: "",
  };
}

export function configToHealth(config: JiraConfig | null): IntegrationAuthHealth | null {
  if (!config?.hasSecret) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
}

// defaultAuthForInstance returns the canonical auth method for an instance
// type. Used when the user switches Instance type and the current auth method
// is no longer valid for the new type (e.g. PAT picked for Cloud).
export function defaultAuthForInstance(instance: JiraInstanceType): JiraAuthMethod {
  return instance === "server" ? "pat" : "api_token";
}

// authAllowedForInstance reports whether an auth method is allowed for a given
// instance type. Mirrors the backend validation so the user can't submit an
// invalid combination. session_cookie is Cloud-only today because the backend
// wraps the secret under cloud.session.token / tenant.session.token cookie
// names — Server/DC uses JSESSIONID, so the wrapping is a no-op there until we
// add a Server-aware path.
export function authAllowedForInstance(auth: JiraAuthMethod, instance: JiraInstanceType): boolean {
  if (auth === "api_token") return instance === "cloud";
  if (auth === "pat") return instance === "server";
  if (auth === "session_cookie") return instance === "cloud";
  return false;
}

function normalizeComparableSiteUrl(value: string): string {
  const trimmed = value.trim().replace(/\/+$/, "");
  if (!trimmed) return "";
  return trimmed.includes("://") ? trimmed : `https://${trimmed}`;
}

// savedSecretMatches reports whether the saved secret can be reused against
// the current form values. Reuse is only safe when every identity component
// of the saved credential still matches: same auth method, same instance
// type, same Jira host, and — for Cloud api_token where the basic pair is
// email:token — the same email (case-insensitive). Otherwise the user could
// change the site URL or Cloud account and silently submit the previous
// token to a different host/account.
export function savedSecretMatches(config: JiraConfig | null, form: FormState): boolean {
  if (!config?.hasSecret) return false;
  if (config.authMethod !== form.authMethod) return false;
  if ((config.instanceType || "cloud") !== form.instanceType) return false;
  if (normalizeComparableSiteUrl(config.siteUrl) !== normalizeComparableSiteUrl(form.siteUrl)) {
    return false;
  }
  if (form.authMethod !== "api_token") return true;
  return (config.email ?? "").toLowerCase() === form.email.toLowerCase();
}
