"use client";
import { useEffect } from "react";
import { useToast } from "@/components/toast-provider";
import { MERMAID_ERROR_EVENT } from "./mermaid-utils";
import { t } from "@/lib/i18n";

type ToastFn = ReturnType<typeof useToast>["toast"];
type MermaidErrorDetail = { message: string; taskId?: string | null };

const notifiedTaskIds = new Set<string>();

export function showMermaidErrorToast(
  toast: ToastFn,
  taskId: string | null | undefined,
  message: string,
): void {
  if (taskId) {
    if (notifiedTaskIds.has(taskId)) return;
    notifiedTaskIds.add(taskId);
  }

  toast({ title: t("common:failedToRenderDiagram"), description: message, variant: "error" });
}

export function resetMermaidErrorToastHistoryForTest(): void {
  notifiedTaskIds.clear();
}

export function useMermaidErrorToast(): void {
  const { toast } = useToast();

  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<MermaidErrorDetail>).detail;
      showMermaidErrorToast(toast, detail?.taskId, detail?.message);
    };
    document.addEventListener(MERMAID_ERROR_EVENT, handler);
    return () => document.removeEventListener(MERMAID_ERROR_EVENT, handler);
  }, [toast]);
}
