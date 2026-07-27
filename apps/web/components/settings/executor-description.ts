import { t } from "@lingui/core/macro";
import type { ExecutorType } from "@/lib/types/http";

export function getExecutorDescription(type: ExecutorType): string {
  if (type === "local_pc") return t`Runs agents directly in the repository folder.`;
  if (type === "worktree") return t`Creates git worktrees for isolated agent sessions.`;
  if (type === "local_docker") return t`Runs Docker containers on this machine.`;
  if (type === "remote_docker") return t`Connects to a remote Docker host.`;
  if (type === "sprites") return t`Runs agents in Sprites.dev cloud sandboxes.`;
  if (type === "ssh") return t`Runs agents on a trusted Linux amd64 or macOS host over SSH.`;
  return t`Custom executor.`;
}
