// Registry of executor types presented in the "new executor profile" flow.
// Keep entries here (not inline in the page) so the page file stays under
// the 600-line lint cap and new types can be added without touching layout.

export type ExecutorTypeInfo = {
  executorId: string;
  label: string;
  description: string;
};

export const EXECUTOR_TYPE_MAP: Record<string, ExecutorTypeInfo> = {
  local: {
    executorId: "exec-local",
    label: "common:local",
    description: "settings:runsAgentsDirectlyInTheRepository",
  },
  worktree: {
    executorId: "exec-worktree",
    label: "settings:worktree",
    description: "settings:createsGitWorktreesForIsolatedAgent",
  },
  local_docker: {
    executorId: "exec-local-docker",
    label: "common:docker",
    description: "settings:runsDockerContainersOnThisMachine",
  },
  remote_docker: {
    executorId: "exec-remote-docker",
    label: "settings:remoteDocker",
    description: "settings:connectsToARemoteDockerHost",
  },
  sprites: {
    executorId: "exec-sprites",
    label: "settings:spritesDev",
    description: "settings:runsAgentsInSpritesDevCloud",
  },
  ssh: {
    executorId: "exec-ssh",
    label: "settings:ssh",
    description: "settings:connectsToARemoteHostOver",
  },
};
