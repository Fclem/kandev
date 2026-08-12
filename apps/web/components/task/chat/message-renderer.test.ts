import { describe, expect, it } from "vitest";
import { shouldShowDescriptionStartButton } from "./message-renderer";

const TASK_ID = "t1";
const SESSION_ID = "s1";
const CREATED_STATE = "CREATED";

describe("shouldShowDescriptionStartButton", () => {
  it("shows for a never-started (CREATED) session", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: CREATED_STATE,
        resumeSkipped: false,
        taskState: CREATED_STATE,
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      }),
    ).toBe(true);
  });

  it("shows for a resume-skipped stopped session", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "WAITING_FOR_INPUT",
        resumeSkipped: true,
        taskState: CREATED_STATE,
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      }),
    ).toBe(true);
  });

  it("NEVER shows for a FAILED session even when resume-skipped (recovery owns the affordance)", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "FAILED",
        resumeSkipped: true,
        taskState: CREATED_STATE,
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      }),
    ).toBe(false);
  });

  it("hides while the task is SCHEDULING", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: CREATED_STATE,
        resumeSkipped: false,
        taskState: "SCHEDULING",
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      }),
    ).toBe(false);
  });

  it("hides for RUNNING/STARTING sessions even when resume-skipped (stale-marker defense)", () => {
    for (const runningState of ["RUNNING", "STARTING"] as const) {
      expect(
        shouldShowDescriptionStartButton({
          sessionState: runningState,
          resumeSkipped: true,
          taskState: CREATED_STATE,
          taskId: "t1",
          sessionId: "s1",
        }),
      ).toBe(false);
    }
  });

  it("hides when no task/session context is bound", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: CREATED_STATE,
        resumeSkipped: false,
        taskState: CREATED_STATE,
        taskId: undefined,
        sessionId: undefined,
      }),
    ).toBe(false);
  });
});
