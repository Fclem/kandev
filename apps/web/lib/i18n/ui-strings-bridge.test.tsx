import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { Spinner } from "@kandev/ui/spinner";
import { UIStringsProvider, DEFAULT_UI_STRINGS } from "@kandev/ui/lib/ui-strings";

describe("@kandev/ui built-in strings", () => {
  afterEach(cleanup);

  it("renders the English default with no provider", () => {
    render(<Spinner />);
    expect(screen.getByRole("status").getAttribute("aria-label")).toBe("Loading");
  });

  it("renders an overridden label when a provider supplies one", () => {
    render(
      <UIStringsProvider value={{ ...DEFAULT_UI_STRINGS, loading: "Cargando" }}>
        <Spinner />
      </UIStringsProvider>,
    );
    expect(screen.getByRole("status").getAttribute("aria-label")).toBe("Cargando");
  });
});
