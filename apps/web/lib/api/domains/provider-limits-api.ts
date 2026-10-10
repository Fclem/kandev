import { fetchJson } from "../client";
import type { ApiRequestOptions } from "../client";
import type { AgentProfileLimit } from "@/lib/types/http-agents";

export async function listAgentProfileLimits(
  options?: ApiRequestOptions,
): Promise<AgentProfileLimit[]> {
  return fetchJson<AgentProfileLimit[]>("/api/v1/agent-profiles/limits", options);
}
