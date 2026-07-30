import { ApiError } from "@/lib/api/client";
import type {
  GitHubAppManifestOwnerType,
  GitHubAppOwnerType,
  GitHubAppRegistrationErrorBody,
  GitHubAppVisibility,
} from "@/lib/types/github";
import { t } from "@/lib/i18n";

const ownerPattern = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$/;
const slugPattern = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,98}[A-Za-z0-9])?$/;

export type AppSetupErrors = Partial<
  Record<
    | "displayName"
    | "ownerLogin"
    | "publicBaseUrl"
    | "appId"
    | "clientId"
    | "clientSecret"
    | "privateKey"
    | "webhookSecret"
    | "slug",
    string
  >
>;

export function normalizePublicBaseUrl(rawValue: string, errors: AppSetupErrors) {
  let url: URL;
  try {
    url = new URL(rawValue.trim());
  } catch {
    errors.publicBaseUrl = "Enter a public HTTPS origin.";
    return null;
  }
  if (url.protocol !== "https:" || url.username || url.password) {
    errors.publicBaseUrl = "Enter a public HTTPS origin.";
    return null;
  }
  if (url.hostname === "localhost" || url.hostname.endsWith(".localhost")) {
    errors.publicBaseUrl = "Enter a public host, not localhost.";
    return null;
  }
  if ((url.pathname && url.pathname !== "/") || url.search || url.hash) {
    errors.publicBaseUrl = "Enter an origin without a path, query, or fragment.";
    return null;
  }
  return url.origin;
}

export function validateRegistrationBasics(input: {
  displayName: string;
  ownerType: GitHubAppManifestOwnerType | GitHubAppOwnerType;
  ownerLogin: string;
  publicBaseUrl: string;
}) {
  const errors: AppSetupErrors = {};
  const displayName = input.displayName.trim();
  const ownerLogin = input.ownerLogin.trim();
  if (!displayName || displayName.length > 100)
    errors.displayName = "Enter a name up to 100 characters.";
  if (!ownerPattern.test(ownerLogin)) errors.ownerLogin = "Enter a valid GitHub account login.";
  const publicBaseUrl = normalizePublicBaseUrl(input.publicBaseUrl, errors);
  return { displayName, ownerLogin, publicBaseUrl, errors };
}

export function validateImportSecrets(input: {
  appId: string;
  clientId: string;
  clientSecret: string;
  privateKey: string;
  webhookSecret: string;
  slug: string;
}) {
  const errors: AppSetupErrors = {};
  const appId = Number(input.appId);
  if (!Number.isSafeInteger(appId) || appId <= 0) errors.appId = "Enter the numeric GitHub App ID.";
  if (!input.clientId.trim()) errors.clientId = "Enter the App client ID.";
  if (!input.clientSecret) errors.clientSecret = "Enter the App client secret.";
  if (!input.privateKey.trim()) errors.privateKey = "Paste the App private key.";
  if (!input.webhookSecret) errors.webhookSecret = "Enter the webhook secret.";
  if (!slugPattern.test(input.slug.trim())) errors.slug = "Enter the GitHub App slug.";
  return { appId, errors };
}

export function submitManifestToGitHub(registrationUrl: string, manifest: unknown) {
  const form = document.createElement("form");
  form.method = "POST";
  form.action = registrationUrl;
  const input = document.createElement("input");
  input.type = "hidden";
  input.name = "manifest";
  input.value = JSON.stringify(manifest);
  form.appendChild(input);
  document.body.appendChild(form);
  form.submit();
}

export type CallbackNotice = {
  tone: "success" | "info" | "error";
  title: string;
  description: string;
};

export function callbackNotice(code: string): CallbackNotice {
  if (code === "app_registered")
    return {
      tone: "success",
      title: t("github:githubAppAdded"),
      description: t("github:theAppIsReadyToSelect"),
    };
  if (code === "app_connected")
    return {
      tone: "success",
      title: t("github:githubAppConnected"),
      description: t("github:workspaceAutomationNowUsesTheVerified"),
    };
  if (code === "personal_connected")
    return {
      tone: "success",
      title: t("github:githubIdentityConnected"),
      description: t("github:myGithubNowUsesYourVerified"),
    };
  if (code === "github_app_registration_cancelled")
    return {
      tone: "info",
      title: t("github:githubAppSetupCancelled"),
      description: t("github:theExistingWorkspaceConnectionWasNot"),
    };
  return {
    tone: "error",
    title: t("github:githubSetupWasNotCompleted"),
    description: t("github:theGithubResponseCouldNotBe"),
  };
}

export function appRegistrationError(error: unknown) {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") return null;
  return error.body as GitHubAppRegistrationErrorBody;
}

export function githubAppSettingsURL(ownerType: GitHubAppOwnerType, owner: string, slug: string) {
  if (!owner || !slug) return "https://github.com/settings/apps";
  return ownerType === "Organization"
    ? `https://github.com/organizations/${encodeURIComponent(owner)}/settings/apps/${encodeURIComponent(slug)}`
    : `https://github.com/settings/apps/${encodeURIComponent(slug)}`;
}

export const defaultVisibility: GitHubAppVisibility = "private";
