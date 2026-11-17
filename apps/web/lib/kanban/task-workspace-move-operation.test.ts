import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { TaskTransferRequest } from "@/lib/types/http";
import { settleWorkspaceMove } from "./task-workspace-move-operation";

const body = {} as TaskTransferRequest;

function checking() {
  return { status: "submitting" as const, taskId: "task-1", body, attempt: "check" as const };
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
