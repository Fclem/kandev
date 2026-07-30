"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";

/** Translated toast copy for one merge-request action. */
export type MRActionMessages = {
  success: string;
  /** Title of the error toast. The failure detail comes from the API. */
  failure: string;
};

export function useMRActions(onRefresh: () => void) {
  const { t } = useTranslation();
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const { toast } = useToast();

  const run = useCallback(
    /**
     * `key` only marks which action is in flight (callers just null-check
     * `pendingAction`), so it stays an untranslated identifier. The user-visible
     * strings are passed in already translated — the previous
     * `` `${label} failed` `` composed a sentence from an English label, which no
     * other language can reproduce.
     */
    async (key: string, action: () => Promise<unknown>, messages: MRActionMessages) => {
      setPendingAction(key);
      try {
        await action();
        toast({ description: messages.success, variant: "success" });
        onRefresh();
        return true;
      } catch (error) {
        toast({
          title: messages.failure,
          description: error instanceof Error ? error.message : t("gitlab:gitlabRejectedTheAction"),
          variant: "error",
        });
        return false;
      } finally {
        setPendingAction(null);
      }
    },
    [onRefresh, t, toast],
  );

  return { pendingAction, run };
}
