// Wire shapes for the dedicated personal GitHub star connection. Split out
// of `github.ts` to keep that barrel under the file-length cap.
// Dedicated personal GitHub star connection (Settings > Plugins). This is a
// separate OAuth App authorization from the GitHub App user-to-server
// connection; `available` is false when the deployment cannot support it, in
// which case the UI renders no star control and issues no start request.
export type GitHubStarConnectionStatus = {
  user_id: string;
  credential_generation: number;
  active: boolean;
  capable: boolean;
  reconnect_required: boolean;
  available: boolean;
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
