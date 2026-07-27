import { t } from "@lingui/core/macro";

const MULTI_REPO_SUPPORTED_EXECUTOR_TYPES = new Set(["worktree", "local_docker", "ssh", "sprites"]);

/**
 * Returns the selector explanation for runtimes that cannot launch a task
 * with sibling repositories. This is deliberately a pure capability check:
 * changing repository rows must never replace a user's supported executor.
 */
export function getMultiRepoExecutorDisabledReason(executorType: string | null | undefined) {
  if (MULTI_REPO_SUPPORTED_EXECUTOR_TYPES.has(executorType ?? "")) return null;
  if (executorType === "local" || executorType === "local_pc") {
    return t`Multi-repo tasks are unavailable on Local until its initial launch path can project sibling repositories.`;
  }
  if (executorType === "remote_docker") {
    return t`Multi-repo tasks are unavailable on Remote Docker until it supports creating task instances.`;
  }
  return t`Multi-repo tasks are not supported by this executor.`;
}
