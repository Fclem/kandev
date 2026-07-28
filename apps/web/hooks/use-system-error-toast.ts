"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

/**
 * Watches for backend `system.error` frames and shows an error toast. Every
 * frame is surfaced — unlike task/session notifications there is no natural
 * entity id to deduplicate on, and a repeated backend error is still news.
 * Mount once inside ToastProvider.
 */
export function useSystemErrorToast() {
  const notification = useAppStore((s) => s.systemErrorNotification);
  const clearNotification = useAppStore((s) => s.setSystemErrorNotification);
  const { toast } = useToast();

  useEffect(() => {
    if (!notification) return;
    toast({
      title: notification.code ? `System error (${notification.code})` : "System error",
      description: notification.message,
      variant: "error",
    });
    clearNotification(null);
  }, [notification, toast, clearNotification]);
}
