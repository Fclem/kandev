"use client";
import { useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import {
  IconCheck,
  IconChevronDown,
  IconClipboard,
  IconPlugConnected,
  IconCode,
  IconTools,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { getBackendConfig } from "@/lib/config";
import {
  buildAuggieCliCommand,
  buildAuggieConfig,
  buildClaudeCodeCliCommand,
  buildClaudeCodeConfig,
  buildCodexCliCommand,
  buildCodexConfig,
  buildCopilotCliConfig,
  buildCursorConfig,
  buildOpenCodeConfig,
} from "@/lib/settings/external-mcp-snippets";
import { EXTERNAL_MCP_TOOL_GROUPS, countExternalMcpTools } from "@/lib/settings/external-mcp-tools";

export function ExternalMcpSettings() {
  const { t } = useTranslation();
  const baseUrl = useMemo(() => getBackendConfig().apiBaseUrl.replace(/\/$/, ""), []);
  const streamableUrl = `${baseUrl}/mcp`;
  const sseUrl = `${baseUrl}/mcp/sse`;
  const [copied, setCopied] = useState<string | null>(null);

  function handleCopy(text: string) {
    if (typeof navigator === "undefined") return;
    navigator.clipboard
      .writeText(text)
      .then(() => {
        setCopied(text);
        setTimeout(() => setCopied(null), 2000);
      })
      .catch(() => {
        // Best-effort: clipboard may be unavailable in non-secure contexts.
      });
  }

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{t("common:externalMcp")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          <Trans i18nKey="settings:useThisIfYouWantTo">
            Use this if you want to manage Kandev from coding agents that run{" "}
            <strong>{t("settings:outside")}</strong> Kandev (e.g. Claude Code, Cursor, or Codex on
            your host), or from <strong>{t("settings:passthroughAgents")}</strong> running inside
            Kandev. <br />
            Agents launched inside Kandev in their normal mode already have the Kandev MCP wired in
            automatically, no setup needed.
          </Trans>
        </p>
      </div>

      <ToolsPreview />

      <Separator />

      <SettingsSection
        icon={<IconPlugConnected className="h-5 w-5" />}
        title={t("settings:endpoints")}
        description={t("settings:availableOnTheSameHostAnd")}
      >
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("settings:streamableHttp")}</CardTitle>
          </CardHeader>
          <CardContent>
            <UrlRow url={streamableUrl} copied={copied} onCopy={handleCopy} />
            <p className="text-xs text-muted-foreground mt-2">
              {t("settings:recommendedForClaudeCodeCursorAnd")}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t("settings:serverSentEventsSse")}</CardTitle>
          </CardHeader>
          <CardContent>
            <UrlRow url={sseUrl} copied={copied} onCopy={handleCopy} />
            <p className="text-xs text-muted-foreground mt-2">
              {t("settings:compatibilityTransportForOlderMcpClients")}
            </p>
          </CardContent>
        </Card>
      </SettingsSection>

      <Separator />

      <SnippetsSection streamableUrl={streamableUrl} copied={copied} onCopy={handleCopy} />
    </div>
  );
}

function ToolsPreview() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const total = countExternalMcpTools();
  const groupCount = EXTERNAL_MCP_TOOL_GROUPS.length;
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-md border bg-muted/30">
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex w-full items-center gap-3 px-4 py-3 text-left cursor-pointer hover:bg-muted/50 rounded-md"
        >
          <IconTools className="h-4 w-4 text-muted-foreground shrink-0" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium">{t("settings:availableTools")}</p>
            <p className="text-xs text-muted-foreground">
              {t("settings:toolsAcrossCategoriesExpandToPreview", { total, groupCount })}
            </p>
          </div>
          <IconChevronDown
            className={`h-4 w-4 text-muted-foreground shrink-0 transition-transform ${
              open ? "rotate-180" : ""
            }`}
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="px-4 pb-4 pt-1 space-y-4">
        {EXTERNAL_MCP_TOOL_GROUPS.map((group) => (
          <div key={group.title} className="space-y-1.5">
            <div>
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {group.title}
              </p>
              <p className="text-xs text-muted-foreground">{group.description}</p>
            </div>
            <ul className="space-y-1">
              {group.tools.map((tool) => (
                <li key={tool.name} className="flex gap-2 text-xs">
                  <code className="font-mono text-foreground shrink-0">{tool.name}</code>
                  <span className="text-muted-foreground">&mdash; {tool.description}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

function SnippetsSection({
  streamableUrl,
  copied,
  onCopy,
}: {
  streamableUrl: string;
  copied: string | null;
  onCopy: (text: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsSection
      icon={<IconCode className="h-5 w-5" />}
      title={t("settings:configurationSnippets")}
      description={t("settings:pasteTheseIntoYourAgentS")}
    >
      <SnippetCard
        title={t("settings:claudeCode")}
        subtitle={t("settings:claudeJsonOrRunTheCli")}
        snippet={buildClaudeCodeConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
        extraSnippet={buildClaudeCodeCliCommand(streamableUrl)}
        extraSnippetLabel={t("settings:oneLinerWritesToClaudeJson")}
      />
      <SnippetCard
        title={t("settings:cursor")}
        subtitle="~/.cursor/mcp.json"
        snippet={buildCursorConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
      />
      <SnippetCard
        title="Codex"
        subtitle={t("settings:codexConfigTomlOrRunThe")}
        snippet={buildCodexConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
        extraSnippet={buildCodexCliCommand(streamableUrl)}
        extraSnippetLabel={t("settings:oneLinerWritesToCodexConfig")}
      />
      <SnippetCard
        title={t("settings:auggieCli")}
        subtitle={t("settings:augmentSettingsJsonOrRunThe")}
        snippet={buildAuggieConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
        extraSnippet={buildAuggieCliCommand(streamableUrl)}
        extraSnippetLabel={t("settings:oneLinerWritesToSettingsJson")}
      />
      <SnippetCard
        title="OpenCode"
        subtitle={t("settings:opencodeJsonProjectOrConfigOpencode")}
        snippet={buildOpenCodeConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
      />
      <SnippetCard
        title={t("settings:githubCopilotCli")}
        subtitle="~/.copilot/mcp-config.json"
        snippet={buildCopilotCliConfig(streamableUrl)}
        copied={copied}
        onCopy={onCopy}
      />
    </SettingsSection>
  );
}

function UrlRow({
  url,
  copied,
  onCopy,
}: {
  url: string;
  copied: string | null;
  onCopy: (text: string) => void;
}) {
  const { t } = useTranslation();
  const isCopied = copied === url;
  return (
    <div className="flex items-center gap-1 rounded-md bg-muted px-2 py-1.5 font-mono text-xs">
      <code className="flex-1 truncate">{url}</code>
      <Button
        variant="ghost"
        size="sm"
        className="h-7 w-7 p-0 cursor-pointer shrink-0"
        aria-label={isCopied ? t("settings:copied") : t("settings:copyUrl")}
        onClick={() => onCopy(url)}
      >
        {isCopied ? (
          <IconCheck className="h-3.5 w-3.5 text-green-500" />
        ) : (
          <IconClipboard className="h-3.5 w-3.5 text-muted-foreground" />
        )}
      </Button>
    </div>
  );
}

function SnippetCard({
  title,
  subtitle,
  snippet,
  copied,
  onCopy,
  extraSnippet,
  extraSnippetLabel,
}: {
  title: string;
  subtitle: string;
  snippet: string;
  copied: string | null;
  onCopy: (text: string) => void;
  extraSnippet?: string;
  extraSnippetLabel?: string;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <p className="text-xs text-muted-foreground font-mono">{subtitle}</p>
      </CardHeader>
      <CardContent className="space-y-3">
        <SnippetBlock snippet={snippet} copied={copied} onCopy={onCopy} />
        {extraSnippet ? (
          <div className="space-y-1.5">
            {extraSnippetLabel ? (
              <p className="text-xs text-muted-foreground">{extraSnippetLabel}</p>
            ) : null}
            <SnippetBlock snippet={extraSnippet} copied={copied} onCopy={onCopy} />
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function SnippetBlock({
  snippet,
  copied,
  onCopy,
}: {
  snippet: string;
  copied: string | null;
  onCopy: (text: string) => void;
}) {
  const { t } = useTranslation();
  const isCopied = copied === snippet;
  return (
    <div className="relative">
      <pre className="overflow-x-auto rounded-md bg-muted p-4 pr-12 font-mono text-xs">
        <code className="whitespace-pre-wrap break-all">{snippet}</code>
      </pre>
      <Button
        variant="ghost"
        size="sm"
        className="absolute right-2 top-2 cursor-pointer"
        onClick={() => onCopy(snippet)}
        title={t("settings:copyToClipboard")}
      >
        {isCopied ? (
          <IconCheck className="h-4 w-4 text-green-500" />
        ) : (
          <IconClipboard className="h-4 w-4" />
        )}
      </Button>
    </div>
  );
}
