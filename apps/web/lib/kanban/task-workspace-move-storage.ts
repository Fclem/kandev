import type { TaskTransferRequest } from "@/lib/types/http";

/**
 * Tab-scoped persistence for unresolved workspace-move operations, so a full
 * page reload keeps the operation identity and can offer `Check result`.
 *
 * A record holds only the request identity (idempotency key, task, expected
 * source placement and generation, destination IDs), the transfer policy, a
 * digest of the immutable request, and the authenticated caller. It never
 * holds transcript content, credentials, tokens, or authority. Records are
 * partitioned by caller and task and survive identity changes and logout; only
 * a committed receipt or an authoritative terminal rejection removes one.
 */

const STORAGE_PREFIX = "kandev.task-workspace-move.v2:"; // i18n-exempt: storage key prefix
const TRANSFER_POLICY = "preserve-task-identity-v1"; // i18n-exempt: server policy identifier

const BODY_FIELDS = [
  "expected_source_workspace_id",
  "expected_source_workflow_id",
  "expected_source_workflow_step_id",
  "expected_task_updated_at",
  "destination_workspace_id",
  "destination_workflow_id",
  "destination_workflow_step_id",
  "idempotency_key",
] as const;

type StoredRecord = {
  caller: string;
  taskId: string;
  body: TaskTransferRequest;
  policy: string;
  digest: string;
};

export type RestoredOperations = {
  bodies: Map<string, TaskTransferRequest>;
  /** Tasks whose stored record existed but could not be validated. */
  invalidTaskIds: Set<string>;
  /** Storage could not be read at all. */
  unavailable: boolean;
};

function storage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.sessionStorage;
  } catch {
    return null;
  }
}

/** One entry per caller and task, so callers never read or replace each other's records. */
function callerPrefix(caller: string): string {
  return `${STORAGE_PREFIX}${encodeURIComponent(caller)}:`;
}

function entryKey(caller: string, taskId: string): string {
  return `${callerPrefix(caller)}${encodeURIComponent(taskId)}`;
}

/** FNV-1a over the canonical field order of the immutable request. */
export function transferRequestDigest(taskId: string, body: TaskTransferRequest): string {
  const canonical = JSON.stringify([taskId, TRANSFER_POLICY, ...BODY_FIELDS.map((f) => body[f])]);
  let hash = 0x811c9dc5;
  for (let index = 0; index < canonical.length; index++) {
    hash ^= canonical.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash.toString(16).padStart(8, "0");
}

function validBody(value: unknown): value is TaskTransferRequest {
  if (!value || typeof value !== "object") return false;
  const keys = Object.keys(value);
  if (keys.length !== BODY_FIELDS.length) return false;
  return BODY_FIELDS.every((field) => {
    const fieldValue: unknown = Reflect.get(value, field);
    return typeof fieldValue === "string" && fieldValue.length > 0;
  });
}

function validRecord(value: unknown, caller: string, taskId: string): value is StoredRecord {
  if (!value || typeof value !== "object") return false;
  const body: unknown = Reflect.get(value, "body");
  return (
    Reflect.get(value, "caller") === caller &&
    Reflect.get(value, "taskId") === taskId &&
    Reflect.get(value, "policy") === TRANSFER_POLICY &&
    validBody(body) &&
    Reflect.get(value, "digest") === transferRequestDigest(taskId, body)
  );
}

function parse(raw: string | null): unknown {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

/**
 * Reads only this caller's records. Other callers' records stay stored,
 * untouched and invisible, until their own caller returns.
 */
export function restoreWorkspaceMoves(caller: string): RestoredOperations {
  const restored: RestoredOperations = {
    bodies: new Map(),
    invalidTaskIds: new Set(),
    unavailable: false,
  };
  const store = storage();
  if (!store) return { ...restored, unavailable: true };
  const prefix = callerPrefix(caller);
  try {
    for (let index = 0; index < store.length; index++) {
      const key = store.key(index);
      if (!key?.startsWith(prefix)) continue;
      const taskId = decodeURIComponent(key.slice(prefix.length));
      const record = parse(store.getItem(key));
      if (validRecord(record, caller, taskId)) restored.bodies.set(taskId, record.body);
      else restored.invalidTaskIds.add(taskId);
    }
  } catch {
    return { ...restored, unavailable: true };
  }
  return restored;
}

/** Stores one unresolved request for this caller and task; returns false when storage is unavailable. */
export function persistWorkspaceMove(
  caller: string,
  taskId: string,
  body: TaskTransferRequest,
): boolean {
  const store = storage();
  if (!store) return false;
  const record: StoredRecord = {
    caller,
    taskId,
    body,
    policy: TRANSFER_POLICY,
    digest: transferRequestDigest(taskId, body),
  };
  try {
    store.setItem(entryKey(caller, taskId), JSON.stringify(record));
    return true;
  } catch {
    return false;
  }
}

/** Forgets this caller's record for one task after a committed receipt or an authoritative terminal rejection. */
export function forgetWorkspaceMove(caller: string, taskId: string): void {
  try {
    storage()?.removeItem(entryKey(caller, taskId));
  } catch {
    // Storage that cannot be written holds nothing to forget.
  }
}
