import { fetchJson, type ApiRequestOptions } from "../client";
import type { SessionUsageTotals, UsageTurn, UsageTurnsPage } from "@/lib/types/conversation-usage";

export async function getSessionUsageTotals(
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<SessionUsageTotals> {
  return fetchJson<SessionUsageTotals>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/usage`,
    options,
  );
}

export type ListSessionUsageTurnsOptions = ApiRequestOptions & {
  cursor?: string;
  limit?: number;
};

export async function listSessionUsageTurns(
  taskId: string,
  sessionId: string,
  options?: ListSessionUsageTurnsOptions,
): Promise<UsageTurnsPage> {
  const { cursor, limit = 1, ...requestOptions } = options ?? {};
  const query = new URLSearchParams({ limit: String(limit) });
  if (cursor) query.set("cursor", cursor);
  return fetchJson<UsageTurnsPage>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/usage/turns?${query}`,
    options ? requestOptions : undefined,
  );
}

export async function getSessionUsageTurn(
  taskId: string,
  sessionId: string,
  turnId: string,
  options?: ApiRequestOptions,
): Promise<UsageTurn> {
  return fetchJson<UsageTurn>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/usage/turns/${encodeURIComponent(turnId)}`,
    options,
  );
}
