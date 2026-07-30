import type { ContainerLiveStatus, TaskEnvironment } from "@/lib/api/domains/task-environment-api";

export type StatusTone = "running" | "stopped" | "warn" | "error" | "neutral";

/**
 * A status label is either a catalog key (our own wording) or a raw string the
 * container runtime reported (`container.status`, an exit code). Docker's states
 * are not ours to translate, so both cases have to be representable — and callers
 * must go through `environmentStatusLabel` rather than reading either field.
 */
export type ExecutorEnvironmentStatus = {
  labelKey?: string;
  /** Verbatim runtime text, used only when there is no `labelKey`. */
  rawLabel?: string;
  tone: StatusTone;
};

/** Resolve a status to display text. `t` comes from the caller's useTranslation. */
export function environmentStatusLabel(
  t: (key: string) => string,
  status: Pick<ExecutorEnvironmentStatus, "labelKey" | "rawLabel"> | null,
): string {
  if (!status) return "";
  return status.labelKey ? t(status.labelKey) : (status.rawLabel ?? "");
}

export type EnvironmentStatusSnapshot = ExecutorEnvironmentStatus & {
  key: string;
};

export function getEnvironmentStatusSnapshot(
  env: TaskEnvironment | null,
  container: ContainerLiveStatus | null,
): EnvironmentStatusSnapshot {
  if (!env) {
    return { key: "none", labelKey: "task:notCreated", tone: "neutral" };
  }
  const status = resolveExecutorEnvironmentStatus(env, container);
  return { ...status, key: `${status.tone}:${status.labelKey ?? status.rawLabel ?? ""}` };
}

export function resolveExecutorEnvironmentStatus(
  env: TaskEnvironment,
  container: ContainerLiveStatus | null,
): ExecutorEnvironmentStatus {
  if (container) {
    return resolveContainerStatus(container);
  }
  return resolveEnvStatus(env.status);
}

const CONTAINER_STATUS_TONES: Record<string, StatusTone> = {
  paused: "warn",
  restarting: "warn",
  dead: "error",
};

function resolveContainerStatus(container: ContainerLiveStatus): ExecutorEnvironmentStatus {
  if (container.missing) return { labelKey: "task:missing", tone: "warn" };
  if (container.state === "running") {
    // container.status is Docker's own text ("Up 3 minutes"); pass it through.
    return container.status
      ? { rawLabel: container.status, tone: "running" }
      : { labelKey: "task:running", tone: "running" };
  }
  if (container.state === "exited") {
    return container.exit_code
      ? { rawLabel: `exited (${container.exit_code})`, tone: "error" }
      : { labelKey: "task:exited", tone: "stopped" };
  }
  const tone = CONTAINER_STATUS_TONES[container.state] ?? "neutral";
  return container.state ? { rawLabel: container.state, tone } : { labelKey: "task:unknown", tone };
}

const ENV_STATUS_MAP: Record<string, ExecutorEnvironmentStatus> = {
  ready: { labelKey: "task:ready", tone: "running" },
  creating: { labelKey: "task:starting2", tone: "warn" },
  stopped: { labelKey: "task:stopped", tone: "stopped" },
  failed: { labelKey: "task:failed3", tone: "error" },
};

function resolveEnvStatus(status: string): ExecutorEnvironmentStatus {
  return (
    ENV_STATUS_MAP[status] ??
    (status ? { rawLabel: status, tone: "neutral" } : { labelKey: "task:unknown", tone: "neutral" })
  );
}
