import { ApiError } from "@/lib/api/client";
import type { TaskTransferReceipt, TaskTransferRequest } from "@/lib/types/http";
import {
  forgetWorkspaceMove,
  persistWorkspaceMove,
  restoreWorkspaceMoves,
} from "./task-workspace-move-storage";

/**
 * One submitted workspace transfer. The request body, including its
 * idempotency key, is immutable from the first POST until the operation is
 * resolved: a committed receipt or an authoritative `operation_rejected`
 * answer to a result check. A denied, unavailable, or otherwise conflicting
 * check never proves the original move failed, so it keeps the operation
 * unresolved.
 */
export type WorkspaceMoveOperation =
  | { status: "submitting"; taskId: string; body: TaskTransferRequest; attempt: "submit" | "check" }
  | { status: "uncertain"; taskId: string; body: TaskTransferRequest }
  | {
      status: "check-unavailable";
      taskId: string;
      body: TaskTransferRequest;
      code?: TransferRejectionCode;
    }
  | { status: "committed"; taskId: string; body: TaskTransferRequest; receipt: TaskTransferReceipt }
  | { status: "rejected"; taskId: string; body: TaskTransferRequest; code: TransferRejectionCode }
  | { status: "operation-rejected"; taskId: string; body: TaskTransferRequest };

/** Rejections of an original submission that are safe to show the caller. */
export type TransferRejectionCode =
  | "source_changed"
  | "destination_unavailable"
  | "step_incompatible"
  | "destination_full"
  | "workspace_relation_unmapped"
  | "profile_mapping_required"
  | "pending_step_unmapped"
  | "cleanup_active"
  | "idempotency_conflict"
  | "transfer_unsupported"
  | "relations_busy"
  | "forbidden"
  | "unavailable"
  | "invalid";

const KNOWN_CONFLICT_CODES = new Set<string>([
  "source_changed",
  "destination_unavailable",
  "step_incompatible",
  "destination_full",
  "workspace_relation_unmapped",
  "profile_mapping_required",
  "pending_step_unmapped",
  "cleanup_active",
  "idempotency_conflict",
  "transfer_unsupported",
  "relations_busy",
]);

function conflictCode(error: ApiError): string | null {
  const body = error.body;
  if (!body || typeof body !== "object" || !("code" in body)) return null;
  return typeof body.code === "string" ? body.code : null;
}

/**
 * Classifies a transfer HTTP outcome. `attempt` distinguishes the original
 * submission from an explicit result check of the same body and key.
 */
export function settleWorkspaceMove(
  operation: Extract<WorkspaceMoveOperation, { status: "submitting" }>,
  outcome: { receipt: TaskTransferReceipt } | { error: unknown },
): WorkspaceMoveOperation {
  const { taskId, body } = operation;
  if ("receipt" in outcome) return { status: "committed", taskId, body, receipt: outcome.receipt };
  const error = outcome.error;
  const status = error instanceof ApiError ? error.status : 0;
  if (operation.attempt === "check") return settleCheckAttempt(taskId, body, error, status);
  if (!(error instanceof ApiError) || status >= 500 || status === 0 || status === 499) {
    return { status: "uncertain", taskId, body };
  }
  return { status: "rejected", taskId, body, code: originalRejectionCode(error) };
}

function settleCheckAttempt(
  taskId: string,
  body: TaskTransferRequest,
  error: unknown,
  status: number,
): WorkspaceMoveOperation {
  if (status === 409 && error instanceof ApiError && conflictCode(error) === "operation_rejected") {
    return { status: "operation-rejected", taskId, body };
  }
  if (status === 0 || status >= 500) return { status: "uncertain", taskId, body };
  if (status === 409 && error instanceof ApiError) {
    const code = conflictCode(error);
    if (code && KNOWN_CONFLICT_CODES.has(code)) {
      return { status: "check-unavailable", taskId, body, code: code as TransferRejectionCode };
    }
  }
  return { status: "check-unavailable", taskId, body };
}

function originalRejectionCode(error: ApiError): TransferRejectionCode {
  if (error.status === 403) return "forbidden";
  if (error.status === 404) return "unavailable";
  if (error.status === 409) {
    const code = conflictCode(error);
    return code && KNOWN_CONFLICT_CODES.has(code)
      ? (code as TransferRejectionCode)
      : "transfer_unsupported";
  }
  return "invalid";
}

/** Unresolved operations keep their submitted choices and key. */
export function isUnresolvedWorkspaceMove(operation: WorkspaceMoveOperation | null): boolean {
  return (
    operation?.status === "submitting" ||
    operation?.status === "uncertain" ||
    operation?.status === "check-unavailable"
  );
}

type Listener = () => void;

/**
 * Registry of workspace-move operations, scoped by authenticated identity and
 * task. Only the current caller's operations are visible. Unresolved request
 * identities are kept in tab-scoped storage per caller and task, so a reload,
 * an identity switch away and back, or a logout and login restores them as
 * `uncertain` (offering `Check result`); no POST is replayed automatically.
 * Only a committed receipt or an authoritative terminal rejection forgets one.
 */
export class WorkspaceMoveOperations {
  private identity: string | null = null;
  private readonly byTask = new Map<string, WorkspaceMoveOperation>();
  /** The in-flight request admitted for each task; only it may settle that task. */
  private readonly live = new Map<string, object>();
  private readonly unverifiable = new Set<string>();
  /** Committed receipts hidden as `submitting` until canonical membership is reconciled. */
  private readonly reconciling = new Map<string, WorkspaceMoveOperation>();
  private storageUnavailable = false;
  private readonly listeners = new Set<Listener>();

  get(identity: string, taskId: string): WorkspaceMoveOperation | null {
    this.adopt(identity);
    return this.reconciling.get(taskId) ?? this.byTask.get(taskId) ?? null;
  }

  /** Hides an accepted receipt from the visible surface until membership is reconciled. */
  beginReconcile(identity: string, taskId: string): object | null {
    if (identity !== this.identity) return null;
    const committed = this.byTask.get(taskId);
    if (committed?.status !== "committed") return null;
    const overlay = {
      status: "submitting" as const,
      taskId,
      body: committed.body,
      attempt: "submit" as const,
    };
    this.reconciling.set(taskId, overlay);
    this.emit();
    return overlay;
  }

  /** Clears only the overlay this reconcile created; a superseded token is ignored. */
  endReconcile(identity: string, taskId: string, overlay: object): void {
    if (identity !== this.identity) return;
    if (this.reconciling.get(taskId) !== overlay) return;
    this.reconciling.delete(taskId);
    this.emit();
  }

  /** A restored record for this task failed validation and cannot be checked here. */
  isUnverifiable(identity: string, taskId: string): boolean {
    this.adopt(identity);
    return this.storageUnavailable || this.unverifiable.has(taskId);
  }

  /** Admits one request synchronously; a second click while busy is refused. */
  begin(
    identity: string,
    taskId: string,
    body: TaskTransferRequest,
    attempt: "submit" | "check",
  ): Extract<WorkspaceMoveOperation, { status: "submitting" }> | null {
    this.adopt(identity);
    const current = this.byTask.get(taskId);
    if (this.reconciling.has(taskId)) return null;
    if (current?.status === "submitting") return null;
    if (attempt === "submit" && isUnresolvedWorkspaceMove(current ?? null)) return null;
    if (attempt === "submit" && (this.storageUnavailable || this.unverifiable.has(taskId))) {
      return null;
    }
    if (attempt === "check" && (!current || !isUnresolvedWorkspaceMove(current))) return null;
    const next = { status: "submitting" as const, taskId, body, attempt };
    // The record must be durable before any request can leave this tab.
    if (!persistWorkspaceMove(identity, taskId, body)) return null;
    this.live.set(taskId, next);
    this.set(taskId, next);
    return next;
  }

  settle(identity: string, pending: object, operation: WorkspaceMoveOperation): void {
    if (identity !== this.identity) return;
    // A superseded or pre-switch request must not overwrite a newer outcome.
    if (this.live.get(operation.taskId) !== pending) return;
    const current = this.byTask.get(operation.taskId);
    if (!current || current.body.idempotency_key !== operation.body.idempotency_key) return;
    this.live.delete(operation.taskId);
    this.set(operation.taskId, operation);
  }

  /** Forgets a resolved operation. Unresolved and unverifiable records are never dismissed here. */
  clearResolved(identity: string, taskId: string): void {
    this.adopt(identity);
    if (isUnresolvedWorkspaceMove(this.byTask.get(taskId) ?? null)) return;
    if (this.byTask.delete(taskId)) this.emit();
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private adopt(identity: string): void {
    if (identity === this.identity) return;
    const hadState = this.byTask.size > 0 || this.unverifiable.size > 0 || this.storageUnavailable;
    this.identity = identity;
    this.byTask.clear();
    this.reconciling.clear();
    this.live.clear();
    this.unverifiable.clear();
    // Another caller's stored records are hidden, never cleared.
    const restored = restoreWorkspaceMoves(identity);
    this.storageUnavailable = restored.unavailable;
    for (const [taskId, body] of restored.bodies) {
      this.byTask.set(taskId, { status: "uncertain", taskId, body });
    }
    for (const taskId of restored.invalidTaskIds) this.unverifiable.add(taskId);
    if (hadState) this.emit();
  }

  private set(taskId: string, operation: WorkspaceMoveOperation): void {
    this.byTask.set(taskId, operation);
    if (this.identity !== null) {
      if (isUnresolvedWorkspaceMove(operation)) {
        persistWorkspaceMove(this.identity, taskId, operation.body);
      } else {
        forgetWorkspaceMove(this.identity, taskId);
      }
    }
    this.emit();
  }

  private emit(): void {
    for (const listener of this.listeners) listener();
  }
}

const registries = new WeakMap<object, WorkspaceMoveOperations>();

export function workspaceMoveOperations(owner: object): WorkspaceMoveOperations {
  let registry = registries.get(owner);
  if (!registry) {
    registry = new WorkspaceMoveOperations();
    registries.set(owner, registry);
  }
  return registry;
}
