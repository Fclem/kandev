import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ComposerAgentStartHint } from "./composer-agent-start-hint";

const HINT_TEST_ID = "composer-agent-start-hint";

afterEach(cleanup);

describe("ComposerAgentStartHint", () => {
  it("renders the hint line when shown", () => {
    render(<ComposerAgentStartHint show />);
    expect(screen.getByTestId(HINT_TEST_ID)).toBeTruthy();
  });

  it("renders nothing when hidden", () => {
    render(<ComposerAgentStartHint show={false} />);
    expect(screen.queryByTestId(HINT_TEST_ID)).toBeNull();
  });
});
