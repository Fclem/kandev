import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { AgentUpdateJob, AgentUpdatePreview } from "@/lib/api";
import { RuntimeVersionSummary } from "./runtime-version-summary";

afterEach(cleanup);
const preview: AgentUpdatePreview = {
  agent_name: "omp-acp",
  update_mode: "self_update",
  package: "@oh-my-pi/pi-coding-agent",
  current_version: "1.0.0",
  target_version: "",
  stable_latest_version: "1.1.0",
  operation: "update",
  command: ["omp", "update"],
  command_string: "omp update",
};

describe("automatic OMP version summaries", () => {
  it("shows the observed installation after a changed check rather than its previous or stable version", () => {
    const job: AgentUpdateJob = {
      agent_name: "omp-acp",
      job_id: "changed",
      automatic: true,
      update_mode: "self_update",
      status: "succeeded",
      operation: "update",
      previous_version: "1.0.0",
      current_version: "1.0.0",
      effective_version: "1.2.0",
      started_at: "2026-10-10T12:00:00Z",
    };
    render(<RuntimeVersionSummary agentName="omp-acp" preview={preview} job={job} />);
    expect(screen.getByTestId("agent-update-current-omp-acp").textContent).toContain("1.2.0");
    expect(screen.getByTestId("agent-update-stable-reference-omp-acp").textContent).toContain(
      "1.1.0",
    );
    const summary = screen.getByRole("status");
    expect(summary.textContent).toContain("1.0.0");
    expect(summary.textContent).toContain("1.2.0");
    expect(summary.textContent).not.toMatch(/up.?to.?date|updated successfully/i);
  });

  it("reports an adopted no-change check without treating the stable reference as an update target", () => {
    const job: AgentUpdateJob = {
      agent_name: "omp-acp",
      job_id: "same",
      automatic: true,
      update_mode: "self_update",
      status: "succeeded",
      operation: "up_to_date",
      current_version: "1.2.0",
      effective_version: "1.2.0",
      started_at: "2026-10-10T12:00:00Z",
    };
    render(<RuntimeVersionSummary agentName="omp-acp" preview={preview} job={job} />);
    expect(screen.getByRole("status").textContent).toContain("1.2.0");
    expect(screen.getByRole("status").textContent).not.toMatch(
      /up.?to.?date|updated successfully/i,
    );
  });
});
