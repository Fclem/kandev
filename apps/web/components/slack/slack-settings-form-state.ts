import type { IntegrationAuthHealth } from "@/components/integrations/auth-status-banner";
import type { SlackConfig } from "@/lib/types/slack";

// Form-state shape, defaults, and the pure config<->form/health mappers for the
// Slack settings page. Extracted to keep slack-settings.tsx under 600 lines.

export type FormState = {
  utilityAgentId: string;
  commandPrefix: string;
  pollIntervalSeconds: number;
  token: string;
  cookie: string;
};

export const DEFAULT_PREFIX = "!kandev";
export const DEFAULT_POLL_INTERVAL_SECONDS = 30;

export const emptyForm: FormState = {
  utilityAgentId: "",
  commandPrefix: DEFAULT_PREFIX,
  pollIntervalSeconds: DEFAULT_POLL_INTERVAL_SECONDS,
  token: "",
  cookie: "",
};

export function configToForm(cfg: SlackConfig | null): FormState {
  if (!cfg) return emptyForm;
  return {
    utilityAgentId: cfg.utilityAgentId,
    commandPrefix: cfg.commandPrefix || DEFAULT_PREFIX,
    pollIntervalSeconds: cfg.pollIntervalSeconds || DEFAULT_POLL_INTERVAL_SECONDS,
    token: "",
    cookie: "",
  };
}

export function configToHealth(config: SlackConfig | null): IntegrationAuthHealth | null {
  if (!config?.hasToken || !config.hasCookie) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
}
