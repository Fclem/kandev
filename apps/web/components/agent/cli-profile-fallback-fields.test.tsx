import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ModelFallbackFields } from "./cli-profile-fallback-fields";

afterEach(cleanup);

const ARIA_CHECKED = "aria-checked";

function RecoveryHarness({
  fallback = "fallback",
  automatic = false,
}: {
  fallback?: string;
  automatic?: boolean;
}) {
  const [model, setModel] = useState(fallback);
  const [auto, setAuto] = useState(automatic);
  const [exact, setExact] = useState(false);
  const [limit, setLimit] = useState(true);
  const [resume, setResume] = useState(true);
  return (
    <ModelFallbackFields
      availableModels={[{ id: "fallback", name: "Fallback model" }]}
      fallbackModel={model}
      fallbackModelGone={false}
      autoFallback={auto}
      requireExactModel={exact}
      limitFallback={limit}
      resumeAfterReset={resume}
      currentModelId="primary"
      onFallbackModelChange={setModel}
      onAutoFallbackChange={setAuto}
      onRequireExactModelChange={setExact}
      onLimitFallbackChange={setLimit}
      onResumeAfterResetChange={setResume}
    />
  );
}

function recoverySwitch() {
  return screen.getByRole("switch", {
    name: "Use fallback model when limited",
  }) as HTMLButtonElement;
}

describe("limit recovery settings", () => {
  it("preserves dormant fallback while exact mode still permits reset recovery", () => {
    render(<RecoveryHarness />);
    fireEvent.click(screen.getByTestId("profile-fallback-settings-trigger"));
    expect(recoverySwitch().disabled).toBe(false);
    fireEvent.click(screen.getByRole("switch", { name: "Require exact model" }));
    expect(recoverySwitch().disabled).toBe(true);
    expect(recoverySwitch().getAttribute(ARIA_CHECKED)).toBe("true");
    const resume = screen.getByRole("switch", { name: "Resume after reset" }) as HTMLButtonElement;
    expect(resume.disabled).toBe(false);
    fireEvent.click(resume);
    expect(resume.getAttribute(ARIA_CHECKED)).toBe("false");
    fireEvent.click(screen.getByRole("switch", { name: "Require exact model" }));
    expect(recoverySwitch().disabled).toBe(false);
    expect(recoverySwitch().getAttribute(ARIA_CHECKED)).toBe("true");
  });

  it.each([{ fallback: "" }, { automatic: true }])(
    "retains the saved choice when ineligible: %j",
    (props) => {
      render(<RecoveryHarness {...props} />);
      fireEvent.click(screen.getByTestId("profile-fallback-settings-trigger"));
      expect(recoverySwitch().disabled).toBe(true);
      expect(recoverySwitch().getAttribute(ARIA_CHECKED)).toBe("true");
      expect(
        (screen.getByRole("switch", { name: "Resume after reset" }) as HTMLButtonElement).disabled,
      ).toBe(false);
    },
  );
});
