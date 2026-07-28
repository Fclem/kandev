import { t } from "@/lib/i18n";

const MULTI_REPO_SUPPORTED_EXECUTOR_TYPES = new Set(["worktree", "local_docker", "ssh", "sprites"]);

/**
 * Returns the selector explanation for runtimes that cannot launch a task
 * with sibling repositories. This is deliberately a pure capability check:
 * changing repository rows must never replace a user's supported executor.
 */
export function getMultiRepoExecutorDisabledReason(executorType: string | null | undefined) {
  if (MULTI_REPO_SUPPORTED_EXECUTOR_TYPES.has(executorType ?? "")) return null;
  if (executorType === "local" || executorType === "local_pc") {
    return t("task:multiRepoTasksAreUnavailableOn");
  }
  if (executorType === "remote_docker") {
    return t("task:multiRepoTasksAreUnavailableOn2");
  }
  return t("task:multiRepoTasksAreNotSupported");
}
