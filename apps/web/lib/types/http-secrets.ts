export type SecretScope = "global" | "workspace";

export interface SecretListItem {
  id: string;
  name: string;
  has_value: boolean;
  scope?: SecretScope;
  workspace_id?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateSecretRequest {
  name: string;
  value: string;
  scope?: SecretScope;
  workspace_id?: string;
}

export interface UpdateSecretRequest {
  name?: string;
  value?: string;
}

/**
 * Request body for copy/move operations. `name` is optional with presence
 * semantics matching the backend: omitted means "use the source secret's
 * name". It must never be emitted as `null` (the backend rejects it).
 */
export interface CopyMoveSecretRequest {
  target_scope: SecretScope;
  target_workspace_id?: string;
  name?: string;
}

export interface RevealSecretResponse {
  value: string;
}
