"use client";

import { useSystemErrorToast } from "@/hooks/use-system-error-toast";

/** Mounts the system-error toast hook inside the ToastProvider tree. */
export function SystemErrorToastBridge() {
  useSystemErrorToast();
  return null;
}
