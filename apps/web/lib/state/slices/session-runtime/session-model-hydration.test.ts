import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import { sessionModelsEntryFromTaskSession } from "./session-model-hydration";
import { createAppStore } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/http";

function session(metadata: Record<string, unknown>): TaskSession {
  return {
    id: sessionId("session-1"),
    task_id: taskId("task-1"),
    state: "WAITING_FOR_INPUT",
    started_at: "2026-11-16T00:00:00Z",
    updated_at: "2026-11-16T00:00:00Z",
    metadata,
  };
}

describe("sessionModelsEntryFromTaskSession", () => {
  it("hydrates persisted model and config state for a resumed session", () => {
    expect(
      sessionModelsEntryFromTaskSession(
        session({
          runtime_config: { model: "mock-fast", config_options: { effort: "medium" } },
          runtime_config_overrides: { config_options: { effort: "high" } },
          acp_config_baseline: { model: "mock-fast", effort: "medium" },
          acp_model_state: {
            current_model_id: "mock-fast",
            config_options_settled: true,
            models: [{ model_id: "mock-fast", name: "Mock Fast" }],
            config_options: [
              {
                type: "select",
                id: "effort",
                name: "Effort",
                current_value: "medium",
              },
            ],
          },
        }),
      ),
    ).toEqual({
      currentModelId: "mock-fast",
      models: [
        {
          modelId: "mock-fast",
          name: "Mock Fast",
          description: undefined,
          usageMultiplier: undefined,
        },
      ],
      configOptions: [
        {
          type: "select",
          id: "effort",
          name: "Effort",
          description: undefined,
          currentValue: "high",
          category: undefined,
          options: undefined,
        },
      ],
      configOptionsSettled: true,
      configBaseline: { model: "mock-fast", effort: "medium" },
    });
  });

  it("returns no entry when a session has no usable model snapshot", () => {
    expect(sessionModelsEntryFromTaskSession(session({}))).toBeUndefined();
    expect(sessionModelsEntryFromTaskSession(null)).toBeUndefined();
  });
});

it("keeps a live tombstone and newer provider outcome across stale full-session HTTP hydration", () => {
  const owner = { execution_id: "replacement", attempt_id: "resume", source_generation: 1 };
  const absent = { present: false, supported: false, choices: null };
  const record = {
    version: 1,
    revision: 5,
    profile_id: "profile",
    owner,
    disposition: "cleared",
    reason: "complete",
  };
  const provider = {
    revision: 3,
    profile_id: "profile",
    owner,
    result: { outcome: "unsupported", models: absent, modes: absent },
  };
  const intent = {
    runtime_config: { model: "kept", mode: "plan", config_options: { effort: "high" } },
    acp_config_baseline: { effort: "medium" },
    composer_draft: "Unsent work",
  };
  const store = createAppStore({
    taskSessions: {
      items: {
        "session-1": session({
          ...intent,
          profile_catalog_refresh: record,
          acp_model_state: { catalog_discovery: provider },
        }),
      },
    },
  });
  store.getState().hydrateSessionCatalogRefresh(
    session({
      ...intent,
      profile_catalog_refresh: {
        ...record,
        revision: 4,
        disposition: "host_candidates",
        models: [{ id: "obsolete" }],
      },
      acp_model_state: {
        catalog_discovery: {
          ...provider,
          revision: 2,
          owner: { execution_id: "previous", attempt_id: "previous", source_generation: 9 },
          result: {
            outcome: "complete",
            models: { present: true, supported: true, choices: [{ id: "obsolete" }] },
            modes: absent,
          },
        },
      },
    }),
  );
  const state = store.getState();
  expect(state.sessionCatalogRefresh.bySessionId["session-1"].record?.disposition).toBe("cleared");
  expect(state.sessionCatalogRefresh.bySessionId["session-1"].provider?.result.outcome).toBe(
    "unsupported",
  );
  expect(state.taskSessions.items["session-1"].metadata?.runtime_config).toEqual(
    intent.runtime_config,
  );
  expect(state.taskSessions.items["session-1"].metadata?.acp_config_baseline).toEqual(
    intent.acp_config_baseline,
  );
  expect(state.taskSessions.items["session-1"].metadata?.composer_draft).toBe("Unsent work");
});

it("does not discard valid terminal provider evidence because the separate host notice is malformed", () => {
  const absent = { present: false, supported: false, choices: null };
  const store = createAppStore({
    taskSessions: {
      items: {
        "session-1": session({
          profile_catalog_refresh: { version: 1, revision: -1 },
          acp_model_state: {
            catalog_discovery: {
              revision: 1,
              profile_id: "profile",
              owner: { execution_id: "execution", attempt_id: "attempt", source_generation: 1 },
              result: { outcome: "unsupported", models: absent, modes: absent },
            },
          },
        }),
      },
    },
  });
  const entry = store.getState().sessionCatalogRefresh.bySessionId["session-1"];
  expect(entry.record).toBeUndefined();
  expect(entry.provider?.result.outcome).toBe("unsupported");
});
