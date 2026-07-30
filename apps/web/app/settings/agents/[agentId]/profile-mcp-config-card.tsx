"use client";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { useProfileMcpConfig } from "./use-profile-mcp-config";
import type { AgentProfileMcpConfig } from "@/lib/types/http";

type ProfileMcpConfigCardProps = {
  profileId: string;
  supportsMcp: boolean;
  /**
   * Whether the profile is in CLI passthrough mode. When true (and
   * mcpInjection is set), the card explains how kandev injects MCP servers
   * into the agent's CLI.
   */
  cliPassthrough?: boolean;
  /**
   * Human-readable phrase describing the passthrough MCP injection mechanism
   * (from PassthroughConfig.mcp_injection). Only rendered when cliPassthrough.
   */
  mcpInjection?: string;
  initialConfig?: AgentProfileMcpConfig | null;
  draftState?: {
    enabled: boolean;
    servers: string;
    dirty: boolean;
    error: string | null;
  };
  onDraftStateChange?: (next: {
    enabled?: boolean;
    servers?: string;
    dirty?: boolean;
    error?: string | null;
  }) => void;
  onToastError: (error: unknown) => void;
};

const POPULAR_SERVERS: Record<string, Record<string, unknown>> = {
  playwright: {
    type: "stdio",
    command: "npx",
    args: ["-y", "@modelcontextprotocol/server-playwright"],
  },
  "chrome-devtools": {
    type: "stdio",
    command: "npx",
    args: ["-y", "@modelcontextprotocol/server-chrome-devtools"],
  },
  context7: {
    type: "stdio",
    command: "npx",
    args: ["-y", "@context7/mcp"],
    env: {
      CONTEXT7_API_KEY: "your_api_key_here",
    },
  },
  github: {
    type: "stdio",
    command: "npx",
    args: ["-y", "@modelcontextprotocol/server-github"],
    env: {
      GITHUB_TOKEN: "your_token_here",
    },
  },
};

const KANDEV_TOOLS_DESCRIPTION: string = "settings:toolsListWorkspacesListBoardsList";

type PopularServerButtonProps = {
  label: string;
  displayName: string;
  onApply: (label: string) => void;
};

function PopularServerButton({ label, displayName, onApply }: PopularServerButtonProps) {
  return (
    <button
      type="button"
      className="text-xs rounded-full border border-muted-foreground/30 px-2 py-1 hover:bg-muted cursor-pointer"
      onClick={() => onApply(label)}
    >
      + {displayName}
    </button>
  );
}

function applyPopularServerToJson(
  currentServers: string,
  label: string,
  isDraft: boolean,
  onDraftStateChange?: (next: { servers?: string; dirty?: boolean; error?: string | null }) => void,
  handleMcpServersChange?: (value: string) => void,
) {
  const base = currentServers.trim() || '{\n  "mcpServers": {}\n}';
  let parsed: Record<string, unknown> = {};
  try {
    parsed = JSON.parse(base) as Record<string, unknown>;
  } catch {
    return;
  }
  const root =
    parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : { mcpServers: {} };
  const servers = (
    root.mcpServers && typeof root.mcpServers === "object" && !Array.isArray(root.mcpServers)
      ? root.mcpServers
      : {}
  ) as Record<string, unknown>;

  if (servers[label]) return;
  servers[label] = POPULAR_SERVERS[label] ?? { type: "stdio", command: "npx", args: ["-y"] };
  root.mcpServers = servers;
  const nextValue = JSON.stringify(root, null, 2);

  if (isDraft) {
    onDraftStateChange?.({ servers: nextValue, dirty: true, error: null });
    return;
  }
  handleMcpServersChange?.(nextValue);
}

function validateDraftServers(value: string): string | null {
  if (!value.trim()) return null;
  try {
    const parsed = JSON.parse(value);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return t("settings:mcpServersConfigMustBeA");
    }
    if ("mcpServers" in parsed) {
      const nested = (parsed as { mcpServers?: unknown }).mcpServers;
      if (!nested || typeof nested !== "object" || Array.isArray(nested)) {
        return t("settings:mcpserversMustBeAJsonObject");
      }
    }
  } catch {
    return t("settings:invalidJson");
  }
  return null;
}

function PassthroughMcpInjectionHint({
  cliPassthrough,
  mcpInjection,
}: {
  cliPassthrough?: boolean;
  mcpInjection?: string;
}) {
  const { t } = useTranslation();
  if (!cliPassthrough || !mcpInjection) return null;
  return (
    <p className="text-xs text-muted-foreground">
      {t("settings:inCliPassthroughModeKandevInjects", { mcpInjection })}
    </p>
  );
}

type McpServersEditorProps = {
  profileId: string;
  currentServers: string;
  currentError: string | null;
  isDirty: boolean;
  isDraft: boolean;
  isEditableProfile: boolean;
  cliPassthrough?: boolean;
  mcpInjection?: string;
  onDraftStateChange?: (next: { servers?: string; dirty?: boolean; error?: string | null }) => void;
  handleMcpServersChange: (value: string) => void;
};

function McpServersEditor({
  profileId,
  currentServers,
  currentError,
  isDirty,
  isDraft,
  isEditableProfile,
  cliPassthrough,
  mcpInjection,
  onDraftStateChange,
  handleMcpServersChange,
}: McpServersEditorProps) {
  const { t: translate } = useTranslation();
  const handleApplyServer = (label: string) => {
    applyPopularServerToJson(
      currentServers,
      label,
      isDraft,
      onDraftStateChange,
      handleMcpServersChange,
    );
  };

  const handleDraftChange = (value: string) => {
    if (!onDraftStateChange) return;
    const error = validateDraftServers(value);
    onDraftStateChange({ servers: value, dirty: true, error });
  };

  return (
    <div className="space-y-2">
      <Label htmlFor={`mcp-servers-${profileId}`}>{t("settings:mcpServersJson")}</Label>
      <Textarea
        id={`mcp-servers-${profileId}`}
        className="min-h-[200px] font-mono text-xs"
        value={currentServers}
        onChange={(event) => {
          if (isDraft) {
            handleDraftChange(event.target.value);
            return;
          }
          handleMcpServersChange(event.target.value);
        }}
        disabled={!isEditableProfile && !isDraft}
        data-settings-dirty={isDirty}
        data-testid={`mcp-servers-${profileId}`}
      />
      <p className="text-xs text-muted-foreground">{t("settings:mcpDefinitionsAreStoredInThe")}</p>
      <PassthroughMcpInjectionHint cliPassthrough={cliPassthrough} mcpInjection={mcpInjection} />
      <p className="text-xs font-medium text-muted-foreground">{t("settings:builtIn")}</p>
      <div className="flex flex-wrap gap-2 mb-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="text-xs rounded-full border border-primary/50 bg-primary/10 px-2 py-1 text-primary">
              {t("settings:kandevMcp")}
            </span>
          </TooltipTrigger>
          <TooltipContent side="bottom" className="max-w-[320px] text-xs">
            <p className="font-medium mb-1">{t("settings:automaticallyAvailable")}</p>
            <p>{translate(KANDEV_TOOLS_DESCRIPTION)}</p>
          </TooltipContent>
        </Tooltip>
      </div>
      <p className="text-xs font-medium text-muted-foreground">{t("settings:popularServers")}</p>
      <div className="flex flex-wrap gap-2">
        <PopularServerButton
          label={t("settings:playwright")}
          displayName={t("settings:playwrightMcp")}
          onApply={handleApplyServer}
        />
        <PopularServerButton
          label="chrome-devtools"
          displayName={t("settings:chromeDevtoolsMcp")}
          onApply={handleApplyServer}
        />
        <PopularServerButton
          label={t("settings:context7")}
          displayName={t("settings:context7Mcp")}
          onApply={handleApplyServer}
        />
        <PopularServerButton label={t("settings:github")} displayName={t("settings:githubMcp")} onApply={handleApplyServer} />
      </div>
      {currentError && <p className="text-sm text-destructive">{currentError}</p>}
    </div>
  );
}

type McpConfigState = {
  isDraft: boolean;
  isEditableProfile: boolean;
  currentEnabled: boolean;
  currentServers: string;
  currentError: string | null;
  currentDirty: boolean;
  enabledDirty: boolean;
  serversDirty: boolean;
};

type ResolveMcpConfigInput = {
  draftState: ProfileMcpConfigCardProps["draftState"];
  profileId: string;
  mcpEnabled: boolean;
  mcpServers: string;
  mcpBaselineEnabled: boolean;
  mcpBaselineServers: string;
  mcpError: string | null;
};

function resolveMcpConfigState(input: ResolveMcpConfigInput): McpConfigState {
  const isDraft = Boolean(input.draftState);
  const isEditableProfile =
    !isDraft && Boolean(input.profileId) && !input.profileId.startsWith("draft-");
  const baselineEnabled = isDraft ? false : input.mcpBaselineEnabled;
  const baselineServers = isDraft ? '{\n  "mcpServers": {}\n}' : input.mcpBaselineServers;
  const currentEnabled = isDraft ? (input.draftState?.enabled ?? false) : input.mcpEnabled;
  const currentServers = isDraft ? (input.draftState?.servers ?? "") : input.mcpServers;
  const enabledDirty = currentEnabled !== baselineEnabled;
  const serversDirty = currentServers !== baselineServers;
  return {
    isDraft,
    isEditableProfile,
    currentEnabled,
    currentServers,
    currentError: isDraft ? (input.draftState?.error ?? null) : input.mcpError,
    currentDirty: enabledDirty || serversDirty,
    enabledDirty,
    serversDirty,
  };
}

type McpEnableToggleProps = {
  currentEnabled: boolean;
  isDirty: boolean;
  isDraft: boolean;
  isEditableProfile: boolean;
  onDraftStateChange?: (next: { enabled?: boolean; dirty?: boolean }) => void;
  setMcpEnabled: (enabled: boolean) => void;
};

function McpEnableToggle({
  currentEnabled,
  isDirty,
  isDraft,
  isEditableProfile,
  onDraftStateChange,
  setMcpEnabled,
}: McpEnableToggleProps) {
  const { t } = useTranslation();
  return (
    <div
      className="flex items-center justify-between rounded-md border p-3"
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
      data-testid="mcp-enabled-row"
    >
      <div className="space-y-1">
        <Label>{t("settings:enableMcp")}</Label>
        <p className="text-xs text-muted-foreground">{t("settings:allowThisProfileToUseMcp")}</p>
      </div>
      <Switch
        checked={currentEnabled}
        data-settings-dirty={isDirty}
        data-testid="mcp-enabled"
        onCheckedChange={(checked) => {
          if (isDraft) {
            onDraftStateChange?.({ enabled: checked, dirty: true });
            return;
          }
          setMcpEnabled(checked);
        }}
        disabled={!isEditableProfile && !isDraft}
      />
    </div>
  );
}

function McpProfileHint({
  isDraft,
  isEditableProfile,
}: {
  isDraft: boolean;
  isEditableProfile: boolean;
}) {
  const { t: translate } = useTranslation();
  if (isEditableProfile) return null;
  return (
    <p className="text-xs text-muted-foreground">
      {isDraft
        ? translate`MCP config will be applied after the profile is saved.`
        : translate`Save this profile to configure MCP servers.`}
    </p>
  );
}

export function ProfileMcpConfigCard({
  profileId,
  supportsMcp,
  cliPassthrough,
  mcpInjection,
  initialConfig,
  draftState,
  onDraftStateChange,
  onToastError,
}: ProfileMcpConfigCardProps) {
  const { t } = useTranslation();
  const {
    mcpEnabled,
    mcpServers,
    mcpBaselineEnabled,
    mcpBaselineServers,
    mcpError,
    setMcpEnabled,
    handleMcpServersChange,
    handleSaveMcp,
    resetMcpDraft,
  } = useProfileMcpConfig({ profileId, supportsMcp, initialConfig, onToastError });

  const state = resolveMcpConfigState({
    draftState,
    profileId,
    mcpEnabled,
    mcpServers,
    mcpBaselineEnabled,
    mcpBaselineServers,
    mcpError,
  });
  useSettingsSaveContributor({
    id: `agent-profile-mcp:${profileId}`,
    revision: JSON.stringify({
      enabled: state.currentEnabled,
      servers: state.currentServers,
    }),
    isDirty: supportsMcp && state.isEditableProfile && state.currentDirty,
    canSave: !state.currentError,
    invalidReason: state.currentError ?? undefined,
    save: handleSaveMcp,
    discard: resetMcpDraft,
  });

  if (!supportsMcp) return null;

  return (
    <SettingsCard isDirty={state.currentDirty}>
      <CardHeader>
        <CardTitle>{t("settings:mcpConfiguration")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <McpProfileHint isDraft={state.isDraft} isEditableProfile={state.isEditableProfile} />
        <McpEnableToggle
          currentEnabled={state.currentEnabled}
          isDirty={state.enabledDirty}
          isDraft={state.isDraft}
          isEditableProfile={state.isEditableProfile}
          onDraftStateChange={onDraftStateChange}
          setMcpEnabled={setMcpEnabled}
        />
        <McpServersEditor
          profileId={profileId}
          currentServers={state.currentServers}
          currentError={state.currentError}
          isDirty={state.serversDirty}
          isDraft={state.isDraft}
          isEditableProfile={state.isEditableProfile}
          cliPassthrough={cliPassthrough}
          mcpInjection={mcpInjection}
          onDraftStateChange={onDraftStateChange}
          handleMcpServersChange={handleMcpServersChange}
        />
      </CardContent>
    </SettingsCard>
  );
}
