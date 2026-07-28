import { t } from "@/lib/i18n";
import type { ExecutorType } from "@/lib/types/http";

export function getExecutorDescription(type: ExecutorType): string {
  if (type === "local_pc") return t("settings:runsAgentsDirectlyInTheRepository");
  if (type === "worktree") return t("settings:createsGitWorktreesForIsolatedAgent");
  if (type === "local_docker") return t("settings:runsDockerContainersOnThisMachine");
  if (type === "remote_docker") return t("settings:connectsToARemoteDockerHost");
  if (type === "sprites") return t("settings:runsAgentsInSpritesDevCloud");
  if (type === "ssh") return t("settings:runsAgentsOnATrustedLinux");
  return t("settings:customExecutor");
}
