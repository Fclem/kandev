import { useCallback } from "react";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";

type GitOperationResult = { success: boolean; output: string; error?: string };

/**
 * Wraps a git operation with toast feedback (loading → success/error).
 */
export function useGitWithFeedback() {
  const { t } = useTranslation();
  const { toast, updateToast } = useToast();

  const run = useCallback(
    async (operation: () => Promise<GitOperationResult>, operationName: string) => {
      const toastId = toast({
        title: `${operationName}...`,
        variant: "loading",
      });
      try {
        const result = await operation();
        if (result.success) {
          updateToast(toastId, {
            title: t("common:successful2", { operationName }),
            description: result.output.slice(0, 200) || t("common:completed", { operationName }),
            variant: "success",
          });
        } else {
          updateToast(toastId, {
            title: t("common:failed3", { operationName }),
            description: result.error || t("common:anErrorOccurred"),
            variant: "error",
          });
        }
      } catch (e) {
        updateToast(toastId, {
          title: t("common:failed3", { operationName }),
          description: e instanceof Error ? e.message : t("common:anUnexpectedErrorOccurred"),
          variant: "error",
        });
      }
    },
    [toast, updateToast],
  );

  return run;
}
