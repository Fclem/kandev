import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  GitHubStarConnectionStatus,
  GitHubStarConnectionStartRequest,
  GitHubStarConnectionStartResponse,
  GitHubStarStatesRequest,
  GitHubStarStatesResponse,
  GitHubStarMutationRequest,
  GitHubStarMutationResponse,
} from "@/lib/types/github";

/**
 * Dedicated personal GitHub star connection (Settings > Plugins), re-exported
 * from `github-api` like the other GitHub domain modules.
 *
 * These calls never carry credentials and never build a GitHub API path:
 * repository URLs travel as raw plugin metadata and the backend canonicalizes
 * them. The `user/starred` routes describe the authenticated Kandev user's own
 * GitHub star, which only the dedicated OAuth App connection can perform.
 */
export async function fetchGitHubStarConnection(
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<GitHubStarConnectionStatus>(
    `/api/v1/github/user/starred/connection?${query.toString()}`,
    options,
  );
}

export async function startGitHubStarConnection(
  payload: GitHubStarConnectionStartRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubStarConnectionStartResponse>(
    "/api/v1/github/user/starred/connection/start",
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
    },
  );
}

export async function fetchGitHubStarStates(
  payload: GitHubStarStatesRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubStarStatesResponse>("/api/v1/github/user/starred/status", {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function starGitHubRepository(
  payload: GitHubStarMutationRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubStarMutationResponse>("/api/v1/github/user/starred", {
    ...options,
    init: { ...(options?.init ?? {}), method: "PUT", body: JSON.stringify(payload) },
  });
}

export async function unstarGitHubRepository(
  payload: GitHubStarMutationRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<GitHubStarMutationResponse>("/api/v1/github/user/starred", {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE", body: JSON.stringify(payload) },
  });
}
