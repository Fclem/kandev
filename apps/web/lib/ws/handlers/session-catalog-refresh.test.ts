import { afterEach, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { registerWsHandlers } from "@/lib/ws/router";
import {
  sessionId as makeSessionId,
  taskId as makeTaskId,
  type TaskSession,
} from "@/lib/types/http";

const taskId = "catalog-task";
const sessionId = "catalog-session";
const owner = { execution_id: "execution-1", attempt_id: "attempt-1", source_generation: 1 };
const pending = {
  version: 1,
  revision: 4,
  profile_id: "profile",
  digest: "host-digest",
  owner,
  disposition: "host_candidates",
  reason: "new_candidates",
  source: "task_probe_v1",
  models: [{ id: "new-model", name: "New model" }],
  modes: [{ id: "new-mode", name: "New mode" }],
  probe: {
    outcome: "complete",
    models: { present: true, supported: true, choices: [{ id: "new-model", name: "New model" }] },
    modes: { present: true, supported: true, choices: [{ id: "new-mode", name: "New mode" }] },
  },
};
const disposers: Array<() => void> = [];
afterEach(() => {
  for (const dispose of disposers.splice(0)) dispose();
});

function harness() {
  const metadata = {
    runtime_config: {
      model: "selected",
      mode: "selected-mode",
      config_options: { effort: "high" },
    },
    runtime_config_overrides: { model: "selected" },
    acp_config_baseline: { effort: "medium" },
    composer_draft: "unsent work",
  };
  const session: TaskSession = {
    id: makeSessionId(sessionId),
    task_id: makeTaskId(taskId),
    state: "WAITING_FOR_INPUT",
    agent_execution_id: owner.execution_id,
    started_at: "2026-11-16T00:00:00Z",
    updated_at: "2026-11-16T00:00:00Z",
    metadata,
  };
  const store = createAppStore({ taskSessions: { items: { [sessionId]: session } } });
  const registration = registerWsHandlers(store);
  disposers.push(registration.dispose);
  const dispatch = (payload: unknown, action = "session.catalog_refresh_updated") => {
    // The socket boundary deliberately accepts malformed payloads before schema validation.
    const handlers = registration.handlers as Record<
      string,
      ((message: unknown) => void) | undefined
    >;
    handlers[action]?.({ type: "notification", action, payload });
  };
  const catalog = () => store.getState().sessionCatalogRefresh.bySessionId[sessionId];
  return { store, dispatch, catalog, metadata };
}

it("projects only the scoped observation without changing selected values, baselines, or draft", () => {
  const { store, dispatch, catalog, metadata } = harness();
  const original = store.getState().taskSessions.items[sessionId].metadata;
  dispatch({ task_id: taskId, session_id: sessionId, record: pending });
  expect(catalog()?.record).toEqual(pending);
  expect(catalog()?.taskId).toBe(taskId);
  expect(store.getState().taskSessions.items[sessionId].metadata).toEqual(original);
  expect(original).toEqual(metadata);
  expect(store.getState().sessionModels.bySessionId[sessionId]).toBeUndefined();
});

it("keeps a committed clearing tombstone over delayed pending notifications", () => {
  const { dispatch, catalog } = harness();
  dispatch({ task_id: taskId, session_id: sessionId, record: pending });
  const terminal = {
    ...pending,
    revision: 5,
    models: undefined,
    modes: undefined,
    disposition: "cleared",
    reason: "discovery_complete",
    result: {
      outcome: "complete",
      models: { present: true, supported: true, choices: [] },
      modes: { present: false, supported: false, choices: null },
    },
  };
  dispatch({ task_id: taskId, session_id: sessionId, record: terminal });
  dispatch({ task_id: taskId, session_id: sessionId, record: pending });
  expect(catalog()?.record).toEqual(terminal);
});

it("rejects a task/session mismatch and malformed revision instead of retiring current evidence", () => {
  const { dispatch, catalog } = harness();
  dispatch({ task_id: taskId, session_id: sessionId, record: pending });
  dispatch({ task_id: "another-task", session_id: sessionId, record: { ...pending, revision: 6 } });
  dispatch({
    task_id: taskId,
    session_id: sessionId,
    record: { ...pending, revision: Number.NaN },
  });
  expect(catalog()?.record).toEqual(pending);
});

it("retains independent provider availability without fabricating host notice settlement", () => {
  const { dispatch, catalog, store, metadata } = harness();
  dispatch({ task_id: taskId, session_id: sessionId, record: pending });
  const provider = {
    revision: 1,
    owner,
    profile_id: "profile",
    result: {
      outcome: "unsupported",
      generation: 7,
      models: { present: false, supported: false, choices: null },
      modes: { present: false, supported: false, choices: null },
    },
  };
  dispatch(
    { task_id: taskId, session_id: sessionId, provider },
    "session.catalog_discovery_updated",
  );
  expect(catalog()?.provider).toEqual(provider);
  expect(catalog()?.record).toEqual(pending);
  expect(store.getState().taskSessions.items[sessionId].metadata).toEqual(metadata);
});

// @covers AC-AGENTS-SESSION-CATALOG-001.7
it("accepts candidates only from the target's own task probe and keeps per-surface support", () => {
  const { dispatch, catalog } = harness();
  const legacyGeneric = { ...pending, revision: 7, source: undefined, probe: undefined };
  dispatch({ task_id: taskId, session_id: sessionId, record: legacyGeneric });
  expect(catalog()?.record).toMatchObject({
    revision: 7,
    disposition: "rediscovery_required",
    reason: "context_unverified",
  });
  expect(catalog()?.record?.models).toBeUndefined();
  expect(catalog()?.record?.modes).toBeUndefined();

  const modelOnly = {
    ...pending,
    revision: 8,
    probe: {
      outcome: "complete",
      models: { present: true, supported: true, choices: [{ id: "new-model", name: "New model" }] },
      modes: { present: false, supported: false, choices: null },
    },
  };
  dispatch({ task_id: taskId, session_id: sessionId, record: modelOnly });
  expect(catalog()?.record?.disposition).toBe("host_candidates");
  expect(catalog()?.record?.models).toEqual([{ id: "new-model", name: "New model" }]);
  expect(catalog()?.record?.modes).toBeUndefined();
});
