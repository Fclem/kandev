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
    // A busy relation lock is transient and establishes nothing about the original attempt.
    if (code === "relations_busy") return { status: "uncertain", taskId, body };
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

  /** Pure read for render; returns null until `adopt` has switched to this caller. */
  get(identity: string, taskId: string): WorkspaceMoveOperation | null {
    if (identity !== this.identity) return null;
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
    return identity === this.identity && this.unverifiable.has(taskId);
  }

  /**
   * Tab storage cannot be read, or refused a submission's write, for this caller,
   * so no new move can be made durable here.
   */
  isStorageUnavailable(identity: string): boolean {
    return identity === this.identity && this.storageUnavailable;
  }

  /**
   * Admits one request synchronously; a second click while busy is refused, and
   * so is a request from a caller other than the adopted one.
   */
  begin(
    identity: string,
    taskId: string,
    body: TaskTransferRequest,
    attempt: "submit" | "check",
  ): Extract<WorkspaceMoveOperation, { status: "submitting" }> | null {
    if (identity !== this.identity || !this.admits(taskId, attempt)) return null;
    const next = { status: "submitting" as const, taskId, body, attempt };
    // A new request must be durable before it leaves this tab. A check re-sends a
    // record that was already stored, so only a submission writes here.
    if (attempt === "submit" && !persistWorkspaceMove(identity, taskId, body)) {
      this.markStorageUnavailable(taskId);
      return null;
    }
    this.live.set(taskId, next);
    this.set(taskId, next);
    return next;
  }

  /**
   * A refused write disables new moves for this caller. The submitting task's
   * outcome and every resolved rejection are dropped, so each task's form shows
   * why moves are off instead of a stale rejection that still looks retryable.
   */
  private markStorageUnavailable(taskId: string): void {
    this.storageUnavailable = true;
    this.byTask.delete(taskId);
    for (const [id, operation] of this.byTask) {
      if (operation.status === "rejected" || operation.status === "operation-rejected") {
        this.byTask.delete(id);
      }
    }
    this.emit();
  }

  /** Whether the adopted caller may start this attempt for the task now. */
  private admits(taskId: string, attempt: "submit" | "check"): boolean {
    const current = this.byTask.get(taskId) ?? null;
    if (this.reconciling.has(taskId) || current?.status === "submitting") return false;
    if (attempt === "check") return isUnresolvedWorkspaceMove(current);
    return (
      !isUnresolvedWorkspaceMove(current) &&
      !this.storageUnavailable &&
      !this.unverifiable.has(taskId)
    );
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
    if (identity !== this.identity) return;
    if (isUnresolvedWorkspaceMove(this.byTask.get(taskId) ?? null)) return;
    if (this.byTask.delete(taskId)) this.emit();
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  /**
   * Switches the visible caller and restores that caller's stored records. It
   * reads storage and notifies listeners, so call it from an effect or an event,
   * never during render.
   */
  adopt(identity: string): void {
    if (identity === this.identity) return;
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
    this.emit();
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
