// Registry of executor types presented in the "new executor profile" flow.
// Keep entries here (not inline in the page) so the page file stays under
// the 600-line lint cap and new types can be added without touching layout.

import { msg } from "@lingui/core/macro";
import type { MessageDescriptor } from "@lingui/core";

export type ExecutorTypeInfo = {
  executorId: string;
  label: MessageDescriptor;
  description: MessageDescriptor;
};

export const EXECUTOR_TYPE_MAP: Record<string, ExecutorTypeInfo> = {
  local: {
    executorId: "exec-local",
    label: msg`Local`,
    description: msg`Runs agents directly in the repository folder.`,
  },
  worktree: {
    executorId: "exec-worktree",
    label: msg`Worktree`,
    description: msg`Creates git worktrees for isolated agent sessions.`,
  },
  local_docker: {
    executorId: "exec-local-docker",
    label: msg`Docker`,
    description: msg`Runs Docker containers on this machine.`,
  },
  remote_docker: {
    executorId: "exec-remote-docker",
    label: msg`Remote Docker`,
    description: msg`Connects to a remote Docker host.`,
  },
  sprites: {
    executorId: "exec-sprites",
    label: msg`Sprites.dev`,
    description: msg`Runs agents in Sprites.dev cloud sandboxes.`,
  },
  ssh: {
    executorId: "exec-ssh",
    label: msg`SSH`,
    description: msg`Connects to a remote host over SSH and runs agentctl there.`,
  },
};
