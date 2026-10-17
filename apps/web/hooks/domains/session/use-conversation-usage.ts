"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  getSessionUsageTotals,
  getSessionUsageTurn,
  listSessionUsageTurns,
} from "@/lib/api/domains/conversation-usage-api";
import type { SessionUsageTotals, UsageTurn } from "@/lib/types/conversation-usage";
import { usageForCurrentTurn } from "@/lib/utils/conversation-usage";
import { useSessionTurnsState } from "./use-session-turns";

type UsageSummaryState = {
  key: string;
  invalidation: number;
  totals: SessionUsageTotals | null;
  turns: UsageTurn[];
  loading: boolean;
  error: boolean;
};

type UsageDetailState = {
  key: string;
  turnId: string;
  invalidation: number;
  turn: UsageTurn | null;
  loading: boolean;
  error: boolean;
};

const emptySummary = (key: string): UsageSummaryState => ({
  key,
  invalidation: -1,
  totals: null,
  turns: [],
  loading: false,
  error: false,
});

function projectUsageSummary(summary: UsageSummaryState, key: string, invalidation: number) {
  const current = summary.key === key ? summary : emptySummary(key);
  return current.invalidation === invalidation ? current : { ...current, totals: null, turns: [] };
}

export function useConversationUsage(
  taskId: string | null,
  sessionId: string | null,
  detailActive = true,
) {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const invalidation = useAppStore((state) =>
    sessionId ? (state.usageInvalidation.bySessionId[sessionId] ?? 0) : 0,
  );
  const { turns } = useSessionTurnsState(sessionId);
  const [summary, setSummary] = useState<UsageSummaryState>(() => emptySummary(""));
  const [detail, setDetail] = useState<UsageDetailState | null>(null);
  const summaryRequest = useRef(0);
  const detailRequest = useRef(0);
  const detailDemand = useRef<{ key: string; turnId: string } | null>(null);
  const key = taskId && sessionId ? `${taskId}\u0000${sessionId}` : "";

  const refresh = useCallback(async () => {
    if (!detailActive || !taskId || !sessionId) return;
    const request = ++summaryRequest.current;
    const requestKey = `${taskId}\u0000${sessionId}`;
    setSummary((current) =>
      current.key === requestKey
        ? { ...current, loading: true, error: false }
        : { ...emptySummary(requestKey), loading: true },
    );
    try {
      const [totals, page] = await Promise.all([
        getSessionUsageTotals(taskId, sessionId),
        listSessionUsageTurns(taskId, sessionId),
      ]);
      if (request !== summaryRequest.current) return;
      setSummary({
        key: requestKey,
        invalidation,
        totals,
        turns: page.turns,
        loading: false,
        error: false,
      });
    } catch {
      if (request !== summaryRequest.current) return;
      setSummary((current) =>
        current.key === requestKey
          ? { ...current, loading: false, error: true }
          : { ...emptySummary(requestKey), error: true },
      );
    }
  }, [detailActive, taskId, sessionId, invalidation]);

  useEffect(() => {
    if (!detailActive || !taskId || !sessionId || connectionStatus !== "connected") return;
    void refresh();
    return () => {
      summaryRequest.current += 1;
    };
  }, [connectionStatus, detailActive, invalidation, refresh, sessionId, taskId]);

  const loadTurnDetail = useCallback(
    async (turnId: string) => {
      if (!detailActive || !taskId || !sessionId) return;
      const request = ++detailRequest.current;
      const requestKey = `${taskId}\u0000${sessionId}`;
      detailDemand.current = { key: requestKey, turnId };
      setDetail({ key: requestKey, turnId, invalidation, turn: null, loading: true, error: false });
      try {
        const turn = await getSessionUsageTurn(taskId, sessionId, turnId);
        if (request !== detailRequest.current) return;
        setDetail({ key: requestKey, turnId, invalidation, turn, loading: false, error: false });
      } catch {
        if (request !== detailRequest.current) return;
        setDetail({
          key: requestKey,
          turnId,
          invalidation,
          turn: null,
          loading: false,
          error: true,
        });
      }
    },
    [detailActive, taskId, sessionId, invalidation],
  );

  useEffect(() => {
    const demand = detailDemand.current;
    if (connectionStatus === "connected" && demand?.key === key) {
      void loadTurnDetail(demand.turnId);
    }
    return () => {
      detailRequest.current += 1;
    };
  }, [connectionStatus, invalidation, key, loadTurnDetail]);

  const { turns: usageTurns, ...visibleState } = projectUsageSummary(summary, key, invalidation);
  const latestTurn = useMemo(() => usageForCurrentTurn(usageTurns, turns), [usageTurns, turns]);
  const currentDetail = detail?.key === key && detail.invalidation === invalidation ? detail : null;
  return {
    ...visibleState,
    latestTurn,
    detail: currentDetail,
    refresh,
    loadTurnDetail,
  };
}
