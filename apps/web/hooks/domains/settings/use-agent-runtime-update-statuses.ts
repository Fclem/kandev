"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { listAgentUpdateStatuses, type AgentUpdateJob, type AgentUpdateStatus } from "@/lib/api";

type StatusByAgent = Record<string, AgentUpdateStatus>;
type RefreshOutcome = "applied" | "superseded" | "failed";

function indexStatuses(statuses: AgentUpdateStatus[]): StatusByAgent {
  return Object.fromEntries(statuses.map((status) => [status.agent_name, status]));
}

export function useAgentRuntimeUpdateStatuses(updateJobs: Record<string, AgentUpdateJob>) {
  const [statusByAgent, setStatusByAgent] = useState<StatusByAgent>({});
  const observedSuccessfulJobs = useRef(new Set<string>());
  const pendingSuccessfulJobs = useRef(new Set<string>());
  const latestRefresh = useRef(0);
  const activeRefreshes = useRef(0);
  const observerRefreshing = useRef(false);
  const successorNeeded = useRef(false);
  const scheduleSuccessor = useRef<() => void>(() => {});

  const refresh = useCallback(async (): Promise<RefreshOutcome> => {
    const generation = ++latestRefresh.current;
    const successfulJobsAtStart = new Set(pendingSuccessfulJobs.current);
    activeRefreshes.current++;
    try {
      const response = await listAgentUpdateStatuses({ cache: "no-store" });
      if (generation !== latestRefresh.current) {
        return "superseded";
      }
      setStatusByAgent(indexStatuses(response.statuses));
      for (const id of successfulJobsAtStart) {
        observedSuccessfulJobs.current.add(id);
        pendingSuccessfulJobs.current.delete(id);
      }
      return "applied";
    } catch {
      // A failed hint read must not replace a last-good map or observe jobs.
      return generation === latestRefresh.current ? "failed" : "superseded";
    } finally {
      activeRefreshes.current--;
      queueMicrotask(() => scheduleSuccessor.current());
    }
  }, []);

  const startObserverRefresh = useCallback(() => {
    if (
      observerRefreshing.current ||
      activeRefreshes.current ||
      !pendingSuccessfulJobs.current.size
    ) {
      return;
    }
    observerRefreshing.current = true;
    successorNeeded.current = false;
    void refresh().then((outcome) => {
      observerRefreshing.current = false;
      if (outcome === "superseded" && pendingSuccessfulJobs.current.size) {
        successorNeeded.current = true;
      }
      scheduleSuccessor.current();
    });
  }, [refresh]);

  const schedule = useCallback(() => {
    if (successorNeeded.current && !observerRefreshing.current && !activeRefreshes.current) {
      startObserverRefresh();
    }
  }, [startObserverRefresh]);

  useEffect(() => {
    scheduleSuccessor.current = schedule;
  }, [schedule]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    let added = false;
    for (const job of Object.values(updateJobs)) {
      if (
        !job.job_id ||
        job.status !== "succeeded" ||
        observedSuccessfulJobs.current.has(job.job_id) ||
        pendingSuccessfulJobs.current.has(job.job_id)
      ) {
        continue;
      }
      pendingSuccessfulJobs.current.add(job.job_id);
      added = true;
    }
    if (added) {
      successorNeeded.current = true;
      schedule();
    }
  }, [schedule, updateJobs]);

  return { refresh, statusByAgent };
}
