import { describe, expect, it } from "vitest";
import { shouldShowDescriptionStartButton } from "./message-renderer";

describe("shouldShowDescriptionStartButton", () => {
  it("shows for a never-started (CREATED) session", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "CREATED",
        resumeSkipped: false,
        taskState: "CREATED",
        taskId: "t1",
        sessionId: "s1",
      }),
    ).toBe(true);
  });

  it("shows for a resume-skipped stopped session", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "WAITING_FOR_INPUT",
        resumeSkipped: true,
        taskState: "CREATED",
        taskId: "t1",
        sessionId: "s1",
      }),
    ).toBe(true);
  });

  it("NEVER shows for a FAILED session even when resume-skipped (recovery owns the affordance)", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "FAILED",
        resumeSkipped: true,
        taskState: "CREATED",
        taskId: "t1",
        sessionId: "s1",
      }),
    ).toBe(false);
  });

  it("hides while the task is SCHEDULING", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "CREATED",
        resumeSkipped: false,
        taskState: "SCHEDULING",
        taskId: "t1",
        sessionId: "s1",
      }),
    ).toBe(false);
  });

  it("hides when no task/session context is bound", () => {
    expect(
      shouldShowDescriptionStartButton({
        sessionState: "CREATED",
        resumeSkipped: false,
        taskState: "CREATED",
        taskId: undefined,
        sessionId: undefined,
      }),
    ).toBe(false);
  });
});
