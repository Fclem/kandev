import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { TaskTransferReceipt, TaskTransferRequest } from "@/lib/types/http";
import { settleWorkspaceMove, WorkspaceMoveOperations } from "./task-workspace-move-operation";

const body = {} as TaskTransferRequest;

function checking() {
  return { status: "submitting" as const, taskId: "task-1", body, attempt: "check" as const };
}

function submitting() {
  return { status: "submitting" as const, taskId: "task-1", body, attempt: "submit" as const };
}

function conflict(code: string) {
  return new ApiError("conflict", 409, { code });
}

describe("settleWorkspaceMove check outcomes", () => {
  it("keeps a refused check unresolved and names the refusal code", () => {
    const settled = settleWorkspaceMove(checking(), { error: conflict("source_changed") });
    expect(settled).toEqual({
      status: "check-unavailable",
      taskId: "task-1",
      body,
      code: "source_changed",
    });
  });

  it("does not attach a code when the conflict is not a known refusal", () => {
    const settled = settleWorkspaceMove(checking(), { error: conflict("something_new") });
    expect(settled).toEqual({ status: "check-unavailable", taskId: "task-1", body });
  });

  it("keeps a busy relation lock on check uncertain, not a permissions failure", () => {
    const settled = settleWorkspaceMove(checking(), { error: conflict("relations_busy") });
    expect(settled).toEqual({ status: "uncertain", taskId: "task-1", body });
  });

  it("does not attach a code to a non-conflict check failure", () => {
    const settled = settleWorkspaceMove(checking(), {
      error: new ApiError("forbidden", 403, { code: "forbidden" }),
    });
    expect(settled).toEqual({ status: "check-unavailable", taskId: "task-1", body });
  });

  it("resolves only on an authoritative operation_rejected", () => {
    const settled = settleWorkspaceMove(checking(), { error: conflict("operation_rejected") });
    expect(settled).toEqual({ status: "operation-rejected", taskId: "task-1", body });
  });
});

describe("settleWorkspaceMove submit outcomes", () => {
  it("settles a submit refused by a busy relation lock as rejected with its code", () => {
    const settled = settleWorkspaceMove(submitting(), { error: conflict("relations_busy") });
    expect(settled).toEqual({
      status: "rejected",
      taskId: "task-1",
      body,
      code: "relations_busy",
    });
  });
});

describe("WorkspaceMoveOperations reconcile overlay", () => {
  const overlayBody = { idempotency_key: "key-overlay" } as TaskTransferRequest;
  const receipt = {} as TaskTransferReceipt;

  function commitTask(registry: WorkspaceMoveOperations, identity: string, taskId: string) {
    registry.adopt(identity);
    const pending = registry.begin(identity, taskId, overlayBody, "submit");
    if (!pending) throw new Error("admission refused");
    registry.settle(identity, pending, {
      status: "committed",
      taskId,
      body: overlayBody,
      receipt,
    });
  }

  it("refuses a new submit while the committed receipt is reconciling", () => {
    const registry = new WorkspaceMoveOperations();
    commitTask(registry, "user-a", "task-refuse");
    const overlay = registry.beginReconcile("user-a", "task-refuse");
    expect(overlay).not.toBeNull();
    expect(registry.begin("user-a", "task-refuse", overlayBody, "submit")).toBeNull();
  });

  it("ignores a stale overlay token after an identity switch and return", () => {
    const registry = new WorkspaceMoveOperations();
    const taskId = "task-stale";
    commitTask(registry, "user-a", taskId);
    const firstOverlay = registry.beginReconcile("user-a", taskId);
    if (!firstOverlay) throw new Error("expected overlay");
    registry.adopt("user-b");
    registry.adopt("user-a");
    commitTask(registry, "user-a", taskId);
    const currentOverlay = registry.beginReconcile("user-a", taskId);
    if (!currentOverlay) throw new Error("expected overlay");

    registry.endReconcile("user-a", taskId, firstOverlay);
    expect(registry.get("user-a", taskId)?.status).toBe("submitting");

    registry.endReconcile("user-a", taskId, currentOverlay);
    expect(registry.get("user-a", taskId)?.status).toBe("committed");
  });
});
