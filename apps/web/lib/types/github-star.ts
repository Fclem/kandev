// Wire shapes for the personal GitHub star connection. Split out of
// `github.ts` to keep that barrel under the file-length cap.

/** The GitHub identity star calls act as; never carries credential material. */
export type GitHubStarActor = {
  /** `personal_star_oauth`, `github_app_user`, `pat`, `gh_cli`, or `legacy_shared`. */
  source: string;
  login: string;
  /** True when every workspace member stars as this account. */
  shared: boolean;
};

/** Consent flow that can produce an actor when none resolved. */
export type GitHubStarConnectPath = "personal_star_oauth" | "github_app_user";

// Star connection status (Settings > Plugins). `available` is false when no
// actor, connect path, or dedicated connection exists, in which case the UI
// renders no star control and issues no start request.
export type GitHubStarConnectionStatus = {
  user_id: string;
  credential_generation: number;
  active: boolean;
  capable: boolean;
  reconnect_required: boolean;
  available: boolean;
  actor?: GitHubStarActor | null;
  connect?: GitHubStarConnectPath | null;
  remediation?: "token_scope" | "app_permission" | null;
};

export type GitHubStarConnectionStartRequest = {
  workspace_id: string;
  /** Absolute same-origin path under Settings > Plugins. */
  return_path: string;
};

export type GitHubStarConnectionStartResponse = {
  authorization_url: string;
  state_expires_at: string;
};

export type GitHubStarStatesRequest = {
  workspace_id: string;
  /** Raw repository URLs; the backend owns canonicalization. */
  repository_urls: string[];
};

/** `null` per repository means the provider call failed: unknown, not unstarred. */
export type GitHubStarStatesResponse = {
  states: Record<string, boolean | null>;
};

export type GitHubStarMutationRequest = {
  workspace_id: string;
  repository_url: string;
};

export type GitHubStarMutationResponse = {
  starred: boolean;
};
