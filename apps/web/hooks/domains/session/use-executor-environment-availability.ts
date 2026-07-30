"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import {
  fetchTaskEnvironmentLive,
  type ContainerLiveStatus,
  type TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import {
  resolveExecutorEnvironmentStatus,
  type ExecutorEnvironmentStatus,
} from "@/components/task/executor-environment-status";

const POLL_INTERVAL_MS = 3000;

/** The one status that is "not yet ready" rather than a failure. */
const STARTING_LABEL_KEY = "task:starting2";

export function isExecutorEnvironmentUnavailable(
  status: ExecutorEnvironmentStatus | null,
): boolean {
  if (!status) return false;
  if (status.tone === "running" || status.tone === "neutral") return false;
  // Compare the catalog KEY, not the rendered label: the label is translated, so
  // matching on its text would stop matching in every non-English locale.
  return status.labelKey !== STARTING_LABEL_KEY;
}

export function useExecutorEnvironmentAvailability(taskId: string | null, enabled: boolean) {
  const [env, setEnv] = useState<TaskEnvironment | null>(null);
  const [container, setContainer] = useState<ContainerLiveStatus | null>(null);
  const inFlight = useRef(false);

  useEffect(() => {
    setEnv(null);
    setContainer(null);
  }, [taskId]);

  const load = useCallback(async () => {
    if (!taskId || inFlight.current) return;
    inFlight.current = true;
    try {
      const data = await fetchTaskEnvironmentLive(taskId);
      setEnv(data.environment);
      setContainer(data.container ?? null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setEnv(null);
        setContainer(null);
      }
    } finally {
      inFlight.current = false;
    }
  }, [taskId]);

  useEffect(() => {
    if (!enabled || !taskId) return;
    void load();
    const interval = window.setInterval(() => void load(), POLL_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [enabled, taskId, load]);

  const status = useMemo(
    () => (env ? resolveExecutorEnvironmentStatus(env, container) : null),
    [env, container],
  );
  const unavailable = isExecutorEnvironmentUnavailable(status);

  return { status, unavailable };
}
