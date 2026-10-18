"use client";

import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useAgentRuntimeUpdateStatuses } from "@/hooks/domains/settings/use-agent-runtime-update-statuses";
import type { AgentUpdateStatus } from "@/lib/api";
import { SettingsGroup, SettingsRow } from "./settings-group";
import { settingsActionClassName } from "./settings-control";
import { useSettingsTargetRegistration } from "./settings-target-provider";
import { useRuntimeAutoUpdatePolicy } from "./use-runtime-auto-update-policy";

const RUNTIME_UNKNOWN_KEY = "agents:runtimeUnknown";

function runtimeManageKey(harnessControl: boolean, managedFallback?: boolean): string {
  if (harnessControl) return "agents:runtimeOMPManageUpdate";
  if (managedFallback) return "agents:runtimeFallbackManage";
  return "agents:runtimeManage";
}

function hasManagedControl(status: AgentUpdateStatus, hasControl: boolean, canManage: boolean) {
  return (
    (status.management === "managed" || status.managed_fallback) &&
    status.available &&
    status.enabled &&
    hasControl &&
    canManage
  );
}

function harnessOutcomeKey(status: AgentUpdateStatus): string {
  const outcome = status.last_outcome;
  if (outcome?.status === "running") return "agents:runtimeOMPRunning";
  if (outcome?.status === "succeeded") {
    if (outcome.operation === "update") return "agents:runtimeOMPChangedBody";
    if (outcome.operation === "repair") return "agents:runtimeOMPRepairedBody";
    return "agents:runtimeOMPNoChangeBody";
  }
  if (!status.auto_update || status.auto_update_paused) return "agents:runtimeOMPIncompleteBody";
  return outcome?.status === "interrupted"
    ? "agents:runtimeOMPInterruptedBody"
    : "agents:runtimeOMPRetryBody";
}

function HarnessRuntimeOutcome({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  const outcome = status.last_outcome;
  const change = status.last_change;
  const hasOutput =
    outcome?.output_excerpt &&
    (outcome.operation === "up_to_date" ||
      outcome.status === "failed" ||
      outcome.status === "interrupted");
  return (
    <div className="min-w-0 space-y-2">
      {change && (
        <p className="text-sm">
          {t("agents:runtimeOMPLastChange", {
            previous: change.previous_version,
            version: change.observed_version,
          })}
        </p>
      )}
      {outcome && (
        <p className="text-sm" role="status">
          {t(harnessOutcomeKey(status), {
            agent: status.display_name,
            previous: outcome.previous_version || t(RUNTIME_UNKNOWN_KEY),
            version: outcome.observed_version || t(RUNTIME_UNKNOWN_KEY),
          })}
        </p>
      )}
      {status.auto_update_paused && (
        <p
          className="text-sm"
          role="status"
          data-testid={`runtime-omp-paused-${status.agent_name}`}
        >
          {t("agents:runtimeOMPPausedBody", { agent: status.display_name })}
        </p>
      )}
      {hasOutput && (
        <details className="min-w-0">
          <summary className="cursor-pointer text-sm max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
            {t("agents:runtimeOMPOutput")}
          </summary>
          <pre className="max-w-full whitespace-pre-wrap break-words text-xs">
            {outcome.output_excerpt}
          </pre>
        </details>
      )}
    </div>
  );
}

function RuntimeOutcome({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  if (status.update_mode === "self_update") return <HarnessRuntimeOutcome status={status} />;
  const outcome = status.last_outcome;
  if (!outcome) return null;
  const values = {
    agent: status.display_name,
    previous: outcome.previous_version,
    version: outcome.target_version,
  };
  let key = "agents:runtimeFailedBody";
  if (outcome.status === "running") key = "agents:runtimeRunning";
  if (outcome.status === "interrupted") key = "agents:runtimeInterruptedBody";
  if (outcome.status === "succeeded") key = "agents:runtimeSucceededBody";
  return (
    <p className="text-sm" role="status">
      {t(key, values)}
    </p>
  );
}

function RuntimeAction({ status, hasControl }: { status: AgentUpdateStatus; hasControl: boolean }) {
  const { t } = useTranslation();
  const canManage = useIsAdmin();
  const managedControl = hasManagedControl(status, hasControl, canManage);
  const harnessControl =
    status.update_mode === "self_update" &&
    (status.auto_update_supported || status.auto_update) &&
    hasControl &&
    canManage;
  return (
    <div className="flex flex-col gap-2 md:flex-row md:flex-wrap">
      {(managedControl || harnessControl) && (
        <Button asChild variant="outline" className={settingsActionClassName("shrink-0")}>
          <a href={`#installed-agent-${status.agent_name}`}>
            {t(runtimeManageKey(harnessControl, status.managed_fallback))}
          </a>
        </Button>
      )}
      {!managedControl || status.managed_fallback
        ? status.guidance_url && (
            <Button asChild variant="outline" className={settingsActionClassName("shrink-0")}>
              <a href={status.guidance_url} target="_blank" rel="noopener noreferrer">
                {t("agents:runtimeManualGuidance")}
              </a>
            </Button>
          )
        : null}
    </div>
  );
}

function RuntimeVersionSummary({ status }: { status: AgentUpdateStatus }) {
  const { t } = useTranslation();
  const managed = status.management === "managed";
  let ownerKey = "agents:runtimeUnsupported";
  if (managed) ownerKey = "agents:runtimeManaged";
  if (status.management === "manual") ownerKey = "agents:runtimeManual";
  return (
    <div className="min-w-0 flex-1 space-y-1">
      <div className="flex flex-col gap-1 md:flex-row md:flex-wrap md:items-baseline md:gap-x-3">
        <h4 className="text-sm font-semibold">{status.display_name || status.agent_name}</h4>
        <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span>
            {t(managed ? "agents:runtimeSelected" : "agents:runtimeObserved", {
              version: status.effective_version || t(RUNTIME_UNKNOWN_KEY),
            })}
          </span>
          <span>
            {t(
              status.update_mode === "self_update"
                ? "agents:stableLatestReference"
                : "agents:runtimeLatest",
              {
                version: status.latest_version || t(RUNTIME_UNKNOWN_KEY),
              },
            )}
          </span>
        </div>
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span className="min-w-0 break-all font-mono">
          {t("agents:runtimeVersionSource", { runtime: status.source || status.runtime_id })}
        </span>
        <span>{t(ownerKey)}</span>
      </div>
    </div>
  );
}

function RuntimePolicyRow({
  status,
  hasControl,
}: {
  status: AgentUpdateStatus;
  hasControl: boolean;
}) {
  const { t } = useTranslation();
  const canManage = useIsAdmin();
  const { draft, setDraft, isDirty } = useRuntimeAutoUpdatePolicy(status);
  const managed = status.management === "managed";
  const showAutomatic = managed || status.auto_update_supported || status.auto_update;
  const harnessAutomatic = status.update_mode === "self_update" && showAutomatic;
  const targetId = `runtime-update-${status.agent_name}`;
  const registerTarget = useSettingsTargetRegistration(targetId);
  const controlId = `runtime-automatic-${status.agent_name}`;
  let descriptionId: string | undefined;
  if (managed) descriptionId = "runtime-automatic-help";
  if (harnessAutomatic) descriptionId = `${controlId}-help`;
  return (
    <div
      id={targetId}
      ref={registerTarget}
      className="min-w-0 scroll-mt-4 space-y-2 py-3"
      data-testid={`runtime-policy-${status.agent_name}`}
      data-settings-dirty={isDirty}
    >
      <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between md:gap-4">
        <RuntimeVersionSummary status={status} />
        <div className="flex flex-col gap-2 md:shrink-0 md:flex-row md:items-center md:gap-3">
          {showAutomatic && (
            <SettingsRow
              label={t("agents:runtimeAutomatic")}
              controlId={controlId}
              touchTarget="switch"
              isDirty={isDirty}
              className="flex-row items-center gap-2 py-0 first:pt-0 last:pb-0"
              controlWrapperClassName="max-md:w-auto"
              control={
                <Switch
                  id={controlId}
                  aria-describedby={descriptionId}
                  checked={draft}
                  onCheckedChange={setDraft}
                  disabled={!canManage || (!status.auto_update_supported && !status.auto_update)}
                />
              }
            />
          )}
          <RuntimeAction status={status} hasControl={hasControl} />
        </div>
      </div>
      {(!status.available || !status.enabled) && (
        <p className="text-xs text-muted-foreground">{t("agents:runtimeNotAvailable")}</p>
      )}
      {harnessAutomatic && (
        <p id={descriptionId} className="text-xs text-muted-foreground">
          {t("agents:runtimeOMPAutomaticHelp")}
        </p>
      )}
      {!managed && !harnessAutomatic && (
        <p className="text-xs text-muted-foreground">
          {t(
            status.management === "manual"
              ? "agents:runtimeManualHelp"
              : "agents:runtimeUnsupportedHelp",
          )}
        </p>
      )}
      <RuntimeOutcome status={status} />
    </div>
  );
}

export function AgentRuntimePolicies({
  hasRuntimeControl,
}: {
  hasRuntimeControl: (agentName: string) => boolean;
}) {
  const { t } = useTranslation();
  const { statusByAgent } = useAgentRuntimeUpdateStatuses();
  const registerGroup = useSettingsTargetRegistration("runtime-updates");
  const statuses = Object.values(statusByAgent);
  const active = statuses.filter((s) => s.available && s.enabled);
  const inactive = statuses.filter((s) => !s.available || !s.enabled);
  const row = (s: AgentUpdateStatus) => (
    <RuntimePolicyRow
      key={`${s.agent_name}:${s.runtime_id}`}
      status={s}
      hasControl={hasRuntimeControl(s.agent_name)}
    />
  );
  return (
    <SettingsGroup
      id="runtime-updates"
      title={<span ref={registerGroup}>{t("agents:runtimeUpdatesTitle")}</span>}
      collapsible
      defaultOpen={false}
      className="border-t pt-4"
    >
      <div className="space-y-1 py-2 text-xs text-muted-foreground">
        <p>{t("agents:runtimeUpdatesDescription")}</p>
        {statuses.some((status) => status.management === "managed") && (
          <p id="runtime-automatic-help">{t("agents:runtimeAutomaticHelp")}</p>
        )}
      </div>
      <div className="divide-y">{active.map(row)}</div>
      {inactive.length > 0 && (
        <details className="min-w-0">
          <summary className="flex cursor-pointer items-center py-2 text-xs max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
            {t("agents:runtimeOtherRegistrations", { count: inactive.length })}
          </summary>
          <div className="divide-y">{inactive.map(row)}</div>
        </details>
      )}
    </SettingsGroup>
  );
}
